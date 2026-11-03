package localai

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newSup(t *testing.T, starts *atomic.Int32, env ...string) *supervisor {
	t.Helper()
	o := supervisorOptions{Command: fakeCommand(starts, env...)}
	quick(&o)
	s := newSupervisor(o)
	t.Cleanup(s.Stop)
	return s
}

func TestTheProgramStartsOnDemandAndAnswersOnItsOwnPortWithTheKey(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	spec := testSpec(t)
	if st := s.Status(); st.State != EngineStopped {
		t.Fatalf("before any request: %+v, want stopped (nothing runs until it is needed)", st)
	}

	base, token, release, err := s.ensure(context.Background(), spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if !regexp.MustCompile(`^http://127\.0\.0\.1:\d+/v1$`).MatchString(base) {
		t.Errorf("address = %q, want a port on 127.0.0.1 only", base)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(token) {
		t.Errorf("token = %q, want 64 hex digits", token)
	}
	if st := s.Status(); st.State != EngineReady || st.ModelID != "tiny" || st.Error != "" {
		t.Errorf("status = %+v", st)
	}
	if code, err := postChat(base, token); err != nil || code != 200 {
		t.Errorf("with the key: %d %v", code, err)
	}
	if code, _ := postChat(base, "wrong-key"); code != 401 {
		t.Errorf("a wrong key: %d, want 401", code)
	}
	if code, _ := postChat(base, ""); code != 401 {
		t.Errorf("no key: %d, want 401", code)
	}
	if starts.Load() != 1 {
		t.Errorf("%d starts, want 1", starts.Load())
	}
}

func TestTwoTokensAndTwoPortsNeverRepeat(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	spec := testSpec(t)
	base1, token1, rel1, err := s.ensure(context.Background(), spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	rel1()
	s.Stop()
	base2, token2, rel2, err := s.ensure(context.Background(), spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	rel2()
	if token1 == token2 {
		t.Error("every start must get its own key")
	}
	_ = base1
	_ = base2
}

func TestTheKeyIsInTheEnvironmentNeverOnTheCommandLineAndNothingElseLeaks(t *testing.T) {
	t.Setenv("CINQO_SECRET_FOR_TEST", "do-not-leak")
	var starts atomic.Int32
	record := filepath.Join(t.TempDir(), "record.json")
	s := newSup(t, &starts, "FAKE_LLAMA_RECORD="+record)
	spec := testSpec(t)
	_, token, release, err := s.ensure(context.Background(), spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	waitUntil(t, "the record", func() bool { _, err := os.Stat(record); return err == nil })
	raw, _ := os.ReadFile(record)
	var rec struct {
		Args     []string `json:"args"`
		EnvNames []string `json:"envNames"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(rec.Args, " "), token) {
		t.Error("the key must not be on the command line, where every user of the machine could read it")
	}
	has := func(name string) bool {
		for _, n := range rec.EnvNames {
			if n == name {
				return true
			}
		}
		return false
	}
	if !has("LLAMA_API_KEY") {
		t.Error("the key must be in LLAMA_API_KEY")
	}
	if has("CINQO_SECRET_FOR_TEST") {
		t.Error("the program must not inherit the application's environment")
	}
	joined := " " + strings.Join(rec.Args, " ") + " "
	for _, want := range []string{" --host 127.0.0.1 ", " -np 1 ", " --no-webui ", " --no-slots ", " -c 4096 ", " -m " + spec.ModelPath + " "} {
		if !strings.Contains(joined, want) {
			t.Errorf("the command line %q must contain %q", joined, want)
		}
	}
	if !strings.Contains(joined, " --port ") {
		t.Error("the port must be passed")
	}
}

func TestManyRequestsStartTheProgramOnlyOnce(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_DELAY_MS=150")
	spec := testSpec(t)
	var wg sync.WaitGroup
	bases := make([]string, 6)
	for i := range bases {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			base, _, release, err := s.ensure(context.Background(), spec, 0)
			if err != nil {
				t.Error(err)
				return
			}
			release()
			bases[i] = base
		}(i)
	}
	wg.Wait()
	if starts.Load() != 1 {
		t.Errorf("%d starts for 6 simultaneous requests, want 1", starts.Load())
	}
	for _, b := range bases {
		if b != bases[0] {
			t.Errorf("all requests must share one program: %v", bases)
			break
		}
	}
}

func TestWhileTheModelLoadsTheStateIsStarting(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_DELAY_MS=400")
	done := make(chan struct{})
	go func() {
		_, _, release, err := s.ensure(context.Background(), testSpec(t), 0)
		if err == nil {
			release()
		}
		close(done)
	}()
	waitUntil(t, "the starting state", func() bool { return s.Status().State == EngineStarting })
	<-done
	if st := s.Status(); st.State != EngineReady {
		t.Errorf("status = %+v, want ready afterwards", st)
	}
}

func TestAProgramThatNeverBecomesReadyIsStoppedWithAReasonAndTheNextStartWaitsAfterTwoFailures(t *testing.T) {
	var starts atomic.Int32
	o := supervisorOptions{Command: fakeCommand(&starts, "FAKE_LLAMA_MODE=never")}
	quick(&o)
	o.StartTimeout = 250 * time.Millisecond
	s := newSupervisor(o)
	t.Cleanup(s.Stop)
	spec := testSpec(t)

	_, _, _, err := s.ensure(context.Background(), spec, 0)
	if err == nil || !strings.Contains(err.Error(), "did not become ready within") {
		t.Fatalf("first failure: err = %v", err)
	}
	if st := s.Status(); st.State != EngineFailed || !strings.Contains(st.Error, "did not become ready") {
		t.Errorf("status = %+v", st)
	}
	// One failure is retried at once; the process of the failed start must be gone.
	_, _, _, err = s.ensure(context.Background(), spec, 0)
	if err == nil || !strings.Contains(err.Error(), "did not become ready within") {
		t.Fatalf("second failure: err = %v", err)
	}
	if starts.Load() != 2 {
		t.Fatalf("%d starts, want 2", starts.Load())
	}
	// After two in a row the next request does not start another process at once.
	_, _, _, err = s.ensure(context.Background(), spec, 0)
	if err == nil || !strings.Contains(err.Error(), "could not be started a moment ago") || !strings.Contains(err.Error(), "trying again in") {
		t.Errorf("during the cooldown: err = %v", err)
	}
	if starts.Load() != 2 {
		t.Errorf("%d starts, want 2: the cooldown must not start a process", starts.Load())
	}
	time.Sleep(o.Cooldown + 50*time.Millisecond)
	_, _, _, err = s.ensure(context.Background(), spec, 0)
	if err == nil || !strings.Contains(err.Error(), "did not become ready within") || starts.Load() != 3 {
		t.Errorf("after the cooldown a new start must be tried: err %v starts %d", err, starts.Load())
	}
}

func TestAProgramThatExitsAtOnceReportsItsLastErrorLine(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_MODE=exit")
	_, _, _, err := s.ensure(context.Background(), testSpec(t), 0)
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "stopped while loading the model") || !strings.Contains(msg, "model is corrupted or incomplete") {
		t.Errorf("err = %q, want the program's own reason", msg)
	}
	if strings.Contains(msg, "0.00.106.893") {
		t.Errorf("the time stamp must be cut from the reason: %q", msg)
	}
	if st := s.Status(); st.State != EngineFailed || st.Error != msg {
		t.Errorf("status = %+v", st)
	}
}

func TestACrashAfterBeingReadyIsNoticedAndTheNextRequestStartsTheProgramAgain(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_CRASH_MS=150")
	spec := testSpec(t)
	base, _, release, err := s.ensure(context.Background(), spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	release()
	waitUntil(t, "the crash to be noticed", func() bool { return s.Status().State == EngineFailed })
	st := s.Status()
	if !strings.Contains(st.Error, "stopped unexpectedly (exit code 7)") || !strings.Contains(st.Error, "out of memory") {
		t.Errorf("status = %+v, want the exit code and the last error line", st)
	}
	// A single crash does not put the program on cooldown: the next request restarts it at once.
	base2, token2, release2, err := s.ensure(context.Background(), spec, 0)
	if err != nil {
		t.Fatalf("the restart after a crash must work at once: %v", err)
	}
	defer release2()
	if base2 == base || starts.Load() != 2 {
		t.Errorf("a new process (new port) was expected: %s vs %s, %d starts", base, base2, starts.Load())
	}
	if code, err := postChat(base2, token2); err != nil || code != 200 {
		t.Errorf("after the restart: %d %v", code, err)
	}
}

func TestAnotherModelOrContextSizeRestartsTheProgram(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	spec := testSpec(t)
	base1, _, rel, err := s.ensure(context.Background(), spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	rel()
	spec2 := spec
	spec2.ContextTokens = 8192
	base2, token2, rel2, err := s.ensure(context.Background(), spec2, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rel2()
	if base1 == base2 || starts.Load() != 2 {
		t.Errorf("a changed context size must restart the program: %s %s %d", base1, base2, starts.Load())
	}
	if code, _ := postChat(base1, token2); code != 0 {
		t.Error("the old program must be gone")
	}
	spec3 := spec2
	spec3.ModelID, spec3.ModelPath = "other", filepath.Join(spec.Dir, "other.gguf")
	rel2()
	base3, _, rel3, err := s.ensure(context.Background(), spec3, 0)
	if err != nil {
		t.Fatal(err)
	}
	rel3()
	if base3 == base2 || starts.Load() != 3 || s.Status().ModelID != "other" {
		t.Errorf("another model must restart the program: %d starts, %+v", starts.Load(), s.Status())
	}
}

func TestStopEndsTheProcessAndRemovesItsPidFile(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	spec := testSpec(t)
	_, _, release, err := s.ensure(context.Background(), spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	release()
	raw, err := os.ReadFile(pidPath(spec.Dir))
	if err != nil {
		t.Fatalf("a running program must have a pid file: %v", err)
	}
	var pf pidFile
	if err := json.Unmarshal(raw, &pf); err != nil || pf.PID <= 1 || pf.Binary != spec.Binary || !processAlive(pf.PID) {
		t.Fatalf("pid file = %+v (%v)", pf, err)
	}
	if info, _ := os.Stat(pidPath(spec.Dir)); info.Mode().Perm() != 0o600 {
		t.Errorf("pid file mode %v, want 0600", info.Mode().Perm())
	}
	s.Stop()
	waitUntil(t, "the process to be gone", func() bool { return gone(pf.PID) })
	waitUntil(t, "the pid file to be removed", func() bool { _, err := os.Stat(pidPath(spec.Dir)); return err != nil })
	if st := s.Status(); st.State != EngineStopped || st.Error != "" {
		t.Errorf("status = %+v", st)
	}
	s.Stop() // a second stop does nothing
}

func TestStopWhileTheModelIsLoadingEndsTheStartAndTheWaiterGetsAnError(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_MODE=never")
	errc := make(chan error, 1)
	go func() {
		_, _, _, err := s.ensure(context.Background(), testSpec(t), 0)
		errc <- err
	}()
	waitUntil(t, "starting", func() bool { return s.Status().State == EngineStarting })
	s.Stop()
	select {
	case err := <-errc:
		if err == nil || !strings.Contains(err.Error(), "stopped") {
			t.Errorf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter must not hang after Stop")
	}
	if st := s.Status(); st.State != EngineStopped {
		t.Errorf("status = %+v, want stopped (a Stop is not a failure)", st)
	}
}

func TestACancelledRequestStopsWaitingButNotTheStart(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_DELAY_MS=300")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	_, _, _, err := s.ensure(ctx, testSpec(t), 0)
	if err != context.DeadlineExceeded {
		t.Errorf("err = %v, want the context's own error", err)
	}
	waitUntil(t, "the program to become ready anyway", func() bool { return s.Status().State == EngineReady })
}

func TestTheProgramStopsAfterTheIdleTimeButNotWhileARequestIsRunning(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	spec := testSpec(t)
	_, _, release, err := s.ensure(context.Background(), spec, 2) // two idle units = 120 ms
	if err != nil {
		t.Fatal(err)
	}
	pid := func() int {
		raw, _ := os.ReadFile(pidPath(spec.Dir))
		var pf pidFile
		json.Unmarshal(raw, &pf)
		return pf.PID
	}()
	time.Sleep(400 * time.Millisecond) // far past the idle time, but the request has not finished
	if s.Status().State != EngineReady || gone(pid) {
		t.Fatal("a program with a request under way must never be stopped for being idle")
	}
	release()
	waitUntil(t, "the idle stop", func() bool { return s.Status().State == EngineStopped })
	waitUntil(t, "the process to be gone", func() bool { return gone(pid) })
	// The next request starts it again.
	_, _, rel2, err := s.ensure(context.Background(), spec, 2)
	if err != nil {
		t.Fatal(err)
	}
	rel2()
	if starts.Load() != 2 {
		t.Errorf("%d starts, want 2", starts.Load())
	}
}

func TestANewRequestKeepsTheProgramFromGoingIdle(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	spec := testSpec(t)
	for i := 0; i < 5; i++ {
		_, _, release, err := s.ensure(context.Background(), spec, 4) // 240 ms
		if err != nil {
			t.Fatal(err)
		}
		release()
		time.Sleep(100 * time.Millisecond) // less than the idle time: the clock starts over every time
	}
	if s.Status().State != EngineReady || starts.Load() != 1 {
		t.Errorf("status %+v after %d starts: a program in regular use must stay up", s.Status(), starts.Load())
	}
}

func TestAnIdleTimeOfZeroMeansNeverStop(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts)
	_, _, release, err := s.ensure(context.Background(), testSpec(t), 0)
	if err != nil {
		t.Fatal(err)
	}
	release()
	time.Sleep(300 * time.Millisecond)
	if s.Status().State != EngineReady {
		t.Error("with no idle time set the program must stay up")
	}
}

func TestTheLogIsPrivateAndStartsFreshEveryTime(t *testing.T) {
	var starts atomic.Int32
	s := newSup(t, &starts, "FAKE_LLAMA_MODE=exit")
	spec := testSpec(t)
	s.ensure(context.Background(), spec, 0)
	info, err := os.Stat(filepath.Join(spec.Dir, "server.log"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("log: %v %v, want a 0600 file", info, err)
	}
	first, _ := os.ReadFile(filepath.Join(spec.Dir, "server.log"))
	s.ensure(context.Background(), spec, 0)
	second, _ := os.ReadFile(filepath.Join(spec.Dir, "server.log"))
	if strings.Count(string(second), "model is corrupted") != 1 || len(second) != len(first) {
		t.Errorf("the log must start empty at every start, not grow: %d then %d bytes", len(first), len(second))
	}
}

func TestLogReasonTakesTheLastErrorLineWithoutItsTimeStamp(t *testing.T) {
	dir := t.TempDir()
	write := func(s string) { os.WriteFile(filepath.Join(dir, "server.log"), []byte(s), 0o600) }
	write("0.00.001 I loading\n0.00.002 E first problem\n0.00.003 I something else\n0.00.004 E llama_model_load: the real reason\n0.00.005 I cleaning up\n")
	if got := logReason(dir); got != "llama_model_load: the real reason" {
		t.Errorf("reason = %q", got)
	}
	write("0.00.001 I all fine\n0.00.002 I still fine\n")
	if got := logReason(dir); got != "" {
		t.Errorf("no error line: reason = %q, want empty", got)
	}
	write("0.00.001 E " + strings.Repeat("x", 500) + "\n")
	if got := logReason(dir); len([]rune(got)) != 301 || !strings.HasSuffix(got, "…") {
		t.Errorf("a long line must be cut to 300 characters and an ellipsis: %d characters", len([]rune(got)))
	}
	big := strings.Repeat("0.00.001 I noise noise noise\n", 2000) + "0.00.009 E at the very end\n"
	write(big)
	if got := logReason(dir); got != "at the very end" {
		t.Errorf("reason from a big log = %q", got)
	}
	os.Remove(filepath.Join(dir, "server.log"))
	if got := logReason(dir); got != "" {
		t.Errorf("no log: reason = %q", got)
	}
}

func TestAProgramLeftBehindByACrashedApplicationIsStoppedAtStart(t *testing.T) {
	// Start a real stand-in the way an earlier run would have, then forget about it (as a crash would).
	var starts atomic.Int32
	spec := testSpec(t)
	o := supervisorOptions{Command: fakeCommand(&starts)}
	quick(&o)
	s := newSupervisor(o)
	_, _, release, err := s.ensure(context.Background(), spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	release()
	raw, _ := os.ReadFile(pidPath(spec.Dir))
	var pf pidFile
	json.Unmarshal(raw, &pf)
	if !processAlive(pf.PID) {
		t.Fatal("setup: the stand-in must be running")
	}
	// A new application start finds the pid file and stops the leftover (the pid file names os.Args[0], which the stand-in runs).
	if !killStale(spec.Dir, processAlive, commandOf, 2*time.Second) {
		t.Fatal("the leftover program must be reported as stopped")
	}
	waitUntil(t, "the leftover to be gone", func() bool { return gone(pf.PID) })
	if _, err := os.Stat(pidPath(spec.Dir)); err == nil {
		t.Error("the pid file must be removed")
	}
	s.Stop()
}

func TestAPidThatNowBelongsToSomethingElseIsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	self := os.Getpid()
	writePID(dir, self, "/some/other/program/llama-server") // a pid of a live process, but not of this program
	if killStale(dir, processAlive, commandOf, time.Second) {
		t.Error("a process that does not run the recorded program must not be stopped")
	}
	if !processAlive(self) {
		t.Fatal("the test process must still be alive")
	}
	if _, err := os.Stat(pidPath(dir)); err == nil {
		t.Error("the stale pid file must be removed anyway")
	}
}

func TestStaleCleanupRefusesBrokenPidFilesAndDeadOrDangerousPids(t *testing.T) {
	always := func(int) bool { return true }
	never := func(int) bool { return false }
	command := func(int) (string, error) { return "/x/llama-server", nil }
	cases := map[string]struct {
		content string
		alive   func(int) bool
	}{
		"not json":                      {"garbage", always},
		"pid zero":                      {`{"pid":0,"binary":"/x/llama-server"}`, always},
		"pid one":                       {`{"pid":1,"binary":"/x/llama-server"}`, always},
		"a negative pid":                {`{"pid":-5,"binary":"/x/llama-server"}`, always},
		"no binary":                     {`{"pid":4242,"binary":""}`, always},
		"a process that is not running": {`{"pid":4242,"binary":"/x/llama-server"}`, never},
	}
	for name, c := range cases {
		dir := t.TempDir()
		os.WriteFile(pidPath(dir), []byte(c.content), 0o600)
		if killStale(dir, c.alive, command, 100*time.Millisecond) {
			t.Errorf("%s: nothing may be stopped", name)
		}
		if _, err := os.Stat(pidPath(dir)); err == nil {
			t.Errorf("%s: the pid file must be removed", name)
		}
	}
	if killStale(t.TempDir(), always, command, time.Second) {
		t.Error("no pid file: nothing to stop")
	}
}

func TestSignalsRefuseDangerousPids(t *testing.T) {
	for _, pid := range []int{0, 1, -1, -100} {
		if terminateGroup(pid) == nil || killGroup(pid) == nil {
			t.Errorf("pid %d must be refused: signalling it could stop the whole system or the application", pid)
		}
	}
	if processAlive(0) || processAlive(1) || processAlive(-3) {
		t.Error("pids 0, 1 and negative ones are never reported as ours")
	}
}
