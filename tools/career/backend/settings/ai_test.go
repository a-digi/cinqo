package settings

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

func call(method, body string) (int, map[string]string) {
	rec := httptest.NewRecorder()
	AIHandler(rec, httptest.NewRequest(method, "/settings/ai", strings.NewReader(body)))
	var out map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestDefaultsToNotChosen(t *testing.T) {
	setup(t)
	a, err := GetAI()
	if err != nil || a.PlatformID != "" || a.Model != "" {
		t.Errorf("fresh install: %+v, %v", a, err)
	}
	code, out := call(http.MethodGet, "")
	if code != 200 || out["platformId"] != "" || out["model"] != "" {
		t.Errorf("GET: %d %v", code, out)
	}
}

func TestSaveRoundTripAndClear(t *testing.T) {
	setup(t)
	code, out := call(http.MethodPut, `{"platformId":"  openrouter  ","model":" openrouter/free "}`)
	if code != 200 || out["platformId"] != "openrouter" || out["model"] != "openrouter/free" {
		t.Fatalf("PUT: %d %v", code, out)
	}
	if code, out := call(http.MethodGet, ""); code != 200 || out["platformId"] != "openrouter" || out["model"] != "openrouter/free" {
		t.Errorf("GET after save: %d %v", code, out)
	}
	// A platform on its own (the platform has no model list) is fine.
	if code, out := call(http.MethodPut, `{"platformId":"anthropic","model":""}`); code != 200 || out["model"] != "" {
		t.Errorf("platform only: %d %v", code, out)
	}
	// Clearing both returns to "not chosen".
	if code, _ := call(http.MethodPut, `{"platformId":"","model":""}`); code != 200 {
		t.Errorf("clear: %d", code)
	}
	if a, _ := GetAI(); a.PlatformID != "" || a.Model != "" {
		t.Errorf("after clear: %+v", a)
	}
}

func TestRefusals(t *testing.T) {
	setup(t)
	call(http.MethodPut, `{"platformId":"keep","model":"me"}`)
	for name, body := range map[string]string{
		"model without a platform": `{"platformId":"","model":"gpt"}`,
		"blank platform, a model":  `{"platformId":"   ","model":"gpt"}`,
		"too long platform":        `{"platformId":"` + strings.Repeat("p", 201) + `","model":""}`,
		"too long model":           `{"platformId":"p","model":"` + strings.Repeat("m", 201) + `"}`,
		"NUL byte":                 `{"platformId":"a\u0000b","model":""}`,
		"not json":                 `nope`,
		"wrong types":              `{"platformId":5}`,
	} {
		if code, _ := call(http.MethodPut, body); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}
	if a, _ := GetAI(); a.PlatformID != "keep" || a.Model != "me" {
		t.Errorf("a refused save changed the setting: %+v", a)
	}
	for _, m := range []string{http.MethodPost, http.MethodDelete} {
		if code, _ := call(m, ""); code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, code)
		}
	}
}

func TestOnlyOneRowCanExist(t *testing.T) {
	setup(t)
	if _, err := db.JobsDB.Exec(`INSERT INTO ai_settings (id) VALUES (2)`); err == nil {
		t.Error("a second row was accepted")
	}
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM ai_settings`).Scan(&n); err != nil || n != 1 {
		t.Errorf("rows = %d, %v", n, err)
	}
}
