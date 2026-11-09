package localai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/a-digi/cinqo/src/localai/entity"
)

// The states and the status of the model program are shared with the API (entity), and used here under short names.
type (
	EngineState  = entity.EngineState
	EngineStatus = entity.EngineStatus
)

const (
	EngineStopped  = entity.EngineStopped
	EngineStarting = entity.EngineStarting
	EngineReady    = entity.EngineReady
	EngineFailed   = entity.EngineFailed
)

// launch describes one run of the program: which program, which model, how much context, and the folder it lives in. Two runs
// with the same launch are the same run.
// What every answer of the model program is started with (see launchRun). None of Cinqo's requests sets any of these, so these
// are what applies. Strings because they are command line arguments. Measured on the real CV import with Phi-4-mini
// (plan/ai/small-llm-local/sampling-temperature.md): the program's own default temperature (0.80) made the same prompt skip its first
// tool call on one run, and a temperature of 0 made a turn repeat itself for more than 10,000 tokens without end. So:
//   - a low but not zero temperature, so a small model follows its instructions the same way almost every time and cannot be stuck on
//     one path for ever;
//   - a mild penalty for repeating the last tokens (the program's default is none), the usual second guard against a loop. It is
//     kept small because a tool call is JSON, which repeats its own keys and brackets by nature;
//   - a ceiling on the length of one answer (the program's default is none, "infinity"): whatever else goes wrong, one answer ends. The
//     size is the one the Anthropic client already uses as its default; an answer that is cut at the ceiling is handled like any
//     answer cut by the token limit (the text that came is kept, and a bracket left open in a call is put right by the dialect).
const (
	samplingTemperature = "0.2"
	repeatPenalty       = "1.05"
	maxAnswerTokens     = "4096"
)

type launch struct {
	Binary        string
	ModelPath     string
	ModelID       string
	ContextTokens int
	Dir           string // <data>/local-ai: the log and the pid file live here
}

type supervisorOptions struct {
	// Command builds the command for the program; tests replace it with a stand-in.
	Command func(binary string, args []string) *exec.Cmd
	// Port picks the port the program listens on (always on 127.0.0.1).
	Port func() (int, error)
	// Token makes the key the program requires.
	Token func() string
	// HTTP asks the program whether it is ready.
	HTTP *http.Client

	StartTimeout time.Duration // how long loading the model may take (default 3 minutes)
	HealthEvery  time.Duration // how often readiness is asked (default 250 ms)
	Grace        time.Duration // between asking the program to stop and killing it (default 5 s)
	Cooldown     time.Duration // after two starts in a row that failed, the wait before trying again (default 10 s)
	IdleUnit     time.Duration // what one "idle minute" is (default one minute; tests make it short)
}

func (o *supervisorOptions) defaults() {
	if o.Command == nil {
		o.Command = func(binary string, args []string) *exec.Cmd { return exec.Command(binary, args...) }
	}
	if o.Port == nil {
		o.Port = freePort
	}
	if o.Token == nil {
		o.Token = randomToken
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 3 * time.Second}
	}
	if o.StartTimeout <= 0 {
		o.StartTimeout = 3 * time.Minute
	}
	if o.HealthEvery <= 0 {
		o.HealthEvery = 250 * time.Millisecond
	}
	if o.Grace <= 0 {
		o.Grace = 5 * time.Second
	}
	if o.Cooldown <= 0 {
		o.Cooldown = 10 * time.Second
	}
	if o.IdleUnit <= 0 {
		o.IdleUnit = time.Minute
	}
}

// run is one start of the program and everything that belongs to it.
type run struct {
	spec    launch
	baseURL string
	token   string
	pid     atomic.Int64

	ready  chan struct{} // closed when the start is over, whether it worked (err nil) or not
	exited chan struct{} // closed when the process is gone
	err    error
	cancel context.CancelFunc
}

func (r *run) isExited() bool {
	select {
	case <-r.exited:
		return true
	default:
		return false
	}
}

// supervisor starts the model program when it is needed, waits until it is ready, notices when it dies, restarts it for the
// next request, stops it when nobody used it for a while, and stops it with the application. It is safe for concurrent use.
type supervisor struct {
	o supervisorOptions

	mu            sync.Mutex
	cur           *run
	state         EngineState
	lastErr       string
	modelID       string
	inFlight      int
	idleFor       time.Duration
	idleTimer     *time.Timer
	startFailures int // starts in a row that never became ready
	lastFail      time.Time
}

func newSupervisor(o supervisorOptions) *supervisor {
	o.defaults()
	return &supervisor{o: o, state: EngineStopped}
}

// pid is the process id of the program now, 0 when none runs (never started, stopped, or gone).
func (s *supervisor) pid() int {
	s.mu.Lock()
	r := s.cur
	s.mu.Unlock()
	if r == nil || r.isExited() {
		return 0
	}
	return int(r.pid.Load())
}

// Status is the state of the program now.
func (s *supervisor) Status() EngineStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return EngineStatus{State: s.state, Error: s.lastErr, ModelID: s.modelID}
}

// ensure makes sure the program for spec is running and ready, starting it (or restarting it, when it died or runs another
// model or context size) if needed, and returns its address and the key. The caller must call release when its request is
// over: while any request is under way the program is never stopped for being idle. idle is the idle time in idle units
// (0 = never stop for idleness).
func (s *supervisor) ensure(ctx context.Context, spec launch, idleUnits int) (baseURL, token string, release func(), err error) {
	s.mu.Lock()
	s.idleFor = time.Duration(idleUnits) * s.o.IdleUnit
	r := s.cur
	if r != nil && (r.spec != spec || r.isExited()) {
		old := r
		s.cur = nil
		s.mu.Unlock()
		s.terminate(old)
		s.mu.Lock()
		r = s.cur // another caller may have started one meanwhile
	}
	if r == nil {
		if s.startFailures >= 2 && time.Since(s.lastFail) < s.o.Cooldown {
			wait := (s.o.Cooldown - time.Since(s.lastFail)).Round(time.Second)
			if wait < time.Second {
				wait = time.Second
			}
			msg := s.lastErr
			s.mu.Unlock()
			return "", "", nil, fmt.Errorf("the model could not be started a moment ago (%s): trying again in %d s", msg, int(wait/time.Second))
		}
		r = s.startLocked(spec)
	}
	s.inFlight++
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.mu.Unlock()

	select {
	case <-r.ready:
	case <-ctx.Done():
		s.release()
		return "", "", nil, ctx.Err()
	}
	if r.err != nil {
		s.release()
		return "", "", nil, r.err
	}
	return r.baseURL, r.token, s.release, nil
}

// release ends one request; when it was the last one and an idle time is set, the idle clock starts.
func (s *supervisor) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inFlight--
	if s.inFlight > 0 || s.idleFor <= 0 || s.cur == nil {
		return
	}
	cur := s.cur
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.idleTimer = time.AfterFunc(s.idleFor, func() { s.idleStop(cur) })
}

// idleStop stops the program if it is still the same run and nobody is using it.
func (s *supervisor) idleStop(r *run) {
	s.mu.Lock()
	if s.cur != r || s.inFlight > 0 || s.state != EngineReady {
		s.mu.Unlock()
		return
	}
	s.cur = nil
	s.state = EngineStopped
	s.lastErr = ""
	s.mu.Unlock()
	s.terminate(r)
}

// Stop stops the program now (and does nothing when it is not running).
func (s *supervisor) Stop() {
	s.mu.Lock()
	r := s.cur
	s.cur = nil
	s.state = EngineStopped
	s.lastErr = ""
	s.startFailures = 0
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.mu.Unlock()
	if r != nil {
		s.terminate(r)
	}
}

// startLocked begins a new run. s.mu must be held.
func (s *supervisor) startLocked(spec launch) *run {
	ctx, cancel := context.WithCancel(context.Background())
	r := &run{spec: spec, ready: make(chan struct{}), exited: make(chan struct{}), cancel: cancel}
	s.cur = r
	s.state = EngineStarting
	s.lastErr = ""
	s.modelID = spec.ModelID
	go s.launchRun(ctx, r)
	return r
}

func (s *supervisor) launchRun(ctx context.Context, r *run) {
	spec := r.spec
	port, err := s.o.Port()
	if err != nil {
		s.failStart(r, errors.New("no free port could be found for the model"))
		return
	}
	r.baseURL = "http://127.0.0.1:" + strconv.Itoa(port) + "/v1"
	r.token = s.o.Token()

	args := []string{
		"-m", spec.ModelPath,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"-c", strconv.Itoa(spec.ContextTokens),
		"-np", "1",
		"--no-webui",
		"--no-slots",
		"--jinja", // the model's own chat template, which tool calls need
		// The sampling of every answer and its ceiling: see the constants.
		"--temp", samplingTemperature,
		"--repeat-penalty", repeatPenalty,
		"-n", maxAnswerTokens,
	}
	cmd := s.o.Command(spec.Binary, args)
	cmd.Dir = filepath.Dir(spec.Binary)
	// A minimal environment: the program gets its key and nothing else of the application's (a test's stand-in may ask for
	// a few FAKE_LLAMA variables of its own).
	extra := cmdEnvExtra(cmd)
	cmd.Env = append([]string{"LLAMA_API_KEY=" + r.token, "PATH=/usr/bin:/bin", "HOME=" + os.Getenv("HOME"), "TMPDIR=" + os.TempDir()}, extra...)
	setProcAttr(cmd)

	if err := os.MkdirAll(spec.Dir, 0o755); err != nil {
		s.failStart(r, errors.New("the folder for the model program's log could not be created"))
		return
	}
	logFile, err := os.OpenFile(filepath.Join(spec.Dir, "server.log"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		s.failStart(r, errors.New("the model program's log could not be created"))
		return
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		s.failStart(r, errors.New("the model program could not be started"))
		return
	}
	r.pid.Store(int64(cmd.Process.Pid))
	_ = writePID(spec.Dir, cmd.Process.Pid, spec.Binary)
	go func() {
		werr := cmd.Wait()
		logFile.Close()
		close(r.exited)
		s.onExit(r, werr)
	}()

	timeout := time.NewTimer(s.o.StartTimeout)
	defer timeout.Stop()
	tick := time.NewTicker(s.o.HealthEvery)
	defer tick.Stop()
	for {
		select {
		case <-r.exited:
			s.failStart(r, errors.New(s.exitReason(spec.Dir, "the model program stopped while loading the model")))
			return
		case <-ctx.Done():
			s.failStart(r, errors.New("the model program was stopped while it was starting"))
			return
		case <-timeout.C:
			s.failStart(r, fmt.Errorf("the model did not become ready within %s", s.o.StartTimeout.Round(time.Second)))
			return
		case <-tick.C:
			if s.healthy(ctx, r) {
				s.mu.Lock()
				if s.cur != r {
					s.mu.Unlock()
					s.failStart(r, errors.New("the model program was stopped while it was starting"))
					return
				}
				s.state = EngineReady
				s.lastErr = ""
				s.startFailures = 0
				s.mu.Unlock()
				close(r.ready)
				return
			}
		}
	}
}

func (s *supervisor) healthy(ctx context.Context, r *run) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(r.baseURL, "/v1")+"/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	resp, err := s.o.HTTP.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// failStart ends a start that did not work: it records why, stops the program and tells everyone who waits.
func (s *supervisor) failStart(r *run, err error) {
	s.mu.Lock()
	if s.cur == r {
		s.cur = nil
		s.state = EngineFailed
		s.lastErr = err.Error()
		s.startFailures++
		s.lastFail = time.Now()
	}
	s.mu.Unlock()
	r.err = err
	s.terminate(r)
	select {
	case <-r.ready:
	default:
		close(r.ready)
	}
}

// onExit runs when the process is gone. A run that was ready and dies on its own is a crash: the state says so, and the next
// request starts the program again.
func (s *supervisor) onExit(r *run, werr error) {
	removePID(r.spec.Dir, int(r.pid.Load()))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != r || s.state != EngineReady {
		return // stopped on purpose, or still starting (the start loop reports it)
	}
	s.cur = nil
	s.state = EngineFailed
	s.lastErr = s.exitReasonLocked(r.spec.Dir, "the model program stopped unexpectedly", werr)
	s.lastFail = time.Now()
}

func (s *supervisor) exitReason(dir, prefix string) string {
	return s.exitReasonLocked(dir, prefix, nil)
}

func (s *supervisor) exitReasonLocked(dir, prefix string, werr error) string {
	reason := prefix
	if werr != nil {
		var ee *exec.ExitError
		if errors.As(werr, &ee) {
			reason += fmt.Sprintf(" (exit code %d)", ee.ExitCode())
		}
	}
	if tail := logReason(dir); tail != "" {
		reason += ": " + tail
	}
	return reason
}

// terminate stops the process of a run: it is asked to stop, and killed if it has not stopped after the grace time.
func (s *supervisor) terminate(r *run) {
	r.cancel()
	pid := int(r.pid.Load())
	if pid <= 1 || r.isExited() {
		return
	}
	_ = terminateGroup(pid)
	select {
	case <-r.exited:
		return
	case <-time.After(s.o.Grace):
	}
	_ = killGroup(pid)
	select {
	case <-r.exited:
	case <-time.After(5 * time.Second):
	}
}

// logReason reads the end of the program's log and returns its last error line, without the time stamp, so the page can
// say why the program failed. The log never holds prompts (the program does not log them at its default level).
func logReason(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, "server.log"))
	if err != nil {
		return ""
	}
	if len(raw) > 16<<10 {
		raw = raw[len(raw)-16<<10:]
	}
	lines := strings.Split(string(raw), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		// llama.cpp lines look like "0.00.106.893 E llama_model_load: error loading model: ..."
		if len(fields) >= 3 && (fields[1] == "E" || strings.Contains(strings.ToLower(line), "error")) {
			msg := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
			msg = strings.TrimSpace(strings.TrimPrefix(msg, fields[1]))
			if len(msg) > 300 {
				msg = msg[:300] + "…"
			}
			return msg
		}
	}
	return ""
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("no randomness: " + err.Error()) // never happens on a working system, and a guessable key is worse than a stop
	}
	return hex.EncodeToString(b)
}

// cmdEnvExtra returns the variables a command factory asked for beyond the minimal environment (tests use it to steer the
// stand-in program); a real command has none.
func cmdEnvExtra(cmd *exec.Cmd) []string {
	var extra []string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "FAKE_LLAMA") {
			extra = append(extra, e)
		}
	}
	return extra
}

type pidFile struct {
	PID    int    `json:"pid"`
	Binary string `json:"binary"`
}

func pidPath(dir string) string { return filepath.Join(dir, "server.pid") }

func writePID(dir string, pid int, binary string) error {
	raw, _ := json.Marshal(pidFile{PID: pid, Binary: binary})
	return os.WriteFile(pidPath(dir), raw, 0o600)
}

// removePID removes the pid file if it still names this pid.
func removePID(dir string, pid int) {
	raw, err := os.ReadFile(pidPath(dir))
	if err != nil {
		return
	}
	var pf pidFile
	if json.Unmarshal(raw, &pf) == nil && pf.PID == pid {
		_ = os.Remove(pidPath(dir))
	}
}

// killStale stops a model program left over from an earlier run of the application that did not shut down cleanly (a crash,
// a kill): the pid file names it, and it is only stopped when that pid still runs the same program (the pid may have been
// given to something else since). It reports whether something was stopped.
func killStale(dir string, alive func(int) bool, command func(int) (string, error), grace time.Duration) bool {
	raw, err := os.ReadFile(pidPath(dir))
	if err != nil {
		return false
	}
	_ = os.Remove(pidPath(dir))
	var pf pidFile
	if json.Unmarshal(raw, &pf) != nil || pf.PID <= 1 || pf.Binary == "" || !alive(pf.PID) {
		return false
	}
	cmdline, err := command(pf.PID)
	if err != nil || !strings.Contains(cmdline, pf.Binary) {
		return false
	}
	_ = terminateGroup(pf.PID)
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !alive(pf.PID) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = killGroup(pf.PID)
	return true
}
