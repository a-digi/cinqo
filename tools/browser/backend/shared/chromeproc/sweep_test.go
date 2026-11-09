package chromeproc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLedger(t *testing.T, path string, entries ...Entry) {
	t.Helper()
	raw, _ := json.Marshal(ledgerFile{Version: ledgerVersion, Chromes: entries})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func withLedgerDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := currentLedgerDir()
	t.Cleanup(func() { setLedgerDir(old) })
	setLedgerDir(dir)
	return dir
}

// A computer after a crash: the earlier run's Chrome 100 (with helpers 101, 103) still runs, id 150 now belongs to another program, 160 is gone;
// 700 and 702 are orphans with our folder, 701 is another running instance's Chrome (its parent is alive); 200 and 300 are not ours.
func crashedComputer(t *testing.T) (*Registry, *fakeProc, string, string) {
	t.Helper()
	root := withRoots(t)
	dir := withLedgerDir(t)
	fp := newFake()
	for _, p := range []int{100, 101, 103, 150, 200, 300, 700, 701, 702, 5000, 800} {
		fp.alive[p] = true
	}
	fp.alive[41], fp.alive[42] = false, false
	fp.parents = map[int]int{100: 1, 101: 100, 103: 101, 150: 1, 200: 1, 300: 5000, 700: 1, 701: 5000, 702: 9999, 5000: 1, 800: 1}
	fp.starts[100] = "Mon Nov 9 14:00:00 2026"
	fp.starts[150] = "Mon Nov 9 16:30:00 2026" // not what the ledger recorded
	fp.cmds[100] = "/Chrome --headless --user-data-dir=" + root + "/katana-aaaa0001"
	fp.cmds[101] = "/Chrome --type=renderer --user-data-dir=" + root + "/katana-aaaa0001"
	fp.cmds[103] = "/Chrome --type=gpu-process --user-data-dir=" + root + "/katana-aaaa0001"
	fp.cmds[150] = "/usr/bin/vim"
	fp.cmds[200] = "/Chrome --user-data-dir=/Users/me/Library/Application Support/Google/Chrome"
	fp.cmds[300] = "/usr/bin/python"
	fp.cmds[700] = "/Chrome --headless --user-data-dir=" + root + "/rod-bbbb0002"
	fp.cmds[701] = "/Chrome --headless --user-data-dir=" + root + "/katana-cccc0003"
	fp.cmds[702] = "/Chrome --type=renderer --user-data-dir=" + root + "/rod-dddd0004"
	fp.cmds[800] = "/Chrome --headless --user-data-dir=" + root + "/katana-eeee0005" // registered in this run
	writeLedger(t, filepath.Join(dir, previousLedgerName),
		Entry{PID: 100, Kind: KindKatana, ProcessStart: "Mon Nov 9 14:00:00 2026", ProfileDir: root + "/katana-aaaa0001"},
		Entry{PID: 150, Kind: KindRod, ProcessStart: "Mon Nov 9 10:00:00 2026", ProfileDir: root + "/rod-ffff0006"},
		Entry{PID: 160, Kind: KindRod, ProcessStart: "Mon Nov 9 10:00:00 2026"},
	)
	for _, name := range []string{"katana-aaaa0001", "rod-bbbb0002", "katana-cccc0003", "rod-dddd0004", "katana-eeee0005", "rod-ffff0006", "rod-gone0007"} {
		os.MkdirAll(filepath.Join(root, name, "Default"), 0o700)
		os.WriteFile(filepath.Join(root, name, "Default", "Cookies"), []byte("x"), 0o600)
	}
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("keep"), 0o600)
	r := New("", fp, nil)
	r.mu.Lock()
	r.entries[800] = &Entry{PID: 800, Kind: KindKatana}
	r.mu.Unlock()
	return r, fp, root, dir
}

func TestTheSweepStopsWhatTheEarlierRunLeftAndOnlyThat(t *testing.T) {
	r, fp, root, dir := crashedComputer(t)
	rep := r.Sweep()

	for _, p := range []int{100, 101, 103, 700, 702} {
		if fp.alive[p] {
			t.Errorf("%d must have been stopped", p)
		}
	}
	for _, p := range []int{150, 200, 300, 701, 5000, 800} {
		if !fp.alive[p] {
			t.Errorf("%d must not be touched", p)
		}
	}
	for _, p := range append(fp.terminated, fp.killed...) {
		switch p {
		case 150, 200, 300, 701, 5000, 800:
			t.Errorf("a signal was sent to %d", p)
		}
	}
	if len(rep.Stopped) != 3 || len(rep.Skipped) != 1 || rep.Skipped[0] != 150 || len(rep.Failed) != 0 {
		t.Errorf("%+v", rep)
	}
	// profile folders: those nothing uses are gone, the used ones and the plain file stay
	for name, want := range map[string]bool{
		"katana-aaaa0001": false, "rod-bbbb0002": false, "rod-dddd0004": false, "rod-ffff0006": false, "rod-gone0007": false,
		"katana-cccc0003": true, "katana-eeee0005": true,
	} {
		_, err := os.Stat(filepath.Join(root, name))
		if (err == nil) != want {
			t.Errorf("%s: kept=%v, want %v", name, err == nil, want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); err != nil {
		t.Error("a plain file in the root is never deleted")
	}
	if rep.FoldersRemoved != 5 {
		t.Errorf("%d folders removed, want 5", rep.FoldersRemoved)
	}
	if _, err := os.Stat(filepath.Join(dir, previousLedgerName)); !os.IsNotExist(err) {
		t.Error("the earlier ledger must be removed when the sweep is done")
	}
	if !strings.Contains(rep.String(), "stopped 3") {
		t.Errorf("%s", rep)
	}
}

func TestASweepOfAComputerWithNothingLeftChangesNothing(t *testing.T) {
	root := withRoots(t)
	withLedgerDir(t)
	fp := newFake()
	fp.alive = map[int]bool{200: true}
	fp.parents = map[int]int{200: 1}
	fp.cmds[200] = "/Chrome --user-data-dir=/Users/me/x"
	rep := New("", fp, nil).Sweep()
	if len(rep.Stopped)+len(rep.Skipped)+len(rep.Failed)+rep.FoldersRemoved != 0 || len(fp.terminated)+len(fp.killed) != 0 {
		t.Errorf("%+v", rep)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("a missing profile root is not created by a sweep: %v", err)
	}
}

func TestWhenTheProcessTableCannotBeReadNothingIsTouchedOrDeleted(t *testing.T) {
	r, fp, root, dir := crashedComputer(t)
	fp.noList = true
	rep := r.Sweep()
	if len(fp.terminated)+len(fp.killed) != 0 || rep.FoldersRemoved != 0 || rep.Note == "" {
		t.Errorf("%+v", rep)
	}
	if _, err := os.Stat(filepath.Join(root, "rod-gone0007")); err != nil {
		t.Error("without knowing what runs, no folder may be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, previousLedgerName)); err != nil {
		t.Error("the earlier ledger must be kept for the next try")
	}
}

func TestADamagedEarlierLedgerStillLetsTheOrphanHuntRun(t *testing.T) {
	r, fp, _, dir := crashedComputer(t)
	os.WriteFile(filepath.Join(dir, previousLedgerName), []byte("{broken"), 0o600)
	rep := r.Sweep()
	if fp.alive[700] || fp.alive[702] || !fp.alive[150] || !fp.alive[701] {
		t.Errorf("orphans stop, the rest stays: %+v", rep)
	}
	if fp.alive[100] {
		t.Log("100 is also an orphan (parent is init) with our folder, so it is stopped by the orphan hunt")
	} else if len(rep.Stopped) < 3 {
		t.Errorf("%+v", rep)
	}
}

func TestAChromeThatCannotBeStoppedIsReportedAsFailed(t *testing.T) {
	r, fp, _, _ := crashedComputer(t)
	fp.ignoreTerm[700], fp.surviveKill[700] = true, true
	rep := r.Sweep()
	if len(rep.Failed) != 1 || rep.Failed[0] != 700 {
		t.Errorf("%+v", rep)
	}
}

func TestAnEntryWithoutAStartTimeIsOnlyStoppedWhenItsCommandLineIsOurs(t *testing.T) {
	root := withRoots(t)
	dir := withLedgerDir(t)
	fp := newFake()
	fp.alive = map[int]bool{900: true, 901: true}
	fp.parents = map[int]int{900: 5000, 901: 5000}
	fp.cmds[900] = "/usr/bin/vim"
	fp.cmds[901] = "/Chrome --user-data-dir=" + root + "/rod-1111aaaa"
	writeLedger(t, filepath.Join(dir, previousLedgerName), Entry{PID: 900, Kind: KindRod}, Entry{PID: 901, Kind: KindRod})
	rep := New("", fp, nil).Sweep()
	if !fp.alive[900] || fp.alive[901] || len(rep.Skipped) != 1 || rep.Skipped[0] != 900 {
		t.Errorf("%+v alive900=%v alive901=%v", rep, fp.alive[900], fp.alive[901])
	}
}

func TestProtectedIdsInTheEarlierLedgerAreSkipped(t *testing.T) {
	withRoots(t)
	dir := withLedgerDir(t)
	fp := newFake()
	me := os.Getpid()
	fp.alive = map[int]bool{me: true, 1: true}
	fp.parents = map[int]int{me: 1, 1: 0}
	fp.starts[me], fp.starts[1] = "now", "boot"
	writeLedger(t, filepath.Join(dir, previousLedgerName), Entry{PID: me, ProcessStart: "now"}, Entry{PID: 1, ProcessStart: "boot"})
	rep := New("", fp, nil).Sweep()
	if len(fp.terminated)+len(fp.killed) != 0 || len(rep.Skipped) != 2 {
		t.Errorf("%+v %v %v", rep, fp.terminated, fp.killed)
	}
}

func TestTheSweepWithoutAProcessReaderDoesNothing(t *testing.T) {
	rep := New("", nil, nil).Sweep()
	if rep.Note == "" || rep.FoldersRemoved != 0 {
		t.Errorf("%+v", rep)
	}
}

func TestFolderInUseNeedsTheWholeFolderName(t *testing.T) {
	table := []ProcInfo{{PID: 1, Command: "/Chrome --user-data-dir=/r/katana-ab12/Default --x"}, {PID: 2, Command: "/Chrome --user-data-dir=/r/katana-ab123"}}
	if !folderInUse(table, "/r/katana-ab12") {
		t.Error("a process inside the folder uses it")
	}
	if folderInUse(table, "/r/katana-ab1") {
		t.Error("a folder whose name is only the start of another's is not in use")
	}
	if !folderInUse(table, "/r/katana-ab123") {
		t.Error("the exact folder is in use")
	}
	if folderInUse(nil, "/r/x") {
		t.Error("nothing runs, nothing is in use")
	}
}

func TestConfigureThenSweepFindsTheEarlierRunsLedger(t *testing.T) {
	root := t.TempDir()
	old := std
	oldDir := currentLedgerDir()
	t.Cleanup(func() { std = old; setLedgerDir(oldDir) })
	oldRoot := ProfileRoot()
	t.Cleanup(func() { setProfileRoot(oldRoot) })

	fp := newFake()
	// an earlier run registered Chrome 41 and died
	prev := New(filepath.Join(root, ledgerName), fp, nil)
	prev.Register(Launch{Kind: KindKatana, PID: 41})
	fp.parents = map[int]int{41: 5000}
	fp.cmds[41] = "/Chrome --headless"
	std = New("", fp, nil)
	if err := Configure(root); err != nil {
		t.Fatal(err)
	}
	rep := Sweep()
	if fp.alive[41] || len(rep.Stopped) != 1 || rep.Stopped[0] != 41 {
		t.Errorf("the entry of the earlier run must be found and stopped: %+v", rep)
	}
}
