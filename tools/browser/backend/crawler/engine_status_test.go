package crawler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
)

// expectedPlaywrightVersion is what the linked library wants.
func expectedPlaywrightVersion(t *testing.T) string {
	t.Helper()
	d, err := playwright.NewDriver()
	if err != nil {
		t.Fatal(err)
	}
	return d.Version
}

// fakeDriver builds a driver directory the way playwright-go lays one out
// (package/cli.js and a node executable), with a stub node whose script
// body is given. It points PLAYWRIGHT_DRIVER_PATH at it and clears the
// other overrides.
func fakeDriver(t *testing.T, nodeScript string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake driver uses a shell script")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "package"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package", "cli.js"), []byte("// fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node"), []byte("#!/bin/sh\n"+nodeScript+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", dir)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	return dir
}

func TestPlaywrightIsAvailableWhenTheDriverIsInstalledAndTheRightVersion(t *testing.T) {
	dir := fakeDriver(t, `echo "Version `+expectedPlaywrightVersion(t)+`"`)
	st := PlaywrightStatus()
	if !st.Available || !strings.Contains(st.Detail, dir) || !strings.Contains(st.Detail, expectedPlaywrightVersion(t)) {
		t.Fatalf("%+v", st)
	}
}

func TestPlaywrightIsNotAvailableWhenTheDriverIsMissing(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", empty)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	st := PlaywrightStatus()
	if st.Available || !strings.Contains(st.Detail, "not installed") || !strings.Contains(st.Detail, empty) {
		t.Fatalf("%+v", st)
	}
	// Unlike playwright.Run(), checking must not create anything.
	if entries, _ := os.ReadDir(empty); len(entries) != 0 {
		t.Errorf("the check must be read-only, but it created %v", entries)
	}
	// The expected (not yet existing) default directory is not created either.
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", filepath.Join(t.TempDir(), "never-created"))
	if st := PlaywrightStatus(); st.Available {
		t.Fatalf("%+v", st)
	}
	if _, err := os.Stat(os.Getenv("PLAYWRIGHT_DRIVER_PATH")); !os.IsNotExist(err) {
		t.Errorf("the driver directory must not be created by a status check")
	}
}

func TestPlaywrightIsNotAvailableWhenItsNodeRuntimeIsMissing(t *testing.T) {
	dir := fakeDriver(t, `echo ok`)
	if err := os.Remove(filepath.Join(dir, "node")); err != nil {
		t.Fatal(err)
	}
	st := PlaywrightStatus()
	if st.Available || !strings.Contains(st.Detail, "Node runtime is missing") {
		t.Fatalf("%+v", st)
	}
}

func TestPlaywrightIsNotAvailableForAWrongVersionAndNeverEchoesProcessOutput(t *testing.T) {
	fakeDriver(t, `echo "Version 0.0.1 SECRET-PROCESS-OUTPUT"`)
	st := PlaywrightStatus()
	if st.Available || !strings.Contains(st.Detail, "not the version") {
		t.Fatalf("%+v", st)
	}
	if strings.Contains(st.Detail, "SECRET-PROCESS-OUTPUT") {
		t.Errorf("the detail must be built from fixed phrases, never raw process output: %q", st.Detail)
	}
}

func TestPlaywrightIsNotAvailableWhenTheDriverFailsToRun(t *testing.T) {
	fakeDriver(t, `echo "boom SECRET-PROCESS-OUTPUT" >&2; exit 3`)
	st := PlaywrightStatus()
	if st.Available || !strings.Contains(st.Detail, "could not be run") || strings.Contains(st.Detail, "SECRET") {
		t.Fatalf("%+v", st)
	}
}

func TestPlaywrightCheckGivesUpOnADriverThatHangs(t *testing.T) {
	fakeDriver(t, `exec sleep 30`)
	orig := engineStatusTimeout
	engineStatusTimeout = 300 * time.Millisecond
	t.Cleanup(func() { engineStatusTimeout = orig })

	start := time.Now()
	st := PlaywrightStatus()
	if st.Available || !strings.Contains(st.Detail, "did not answer in time") {
		t.Fatalf("%+v", st)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("a hanging driver must not hold the request, took %s", elapsed)
	}
}

func TestPlaywrightHonoursTheNodeAndCLIOverrides(t *testing.T) {
	version := expectedPlaywrightVersion(t)
	dir := fakeDriver(t, `exit 9`) // the bundled node must NOT be used when overridden
	other := t.TempDir()
	customNode := filepath.Join(other, "mynode")
	if err := os.WriteFile(customNode, []byte("#!/bin/sh\necho \"Version "+version+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", customNode)
	customCLI := filepath.Join(other, "cli.js")
	if err := os.WriteFile(customCLI, []byte("// fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "package", "cli.js")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLAYWRIGHT_CLI_PATH", customCLI)

	if st := PlaywrightStatus(); !st.Available {
		t.Fatalf("explicit PLAYWRIGHT_NODEJS_PATH and PLAYWRIGHT_CLI_PATH must be honoured: %+v", st)
	}
}

func TestRodStatus(t *testing.T) {
	orig := rodLookPath
	t.Cleanup(func() { rodLookPath = orig })

	t.Setenv("BROWSER_TOOL_CHROME_PATH", "")
	rodLookPath = func() (string, bool) { return "/opt/chrome/chrome", true }
	if st := RodStatus(); !st.Available || st.Detail != "Chrome found at /opt/chrome/chrome" {
		t.Errorf("found: %+v", st)
	}

	rodLookPath = func() (string, bool) { return "", false }
	if st := RodStatus(); st.Available || !strings.Contains(st.Detail, "not found") {
		t.Errorf("missing: %+v", st)
	}

	t.Setenv("BROWSER_TOOL_CHROME_PATH", "/definitely/not/a/browser")
	if st := RodStatus(); st.Available || !strings.Contains(st.Detail, "BROWSER_TOOL_CHROME_PATH") || !strings.Contains(st.Detail, "/definitely/not/a/browser") {
		t.Errorf("bad explicit path: %+v", st)
	}

	good := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(good, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BROWSER_TOOL_CHROME_PATH", good)
	if st := RodStatus(); !st.Available || !strings.Contains(st.Detail, good) {
		t.Errorf("good explicit path: %+v", st)
	}
}

func TestEngineStatusHandler(t *testing.T) {
	version := expectedPlaywrightVersion(t)
	fakeDriver(t, `echo "Version `+version+`"`)
	orig := rodLookPath
	rodLookPath = func() (string, bool) { return "", false }
	t.Cleanup(func() { rodLookPath = orig })
	t.Setenv("BROWSER_TOOL_CHROME_PATH", "")

	rec := httptest.NewRecorder()
	EngineStatusHandler(rec, httptest.NewRequest(http.MethodGet, "/engine-status", nil))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var body map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s: %v", rec.Body.String(), err)
	}
	if body["rod"]["available"] != false || body["playwright"]["available"] != true {
		t.Errorf("unexpected availability: %s", rec.Body.String())
	}
	for _, engine := range []string{"rod", "playwright"} {
		if d, _ := body[engine]["detail"].(string); d == "" {
			t.Errorf("%s must carry a detail sentence: %s", engine, rec.Body.String())
		}
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		EngineStatusHandler(rec, httptest.NewRequest(method, "/engine-status", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", method, rec.Code)
		}
	}
}

func TestManifestDeclaresTheEngineStatusRouteWithItsScope(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []map[string]string `json:"routes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, r := range m.Routes {
		if r["path_suffix"] == "engine-status" {
			if r["method"] != "GET" || r["required_scope"] != "tool:browser:crawl" {
				t.Errorf("route: %+v", r)
			}
			return
		}
	}
	t.Error("manifest.json does not declare the engine-status route")
}

// readManifest returns the Browser tool manifest's declared routes.
func readManifest(t *testing.T) []map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []map[string]string `json:"routes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m.Routes
}
