package jobcleanup

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

func call(method, body string) (int, Settings, string) {
	rec := httptest.NewRecorder()
	SettingsHandler(rec, httptest.NewRequest(method, "/settings/cleanup", strings.NewReader(body)))
	var out Settings
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

const validBody = `{"enabled":true,"olderEnabled":true,"olderThanDays":30,"lowMatchEnabled":true,"minMatchPercent":55,"includeManual":true,"intervalHours":6}`

// A fresh install changes nothing: every rule and the automatic run are off, with the planned default numbers.
func TestFreshInstallHasEverythingOff(t *testing.T) {
	setup(t)
	want := Settings{OlderThanDays: 90, MinMatchPercent: 60, IntervalHours: 24}
	got, err := GetSettings()
	if err != nil || got != want {
		t.Fatalf("defaults: %+v, %v; want %+v", got, err, want)
	}
	code, out, _ := call(http.MethodGet, "")
	if code != 200 || out != want {
		t.Errorf("GET: %d %+v", code, out)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	setup(t)
	code, out, body := call(http.MethodPut, validBody)
	want := Settings{Enabled: true, OlderEnabled: true, OlderThanDays: 30, LowMatchEnabled: true, MinMatchPercent: 55, IncludeManual: true, IntervalHours: 6}
	if code != 200 || out != want {
		t.Fatalf("PUT: %d %s", code, body)
	}
	if got, _ := GetSettings(); got != want {
		t.Errorf("stored: %+v", got)
	}
	// and back to off keeps the numbers
	code, out, _ = call(http.MethodPut, `{"olderThanDays":30,"minMatchPercent":55,"intervalHours":6}`)
	if code != 200 || out.Enabled || out.OlderEnabled || out.LowMatchEnabled || out.IncludeManual || out.OlderThanDays != 30 {
		t.Errorf("switched off: %d %+v", code, out)
	}
}

// The bounds are inclusive and the same literal numbers as the plan; each side is tried.
func TestBounds(t *testing.T) {
	setup(t)
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"days min", `{"olderThanDays":1,"minMatchPercent":60,"intervalHours":24}`, true},
		{"days max", `{"olderThanDays":3650,"minMatchPercent":60,"intervalHours":24}`, true},
		{"days zero", `{"olderThanDays":0,"minMatchPercent":60,"intervalHours":24}`, false},
		{"days over", `{"olderThanDays":3651,"minMatchPercent":60,"intervalHours":24}`, false},
		{"days negative", `{"olderThanDays":-5,"minMatchPercent":60,"intervalHours":24}`, false},
		{"percent min", `{"olderThanDays":90,"minMatchPercent":1,"intervalHours":24}`, true},
		{"percent max", `{"olderThanDays":90,"minMatchPercent":100,"intervalHours":24}`, true},
		{"percent zero", `{"olderThanDays":90,"minMatchPercent":0,"intervalHours":24}`, false},
		{"percent over", `{"olderThanDays":90,"minMatchPercent":101,"intervalHours":24}`, false},
		{"interval min", `{"olderThanDays":90,"minMatchPercent":60,"intervalHours":1}`, true},
		{"interval max", `{"olderThanDays":90,"minMatchPercent":60,"intervalHours":168}`, true},
		{"interval zero", `{"olderThanDays":90,"minMatchPercent":60,"intervalHours":0}`, false},
		{"interval over", `{"olderThanDays":90,"minMatchPercent":60,"intervalHours":169}`, false},
		{"fields missing", `{}`, false},
		{"not json", `nope`, false},
	}
	for _, c := range cases {
		code, _, body := call(http.MethodPut, c.body)
		if c.ok && code != 200 || !c.ok && code != 400 {
			t.Errorf("%s: status %d (%s)", c.name, code, strings.TrimSpace(body))
		}
	}
}

// A refused PUT leaves the stored row exactly as it was.
func TestARefusedSaveStoresNothing(t *testing.T) {
	setup(t)
	call(http.MethodPut, validBody)
	before, _ := GetSettings()
	if code, _, _ := call(http.MethodPut, `{"enabled":false,"olderThanDays":9999,"minMatchPercent":60,"intervalHours":24}`); code != 400 {
		t.Fatalf("status %d", code)
	}
	if after, _ := GetSettings(); after != before {
		t.Errorf("changed: %+v -> %+v", before, after)
	}
}

// The last-run fields belong to the cleanup: a PUT cannot set or clear them.
func TestAPutCannotWriteTheLastRunFields(t *testing.T) {
	setup(t)
	if _, err := db.JobsDB.Exec(`UPDATE job_cleanup_settings SET last_run_at = '2026-01-02 03:04:05', last_run_deleted = 7, last_run_summary = 'older: 7' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	code, out, body := call(http.MethodPut, `{"enabled":true,"olderThanDays":30,"minMatchPercent":55,"intervalHours":6,"lastRunAt":"x","lastRunDeleted":99,"lastRunSummary":"fake"}`)
	if code != 200 {
		t.Fatalf("PUT: %d %s", code, body)
	}
	if out.LastRunAt != "2026-01-02 03:04:05" || out.LastRunDeleted != 7 || out.LastRunSummary != "older: 7" || !out.Enabled {
		t.Errorf("after PUT: %+v", out)
	}
}

// The database itself refuses a second row and an out-of-range value, whatever code writes it.
func TestTheSchemaGuardsTheRow(t *testing.T) {
	setup(t)
	if _, err := db.JobsDB.Exec(`INSERT INTO job_cleanup_settings (id) VALUES (2)`); err == nil {
		t.Error("a second row was accepted")
	}
	for _, q := range []string{
		`UPDATE job_cleanup_settings SET older_than_days = 0`,
		`UPDATE job_cleanup_settings SET older_than_days = 3651`,
		`UPDATE job_cleanup_settings SET min_match_percent = 101`,
		`UPDATE job_cleanup_settings SET interval_hours = 169`,
		`UPDATE job_cleanup_settings SET enabled = 2`,
	} {
		if _, err := db.JobsDB.Exec(q); err == nil {
			t.Errorf("accepted: %s", q)
		}
	}
}

// Opening the databases again keeps what was saved (the seed is INSERT OR IGNORE).
func TestReopeningKeepsTheSavedRow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateSettings(Settings{OlderEnabled: true, OlderThanDays: 45, MinMatchPercent: 70, IntervalHours: 12}); err != nil {
		t.Fatal(err)
	}
	db.JobsDB.Close()
	db.CareerDB.Close()
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer func() { db.JobsDB.Close(); db.CareerDB.Close() }()
	got, _ := GetSettings()
	if !got.OlderEnabled || got.OlderThanDays != 45 || got.MinMatchPercent != 70 || got.IntervalHours != 12 {
		t.Errorf("after reopen: %+v", got)
	}
}

func TestOtherMethodsAreRefused(t *testing.T) {
	setup(t)
	for _, m := range []string{http.MethodPost, http.MethodDelete, http.MethodPatch} {
		if code, _, _ := call(m, validBody); code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, code)
		}
	}
}

func TestTheJSONNamesAreTheOnesThePageUses(t *testing.T) {
	raw, _ := json.Marshal(Settings{})
	for _, k := range []string{"enabled", "olderEnabled", "olderThanDays", "lowMatchEnabled", "minMatchPercent", "includeManual", "intervalHours", "lastRunAt", "lastRunDeleted", "lastRunSummary"} {
		if !strings.Contains(string(raw), `"`+k+`"`) {
			t.Errorf("missing %s in %s", k, raw)
		}
	}
}
