//go:build unix

package chromeproc

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A real process tree: a shell with our profile folder in its command line that runs a child (sleep). StopTree must end both, and a look-alike
// shell with another folder must be refused and keep running.
func TestStopTreeOnARealProcessTree(t *testing.T) {
	root := withRoots(t)
	ours := filepath.Join(root, "katana-test0001")
	other := filepath.Join(filepath.Dir(root), "somebody-else")

	mk := func(dir string) *exec.Cmd {
		c := exec.Command("sh", "-c", "sleep 60; true", "sh", "--user-data-dir="+dir)
		if err := c.Start(); err != nil {
			t.Skip("cannot start a shell")
		}
		t.Cleanup(func() { _ = c.Process.Kill(); _ = c.Wait() })
		return c
	}
	mine, decoy := mk(ours), mk(other)
	r := New("", realProc{}, nil)
	waitForChild := func(parent int) bool {
		for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
			for _, p := range (realProc{}).Parents() {
				if p == parent {
					return true
				}
			}
		}
		return false
	}
	if !waitForChild(mine.Process.Pid) {
		t.Skip("the shell did not start a child here")
	}
	r.Register(Launch{Kind: KindKatana, PID: mine.Process.Pid})

	done := make(chan error, 1)
	go func() { done <- r.StopTree(mine.Process.Pid, 2*time.Second) }()
	// reap the shell as its parent (this test), which is what the tool's own launcher does with its Chrome
	go func() { _ = mine.Wait() }()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if (realProc{}).Alive(mine.Process.Pid) {
		t.Error("the shell must be gone")
	}
	if err := r.StopTree(decoy.Process.Pid, time.Second); err == nil {
		t.Error("a process with another profile folder must be refused")
	}
	if !(realProc{}).Alive(decoy.Process.Pid) {
		t.Error("the decoy must still be running")
	}
}
