package handlertest

import (
	"career-tool-backend/db"
	"career-tool-backend/jobs/jobboard"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func SetupHandlerDB(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
	if _, err := db.JobsDB.Exec(`INSERT INTO portals (id, name) VALUES ('p1', 'P')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://x')`); err != nil {
		t.Fatal(err)
	}
}

func PostJSON(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func Decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response %q is not JSON: %v", rec.Body.String(), err)
	}
	return out
}

func BoardCall(t *testing.T, h http.HandlerFunc, method, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func BoardFixture(t *testing.T) (p1, p2 string) {
	t.Helper()
	SetupHandlerDB(t)
	var err error
	if p1, err = profile.CreateProfile("Ada", "Lovelace"); err != nil {
		t.Fatal(err)
	}
	if p2, err = profile.CreateProfile("Grace", "Hopper"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"j1", "j2"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title) VALUES (?, ?, ?)`, id, "https://x/"+id, "Job "+id); err != nil {
			t.Fatal(err)
		}
	}
	return p1, p2
}

func WantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status %d (want %d): %s", rec.Code, want, rec.Body.String())
	}
}

func StrPtr(s string) *string { return &s }

// MustSet puts a job on a profile's board in a status.
func MustSet(t *testing.T, profileID, jobID, statusID string) {
	t.Helper()
	if _, err := jobboard.SetEntry(profileID, jobID, statusID); err != nil {
		t.Fatalf("SetEntry: %v", err)
	}
}

const RecalcDesc = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
	"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."

func SeedScoringPersona(t *testing.T, profileID, name string, skills []string) string {
	t.Helper()
	id, err := persona.CreatePersona(profileID, name, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{
		PersonaID: id, Headline: StrPtr("Backend Engineer"), DesiredTitles: StrPtr("Software Engineer, Backend Developer"),
		Location: StrPtr("Hamburg"), DesiredLocations: StrPtr("Hamburg, Remote"),
	}); err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if err := persona.AddCareerSkill(id, s); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func MustExec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := db.CareerDB.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func PiCall(h http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

func PiJSON(t *testing.T, rec *httptest.ResponseRecorder, want int) map[string]any {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status %d (want %d): %s", rec.Code, want, rec.Body.String())
	}
	var out map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return out
}
