package shared

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// initSettingsDB points the tool at a fresh temp directory and opens it
// through the real InitDB (so the real schema and migrations run).
func initSettingsDB(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("TOOL_DB_DIR", filepath.Join(root, "db"))
	t.Setenv("TOOL_LOGS_DIR", filepath.Join(root, "logs"))
	t.Setenv("TOOL_CACHE_DIR", filepath.Join(root, "cache"))
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { DB.Close() })
	return filepath.Join(root, "db", "browser.db")
}

func TestFreshInstallDefaultsToRodAndDebugOff(t *testing.T) {
	initSettingsDB(t)
	s, err := LoadBrowserSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.FetchEngine != FetchEngineRod || s.DebugEnabled || s.DebugLogHTML || s.DebugLogChallenge {
		t.Errorf("fresh defaults: %+v", s)
	}
	if DefaultFetchEngine != FetchEngineRod {
		t.Errorf("the default engine must be rod")
	}
}

// A database created before this step has no fetch_engine column; the
// migration must add it as 'rod' without touching the Debug values.
func TestExistingInstallIsUpgradedKeepingTheDebugValues(t *testing.T) {
	root := t.TempDir()
	dbDir := filepath.Join(root, "db")
	t.Setenv("TOOL_DB_DIR", dbDir)
	t.Setenv("TOOL_LOGS_DIR", filepath.Join(root, "logs"))
	t.Setenv("TOOL_CACHE_DIR", filepath.Join(root, "cache"))

	if err := InitDB(); err != nil { // creates the directory and the current schema
		t.Fatal(err)
	}
	DB.Close()

	// Rewind browser_settings to the step-68 shape (no fetch_engine) with values set.
	legacy, err := sql.Open("sqlite", filepath.Join(dbDir, "browser.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`DROP TABLE browser_settings;
		CREATE TABLE browser_settings (id INTEGER PRIMARY KEY CHECK (id = 1), debug_enabled INTEGER NOT NULL DEFAULT 0,
			debug_log_html INTEGER NOT NULL DEFAULT 0, debug_log_challenge INTEGER NOT NULL DEFAULT 0, updated_at TEXT);
		INSERT INTO browser_settings (id, debug_enabled, debug_log_html, debug_log_challenge) VALUES (1, 1, 1, 0)`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	if err := InitDB(); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	t.Cleanup(func() { DB.Close() })
	s, err := LoadBrowserSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.FetchEngine != FetchEngineRod {
		t.Errorf("an existing install must get the default engine, got %q", s.FetchEngine)
	}
	if !s.DebugEnabled || !s.DebugLogHTML || s.DebugLogChallenge {
		t.Errorf("the Debug values must survive the migration: %+v", s)
	}

	// Starting again changes nothing (the migration is idempotent).
	DB.Close()
	if err := InitDB(); err != nil {
		t.Fatalf("second start: %v", err)
	}
	if s2, _ := LoadBrowserSettings(); s2 != s {
		t.Errorf("a second start must not change the settings: %+v vs %+v", s2, s)
	}
}

func TestSaveAndLoadRoundTripsTheEngine(t *testing.T) {
	initSettingsDB(t)
	want := BrowserSettings{DebugEnabled: true, DebugLogHTML: true, DebugLogChallenge: true, FetchEngine: FetchEnginePlaywright}
	if err := SaveBrowserSettings(want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadBrowserSettings()
	if err != nil || got != want {
		t.Fatalf("got %+v (%v), want %+v", got, err, want)
	}
}

func TestSaveRejectsAnInvalidEngineAndWritesNothing(t *testing.T) {
	initSettingsDB(t)
	for _, bad := range []string{"", "chromedp", "Rod", " rod", "rod;DROP TABLE browser_settings"} {
		err := SaveBrowserSettings(BrowserSettings{DebugEnabled: true, FetchEngine: bad})
		if err != ErrInvalidFetchEngine {
			t.Errorf("%q: got %v, want ErrInvalidFetchEngine", bad, err)
		}
	}
	if s, _ := LoadBrowserSettings(); s.DebugEnabled || s.FetchEngine != FetchEngineRod {
		t.Errorf("a rejected save must change nothing: %+v", s)
	}
}

func TestAStoredValueOutsideTheAllowListReadsAsTheDefault(t *testing.T) {
	for stored, want := range map[string]string{
		"rod": "rod", "playwright": "playwright", "": "rod", "chromedp": "rod", "ROD": "rod",
	} {
		if got := fetchEngineOrDefault(stored); got != want {
			t.Errorf("fetchEngineOrDefault(%q) = %q, want %q", stored, got, want)
		}
	}
}

func put(body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/browser-settings", strings.NewReader(body))
	rec := httptest.NewRecorder()
	BrowserSettingsHandler(rec, req)
	return rec
}

func TestSettingsHandlerGetPutAndTheKeepSemantics(t *testing.T) {
	initSettingsDB(t)

	rec := httptest.NewRecorder()
	BrowserSettingsHandler(rec, httptest.NewRequest(http.MethodGet, "/browser-settings", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"fetchEngine":"rod"`) {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}

	// Switch to Playwright.
	if rec := put(`{"debugEnabled":false,"debugLogHtml":false,"debugLogChallenge":false,"fetchEngine":"playwright"}`); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"fetchEngine":"playwright"`) {
		t.Fatalf("switch: %d %s", rec.Code, rec.Body.String())
	}

	// An older client (or the Debug page) that omits fetchEngine must not reset it.
	if rec := put(`{"debugEnabled":true,"debugLogHtml":false,"debugLogChallenge":false}`); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"fetchEngine":"playwright"`) || !strings.Contains(rec.Body.String(), `"debugEnabled":true`) {
		t.Fatalf("omitted must keep: %d %s", rec.Code, rec.Body.String())
	}
	if rec := put(`{"debugEnabled":true,"fetchEngine":""}`); !strings.Contains(rec.Body.String(), `"fetchEngine":"playwright"`) {
		t.Fatalf("an empty value must keep the stored one too: %s", rec.Body.String())
	}

	// Back to Rod.
	if rec := put(`{"fetchEngine":"rod"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"fetchEngine":"rod"`) {
		t.Fatalf("back to rod: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsHandlerRejectsABadEngineAndChangesNothing(t *testing.T) {
	initSettingsDB(t)
	if rec := put(`{"debugEnabled":true,"fetchEngine":"selenium"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown engine must be a 400, got %d %s", rec.Code, rec.Body.String())
	}
	if s, _ := LoadBrowserSettings(); s.DebugEnabled || s.FetchEngine != FetchEngineRod {
		t.Errorf("a rejected PUT must change nothing: %+v", s)
	}
	if rec := put(`not json`); rec.Code != http.StatusBadRequest {
		t.Errorf("a malformed body must be a 400, got %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	BrowserSettingsHandler(rec, httptest.NewRequest(http.MethodDelete, "/browser-settings", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE: %d", rec.Code)
	}
}
