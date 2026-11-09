//go:build unix

package chromeproc

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A real orphan: a shell with our profile folder in its command line whose parent has exited (so init owns it). The sweep must find it by its
// command line, stop it with its child, and delete its folder; a look-alike with another folder must keep running.
func TestTheSweepEndsARealOrphanAndLeavesALookAlike(t *testing.T) {
	root := withRoots(t)
	withLedgerDir(t)
	ours := filepath.Join(root, "katana-realtest")
	os.MkdirAll(ours, 0o700)
	other := filepath.Join(filepath.Dir(root), "not-ours-realtest")
	os.MkdirAll(other, 0o700)

	spawn := func(dir string) {
		// the inner shell is started in the background and the outer one exits at once, so the inner one is left to init
		c := exec.Command("sh", "-c", `(sh -c 'sleep 60; true' sh --user-data-dir="$0" &) ; exit 0`, dir)
		if err := c.Run(); err != nil {
			t.Skip("cannot start a shell")
		}
	}
	spawn(ours)
	spawn(other)
	t.Cleanup(func() {
		_ = exec.Command("pkill", "-f", "--", "--user-data-dir="+other).Run()
		_ = exec.Command("pkill", "-f", "--", "--user-data-dir="+ours).Run()
	})

	var p realProc
	find := func(folder string) bool {
		for _, row := range p.List() {
			if commandUnderRoot(row.Command, folder) {
				return true
			}
		}
		return false
	}
	deadline := time.Now().Add(3 * time.Second)
	for !(find(ours) && find(other)) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !find(ours) || !find(other) {
		t.Skip("the test processes did not show up in the process table")
	}

	rep := New("", p, nil).Sweep()
	if len(rep.Stopped) == 0 {
		t.Fatalf("the orphan must be stopped: %+v", rep)
	}
	deadline = time.Now().Add(3 * time.Second)
	for find(ours) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if find(ours) {
		t.Error("the orphan with our folder must be gone")
	}
	if !find(other) {
		t.Error("the look-alike with another folder must keep running")
	}
	if _, err := os.Stat(ours); !os.IsNotExist(err) {
		t.Errorf("the orphan's folder must be deleted: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("another folder must never be deleted")
	}
}

func TestTheRealProcessTableHasThisProcessWithItsCommandLine(t *testing.T) {
	var p realProc
	found := false
	for _, row := range p.List() {
		if row.PID == os.Getpid() {
			found = true
			if row.PPID != os.Getppid() || row.Command == "" {
				t.Errorf("%+v", row)
			}
		}
	}
	if !found {
		t.Skip("the process table does not list this process here")
	}
}
