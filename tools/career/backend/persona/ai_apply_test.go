package persona

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/profile"
)

func pp(name string, skills []string, roles ...ProposedRole) ProposedPersona {
	return ProposedPersona{
		Name: name, Description: "desc of " + name,
		Details: ProposedDetails{Headline: "h " + name, Summary: "s", Location: "Hamburg", DesiredTitles: "Dev", DesiredLocations: "Remote", MinSalary: 70000},
		Skills:  skills, Experience: roles,
	}
}

func count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.CareerDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestApplyDecisions_CreatesEverythingUnderTheRunsProfile(t *testing.T) {
	pid := setupRunsDB(t)
	other, _ := profile.CreateProfile("Alan", "Turing")
	run := runWith(t, pid, "Backend", "Management")

	created, err := ApplyDecisions(run, []Decision{
		accept(0, pp("  Backend  ", []string{" Go ", "go", "SQL", "GO"}, ProposedRole{Company: " Acme ", Title: "Dev", StartDate: "2020", EndDate: "", Description: " built things "})),
		accept(1, pp("Management", []string{"Leadership"})),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 || created[0].Name != "Backend" || created[1].Name != "Management" || created[0].ID == "" {
		t.Fatalf("created = %+v", created)
	}

	// Under the run's profile, linked to the run, nowhere else.
	if n := count(t, `SELECT COUNT(*) FROM personas WHERE ai_run_id = ? AND profile_id = ?`, run, pid); n != 2 {
		t.Fatalf("%d personas under the run's profile", n)
	}
	if n := count(t, `SELECT COUNT(*) FROM personas WHERE profile_id = ?`, other); n != 0 {
		t.Fatalf("%d personas under another profile", n)
	}
	// Details, with the trimmed text and the salary.
	var headline, summary, location, titles, locs string
	var salary int
	if err := db.CareerDB.QueryRow(`SELECT headline, summary, location, desired_titles, desired_locations, min_salary FROM persona_details WHERE persona_id = ?`, created[0].ID).
		Scan(&headline, &summary, &location, &titles, &locs, &salary); err != nil {
		t.Fatal(err)
	}
	// Only the ends are trimmed: the inner spaces of what was typed stay.
	if headline != "h   Backend" {
		t.Fatalf("headline = %q", headline)
	}
	if location != "Hamburg" || titles != "Dev" || locs != "Remote" || salary != 70000 || summary != "s" {
		t.Fatalf("details = %q %q %q %q %d", location, titles, locs, summary, salary)
	}
	// Skills: trimmed and de-duplicated, case-insensitively.
	rows, err := db.CareerDB.Query(`SELECT skill FROM career_skills WHERE persona_id = ? ORDER BY skill`, created[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var skills []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		skills = append(skills, s)
	}
	rows.Close()
	if strings.Join(skills, ",") != "Go,SQL" {
		t.Fatalf("skills = %v, want Go,SQL", skills)
	}
	// Experience, trimmed.
	var company, desc string
	if err := db.CareerDB.QueryRow(`SELECT company, description FROM career_experience WHERE persona_id = ?`, created[0].ID).Scan(&company, &desc); err != nil || company != "Acme" || desc != "built things" {
		t.Fatalf("experience = %q %q err=%v", company, desc, err)
	}
	// The history shows them, in order; the persona list carries aiRunId.
	runs, _ := ListAiRuns(pid)
	if len(runs[0].Personas) != 2 || runs[0].Personas[0].Name != "Backend" || runs[0].Personas[1].Name != "Management" {
		t.Fatalf("run personas = %+v", runs[0].Personas)
	}
	list, _ := ListPersonas(pid)
	raw, _ := json.Marshal(list)
	if strings.Count(string(raw), `"aiRunId":"`+run+`"`) != 2 {
		t.Fatalf("persona list = %s", raw)
	}
}

// A name that an existing persona of the profile already has is allowed: the review only hints.
func TestApplyDecisions_AllowsANameThePersonaAlreadyHas(t *testing.T) {
	pid := setupRunsDB(t)
	if _, err := CreatePersona(pid, "Backend", ""); err != nil {
		t.Fatal(err)
	}
	run := runWith(t, pid, "Backend")
	if _, err := ApplyDecisions(run, []Decision{accept(0, pp("Backend", nil))}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT COUNT(*) FROM personas WHERE name = 'Backend'`); n != 2 {
		t.Fatalf("%d personas named Backend", n)
	}
}

func apply(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	AiRunApplyHandler(w, httptest.NewRequest(http.MethodPost, "/persona-ai-runs/apply", strings.NewReader(body)))
	return w
}

// The step 112 body (only the accepted personas) is gone: it has no decision for the others.
func TestAiRunApplyHandler_TheOldBodyIsRefused(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha", "Beta")
	one := `[{"name":"Alpha","description":"","details":{"headline":"h","summary":"s","location":"","desiredTitles":"","desiredLocations":"","minSalary":0},"skills":["Go"],"experience":[]}]`
	for name, body := range map[string]string{
		"personas only": `{"runId":"` + run + `","personas":` + one + `}`,
		"not json":      `x`,
		"no run":        `{"decisions":[]}`,
	} {
		if w := apply(t, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400 (%s)", name, w.Code, w.Body)
		}
	}
	if n := count(t, `SELECT COUNT(*) FROM personas`); n != 0 {
		t.Fatalf("%d personas created by refused bodies", n)
	}
	if decidedAt, _ := runRow(t, run); decidedAt != nil {
		t.Fatal("a refused body decided the run")
	}
	w := httptest.NewRecorder()
	AiRunApplyHandler(w, httptest.NewRequest(http.MethodGet, "/persona-ai-runs/apply", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
}
