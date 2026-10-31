package persona

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/profile"
)

func readyRun(t *testing.T, profileID string) string {
	t.Helper()
	id := mustRun(t, profileID)
	if err := FinishAiRun(id, "ready", []byte(personasJSON(okPersona)), ""); err != nil {
		t.Fatal(err)
	}
	return id
}

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

func TestApplyAiRun_CreatesEverythingUnderTheRunsProfile(t *testing.T) {
	pid := setupRunsDB(t)
	other, _ := profile.CreateProfile("Alan", "Turing")
	run := readyRun(t, pid)

	created, err := ApplyAiRun(run, []ProposedPersona{
		pp("  Backend  ", []string{" Go ", "go", "SQL", "GO"}, ProposedRole{Company: " Acme ", Title: "Dev", StartDate: "2020", EndDate: "", Description: " built things "}),
		pp("Management", []string{"Leadership"}),
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

// All or none: a failure while writing the second persona leaves nothing of the first.
func TestApplyAiRun_IsAllOrNothing(t *testing.T) {
	pid := setupRunsDB(t)
	run := readyRun(t, pid)
	exec(t, `CREATE TRIGGER boom BEFORE INSERT ON career_experience WHEN NEW.company = 'BOOM' BEGIN SELECT RAISE(ABORT, 'boom'); END`)

	_, err := ApplyAiRun(run, []ProposedPersona{
		pp("First", []string{"Go"}, ProposedRole{Company: "Fine", Title: "T"}),
		pp("Second", []string{"SQL"}, ProposedRole{Company: "BOOM", Title: "T"}),
	})
	if err == nil {
		t.Fatal("the failing insert was not reported")
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM personas`, `SELECT COUNT(*) FROM persona_details`,
		`SELECT COUNT(*) FROM career_skills`, `SELECT COUNT(*) FROM career_experience`,
	} {
		if n := count(t, q); n != 0 {
			t.Fatalf("%s = %d after a failed apply, want 0", q, n)
		}
	}
	// The run is still applicable.
	exec(t, `DROP TRIGGER boom`)
	if _, err := ApplyAiRun(run, []ProposedPersona{pp("First", nil)}); err != nil {
		t.Fatalf("a retry after the failure: %v", err)
	}
}

func TestApplyAiRun_Refusals(t *testing.T) {
	pid := setupRunsDB(t)
	run := readyRun(t, pid)
	good := []ProposedPersona{pp("Backend", []string{"Go"})}

	if _, err := ApplyAiRun("nope", good); !errors.Is(err, ErrUnknownAiRun) {
		t.Errorf("unknown run: %v", err)
	}
	// Not ready: still generating, and failed.
	gen := mustRun(t, pid)
	if _, err := ApplyAiRun(gen, good); !errors.Is(err, ErrAiRunNotReady) {
		t.Errorf("generating run: %v", err)
	}
	failed := mustRun(t, pid)
	_ = FinishAiRun(failed, "failed", nil, "boom")
	if _, err := ApplyAiRun(failed, good); !errors.Is(err, ErrAiRunNotReady) {
		t.Errorf("failed run: %v", err)
	}
	// Invalid requests: nothing accepted, too many, a duplicate name, markup, a missing name.
	for name, ps := range map[string][]ProposedPersona{
		"none":        nil,
		"too many":    {pp("A", nil), pp("B", nil), pp("C", nil), pp("D", nil)},
		"duplicate":   {pp("Same", nil), pp("same ", nil)},
		"markup":      {pp("<b>x</b>", nil)},
		"blank name":  {pp("   ", nil)},
		"empty skill": {pp("S", []string{" "})},
		"bad role":    {pp("R", nil, ProposedRole{Company: "", Title: "T"})},
	} {
		if _, err := ApplyAiRun(run, ps); !errors.Is(err, ErrInvalidProposal) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if n := count(t, `SELECT COUNT(*) FROM personas`); n != 0 {
		t.Fatalf("%d personas after only refusals", n)
	}

	// Applied once, never twice; deleting what it created makes it applicable again.
	if _, err := ApplyAiRun(run, good); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyAiRun(run, good); !errors.Is(err, ErrAiRunApplied) {
		t.Errorf("second apply: %v", err)
	}
	if n := count(t, `SELECT COUNT(*) FROM personas WHERE ai_run_id = ?`, run); n != 1 {
		t.Fatalf("%d personas after a refused second apply", n)
	}
	exec(t, `DELETE FROM personas WHERE ai_run_id = ?`, run)
	if _, err := ApplyAiRun(run, good); err != nil {
		t.Errorf("apply after the personas were deleted: %v", err)
	}
}

// A name that an existing persona of the profile already has is allowed: the review only hints.
func TestApplyAiRun_AllowsANameThePersonaAlreadyHas(t *testing.T) {
	pid := setupRunsDB(t)
	if _, err := CreatePersona(pid, "Backend", ""); err != nil {
		t.Fatal(err)
	}
	run := readyRun(t, pid)
	if _, err := ApplyAiRun(run, []ProposedPersona{pp("Backend", nil)}); err != nil {
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

func TestAiRunApplyHandler(t *testing.T) {
	pid := setupRunsDB(t)
	run := readyRun(t, pid)
	one := `[{"name":"Backend","description":"","details":{"headline":"h","summary":"s","location":"","desiredTitles":"","desiredLocations":"","minSalary":0},"skills":["Go"],"experience":[]}]`

	for name, body := range map[string]string{"not json": `x`, "no run": `{"personas":` + one + `}`, "unknown run": `{"runId":"nope","personas":` + one + `}`, "nothing": `{"runId":"` + run + `","personas":[]}`, "markup": `{"runId":"` + run + `","personas":[{"name":"<i>x</i>"}]}`} {
		if w := apply(t, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400 (%s)", name, w.Code, w.Body)
		}
	}
	w := apply(t, `{"runId":"`+run+`","personas":`+one+`}`)
	var got struct {
		Personas []aiRunPersona `json:"personas"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil || len(got.Personas) != 1 || got.Personas[0].Name != "Backend" {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	if w := apply(t, `{"runId":"`+run+`","personas":`+one+`}`); w.Code != http.StatusConflict {
		t.Errorf("second apply: %d, want 409", w.Code)
	}
	gen := mustRun(t, pid)
	if w := apply(t, `{"runId":"`+gen+`","personas":`+one+`}`); w.Code != http.StatusConflict {
		t.Errorf("generating run: %d, want 409", w.Code)
	}
	w = httptest.NewRecorder()
	AiRunApplyHandler(w, httptest.NewRequest(http.MethodGet, "/persona-ai-runs/apply", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
}
