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
	want := BrowserSettings{DebugEnabled: true, DebugLogHTML: true, DebugLogChallenge: true, FetchEngine: FetchEngineKatana}
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
	for _, bad := range []string{"", "chromedp", "Rod", " rod", "playwright", "rod;DROP TABLE browser_settings"} {
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
		"rod": "rod", "katana": "katana", "playwright": "rod", "": "rod", "chromedp": "rod", "ROD": "rod",
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

	// Switch to Katana.
	if rec := put(`{"debugEnabled":false,"debugLogHtml":false,"debugLogChallenge":false,"fetchEngine":"katana"}`); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"fetchEngine":"katana"`) {
		t.Fatalf("switch: %d %s", rec.Code, rec.Body.String())
	}

	// An older client (or the Debug page) that omits fetchEngine must not reset it.
	if rec := put(`{"debugEnabled":true,"debugLogHtml":false,"debugLogChallenge":false}`); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"fetchEngine":"katana"`) || !strings.Contains(rec.Body.String(), `"debugEnabled":true`) {
		t.Fatalf("omitted must keep: %d %s", rec.Code, rec.Body.String())
	}
	if rec := put(`{"debugEnabled":true,"fetchEngine":""}`); !strings.Contains(rec.Body.String(), `"fetchEngine":"katana"`) {
		t.Fatalf("an empty value must keep the stored one too: %s", rec.Body.String())
	}

	// Back to Rod.
	if rec := put(`{"fetchEngine":"rod"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"fetchEngine":"rod"`) {
		t.Fatalf("back to rod: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSettingsHandlerRejectsABadEngineAndChangesNothing(t *testing.T) {
	initSettingsDB(t)
	for _, bad := range []string{"selenium", "playwright"} { // plan 187: playwright is an unknown engine now
		rec := put(`{"debugEnabled":true,"fetchEngine":"` + bad + `"}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"rod" or "katana"`) {
			t.Fatalf("%s: an unknown engine must be a 400 naming the two engines, got %d %s", bad, rec.Code, rec.Body.String())
		}
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

// Plan 184: an install whose browser_settings still carries the old
// CHECK (rod/playwright only) is rebuilt so 'katana' can be stored; the
// stored row (engine and every toggle) survives, and a second start changes nothing.
func TestAnOldInstallCanStoreKatanaAfterTheTableRebuild(t *testing.T) {
	root := t.TempDir()
	dbDir := filepath.Join(root, "db")
	t.Setenv("TOOL_DB_DIR", dbDir)
	t.Setenv("TOOL_LOGS_DIR", filepath.Join(root, "logs"))
	t.Setenv("TOOL_CACHE_DIR", filepath.Join(root, "cache"))
	if err := InitDB(); err != nil {
		t.Fatal(err)
	}
	DB.Close()

	legacy, err := sql.Open("sqlite", filepath.Join(dbDir, "browser.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`DROP TABLE browser_settings;
		CREATE TABLE browser_settings (id INTEGER PRIMARY KEY CHECK (id = 1), debug_enabled INTEGER NOT NULL DEFAULT 0,
			debug_log_html INTEGER NOT NULL DEFAULT 0, debug_log_challenge INTEGER NOT NULL DEFAULT 0,
			fetch_engine TEXT NOT NULL DEFAULT 'rod' CHECK (fetch_engine IN ('rod','playwright')),
			human_assist INTEGER NOT NULL DEFAULT 1, updated_at TEXT);
		INSERT INTO browser_settings (id, debug_enabled, debug_log_html, debug_log_challenge, fetch_engine, human_assist) VALUES (1, 1, 0, 1, 'playwright', 0)`); err != nil {
		t.Fatal(err)
	}
	// the old table refuses katana
	if _, err := legacy.Exec(`UPDATE browser_settings SET fetch_engine = 'katana'`); err == nil {
		t.Fatal("the legacy table should refuse 'katana'")
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
	want := BrowserSettings{DebugEnabled: true, DebugLogChallenge: true, FetchEngine: FetchEngineRod, HumanAssist: false} // the stored 'playwright' became 'rod' (plan 187)
	if s != want {
		t.Fatalf("the row changed in the rebuild: %+v, want %+v", s, want)
	}
	want.FetchEngine = FetchEngineKatana
	if err := SaveBrowserSettings(want); err != nil {
		t.Fatalf("save katana: %v", err)
	}
	DB.Close()
	if err := InitDB(); err != nil { // second start: no rebuild, nothing lost
		t.Fatalf("second start: %v", err)
	}
	if got, _ := LoadBrowserSettings(); got != want {
		t.Fatalf("after a second start: %+v, want %+v", got, want)
	}
	var n int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM browser_settings`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows: %d %v", n, err)
	}
}

func TestKatanaIsAValidEngineAndNothingElseNewIs(t *testing.T) {
	if !ValidFetchEngine("katana") {
		t.Error("katana must be valid")
	}
	for _, bad := range []string{"Katana", "katana ", "colly", "http", "playwright"} {
		if ValidFetchEngine(bad) {
			t.Errorf("%q must be invalid", bad)
		}
	}
}

// Plan 187: an install that stored 'playwright' keeps every other value, starts on rod, and its table no
// longer allows the value; an install on katana keeps katana; a second start changes nothing.
func TestTheDropPlaywrightMigrationTurnsTheStoredEngineIntoRodAndKeepsTheRest(t *testing.T) {
	for _, tc := range []struct{ stored, want string }{{"playwright", "rod"}, {"katana", "katana"}, {"rod", "rod"}} {
		t.Run(tc.stored, func(t *testing.T) {
			root := t.TempDir()
			dbDir := filepath.Join(root, "db")
			t.Setenv("TOOL_DB_DIR", dbDir)
			t.Setenv("TOOL_LOGS_DIR", filepath.Join(root, "logs"))
			t.Setenv("TOOL_CACHE_DIR", filepath.Join(root, "cache"))
			if err := InitDB(); err != nil {
				t.Fatal(err)
			}
			DB.Close()
			legacy, err := sql.Open("sqlite", filepath.Join(dbDir, "browser.db"))
			if err != nil {
				t.Fatal(err)
			}
			// the table as plan 184 left it: it allows all three values
			if _, err := legacy.Exec(`DROP TABLE browser_settings;
				CREATE TABLE browser_settings (id INTEGER PRIMARY KEY CHECK (id = 1), debug_enabled INTEGER NOT NULL DEFAULT 0,
					debug_log_html INTEGER NOT NULL DEFAULT 0, debug_log_challenge INTEGER NOT NULL DEFAULT 0,
					fetch_engine TEXT NOT NULL DEFAULT 'rod' CHECK (fetch_engine IN ('rod','playwright','katana')),
					human_assist INTEGER NOT NULL DEFAULT 1, updated_at TEXT);
				INSERT INTO browser_settings (id, debug_enabled, debug_log_html, debug_log_challenge, fetch_engine, human_assist, updated_at)
					VALUES (1, 1, 1, 0, '` + tc.stored + `', 0, '2026-11-01 10:00:00')`); err != nil {
				t.Fatal(err)
			}
			legacy.Close()

			for start := 1; start <= 2; start++ { // the second start must be a no-op
				if err := InitDB(); err != nil {
					t.Fatalf("start %d: %v", start, err)
				}
				s, err := LoadBrowserSettings()
				if err != nil {
					t.Fatal(err)
				}
				want := BrowserSettings{DebugEnabled: true, DebugLogHTML: true, FetchEngine: tc.want}
				if s != want {
					t.Fatalf("start %d: %+v, want %+v", start, s, want)
				}
				var updated, def string
				DB.QueryRow(`SELECT updated_at FROM browser_settings WHERE id = 1`).Scan(&updated)
				DB.QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'browser_settings'`).Scan(&def)
				if updated != "2026-11-01 10:00:00" {
					t.Errorf("start %d: updated_at changed to %q", start, updated)
				}
				if strings.Contains(def, "playwright") {
					t.Errorf("start %d: the table still names playwright: %s", start, def)
				}
				if _, err := DB.Exec(`UPDATE browser_settings SET fetch_engine = 'playwright'`); err == nil {
					t.Errorf("start %d: the table still accepts 'playwright'", start)
				}
				DB.Close()
			}
		})
	}
}

// A fresh install never has the value in its table either.
func TestAFreshInstallTableDoesNotAllowPlaywright(t *testing.T) {
	initSettingsDB(t)
	if _, err := DB.Exec(`UPDATE browser_settings SET fetch_engine = 'playwright'`); err == nil {
		t.Fatal("a fresh table must refuse 'playwright'")
	}
	if _, err := DB.Exec(`UPDATE browser_settings SET fetch_engine = 'katana'`); err != nil {
		t.Fatalf("a fresh table must allow 'katana': %v", err)
	}
}
