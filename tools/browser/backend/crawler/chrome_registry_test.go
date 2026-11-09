package crawler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"browser-tool-backend/shared/chromeproc"
)

func entryFor(pid int) (chromeproc.Entry, bool) {
	for _, e := range chromeproc.Entries() {
		if e.PID == pid {
			return e, true
		}
	}
	return chromeproc.Entry{}, false
}

// A Chrome the rod launch starts is registered with its process id and kind, and the one stop path (closeRodBrowser) releases it (plan 224).
func TestARodChromeIsRegisteredWhileItRunsAndReleasedWhenClosed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	path, err := rodChromePath()
	if err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	b, l, err := launchRodBrowser(path, "")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	pid := l.PID()
	e, ok := entryFor(pid)
	if !ok {
		closeRodBrowser(b, l)
		t.Fatalf("pid %d is not registered: %+v", pid, chromeproc.Entries())
	}
	if e.Kind != chromeproc.KindRod || e.Binary != path || e.ProfileDir == "" {
		t.Errorf("%+v", e)
	}
	// plan 224, step 2: the Chrome has its own named profile folder under the tool's root, and its command line proves it is ours
	if filepath.Dir(e.ProfileDir) != chromeproc.ProfileRoot() || !strings.HasPrefix(filepath.Base(e.ProfileDir), "rod-") {
		t.Errorf("profile folder %q is not a named folder under %q", e.ProfileDir, chromeproc.ProfileRoot())
	}
	if !chromeproc.IsOursPID(pid) {
		t.Errorf("the running Chrome's command line must show a profile folder of this tool")
	}
	closeRodBrowser(b, l)
	if _, ok := entryFor(pid); ok {
		t.Errorf("pid %d must be released by closeRodBrowser", pid)
	}
	if _, err := os.Stat(e.ProfileDir); !os.IsNotExist(err) {
		t.Errorf("the profile folder must be removed when the Chrome is closed: %v", err)
	}
}

func TestAKatanaChromeIsRegisteredAndReleasedByItsStop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	path, err := rodChromePath()
	if err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	b, l, _, err := launchKatanaChrome(path, "")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	pid := l.PID()
	e, ok := entryFor(pid)
	if !ok || e.Kind != chromeproc.KindKatana {
		closeRodBrowser(b, l)
		t.Fatalf("%+v %v", e, ok)
	}
	if filepath.Dir(e.ProfileDir) != chromeproc.ProfileRoot() || !strings.HasPrefix(filepath.Base(e.ProfileDir), "katana-") || !chromeproc.IsOursPID(pid) {
		t.Errorf("profile folder %q, ours=%v", e.ProfileDir, chromeproc.IsOursPID(pid))
	}
	closeRodBrowser(b, l)
	if _, err := os.Stat(e.ProfileDir); !os.IsNotExist(err) {
		t.Errorf("the profile folder must be removed: %v", err)
	}
	if _, ok := entryFor(pid); ok {
		t.Error("closing must release it")
	}
}

// The registry hears of the engine's work by kind, so its idle figure follows the fetches.
func TestAFetchTouchesTheEnginesChromeByKind(t *testing.T) {
	chromeproc.Register(chromeproc.Launch{Kind: chromeproc.KindKatana, PID: 987654})
	chromeproc.Register(chromeproc.Launch{Kind: chromeproc.KindRod, PID: 987655})
	t.Cleanup(func() { chromeproc.Release(987654); chromeproc.Release(987655) })
	before := map[int]chromeproc.Entry{}
	for _, e := range chromeproc.Entries() {
		before[e.PID] = e
	}
	touchEngineChrome(fetchEngineKatana)
	after := map[int]chromeproc.Entry{}
	for _, e := range chromeproc.Entries() {
		after[e.PID] = e
	}
	if after[987654].LastUsedAt.Before(before[987654].LastUsedAt) || after[987655].LastUsedAt != before[987655].LastUsedAt {
		t.Errorf("only the katana Chrome may be touched: %+v -> %+v", before, after)
	}
}

// The tree stopper ends a real headless Chrome with all its helper processes, and the usual close afterwards still works (plan 224, step 2).
func TestStopTreeEndsARealChromeAndItsHelpers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	path, err := rodChromePath()
	if err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	b, l, err := launchRodBrowser(path, "")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	pid := l.PID()
	e, _ := entryFor(pid)
	if err := chromeproc.StopTree(pid, 5*time.Second); err != nil {
		closeRodBrowser(b, l)
		t.Fatalf("StopTree: %v", err)
	}
	if _, ok := entryFor(pid); ok {
		t.Error("a stopped Chrome must be released")
	}
	if err := syscall.Kill(pid, 0); err == nil {
		// the launcher is the parent and reaps it; give it a moment
		time.Sleep(500 * time.Millisecond)
	}
	closed := make(chan struct{})
	go func() { closeRodBrowser(b, l); close(closed) }()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("closeRodBrowser must still return after the Chrome was stopped from outside")
	}
	if e.ProfileDir != "" {
		if _, err := os.Stat(e.ProfileDir); !os.IsNotExist(err) {
			t.Errorf("the profile folder must be removed: %v", err)
		}
	}
	if out, _ := exec.Command("/bin/ps", "-axo", "command=").Output(); strings.Contains(string(out), e.ProfileDir) && e.ProfileDir != "" {
		t.Errorf("a process with our profile folder is still running")
	}
}

// The crash case end to end (plan 224, step 3): a Chrome is left running by "an earlier run" (it is in the ledger, the tool never closed it);
// at the next start the ledger is kept aside by Configure and Sweep stops that Chrome and deletes its folder.
func TestTheStartUpSweepStopsARealChromeLeftByAnEarlierRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	path, err := rodChromePath()
	if err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	b, l, err := launchRodBrowser(path, "")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	pid := l.PID()
	e, ok := entryFor(pid)
	if !ok {
		closeRodBrowser(b, l)
		t.Fatal("not registered")
	}
	// "the tool crashed": nothing is closed. The next start configures the ledger folder again and sweeps.
	if err := chromeproc.Configure(testChromeDir); err != nil {
		closeRodBrowser(b, l)
		t.Fatal(err)
	}
	rep := chromeproc.Sweep()
	if len(rep.Stopped) == 0 {
		closeRodBrowser(b, l)
		t.Fatalf("the Chrome of the earlier run must be stopped: %+v", rep)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && chromeproc.IsOursPID(pid) {
		time.Sleep(50 * time.Millisecond)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		// still listed: it is a zombie waiting for its launcher (this test) to reap it, which closeRodBrowser does below
		t.Logf("pid %d is not reaped yet", pid)
	}
	closed := make(chan struct{})
	go func() { closeRodBrowser(b, l); close(closed) }()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("closeRodBrowser must still return")
	}
	if _, err := os.Stat(e.ProfileDir); !os.IsNotExist(err) {
		t.Errorf("the profile folder must be gone: %v", err)
	}
	if out, _ := exec.Command("/bin/ps", "-axo", "command=").Output(); strings.Contains(string(out), e.ProfileDir) {
		t.Error("no process may still use that folder")
	}
}
