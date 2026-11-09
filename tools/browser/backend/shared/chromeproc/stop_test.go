package chromeproc

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// A small tree: 100 (Chrome, ours) -> 101, 102 -> 103 (helpers), and unrelated processes 200 and 300.
func chromeTable(t *testing.T) (*Registry, *fakeProc, string) {
	t.Helper()
	root := withRoots(t)
	fp := newFake()
	for _, p := range []int{100, 101, 102, 103, 200, 300} {
		fp.alive[p] = true
	}
	fp.parents = map[int]int{100: 5000, 101: 100, 102: 100, 103: 101, 200: 1, 300: 5000}
	fp.starts[100] = "Mon Nov 9 14:00:00 2026"
	fp.cmds[100] = "/Chrome --headless --user-data-dir=" + root + "/katana-ab12"
	fp.cmds[200] = "/Chrome --user-data-dir=/Users/me/Library/Application Support/Google/Chrome"
	fp.cmds[300] = "/usr/bin/vim notes.txt"
	return New("", fp, nil), fp, root
}

func TestStopTreeStopsChildrenFirstThenTheChromeAndReleasesIt(t *testing.T) {
	r, fp, _ := chromeTable(t)
	r.Register(Launch{Kind: KindKatana, PID: 100})
	if err := r.StopTree(100, time.Second); err != nil {
		t.Fatal(err)
	}
	for _, p := range []int{100, 101, 102, 103} {
		if fp.alive[p] {
			t.Errorf("%d must be gone", p)
		}
	}
	if !fp.alive[200] || !fp.alive[300] {
		t.Error("unrelated processes must not be touched")
	}
	if len(fp.killed) != 0 {
		t.Errorf("everything ended on SIGTERM, nothing may be killed: %v", fp.killed)
	}
	// the deepest child is asked before its parent, the Chrome last
	pos := map[int]int{}
	for i, p := range fp.terminated {
		pos[p] = i
	}
	if !(pos[103] < pos[101] && pos[101] < pos[100] && pos[102] < pos[100]) {
		t.Errorf("order %v", fp.terminated)
	}
	if len(r.Entries()) != 0 {
		t.Errorf("a stopped Chrome must be released: %+v", r.Entries())
	}
}

func TestAProcessThatIgnoresTheRequestIsKilledAfterTheGraceAndOnlyThen(t *testing.T) {
	r, fp, _ := chromeTable(t)
	r.Register(Launch{Kind: KindKatana, PID: 100})
	fp.ignoreTerm[102] = true
	start := time.Now()
	if err := r.StopTree(100, 150*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Error("the grace must be given before a kill")
	}
	if len(fp.killed) != 1 || fp.killed[0] != 102 {
		t.Errorf("only the stubborn process may be killed: %v", fp.killed)
	}
	if fp.alive[102] {
		t.Error("it must be gone")
	}
}

func TestAProcessThatSurvivesTheKillIsAnErrorAndStaysRegistered(t *testing.T) {
	r, fp, _ := chromeTable(t)
	r.Register(Launch{Kind: KindKatana, PID: 100})
	fp.ignoreTerm[103], fp.surviveKill[103] = true, true
	if err := r.StopTree(100, 50*time.Millisecond); err == nil || !strings.Contains(err.Error(), "still running") {
		t.Errorf("%v", err)
	}
	if len(r.Entries()) != 1 {
		t.Error("it is not gone, so it must stay registered")
	}
}

func TestADeadChromeIsReleasedAndNothingIsSignalled(t *testing.T) {
	r, fp, _ := chromeTable(t)
	r.Register(Launch{Kind: KindKatana, PID: 100})
	fp.alive[100] = false
	if err := r.StopTree(100, time.Second); err != nil {
		t.Fatal(err)
	}
	if len(fp.terminated)+len(fp.killed) != 0 || len(r.Entries()) != 0 {
		t.Errorf("%v %v %+v", fp.terminated, fp.killed, r.Entries())
	}
}

// The proof rule: nothing is signalled unless it is provably ours.
func TestADecoyIsNeverTouched(t *testing.T) {
	r, fp, root := chromeTable(t)
	// 1. somebody else's Chrome (the person's own profile)
	if err := r.StopTree(200, time.Second); !errors.Is(err, ErrNotOurs) {
		t.Errorf("another Chrome: %v", err)
	}
	// 2. an unrelated program
	if err := r.StopTree(300, time.Second); !errors.Is(err, ErrNotOurs) {
		t.Errorf("an unrelated program: %v", err)
	}
	// 3. our flags, but a profile folder that merely starts like ours
	fp.alive[400] = true
	fp.cmds[400] = "/Chrome --user-data-dir=" + root + "-evil/x"
	if err := r.StopTree(400, time.Second); !errors.Is(err, ErrNotOurs) {
		t.Errorf("a look-alike folder: %v", err)
	}
	// 4. a registered id whose process is not the one that was registered (the id was reused after the Chrome ended)
	r.Register(Launch{Kind: KindRod, PID: 500})
	fp.alive[500] = true
	fp.starts[500] = "Mon Nov 9 14:10:00 2026"
	fp.cmds[500] = "/usr/bin/vim"
	r.mu.Lock()
	r.entries[500].ProcessStart = "Mon Nov 9 09:00:00 2026"
	r.mu.Unlock()
	if err := r.StopTree(500, time.Second); !errors.Is(err, ErrNotOurs) {
		t.Errorf("a reused id: %v", err)
	}
	// 5. a registered process with no recorded start time cannot be told from a reused id
	r.Register(Launch{Kind: KindRod, PID: 600})
	fp.alive[600] = true
	r.mu.Lock()
	r.entries[600].ProcessStart = ""
	r.mu.Unlock()
	if err := r.StopTree(600, time.Second); !errors.Is(err, ErrNotOurs) {
		t.Errorf("no start time: %v", err)
	}
	if len(fp.terminated)+len(fp.killed) != 0 {
		t.Errorf("not one signal may have been sent: %v %v", fp.terminated, fp.killed)
	}
	for _, p := range []int{200, 300, 400, 500, 600} {
		if !fp.alive[p] {
			t.Errorf("%d must still be alive", p)
		}
	}
}

func TestAnUnregisteredChromeInOurFolderCanBeStoppedAnOrphan(t *testing.T) {
	r, fp, root := chromeTable(t)
	fp.alive[700] = true
	fp.cmds[700] = "/Chrome --headless --user-data-dir=" + root + "/rod-9f00"
	fp.parents[700] = 1
	if err := r.StopTree(700, time.Second); err != nil {
		t.Fatal(err)
	}
	if fp.alive[700] {
		t.Error("an orphan with our profile must be stopped")
	}
}

func TestProtectedIdsAreNeverSignalled(t *testing.T) {
	r, fp, _ := chromeTable(t)
	for _, pid := range []int{0, 1, -7, os.Getpid(), os.Getppid()} {
		fp.alive[pid] = true
		if err := r.StopTree(pid, time.Second); !errors.Is(err, ErrProtected) {
			t.Errorf("%d: %v", pid, err)
		}
	}
	if len(fp.terminated)+len(fp.killed) != 0 {
		t.Errorf("%v %v", fp.terminated, fp.killed)
	}
}

func TestAChildThatIsProtectedIsSkippedNotSignalled(t *testing.T) {
	r, fp, _ := chromeTable(t)
	r.Register(Launch{Kind: KindKatana, PID: 100})
	fp.parents[os.Getpid()] = 100 // a table that claims this very process is below the Chrome
	fp.alive[os.Getpid()] = true
	if err := r.StopTree(100, time.Second); err != nil {
		t.Fatal(err)
	}
	for _, p := range append(fp.terminated, fp.killed...) {
		if p == os.Getpid() {
			t.Error("this process must never be signalled")
		}
	}
}

func TestDescendantsAreDeepestFirstAndALoopCannotRunForever(t *testing.T) {
	got := descendants(map[int]int{2: 1, 3: 2, 4: 2, 5: 3, 9: 8}, 1)
	pos := map[int]int{}
	for i, p := range got {
		pos[p] = i
	}
	if len(got) != 4 || !(pos[5] < pos[3] && pos[3] < pos[2] && pos[4] < pos[2]) {
		t.Errorf("%v", got)
	}
	if loop := descendants(map[int]int{2: 1, 1: 2}, 1); len(loop) > 100 {
		t.Errorf("a loop produced %d entries", len(loop))
	}
	if none := descendants(nil, 1); len(none) != 0 {
		t.Errorf("%v", none)
	}
}
