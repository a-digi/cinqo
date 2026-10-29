package shared

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeChrome(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake browser is a shell script")
	}
	path := filepath.Join(t.TempDir(), "fake-chrome")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func noBrowserOnThisMachine(t *testing.T) {
	t.Helper()
	t.Setenv(ChromePathEnv, "")
	orig := LookChrome
	LookChrome = func() (string, bool) { return "", false }
	t.Cleanup(func() { LookChrome = orig })
}

func TestChromeCandidatesPerOSAreChromeAndChromiumOnly(t *testing.T) {
	env := map[string]string{"ProgramFiles": `C:\PF`, "ProgramFiles(x86)": `C:\PF86`, "LocalAppData": `C:\Users\u\AppData\Local`}
	getenv := func(k string) string { return env[k] }

	mac := chromeCandidates("darwin", getenv, "/Users/u")
	if mac[0] != "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" || mac[1] != "/Applications/Chromium.app/Contents/MacOS/Chromium" {
		t.Errorf("macOS: %v", mac)
	}
	if !strings.Contains(strings.Join(mac, "|"), "/Users/u/Applications/Google Chrome.app") {
		t.Errorf("macOS must also check ~/Applications: %v", mac)
	}

	linux := chromeCandidates("linux", getenv, "/home/u")
	for _, want := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "/snap/bin/chromium"} {
		if !contains(linux, want) {
			t.Errorf("linux is missing %q: %v", want, linux)
		}
	}

	win := chromeCandidates("windows", getenv, `C:\Users\u`)
	for _, want := range []string{"chrome", `C:\PF\Google\Chrome\Application\chrome.exe`, `C:\PF86\Google\Chrome\Application\chrome.exe`, `C:\Users\u\AppData\Local\Chromium\Application\chrome.exe`} {
		if !contains(win, want) {
			t.Errorf("windows is missing %q: %v", want, win)
		}
	}

	// Edge is not Chrome or Chromium: never searched for.
	for goos, list := range map[string][]string{"darwin": mac, "linux": linux, "windows": win} {
		for _, c := range list {
			if strings.Contains(strings.ToLower(c), "edge") {
				t.Errorf("%s: %q — Microsoft Edge is deliberately not in the list", goos, c)
			}
		}
	}
	// Missing environment variables must not produce junk relative paths.
	for _, c := range chromeCandidates("windows", func(string) string { return "" }, "") {
		if strings.HasPrefix(c, "Google") || strings.HasPrefix(c, `\`) {
			t.Errorf("an unset root produced %q", c)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestFirstExecutableFindsBothNamesAndAbsolutePaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell-script executables")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "chromium")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	notExecutable := filepath.Join(dir, "chrome-text")
	if err := os.WriteFile(notExecutable, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if got, ok := firstExecutable([]string{"google-chrome", "chromium"}); !ok || got != bin {
		t.Errorf("by name via PATH: %q %v", got, ok)
	}
	if got, ok := firstExecutable([]string{"/no/such/chrome", bin}); !ok || got != bin {
		t.Errorf("by absolute path: %q %v", got, ok)
	}
	if _, ok := firstExecutable([]string{notExecutable, "", "nothing"}); ok {
		t.Errorf("a non-executable file or empty name must not count")
	}
}

func TestChromePathExplicitOverrideMustExistAndIsNeverSilentlyIgnored(t *testing.T) {
	// The search would find something — an explicit override still wins.
	orig := LookChrome
	LookChrome = func() (string, bool) { return "/found/by/search", true }
	t.Cleanup(func() { LookChrome = orig })

	good := fakeChrome(t, "echo hi")
	t.Setenv(ChromePathEnv, good)
	if got, err := ChromePath(); err != nil || got != good {
		t.Fatalf("explicit: %q %v", got, err)
	}

	for name, value := range map[string]string{"missing": "/definitely/not/a/browser", "directory": t.TempDir()} {
		t.Setenv(ChromePathEnv, value)
		_, err := ChromePath()
		var pe *ChromePathError
		if !errors.As(err, &pe) || pe.Path != value {
			t.Errorf("%s: want *ChromePathError, got %v", name, err)
		}
		if errors.Is(err, ErrChromeNotFound) {
			t.Errorf("%s: an unusable override is not 'not found' — it must not fall back to the search", name)
		}
	}
}

func TestChromePathSearchAndNotFound(t *testing.T) {
	t.Setenv(ChromePathEnv, "")
	orig := LookChrome
	t.Cleanup(func() { LookChrome = orig })

	LookChrome = func() (string, bool) { return "/opt/chrome/chrome", true }
	if got, err := ChromePath(); err != nil || got != "/opt/chrome/chrome" {
		t.Errorf("found: %q %v", got, err)
	}
	LookChrome = func() (string, bool) { return "", true } // never an empty path on success
	if _, err := ChromePath(); !errors.Is(err, ErrChromeNotFound) {
		t.Errorf("an empty result must be 'not found': %v", err)
	}
	LookChrome = func() (string, bool) { return "", false }
	if _, err := ChromePath(); !errors.Is(err, ErrChromeNotFound) {
		t.Errorf("not found: %v", err)
	}
}

func TestChromeLaunchPathMatchesTheResolverAndKeepsAnExplicitOverride(t *testing.T) {
	orig := LookChrome
	t.Cleanup(func() { LookChrome = orig })

	t.Setenv(ChromePathEnv, "")
	LookChrome = func() (string, bool) { return "/opt/chrome/chrome", true }
	resolved, _ := ChromePath()
	if got, ok := ChromeLaunchPath(); !ok || got != resolved {
		t.Errorf("chromedp must launch the very path the resolver (and so Rod) uses: %q %v vs %q", got, ok, resolved)
	}

	// An explicit but unusable override is passed through, so it fails loudly
	// instead of quietly starting some other browser.
	t.Setenv(ChromePathEnv, "/definitely/not/a/browser")
	if got, ok := ChromeLaunchPath(); !ok || got != "/definitely/not/a/browser" {
		t.Errorf("explicit override: %q %v", got, ok)
	}

	// Nothing found: no path, chromedp's own search fails as it always did.
	t.Setenv(ChromePathEnv, "")
	LookChrome = func() (string, bool) { return "", false }
	if got, ok := ChromeLaunchPath(); ok || got != "" {
		t.Errorf("nothing found: %q %v", got, ok)
	}
}

func TestChromeVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	if v := ChromeVersion(fakeChrome(t, `echo "Google Chrome 156.0.8078.73 "`)); v != "Google Chrome 156.0.8078.73" {
		t.Errorf("version = %q", v)
	}
	// Anything that is not a plausible version line is not shown.
	if v := ChromeVersion(fakeChrome(t, `echo "<script>alert(1)</script> SECRET"`)); v != "" {
		t.Errorf("odd output must give no version, got %q", v)
	}
	if v := ChromeVersion(fakeChrome(t, `exit 2`)); v != "" {
		t.Errorf("a failing binary gives no version, got %q", v)
	}
	if v := ChromeVersion("/no/such/binary"); v != "" {
		t.Errorf("a missing binary gives no version, got %q", v)
	}
	if v := ChromeVersion(""); v != "" {
		t.Errorf("empty path: %q", v)
	}

	orig := chromeVersionTimeout
	chromeVersionTimeout = 200 * time.Millisecond
	t.Cleanup(func() { chromeVersionTimeout = orig })
	start := time.Now()
	if v := ChromeVersion(fakeChrome(t, `exec sleep 30`)); v != "" || time.Since(start) > 5*time.Second {
		t.Errorf("a hanging binary must time out quickly: %q after %s", v, time.Since(start))
	}
}

func getChromeStatus(t *testing.T) (ChromeStatus, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	ChromeStatusHandler(rec, httptest.NewRequest(http.MethodGet, "/chrome-status", nil))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var st ChromeStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("%s: %v", rec.Body.String(), err)
	}
	return st, rec.Body.String()
}

func TestChromeStatusHandlerStates(t *testing.T) {
	// Installed.
	good := fakeChrome(t, `echo "Chromium 120.0.6099.109"`)
	t.Setenv(ChromePathEnv, good)
	st, body := getChromeStatus(t)
	if !st.Installed || st.Path != good || st.Version != "Chromium 120.0.6099.109" || st.OS != runtime.GOOS || !strings.Contains(st.Detail, good) {
		t.Errorf("installed: %+v", st)
	}
	for _, key := range []string{`"installed"`, `"path"`, `"version"`, `"os"`, `"detail"`} {
		if !strings.Contains(body, key) {
			t.Errorf("the JSON is missing %s: %s", key, body)
		}
	}

	// Not found.
	noBrowserOnThisMachine(t)
	st, _ = getChromeStatus(t)
	if st.Installed || st.Path != "" || st.Version != "" || st.OS != runtime.GOOS || !strings.Contains(st.Detail, "was not found") {
		t.Errorf("not found: %+v", st)
	}

	// An explicit override that is unusable is reported as such, not as "not found".
	t.Setenv(ChromePathEnv, "/definitely/not/a/browser")
	st, _ = getChromeStatus(t)
	if st.Installed || !strings.Contains(st.Detail, ChromePathEnv) || !strings.Contains(st.Detail, "/definitely/not/a/browser") {
		t.Errorf("bad override: %+v", st)
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		ChromeStatusHandler(rec, httptest.NewRequest(method, "/chrome-status", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", method, rec.Code)
		}
	}
}
