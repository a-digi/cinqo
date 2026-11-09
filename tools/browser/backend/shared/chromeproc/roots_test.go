package chromeproc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withRoots(t *testing.T) string {
	t.Helper()
	oldRoot, oldOwned := profileRoot, ownedRoots
	t.Cleanup(func() { rootsMu.Lock(); profileRoot, ownedRoots = oldRoot, oldOwned; rootsMu.Unlock() })
	root := filepath.Join(t.TempDir(), "profiles")
	rootsMu.Lock()
	profileRoot, ownedRoots = root, nil
	rootsMu.Unlock()
	return root
}

func TestTheProfileRootDefaultsToTheTemporaryFolderAndFollowsConfigure(t *testing.T) {
	oldRoot := profileRoot
	t.Cleanup(func() { setProfileRoot(oldRoot) })
	setProfileRoot("")
	if got := ProfileRoot(); got != filepath.Join(os.TempDir(), "cinqo-browser-chrome", "profiles") {
		t.Errorf("%s", got)
	}
	old := std
	t.Cleanup(func() { std = old })
	std = New("", newFake(), nil)
	dir := t.TempDir()
	if err := Configure(dir); err != nil {
		t.Fatal(err)
	}
	if got := ProfileRoot(); got != filepath.Join(dir, "profiles") {
		t.Errorf("Configure must place the root beside the ledger: %s", got)
	}
}

func TestNewProfileDirMakesANamedOwnerOnlyFolderInsideTheRoot(t *testing.T) {
	root := withRoots(t)
	a, err := NewProfileDir(KindKatana)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewProfileDir(KindKatana)
	if a == b {
		t.Error("two folders must never share a name")
	}
	if filepath.Dir(a) != root || !strings.HasPrefix(filepath.Base(a), "katana-") || len(filepath.Base(a)) != len("katana-")+8 {
		t.Errorf("%s", a)
	}
	info, err := os.Stat(a)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Errorf("%v %v", info, err)
	}
}

func TestRemoveProfileDirOnlyRemovesWhatIsDirectlyInsideTheRoot(t *testing.T) {
	root := withRoots(t)
	dir, _ := NewProfileDir(KindRod)
	os.WriteFile(filepath.Join(dir, "Cookies"), []byte("x"), 0o600)
	if err := RemoveProfileDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the folder must be gone: %v", err)
	}
	if err := RemoveProfileDir(dir); err != nil {
		t.Errorf("a folder already gone is not an error: %v", err)
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "keep"), []byte("x"), 0o600)
	for _, bad := range []string{outside, root, filepath.Join(root, "a", "b"), filepath.Join(root, ".."), "", "/", filepath.Join(root, "x", "..", "..", filepath.Base(outside))} {
		if err := RemoveProfileDir(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Error("a refused removal must delete nothing")
	}
}

func TestACommandIsOursOnlyWhenItsProfileIsInsideOneOfOurFolders(t *testing.T) {
	root := withRoots(t)
	AddOwnedRoot("/data/window-profile")
	cases := map[string]bool{
		"/Chrome --headless --user-data-dir=" + root + "/katana-ab12 --remote-debugging-port=0": true,
		"/Chrome --user-data-dir=" + root + " --x":                                              true,  // the root itself
		"/Chrome --user-data-dir=" + root:                                                       true,  // at the end of the line
		"/Chrome --user-data-dir=" + root + "x/katana-ab12":                                     false, // another folder that starts alike
		"/Chrome --user-data-dir=" + root + "-other/a":                                          false,
		"/Chrome --user-data-dir=/Users/me/Library/Application Support/Google/Chrome":           false,
		"/Chrome --headless": false,
		"/Chrome --user-data-dir=/data/window-profile --x": true,
		"/Chrome --user-data-dir=/data/window-profile2":    false,
		"": false,
		"/Chrome --other-user-data-dir=" + root + "/a":                              false,
		"/Chrome --x=" + root + "/a":                                                false,
		"/Chrome --user-data-dir=" + root + "x --user-data-dir=" + root + "/second": true, // a later match still counts
	}
	for cmd, want := range cases {
		if got := IsOurs(cmd); got != want {
			t.Errorf("%q: got %v, want %v", cmd, got, want)
		}
	}
}

func TestAFolderWithSpacesInItsNameStillMatches(t *testing.T) {
	root := withRoots(t)
	spaced := filepath.Join(filepath.Dir(root), "Application Support", "profiles")
	setProfileRoot(spaced)
	if !IsOurs("/Chrome --user-data-dir=" + spaced + "/rod-1234 --headless") {
		t.Error("spaces in the folder name must not break the match")
	}
	if IsOurs("/Chrome --user-data-dir=" + filepath.Dir(spaced) + "/other") {
		t.Error("the parent of the root is not ours")
	}
}

func TestAddOwnedRootIgnoresEmptyRelativeAndRepeatedFolders(t *testing.T) {
	withRoots(t)
	AddOwnedRoot("")
	AddOwnedRoot("relative/dir")
	AddOwnedRoot("/data/a/")
	AddOwnedRoot("/data/a")
	AddOwnedRoot("/data/b/../b")
	got := OwnedRoots()
	count := map[string]int{}
	for _, r := range got {
		count[r]++
	}
	if len(got) != 3 || count["/data/a"] != 1 || count["/data/b"] != 1 || got[0] != ProfileRoot() {
		t.Errorf("%v", got)
	}
}
