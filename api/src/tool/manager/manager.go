// Package manager supervises one child OS process per activated tool's
// backend — verified directly against coco-mda's real PluginManager
// rather than assumed: never a Go plugin.Open() .so, always an
// independently-compiled subprocess spoken to only over HTTP on
// loopback. See
// plan/ai/tools/step-04-enable-disable-and-tool-manager.md.
//
// Package-level state (a mutex-protected map, no struct instance
// threaded through DI) — same idiom already established in this
// codebase by security/scopes' registry for exactly this kind of
// infrastructure-level, process-lifetime singleton.
package manager

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

const (
	basePort            = 20000
	healthCheckTimeout  = 10 * time.Second
	healthCheckInterval = 200 * time.Millisecond
	stopGracePeriod     = 2 * time.Second
	// maxCrashRestarts caps the total number of automatic restarts a
	// tool gets across its running lifetime (reset only by a
	// deliberate external Start — an enable, or a fresh install/update)
	// — not per-attempt, so a tool that keeps crashing shortly after
	// each successful restart still gives up eventually rather than
	// looping forever.
	maxCrashRestarts = 3
)

type runningProcess struct {
	cmd  *exec.Cmd
	port int
	// stopRequested distinguishes a deliberate Stop() from an
	// unexpected crash — supervise reads this once cmd.Wait() returns
	// to decide whether to restart at all.
	stopRequested bool
}

var (
	mu          sync.Mutex
	nextPortVal = basePort
	processes   = map[string]*runningProcess{} // keyed by tool ID
	crashCounts = map[string]int{}
)

// Start spawns t's backend process, if it has one — a no-op for a
// frontend-only tool. Kills any leftover PID from a previous run of
// this same tool first, assigns the next loopback port, waits for
// /healthz to report healthy before marking the tool "running", and
// arms the crash-restart supervisor. Resets this tool's crash-restart
// budget — this is the entry point for a deliberate "start it" request
// (enable, a fresh install/update), not the supervisor's own internal
// retry.
func Start(db *sql.DB, dataDir string, t tool_entity.Tool, corePort int) error {
	mu.Lock()
	crashCounts[t.ID] = 0
	mu.Unlock()
	return startProcess(db, dataDir, t, corePort)
}

// StartAllEnabled (re)starts every tool the database already says is
// enabled — called once at application boot, since nothing else does
// (Start is otherwise only ever reached via a deliberate enable/
// install request). Without this, this package's own in-memory
// processes map starts empty on every restart while the database
// still says "enabled"/"running" from before, leaving every tool's
// proxy route 503ing ("tool is not currently running") until someone
// manually disables and re-enables it — a real, reproduced bug behind
// a live PDF-download failure. Best-effort per tool, matching
// enable_handler.go's own established convention: one tool failing to
// start is reported through warn and does not stop the rest from
// being attempted. See
// plan/ai/tools/step-11-restart-enabled-tools-on-boot.md.
func StartAllEnabled(db *sql.DB, dataDir string, corePort int, warn func(format string, args ...any)) {
	tools, err := tool_query.NewToolQueryRepo(db).List()
	if err != nil {
		warn("tool manager: failed to list tools for boot-time restart: %v", err)
		return
	}
	for _, t := range tools {
		if !t.Enabled {
			continue
		}
		if err := Start(db, dataDir, *t, corePort); err != nil {
			warn("tool %q enabled but failed to start at boot: %v", t.Slug, err)
		}
	}
}

// ToolEnvVars returns the fixed TOOL_DB_DIR/TOOL_UPLOADS_DIR/
// TOOL_TMP_DIR/CORE_API_URL env vars every tool subprocess gets —
// shared by this package's own long-running HTTP-mode spawn and
// tool/mcp's separate on-demand --mcp-mode spawn (discovery/
// invocation), so both give a tool's own code the exact same,
// correctly-absolute-pathed contract. Must be absolute: whichever
// caller spawns the process may set cmd.Dir to the tool's own install
// directory, which would otherwise resolve a relative value against
// the wrong base — verified directly, reproduced as a real
// data-loss-on-update bug before this was extracted into one shared
// function (see this package's own git history / the pdf-generator
// step-02 design doc's "Implemented and verified" section).
//
// dataDir must be the same resolved data directory backendapp.Start
// registered as "data_dir" (defaults to executable-relative, overridden
// via --data) — every caller resolves it from there rather than this
// function assuming a bare "data" CWD-relative literal, which used to
// silently diverge from the app's own real data directory whenever the
// two processes' CWDs differed (a real bug: a tool's own database/
// uploads/tmp directories landing somewhere other than the rest of the
// app's data). See
// plan/ai/build/app/step-22-data-dir-always-executable-relative.md.
func ToolEnvVars(dataDir, slug string, corePort int) ([]string, error) {
	dbDir, err := filepath.Abs(filepath.Join(dataDir, "db", "tools", slug))
	if err != nil {
		return nil, err
	}
	uploadsDir, err := filepath.Abs(filepath.Join(dataDir, "uploads", "tools", slug))
	if err != nil {
		return nil, err
	}
	tmpDir, err := filepath.Abs(filepath.Join(dataDir, "tmp", "tools", slug))
	if err != nil {
		return nil, err
	}
	return []string{
		"TOOL_DB_DIR=" + dbDir,
		"TOOL_UPLOADS_DIR=" + uploadsDir,
		"TOOL_TMP_DIR=" + tmpDir,
		fmt.Sprintf("CORE_API_URL=http://127.0.0.1:%d", corePort),
	}, nil
}

func startProcess(db *sql.DB, dataDir string, t tool_entity.Tool, corePort int) error {
	if t.BackendExecutableRelpath == "" {
		return nil // frontend-only — nothing to spawn
	}

	killIfAlive(t.PID)

	port := nextPort()

	// Must be absolute: when cmd.Dir is set, Go's os/exec resolves a
	// *relative* cmd.Path against cmd.Dir, not the caller's CWD —
	// joining InstallPath (itself relative to CWD) into a relative
	// Path here would then get InstallPath applied twice. Verified
	// directly: reproduced the exact "no such file or directory"
	// failure this caused, confirmed filepath.Abs fixes it. See
	// plan/ai/tools/step-04-enable-disable-and-tool-manager.md.
	execPath, err := filepath.Abs(filepath.Join(t.InstallPath, t.BackendExecutableRelpath))
	if err != nil {
		_ = setStatusAndPID(db, t.ID, "error", 0)
		return err
	}

	toolEnv, err := ToolEnvVars(dataDir, t.Slug, corePort)
	if err != nil {
		_ = setStatusAndPID(db, t.ID, "error", 0)
		return err
	}

	cmd := exec.Command(execPath)
	cmd.Dir = t.InstallPath
	cmd.Env = append(os.Environ(), append(toolEnv, fmt.Sprintf("PORT=%d", port))...)

	if err := cmd.Start(); err != nil {
		_ = setStatusAndPID(db, t.ID, "error", 0)
		return err
	}

	// Persisted immediately — a crash of the host itself still leaves
	// a way to find and kill an orphaned child on the next startup.
	if err := setStatusAndPID(db, t.ID, "starting", cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		go cmd.Wait() // reap — nothing else will
		return err
	}

	mu.Lock()
	processes[t.ID] = &runningProcess{cmd: cmd, port: port}
	mu.Unlock()

	if !waitHealthy(port, healthCheckTimeout) {
		_ = cmd.Process.Kill()
		go cmd.Wait() // reap
		mu.Lock()
		delete(processes, t.ID)
		mu.Unlock()
		_ = setStatusAndPID(db, t.ID, "error", 0)
		return fmt.Errorf("tool %q did not become healthy within %s", t.Slug, healthCheckTimeout)
	}

	if err := setStatusAndPID(db, t.ID, "running", cmd.Process.Pid); err != nil {
		return err
	}

	go supervise(db, dataDir, t, cmd, corePort)

	return nil
}

// supervise waits for the child to exit and, unless that exit was
// requested via Stop, restarts it — capped at maxCrashRestarts total
// (not per attempt) before giving up into a permanent "error" status.
func supervise(db *sql.DB, dataDir string, t tool_entity.Tool, cmd *exec.Cmd, corePort int) {
	_ = cmd.Wait()

	mu.Lock()
	rp, tracked := processes[t.ID]
	stopRequested := tracked && rp.stopRequested
	if tracked {
		delete(processes, t.ID)
	}
	mu.Unlock()

	if stopRequested {
		return // Stop() already set status/pid
	}

	mu.Lock()
	crashCounts[t.ID]++
	count := crashCounts[t.ID]
	mu.Unlock()

	if count > maxCrashRestarts {
		_ = setStatusAndPID(db, t.ID, "error", 0)
		return
	}

	time.Sleep(time.Duration(count) * time.Second) // linear backoff

	if err := startProcess(db, dataDir, t, corePort); err != nil {
		_ = setStatusAndPID(db, t.ID, "error", 0)
	}
}

// Port returns the loopback port a tool's backend is currently bound
// to, if this process tracks it as running. Used by the reverse proxy
// (step 5) to find where to forward a request — reading this in-memory
// map rather than persisting the port to the DB, since it's only ever
// meaningful for the lifetime of the process that assigned it.
func Port(toolID string) (int, bool) {
	mu.Lock()
	defer mu.Unlock()
	rp, ok := processes[toolID]
	if !ok {
		return 0, false
	}
	return rp.port, true
}

// Stop signals t's tracked process (escalating SIGTERM→SIGKILL after a
// short grace period — same idiom already proven in
// api/cmd/app/main.go's closeStaleChromeInstance/stopStaleInstance,
// reused here rather than inventing a third variant), clears the
// persisted PID, and marks the tool "stopped". A no-op if nothing is
// tracked and the DB has no PID either (a frontend-only tool, or one
// already stopped).
func Stop(db *sql.DB, toolID string) error {
	mu.Lock()
	rp, tracked := processes[toolID]
	if tracked {
		rp.stopRequested = true
	}
	mu.Unlock()

	pid := 0
	if tracked {
		pid = rp.cmd.Process.Pid
	} else if dbPID, err := readPID(db, toolID); err == nil {
		pid = dbPID
	}

	if pid > 0 {
		killIfAlive(pid)
	}

	mu.Lock()
	delete(processes, toolID)
	delete(crashCounts, toolID)
	mu.Unlock()

	return setStatusAndPID(db, toolID, "stopped", 0)
}

func nextPort() int {
	mu.Lock()
	defer mu.Unlock()
	p := nextPortVal
	nextPortVal++
	return p
}

// killIfAlive signals pid to stop, escalating to SIGKILL if it's still
// alive after a short grace period. A no-op if pid is zero/negative or
// already dead — matches the liveness-probe idiom already established
// in api/cmd/app/main.go rather than assuming a DB-recorded PID is
// still real.
func killIfAlive(pid int) {
	if pid <= 0 {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	if process.Signal(syscall.Signal(0)) != nil {
		return // not alive
	}

	_ = process.Signal(syscall.SIGTERM)
	time.Sleep(stopGracePeriod)
	if process.Signal(syscall.Signal(0)) == nil {
		_ = process.Signal(syscall.SIGKILL)
	}
}

func waitHealthy(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", port)

	for time.Now().Before(deadline) {
		if resp, err := client.Get(url); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(healthCheckInterval)
	}
	return false
}

func setStatusAndPID(db *sql.DB, toolID, status string, pid int) error {
	_, err := db.Exec(`UPDATE tools SET status = ?, pid = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, pid, toolID)
	return err
}

func readPID(db *sql.DB, toolID string) (int, error) {
	var pid sql.NullInt64
	if err := db.QueryRow(`SELECT pid FROM tools WHERE id = ?`, toolID).Scan(&pid); err != nil {
		return 0, err
	}
	return int(pid.Int64), nil
}
