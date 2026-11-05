package shared

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func useLoginWindowRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "windows")
	old := loginWindowRoot
	loginWindowRoot = func() string { return root }
	t.Cleanup(func() { loginWindowRoot = old })
	return root
}

func TestTheLoginWindowAsksChromeForItsOwnFolderAndAVisibleUnautomatedWindow(t *testing.T) {
	flags := loginWindowFlags("/some/run-123")
	want := map[string]any{
		"user-data-dir":            "/some/run-123",
		"headless":                 false,
		"enable-automation":        false,
		"disable-blink-features":   "AutomationControlled",
		"disable-infobars":         true,
		"disable-notifications":    true,
		"use-mock-keychain":        true,
		"no-first-run":             true,
		"no-default-browser-check": true,
	}
	if len(flags) != len(want) {
		t.Errorf("flags %v", flags)
	}
	for k, v := range want {
		if flags[k] != v {
			t.Errorf("flag %s = %v, want %v", k, flags[k], v)
		}
	}
}

func TestEveryWindowGetsItsOwnNewFolderOutsideTheToolsDurableStorageAndTheCloudflareProfile(t *testing.T) {
	durable := t.TempDir()
	t.Setenv("TOOL_DB_DIR", durable)
	root := useLoginWindowRoot(t)

	a, err := prepareLoginWindowDir()
	if err != nil {
		t.Fatal(err)
	}
	b, err := prepareLoginWindowDir()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two windows share one folder")
	}
	if info, err := os.Stat(root); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the folder all windows live in must be private (0700): %v %v", info, err)
	}
	for _, dir := range []string{a, b} {
		if filepath.Dir(dir) != root || !strings.HasPrefix(filepath.Base(dir), loginWindowDirPrefix) {
			t.Errorf("folder %s is not a run folder directly in %s", dir, root)
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Errorf("a new window's folder must be empty: %v %v", entries, err)
		}
		if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
			t.Errorf("folder mode %v, want 0700", info.Mode().Perm())
		}
		persistent, err := normalSessionProfileDir()
		if err != nil {
			t.Fatal(err)
		}
		if dir == persistent || strings.HasPrefix(dir, persistent+string(filepath.Separator)) || strings.HasPrefix(dir, durable+string(filepath.Separator)) {
			t.Errorf("folder %s is the persistent Cloudflare profile or under the tool's durable storage", dir)
		}
	}
}

func TestTheDefaultRootIsTheSystemTemporaryAreaNeverTheDurableStorage(t *testing.T) {
	durable := t.TempDir()
	t.Setenv("TOOL_DB_DIR", durable)
	root := loginWindowRoot()
	if filepath.Dir(root) != filepath.Clean(os.TempDir()) {
		t.Errorf("root %s is not directly in %s", root, os.TempDir())
	}
	if strings.HasPrefix(root, durable) {
		t.Errorf("root %s is under TOOL_DB_DIR", root)
	}
}

func TestRemovingAWindowsFolderDeletesItWithEverythingInIt(t *testing.T) {
	useLoginWindowRoot(t)
	dir, err := prepareLoginWindowDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "Default", "Network"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Default", "Network", "Cookies"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeLoginWindowDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("folder still there: %v", err)
	}
	if err := removeLoginWindowDir(dir); err != nil {
		t.Errorf("removing a folder that is gone must not fail: %v", err)
	}
}

func TestTheSweepRemovesEveryFolderAnEarlierRunLeftAndNothingElse(t *testing.T) {
	root := useLoginWindowRoot(t)
	if n := SweepLoginWindowFolders(); n != 0 {
		t.Errorf("no root yet: removed %d", n)
	}
	a, _ := prepareLoginWindowDir()
	b, _ := prepareLoginWindowDir()
	if err := os.WriteFile(filepath.Join(a, "leftover"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(root), "not-ours")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if n := SweepLoginWindowFolders(); n != 2 {
		t.Errorf("removed %d, want 2", n)
	}
	for _, d := range []string{a, b} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("%s survived the sweep", d)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("the sweep touched a folder outside its root: %v", err)
	}
}

func TestACloseThatNeverOpenedLeavesNothing(t *testing.T) {
	root := useLoginWindowRoot(t)
	// no Chrome at all: pointing the path at a missing binary makes the open fail, and a failed open must delete its folder
	t.Setenv(ChromePathEnv, filepath.Join(t.TempDir(), "no-such-chrome"))
	if _, err := OpenLoginWindow("https://example.invalid/login"); err == nil {
		t.Fatal("expected an error without Chrome")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Errorf("a failed open left %d folder(s)", len(entries))
	}
}

// The one test that needs a real Chrome and opens a real, visible window for about two seconds, so it only runs when asked:
//
//	BROWSER_TOOL_LIVE_CHROME_TEST=1 go test ./shared -run TestARealLoginWindow
//
// It guards the mistake the live check of step 1 caught: the window must stay open after OpenLoginWindow returns (a timeout context
// handed to the first chromedp.Run closed it again), and closing it must leave no profile folder.
func TestARealLoginWindowStaysOpenUntilClosedAndLeavesNothing(t *testing.T) {
	if os.Getenv("BROWSER_TOOL_LIVE_CHROME_TEST") == "" {
		t.Skip("opens a real visible Chrome window; set BROWSER_TOOL_LIVE_CHROME_TEST=1 to run it")
	}
	if _, err := ChromePath(); err != nil {
		t.Skipf("no Chrome on this machine: %v", err)
	}
	root := useLoginWindowRoot(t)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><title>Sign in</title><body><input type=password></body></html>"))
	}))
	defer site.Close()

	w, err := OpenLoginWindow(site.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.Done():
		t.Fatal("the window closed by itself right after it opened")
	case <-time.After(2 * time.Second):
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Errorf("an open window must have exactly one profile folder, found %d", len(entries))
	}
	w.Close()
	w.Close() // twice is fine
	select {
	case <-w.Done():
	case <-time.After(5 * time.Second):
		t.Error("Done was not closed after Close")
	}
	entries, _ = os.ReadDir(root)
	if len(entries) != 0 {
		t.Errorf("closing the window left %d profile folder(s)", len(entries))
	}
}
