package crawler

import (
	"browser-tool-backend/shared"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRodStatus(t *testing.T) {
	orig := shared.LookChrome
	t.Cleanup(func() { shared.LookChrome = orig })

	t.Setenv("BROWSER_TOOL_CHROME_PATH", "")
	shared.LookChrome = func() (string, bool) { return "/opt/chrome/chrome", true }
	if st := RodStatus(); !st.Available || st.Detail != "Chrome found at /opt/chrome/chrome" {
		t.Errorf("found: %+v", st)
	}

	shared.LookChrome = func() (string, bool) { return "", false }
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
	orig := shared.LookChrome
	shared.LookChrome = func() (string, bool) { return "", false }
	t.Cleanup(func() { shared.LookChrome = orig })
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
	if body["rod"]["available"] != false {
		t.Errorf("unexpected availability: %s", rec.Body.String())
	}
	// Plan 187: Playwright is gone, so the answer names exactly the two engines that exist.
	if len(body) != 2 || body["playwright"] != nil {
		t.Errorf("the answer must hold rod and katana only: %s", rec.Body.String())
	}
	if body["katana"]["available"] != false { // Katana needs the same Chrome as rod
		t.Errorf("katana must be unavailable without Chrome: %s", rec.Body.String())
	}
	for _, engine := range []string{"rod", "katana"} {
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

// Rod and the chromedp sessions must be handed the same Chrome: the
// status page can never say "installed" while the fallback cannot start.
func TestRodAndChromedpResolveTheSameChrome(t *testing.T) {
	t.Setenv("BROWSER_TOOL_CHROME_PATH", "")
	orig := shared.LookChrome
	shared.LookChrome = func() (string, bool) { return "/opt/chrome/chrome", true }
	t.Cleanup(func() { shared.LookChrome = orig })

	rodPath, err := rodChromePath()
	if err != nil {
		t.Fatal(err)
	}
	launch, ok := shared.ChromeLaunchPath()
	if !ok || launch != rodPath {
		t.Errorf("rod uses %q, chromedp launches %q", rodPath, launch)
	}
	if st := shared.CurrentChromeStatus(); !st.Installed || st.Path != rodPath {
		t.Errorf("the Chromium page would report %+v", st)
	}
}

func TestManifestDeclaresTheChromeStatusRouteWithItsScope(t *testing.T) {
	for _, r := range readManifest(t) {
		if r["path_suffix"] == "chrome-status" {
			if r["method"] != "GET" || r["required_scope"] != "tool:browser:crawl" {
				t.Errorf("route: %+v", r)
			}
			return
		}
	}
	t.Error("manifest.json does not declare the chrome-status route")
}
