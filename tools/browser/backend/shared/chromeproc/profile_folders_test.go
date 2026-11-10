package chromeproc

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func profileRegistry(t *testing.T, f *fakeProc, now time.Time) (*Registry, string) {
	t.Helper()
	root := t.TempDir()
	setProfileRoot(root)
	t.Cleanup(func() { setProfileRoot("") })
	return New("", f, func() time.Time { return now }), root
}

func folder(t *testing.T, root, name string, age time.Duration, now time.Time) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	past := now.Add(-age)
	if err := os.Chtimes(dir, past, past); err != nil {
		t.Fatal(err)
	}
	return dir
}

func inUse(list []ProfileFolder) map[string]bool {
	out := map[string]bool{}
	for _, p := range list {
		out[filepath.Base(p.Path)] = p.InUse
	}
	return out
}

func TestAProfileFolderIsInUseWhenAliveRegisteredNamedByAProcessOrYoung(t *testing.T) {
	now := time.Now()
	f := newFake()
	r, root := profileRegistry(t, f, now)
	registered := folder(t, root, "rod-registered", time.Hour, now)
	dead := folder(t, root, "rod-dead", time.Hour, now)
	named := folder(t, root, "rod-named", time.Hour, now)
	folder(t, root, "rod-idle", time.Hour, now)
	folder(t, root, "rod-young", 10*time.Second, now)
	if err := os.WriteFile(filepath.Join(root, "a-file"), []byte("x"), 0o600); err != nil { // not a folder: not listed
		t.Fatal(err)
	}
	r.Register(Launch{Kind: KindRod, PID: 41, ProfileDir: registered})
	f.alive[43] = false
	r.Register(Launch{Kind: KindRod, PID: 43, ProfileDir: dead}) // registered but its process is gone
	f.alive[77] = true
	f.cmds[77] = "chrome --user-data-dir=" + named + " --headless"

	got := inUse(r.profileFolders())
	want := map[string]bool{"rod-registered": true, "rod-dead": false, "rod-named": true, "rod-idle": false, "rod-young": true}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: in use %v, want %v (all: %v)", k, got[k], v, got)
		}
	}
}

func TestWhenTheProcessTableCannotBeReadEveryFolderCountsAsInUse(t *testing.T) {
	now := time.Now()
	f := newFake()
	f.noList = true
	r, root := profileRegistry(t, f, now)
	folder(t, root, "rod-a", 5*time.Hour, now)
	folder(t, root, "rod-b", 5*time.Hour, now)
	for k, v := range inUse(r.profileFolders()) {
		if !v {
			t.Errorf("%s: nothing can be told, so it must be kept", k)
		}
	}
}

func TestNoProfileRootMeansNoFolders(t *testing.T) {
	r, root := profileRegistry(t, newFake(), time.Now())
	_ = os.RemoveAll(root)
	if got := r.profileFolders(); len(got) != 0 {
		t.Errorf("%v", got)
	}
}

func TestTheLedgerFilesAreRecognised(t *testing.T) {
	var yes, no []string
	for _, n := range []string{"chrome-pids.json", ".chrome-pids-123.tmp"} {
		if IsLedgerFile(n) {
			yes = append(yes, n)
		}
	}
	for _, n := range []string{"chrome-pids.previous.json", "chrome-pids.previous.json.damaged", "profiles", "other.json", "chrome-pids.json.bak"} {
		if !IsLedgerFile(n) {
			no = append(no, n)
		}
	}
	sort.Strings(yes)
	if len(yes) != 2 || len(no) != 5 {
		t.Errorf("yes %v no %v", yes, no)
	}
}
