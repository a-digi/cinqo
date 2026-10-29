package shared

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"database/sql"
)

func clearance(domain string, expires time.Time) Clearance {
	return Clearance{Domain: domain, Name: ClearanceCookieName, Value: "tok-" + domain, Path: "/", Secure: true, HTTPOnly: true, SameSite: "None", ExpiresAt: expires, UserAgent: "UA/1", Source: "human"}
}

func TestStoreAndLoadAClearance(t *testing.T) {
	initSettingsDB(t)
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := StoreClearance(clearance(".Example.com", exp)); err != nil {
		t.Fatal(err)
	}
	got, err := ClearancesFor("example.com", time.Now())
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %+v", err, got)
	}
	c := got[0]
	if c.Domain != "example.com" || !strings.HasPrefix(c.Value, "tok-") || !c.Secure || !c.HTTPOnly || c.SameSite != "None" || !c.ExpiresAt.Equal(exp) || c.UserAgent != "UA/1" || c.Source != "human" {
		t.Errorf("round trip: %+v", c)
	}
}

func TestADomainsClearanceAppliesToItsSubdomainsOnly(t *testing.T) {
	initSettingsDB(t)
	exp := time.Now().Add(time.Hour)
	for _, d := range []string{"example.com", "other.org"} {
		if err := StoreClearance(clearance(d, exp)); err != nil {
			t.Fatal(err)
		}
	}
	for host, want := range map[string]int{
		"example.com":          1,
		"www.example.com":      1,
		"a.b.example.com":      1,
		"notexample.com":       0, // a suffix of the string is not a parent domain
		"example.com.evil.net": 0,
		"other.org":            1,
		"unrelated.net":        0,
		"":                     0,
	} {
		got, err := ClearancesFor(host, time.Now())
		if err != nil || len(got) != want {
			t.Errorf("host %q: %d clearances (%v), want %d", host, len(got), err, want)
		}
	}
}

func TestExpiredClearancesAreNeverUsedAndAreDeleted(t *testing.T) {
	initSettingsDB(t)
	if err := StoreClearance(clearance("example.com", time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Hour)
	got, err := ClearancesFor("example.com", later)
	if err != nil || len(got) != 0 {
		t.Fatalf("an expired clearance must not be returned: %v %+v", err, got)
	}
	var n int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM domain_clearance`).Scan(&n); err != nil || n != 0 {
		t.Errorf("the expired row must be deleted, %d left (%v)", n, err)
	}
}

func TestOnlyAUsableClearanceCookieCanBeStored(t *testing.T) {
	initSettingsDB(t)
	future := time.Now().Add(time.Hour)
	bad := map[string]Clearance{
		"another cookie":   {Domain: "example.com", Name: "session", Value: "x", ExpiresAt: future, UserAgent: "UA"},
		"no value":         {Domain: "example.com", Name: ClearanceCookieName, ExpiresAt: future, UserAgent: "UA"},
		"no domain":        {Name: ClearanceCookieName, Value: "x", ExpiresAt: future, UserAgent: "UA"},
		"no user agent":    {Domain: "example.com", Name: ClearanceCookieName, Value: "x", ExpiresAt: future},
		"already expired":  {Domain: "example.com", Name: ClearanceCookieName, Value: "x", ExpiresAt: time.Now().Add(-time.Minute), UserAgent: "UA"},
		"a session cookie": {Domain: "example.com", Name: ClearanceCookieName, Value: "x", UserAgent: "UA"},
		"an absurd value":  {Domain: "example.com", Name: ClearanceCookieName, Value: strings.Repeat("v", maxClearanceValueLen+1), ExpiresAt: future, UserAgent: "UA"},
	}
	for name, c := range bad {
		if err := StoreClearance(c); err != ErrNotAClearance {
			t.Errorf("%s: err = %v, want ErrNotAClearance", name, err)
		}
	}
	var n int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM domain_clearance`).Scan(&n); err != nil || n != 0 {
		t.Errorf("nothing must have been stored, %d rows (%v)", n, err)
	}
}

func TestStoringAgainReplacesTheClearance(t *testing.T) {
	initSettingsDB(t)
	first := clearance("example.com", time.Now().Add(time.Hour))
	second := first
	second.Value, second.UserAgent, second.ExpiresAt = "newer", "UA/2", time.Now().Add(3*time.Hour)
	for _, c := range []Clearance{first, second} {
		if err := StoreClearance(c); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := ClearancesFor("example.com", time.Now())
	if len(got) != 1 || got[0].Value != "newer" || got[0].UserAgent != "UA/2" {
		t.Fatalf("%+v", got)
	}
}

func TestForgetRemovesTheDomainsClearance(t *testing.T) {
	initSettingsDB(t)
	exp := time.Now().Add(time.Hour)
	_ = StoreClearance(clearance("example.com", exp))
	_ = StoreClearance(clearance("other.org", exp))
	if err := DeleteClearances(".Example.com"); err != nil {
		t.Fatal(err)
	}
	if got, _ := ClearancesFor("example.com", time.Now()); len(got) != 0 {
		t.Errorf("forgotten: %+v", got)
	}
	if got, _ := ClearancesFor("other.org", time.Now()); len(got) != 1 {
		t.Errorf("the other domain must stay: %+v", got)
	}
}

func TestTheClearanceTableExistsOnAnUpgradedInstall(t *testing.T) {
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
	if _, err := legacy.Exec(`DROP TABLE domain_clearance`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	if err := InitDB(); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	t.Cleanup(func() { DB.Close() })
	if err := StoreClearance(clearance("example.com", time.Now().Add(time.Hour))); err != nil {
		t.Errorf("the table must be recreated: %v", err)
	}
}

// --- the human-assist setting -------------------------------------------------

func TestHumanAssistIsOnByDefaultAndSurvivesAnUpgrade(t *testing.T) {
	initSettingsDB(t)
	if s, _ := LoadBrowserSettings(); !s.HumanAssist {
		t.Errorf("a fresh install must have human assist on: %+v", s)
	}
	// An install from before this step: no human_assist column.
	root := t.TempDir()
	dbDir := filepath.Join(root, "db")
	t.Setenv("TOOL_DB_DIR", dbDir)
	t.Setenv("TOOL_LOGS_DIR", filepath.Join(root, "logs"))
	t.Setenv("TOOL_CACHE_DIR", filepath.Join(root, "cache"))
	DB.Close()
	if err := InitDB(); err != nil {
		t.Fatal(err)
	}
	DB.Close()
	legacy, _ := sql.Open("sqlite", filepath.Join(dbDir, "browser.db"))
	if _, err := legacy.Exec(`DROP TABLE browser_settings;
		CREATE TABLE browser_settings (id INTEGER PRIMARY KEY CHECK (id = 1), debug_enabled INTEGER NOT NULL DEFAULT 0,
			debug_log_html INTEGER NOT NULL DEFAULT 0, debug_log_challenge INTEGER NOT NULL DEFAULT 0,
			fetch_engine TEXT NOT NULL DEFAULT 'playwright' CHECK (fetch_engine IN ('rod','playwright')), updated_at TEXT);
		INSERT INTO browser_settings (id, debug_enabled, fetch_engine) VALUES (1, 1, 'playwright')`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	if err := InitDB(); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	t.Cleanup(func() { DB.Close() })
	s, err := LoadBrowserSettings()
	if err != nil || !s.HumanAssist || s.FetchEngine != FetchEnginePlaywright || !s.DebugEnabled {
		t.Fatalf("upgrade must default human assist on and keep the rest: %+v %v", s, err)
	}
}

func TestHumanAssistPutKeepsTheStoredValueWhenTheKeyIsMissing(t *testing.T) {
	initSettingsDB(t)
	put := func(body string) string {
		rec := httptest.NewRecorder()
		BrowserSettingsHandler(rec, httptest.NewRequest(http.MethodPut, "/browser-settings", strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %s: %d %s", body, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	if got := put(`{"humanAssist":false}`); !strings.Contains(got, `"humanAssist":false`) {
		t.Fatalf("explicit off: %s", got)
	}
	// An older client / the Debug page saving the rest of the object.
	if got := put(`{"debugEnabled":true,"debugLogHtml":false,"debugLogChallenge":false,"fetchEngine":"rod"}`); !strings.Contains(got, `"humanAssist":false`) {
		t.Fatalf("a missing key must keep off: %s", got)
	}
	if got := put(`{"humanAssist":true}`); !strings.Contains(got, `"humanAssist":true`) {
		t.Fatalf("explicit on: %s", got)
	}
	if got := put(`{"debugEnabled":false}`); !strings.Contains(got, `"humanAssist":true`) {
		t.Fatalf("a missing key must keep on: %s", got)
	}
}
