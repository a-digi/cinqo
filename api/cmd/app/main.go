// Command app is cinqo's single-executable, desktop-tool-style entry
// point: it starts the backend, starts Caddy in-process (serving the
// embedded frontend build and reverse-proxying /api and /auth to the
// backend), and opens a browser — see plan/ai/build/app.md and its
// step-by-step breakdown under plan/ai/build/app/.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/a-digi/coco-logger/logger"
	"github.com/a-digi/coco-server/server"

	caddycore "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	_ "github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile" // registers the "caddyfile" adapter
	_ "github.com/caddyserver/caddy/v2/modules/standard"          // reverse_proxy, file_server, handle, try_files

	"github.com/a-digi/cinqo/cmd/app/webapp"
	"github.com/a-digi/cinqo/src/backendapp"
)

//go:embed embedded.Caddyfile
var embeddedCaddyfile []byte

// caddyPort is the single source of truth for the port embedded.Caddyfile
// itself hardcodes (its site address can't be an {env.*} placeholder —
// see plan/ai/build/app/step-03-embedded-caddy-config.md). Kept here too
// so stopStaleInstance and caddyAddr don't each carry their own copy of
// the literal.
const caddyPort = 7030

var caddyAddr = fmt.Sprintf("http://localhost:%d", caddyPort)

// chromePidFile tracks the PID of the private Chrome instance this app
// opens (see openBrowser/launchPrivateChrome), so a later run can close
// it before opening its own fresh one. Resolved relative to CWD, same
// convention as server.pid (both live in api/, since cmd/app runs with
// CWD=api/ — see the Makefile's run-app target). See
// plan/ai/build/app/step-09-private-chrome-instance-and-pid-tracking.md.
const chromePidFile = "chrome.pid"

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := server.LoadConfig("config.json")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	stoppedPID, err := stopStaleInstance(cfg)
	if err != nil {
		return err
	}
	closeStaleChromeInstance()

	srv, cfg, log, err := backendapp.Start()
	if err != nil {
		return fmt.Errorf("backend: %w", err)
	}
	defer log.Close()

	if stoppedPID != 0 {
		log.Info("stopped previous instance (pid %d) before starting", stoppedPID)
	}

	frontendDir, err := extractFrontend()
	if err != nil {
		return fmt.Errorf("extract embedded frontend: %w", err)
	}
	defer os.RemoveAll(frontendDir)

	if err := startCaddy(cfg.Port, frontendDir); err != nil {
		return fmt.Errorf("start caddy: %w", err)
	}

	if err := waitUntilUp(caddyAddr, 5*time.Second); err != nil {
		log.Warning("caddy did not become reachable in time: %v", err)
	}
	if pid, err := openBrowser(caddyAddr); err != nil {
		log.Warning("could not open a browser automatically: %v", err)
	} else if pid != 0 {
		if err := os.WriteFile(chromePidFile, []byte(strconv.Itoa(pid)), 0o644); err != nil {
			log.Warning("could not record chrome pid: %v", err)
		}
	}

	waitForShutdown(srv, cfg.PidFile, log, frontendDir)
	return nil
}

// stopStaleInstance detects whether a previous cinqo-app instance — or a
// stray `make run-dev` backend, since it writes the exact same PID file
// via the same server.StartServer call — is still running, signals it
// to stop, and waits until this app's own ports are actually free before
// returning. Runs before backendapp.Start() creates the structured
// logger, so progress is reported directly to stdout: the interactive,
// "watch it happen" signal this is meant to be, not something to bury in
// a log file nobody's tailing live. Returns the stopped PID (0 if there
// was nothing to stop) so the caller can log a permanent record of it
// once the real logger exists. See
// plan/ai/build/app/step-07-stale-instance-shutdown-on-startup.md.
func stopStaleInstance(cfg *server.Config) (stoppedPID int, err error) {
	pid, err := server.ReadPID(cfg.PidFile)
	if err != nil {
		return 0, nil // no PID file — nothing to stop
	}

	process, findErr := os.FindProcess(pid)
	alive := findErr == nil && process.Signal(syscall.Signal(0)) == nil
	if !alive {
		// Stale leftover file from an unclean shutdown/crash — nothing
		// is actually running, just clear it and move on.
		_ = server.RemovePID(cfg.PidFile)
		return 0, nil
	}

	fmt.Printf("Shutting down previous cinqo instance (pid %d)", pid)
	_ = server.SendSIGTERM(pid)

	ports := []int{cfg.Port, caddyPort}

	if waitForPortsFree(ports, 10*time.Second) {
		fmt.Println(" done.")
		_ = server.RemovePID(cfg.PidFile)
		return pid, nil
	}

	fmt.Println()
	fmt.Printf("Previous instance (pid %d) did not stop within 10s — forcing it to stop...\n", pid)
	_ = process.Signal(syscall.SIGKILL)

	if !waitForPortsFree(ports, 3*time.Second) {
		return 0, fmt.Errorf(
			"port %d or %d is still in use after force-stopping the previous instance (pid %d) — "+
				"something else may be using it", cfg.Port, caddyPort, pid)
	}

	_ = server.RemovePID(cfg.PidFile)
	return pid, nil
}

// waitForPortsFree polls until every port in ports can be bound, or
// timeout elapses. Prints one "." per poll tick (no newline) so the
// caller's in-progress line grows visibly.
func waitForPortsFree(ports []int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if allPortsFree(ports) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		fmt.Print(".")
		time.Sleep(300 * time.Millisecond)
	}
}

func allPortsFree(ports []int) bool {
	for _, p := range ports {
		if !portFree(p) {
			return false
		}
	}
	return true
}

func portFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// closeStaleChromeInstance closes a private Chrome instance left behind
// by a previous cinqo-app run (tracked via chromePidFile), if one is
// still open, before this run opens its own fresh one. Much simpler
// than stopStaleInstance: there's no port/listening contract to wait
// on, so a short fixed grace period is enough instead of a polling
// loop, and no stdout progress output — closing a leftover browser
// window is expected to be near-instant. See
// plan/ai/build/app/step-09-private-chrome-instance-and-pid-tracking.md.
func closeStaleChromeInstance() {
	data, err := os.ReadFile(chromePidFile)
	if err != nil {
		return // no PID file — nothing to close
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		_ = os.Remove(chromePidFile)
		return
	}

	process, findErr := os.FindProcess(pid)
	alive := findErr == nil && process.Signal(syscall.Signal(0)) == nil
	if !alive {
		// Stale leftover file — the browser window (or whole machine
		// session) was already closed some other way.
		_ = os.Remove(chromePidFile)
		return
	}

	_ = process.Signal(syscall.SIGTERM)
	time.Sleep(2 * time.Second)
	if process.Signal(syscall.Signal(0)) == nil {
		_ = process.Signal(syscall.SIGKILL)
	}

	_ = os.Remove(chromePidFile)
}

// extractFrontend writes the embedded frontend build out to a real temp
// directory — Caddy's stock file_server module serves from a real
// filesystem path, not an in-process fs.FS; writing a custom Caddy
// module to bridge embed.FS directly would be more code for no benefit
// at this scope, so extraction is the deliberate, simpler choice.
func extractFrontend() (string, error) {
	dir, err := os.MkdirTemp("", "cinqo-frontend-*")
	if err != nil {
		return "", err
	}

	err = fs.WalkDir(webapp.FS, "dist", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("dist", path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(webapp.FS, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// startCaddy adapts the embedded Caddyfile (backendAddr/frontendDir
// filled in via {env.*} placeholders — see
// plan/ai/build/app/step-03-embedded-caddy-config.md) and loads it
// in-process. The site's own port is a literal :7030 baked into
// embedded.Caddyfile, not a placeholder — verified in step 3 that a
// Caddyfile site address can't be one.
func startCaddy(backendPort int, frontendDir string) error {
	if err := os.Setenv("CINQO_BACKEND_ADDR", fmt.Sprintf("127.0.0.1:%d", backendPort)); err != nil {
		return err
	}
	if err := os.Setenv("CINQO_FRONTEND_DIR", frontendDir); err != nil {
		return err
	}

	adapter := caddyconfig.GetAdapter("caddyfile")
	if adapter == nil {
		return fmt.Errorf("caddyfile adapter not registered")
	}
	jsonConfig, _, err := adapter.Adapt(embeddedCaddyfile, nil)
	if err != nil {
		return fmt.Errorf("adapt embedded Caddyfile: %w", err)
	}

	return caddycore.Load(jsonConfig, true)
}

// waitUntilUp polls url until it responds (any status code — reachability
// is all that's being checked) or timeout elapses. Same reasoning as
// coco-mda's own run-dev-all Makefile target polling its backend before
// starting its frontend: gives Caddy's listener a moment to actually
// bind before a browser tab is opened against it.
func waitUntilUp(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		if resp, err := client.Get(url); err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", url)
}

// openBrowser tries Google Chrome specifically first (the requirement),
// falling back to the OS's generic "open a URL" mechanism if Chrome
// isn't found — a hard failure when Chrome is merely absent would make
// this mode worse, not better, for that user. Returns the launched
// Chrome process's PID (0 for the generic-fallback path, which hands
// off to the OS and exits immediately — there is no stable PID to learn
// there on any platform) so the caller can record it in chromePidFile.
// See plan/ai/build/app.md's "Opening the browser" section and
// plan/ai/build/app/step-09-private-chrome-instance-and-pid-tracking.md.
func openBrowser(url string) (int, error) {
	switch runtime.GOOS {
	case "darwin":
		chromePath := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(chromePath); err == nil {
			return launchPrivateChrome(chromePath, url)
		}
		return 0, exec.Command("open", url).Start()
	case "linux":
		for _, bin := range []string{"google-chrome", "google-chrome-stable"} {
			if path, err := exec.LookPath(bin); err == nil {
				return launchPrivateChrome(path, url)
			}
		}
		return 0, exec.Command("xdg-open", url).Start()
	case "windows":
		for _, path := range []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		} {
			if _, err := os.Stat(path); err == nil {
				return launchPrivateChrome(path, url)
			}
		}
		return 0, exec.Command("cmd", "/c", "start", "", url).Start()
	default:
		return 0, fmt.Errorf("unsupported OS %q for opening a browser", runtime.GOOS)
	}
}

// launchPrivateChrome spawns a genuinely separate, private (Incognito)
// Chrome instance — not a new tab in whatever Chrome window already
// happens to be open. Verified directly: invoking the real binary with
// a fresh --user-data-dir forces Chrome's own single-instance check to
// treat this as an independent instance (its own PID, own process
// tree), which is also what makes returning a trackable PID possible at
// all — the OS-handoff commands (`open`, `xdg-open`, `cmd /c start`)
// used elsewhere in this file never give one back.
func launchPrivateChrome(chromePath, url string) (int, error) {
	profileDir, err := os.MkdirTemp("", "cinqo-chrome-profile-*")
	if err != nil {
		return 0, err
	}

	cmd := exec.Command(chromePath,
		"--user-data-dir="+profileDir,
		"--incognito",
		"--no-first-run",
		"--no-default-browser-check",
		url,
	)
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(profileDir)
		return 0, err
	}

	return cmd.Process.Pid, nil
}

// waitForShutdown blocks until SIGINT/SIGTERM, then stops Caddy before
// the backend — Caddy is the public-facing listener, so stopping the
// backend first would leave it still accepting connections it can no
// longer proxy anywhere.
func waitForShutdown(srv *http.Server, pidFile string, log logger.Logger, frontendDir string) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	sig := <-ch
	log.Info("Received signal %s, shutting down...", sig)

	if err := caddycore.Stop(); err != nil {
		log.Warning("caddy stop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if srv != nil {
		if err := srv.Shutdown(ctx); err != nil {
			log.Error("backend graceful shutdown failed: %v", err)
		}
	}

	if err := os.RemoveAll(frontendDir); err != nil {
		log.Warning("could not remove extracted frontend dir: %v", err)
	}
	if err := server.RemovePID(pidFile); err != nil {
		log.Warning("could not remove PID file: %v", err)
	}
}
