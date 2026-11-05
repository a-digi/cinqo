package jobshandlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/handlertest"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func s(v string) *string { return &v }

const matchDesc = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
	"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."

// matchFixture makes one job and a profile with a Go persona.
func matchFixture(t *testing.T) (jobID, profileID, personaID string) {
	t.Helper()
	handlertest.SetupHandlerDB(t)
	jobID = "job-1"
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, location, description) VALUES (?, 'https://x.example/1', 'Backend Engineer (m/w/d)', 'Hamburg', ?)`, jobID, matchDesc); err != nil {
		t.Fatal(err)
	}
	var err error
	if profileID, err = profile.CreateProfile("Ada", "Test"); err != nil {
		t.Fatal(err)
	}
	if personaID, err = persona.CreatePersona(profileID, "Backend", ""); err != nil {
		t.Fatal(err)
	}
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: personaID, Headline: s("Backend Engineer"), DesiredTitles: s("Backend Developer"), Location: s("Hamburg")}); err != nil {
		t.Fatal(err)
	}
	for _, sk := range []string{"Go", "Docker", "Kubernetes", "Rust"} {
		if err := persona.AddCareerSkill(personaID, sk); err != nil {
			t.Fatal(err)
		}
	}
	return jobID, profileID, personaID
}

func TestRunMatchScoresStoresAndShowsTheEngineOnTheJob(t *testing.T) {
	jobID, profileID, personaID := matchFixture(t)
	rec := handlertest.BoardCall(t, JobMatchRunHandler, "POST", "/jobs/match/run", `{"jobId":"`+jobID+`","profileId":"`+profileID+`"}`)
	handlertest.WantStatus(t, rec, 200)
	var res struct {
		Score         int      `json:"score"`
		PersonaID     string   `json:"personaId"`
		PersonaName   string   `json:"personaName"`
		MatchedSkills []string `json:"matchedSkills"`
		Mode          string   `json:"mode"`
		EngineVersion string   `json:"engineVersion"`
		Coverage      float64  `json:"coverage"`
		Components    []struct {
			Key       string `json:"key"`
			Weight    int    `json:"weight"`
			Evaluable bool   `json:"evaluable"`
		} `json:"components"`
		Missing []any `json:"missing"`
		Gaps    []any `json:"gaps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Score < 50 || res.Score > 100 || res.PersonaID != personaID || res.PersonaName != "Backend" || res.Mode != "text" || res.EngineVersion != "manual-1" || len(res.Components) != 5 || res.Coverage <= 0 {
		t.Fatalf("result = %s", rec.Body.String())
	}
	if strings.Join(res.MatchedSkills, ",") != "Docker,Go,Kubernetes" || res.Missing == nil || res.Gaps == nil {
		t.Errorf("skills %v missing %v gaps %v", res.MatchedSkills, res.Missing, res.Gaps)
	}

	// the job now shows the manual result
	one := handlertest.BoardCall(t, JobsHandler, "GET", "/jobs?id="+jobID, "")
	var job struct {
		Job struct {
			MatchScore     int             `json:"matchScore"`
			MatchStatus    string          `json:"matchStatus"`
			MatchKind      string          `json:"matchKind"`
			MatchBreakdown json.RawMessage `json:"matchBreakdown"`
		} `json:"job"`
	}
	if err := json.Unmarshal(one.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.Job.MatchScore != res.Score || job.Job.MatchStatus != "completed" || job.Job.MatchKind != "manual" || len(job.Job.MatchBreakdown) == 0 {
		t.Errorf("job = %s", one.Body.String())
	}
}

func TestRunMatchNeverTakesAScoreFromTheRequest(t *testing.T) {
	jobID, profileID, _ := matchFixture(t)
	body := `{"jobId":"` + jobID + `","profileId":"` + profileID + `","score":100,"personaId":"x","matchedSkills":["Hacked"],"kind":"ai","breakdown_json":"{}"}`
	rec := handlertest.BoardCall(t, JobMatchRunHandler, "POST", "/jobs/match/run", body)
	handlertest.WantStatus(t, rec, 200)
	var res struct {
		Score         int      `json:"score"`
		PersonaID     string   `json:"personaId"`
		MatchedSkills []string `json:"matchedSkills"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.Score == 100 || res.PersonaID == "x" || strings.Contains(strings.Join(res.MatchedSkills, ","), "Hacked") {
		t.Errorf("the engine alone decides: %s", rec.Body.String())
	}
	var kind string
	var score int
	_ = db.JobsDB.QueryRow(`SELECT kind, score FROM job_matches WHERE job_id = ?`, jobID).Scan(&kind, &score)
	if kind != "manual" || score != res.Score {
		t.Errorf("stored kind %q score %d", kind, score)
	}
}

func TestRunMatchAnswers(t *testing.T) {
	jobID, profileID, _ := matchFixture(t)
	emptyProfile, _ := profile.CreateProfile("Empty", "Test")
	blankPersonaProfile, _ := profile.CreateProfile("Blank", "Test")
	if _, err := persona.CreatePersona(blankPersonaProfile, "Blank", ""); err != nil {
		t.Fatal(err)
	}
	run := func(body string) *httptest.ResponseRecorder {
		return handlertest.BoardCall(t, JobMatchRunHandler, "POST", "/jobs/match/run", body)
	}
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"no body":          {``, 400},
		"bad json":         {`{`, 400},
		"no job id":        {`{"profileId":"` + profileID + `"}`, 400},
		"no profile id":    {`{"jobId":"` + jobID + `"}`, 400},
		"unknown job":      {`{"jobId":"nope","profileId":"` + profileID + `"}`, 404},
		"unknown profile":  {`{"jobId":"` + jobID + `","profileId":"nope"}`, 404},
		"no personas":      {`{"jobId":"` + jobID + `","profileId":"` + emptyProfile + `"}`, 422},
		"nothing to judge": {`{"jobId":"` + jobID + `","profileId":"` + blankPersonaProfile + `"}`, 422},
	} {
		if rec := run(c.body); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", name, rec.Code, c.want, strings.TrimSpace(rec.Body.String()))
		}
	}
	// the 422 bodies say why, as JSON
	for _, profile := range []string{emptyProfile, blankPersonaProfile} {
		rec := run(`{"jobId":"` + jobID + `","profileId":"` + profile + `"}`)
		var e struct {
			Error   string   `json:"error"`
			Reasons []string `json:"reasons"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error == "" || len(e.Reasons) == 0 || rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("422 body = %q (%v)", rec.Body.String(), err)
		}
	}
	if rec := handlertest.BoardCall(t, JobMatchRunHandler, "GET", "/jobs/match/run", ""); rec.Code != 405 {
		t.Errorf("GET = %d", rec.Code)
	}
	// none of the refused runs stored anything
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_matches`).Scan(&n)
	if n != 0 {
		t.Errorf("%d matches stored by refused runs", n)
	}
}

func TestRequirementsRoutes(t *testing.T) {
	jobID, profileID, _ := matchFixture(t)
	get := func() *httptest.ResponseRecorder {
		return handlertest.BoardCall(t, JobRequirementsHandler, "GET", "/job-requirements?jobId="+jobID, "")
	}
	rec := get()
	handlertest.WantStatus(t, rec, 200)
	if !strings.Contains(rec.Body.String(), `"requirements":[]`) {
		t.Errorf("a new job has an empty list: %s", rec.Body.String())
	}
	rec = handlertest.BoardCall(t, JobRequirementsHandler, "PUT", "/job-requirements", `{"jobId":"`+jobID+`","requirements":[{"term":" Go ","tier":3},{"term":"Terraform","tier":3},{"term":"Docker","tier":1}]}`)
	handlertest.WantStatus(t, rec, 200)
	var saved struct {
		Requirements []struct {
			ID   string `json:"id"`
			Term string `json:"term"`
			Tier int    `json:"tier"`
		} `json:"requirements"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &saved)
	if len(saved.Requirements) != 3 || saved.Requirements[0].Term != "Go" || saved.Requirements[0].ID == "" {
		t.Fatalf("saved = %s", rec.Body.String())
	}
	if again := get(); !strings.Contains(again.Body.String(), `"Terraform"`) {
		t.Errorf("read = %s", again.Body.String())
	}

	// requirements switch the match to requirements mode and make a miss visible
	run := handlertest.BoardCall(t, JobMatchRunHandler, "POST", "/jobs/match/run", `{"jobId":"`+jobID+`","profileId":"`+profileID+`"}`)
	handlertest.WantStatus(t, run, 200)
	var res struct {
		Mode    string `json:"mode"`
		Missing []struct {
			Term string `json:"term"`
			Tier int    `json:"tier"`
		} `json:"missing"`
	}
	_ = json.Unmarshal(run.Body.Bytes(), &res)
	if res.Mode != "requirements" || len(res.Missing) != 1 || res.Missing[0].Term != "Terraform" || res.Missing[0].Tier != 3 {
		t.Errorf("match = %s", run.Body.String())
	}

	// a replaced list; an empty list clears
	handlertest.WantStatus(t, handlertest.BoardCall(t, JobRequirementsHandler, "PUT", "/job-requirements", `{"jobId":"`+jobID+`","requirements":[]}`), 200)
	if cleared := get(); !strings.Contains(cleared.Body.String(), `"requirements":[]`) {
		t.Errorf("cleared = %s", cleared.Body.String())
	}
}

func TestRequirementsRouteErrors(t *testing.T) {
	jobID, _, _ := matchFixture(t)
	put := func(body string) *httptest.ResponseRecorder {
		return handlertest.BoardCall(t, JobRequirementsHandler, "PUT", "/job-requirements", body)
	}
	many := make([]string, 31)
	for i := range many {
		many[i] = `{"term":"skill` + strings.Repeat("x", i) + `","tier":1}`
	}
	for name, c := range map[string]struct {
		rec  *httptest.ResponseRecorder
		want int
	}{
		"get without job":      {handlertest.BoardCall(t, JobRequirementsHandler, "GET", "/job-requirements", ""), 400},
		"get unknown job":      {handlertest.BoardCall(t, JobRequirementsHandler, "GET", "/job-requirements?jobId=nope", ""), 404},
		"put bad json":         {put(`{`), 400},
		"put without job":      {put(`{"requirements":[]}`), 400},
		"put unknown job":      {put(`{"jobId":"nope","requirements":[]}`), 404},
		"put empty term":       {put(`{"jobId":"` + jobID + `","requirements":[{"term":" ","tier":2}]}`), 400},
		"put tier 4":           {put(`{"jobId":"` + jobID + `","requirements":[{"term":"Go","tier":4}]}`), 400},
		"put duplicate":        {put(`{"jobId":"` + jobID + `","requirements":[{"term":"Go","tier":2},{"term":"go","tier":3}]}`), 400},
		"put too long":         {put(`{"jobId":"` + jobID + `","requirements":[{"term":"` + strings.Repeat("a", 121) + `","tier":2}]}`), 400},
		"put too many":         {put(`{"jobId":"` + jobID + `","requirements":[` + strings.Join(many, ",") + `]}`), 400},
		"wrong method":         {handlertest.BoardCall(t, JobRequirementsHandler, "DELETE", "/job-requirements", ""), 405},
		"suggest without job":  {handlertest.BoardCall(t, JobRequirementsSuggestHandler, "GET", "/job-requirements/suggest", ""), 400},
		"suggest unknown job":  {handlertest.BoardCall(t, JobRequirementsSuggestHandler, "GET", "/job-requirements/suggest?jobId=nope", ""), 404},
		"suggest wrong method": {handlertest.BoardCall(t, JobRequirementsSuggestHandler, "POST", "/job-requirements/suggest?jobId="+jobID, ""), 405},
	} {
		if c.rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", name, c.rec.Code, c.want, strings.TrimSpace(c.rec.Body.String()))
		}
	}
	// a refusal says why, as plain text
	if rec := put(`{"jobId":"` + jobID + `","requirements":[{"term":"Go","tier":4}]}`); !strings.Contains(rec.Body.String(), "tier") {
		t.Errorf("the reason is missing: %q", rec.Body.String())
	}
}

func TestSuggestRoute(t *testing.T) {
	jobID, _, _ := matchFixture(t)
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET description = 'Kubernetes Cluster betreiben. Kubernetes und Terraform, Terraform Module.' WHERE id = ?`, jobID); err != nil {
		t.Fatal(err)
	}
	rec := handlertest.BoardCall(t, JobRequirementsSuggestHandler, "GET", "/job-requirements/suggest?jobId="+jobID, "")
	handlertest.WantStatus(t, rec, 200)
	var out struct {
		Suggestions []string `json:"suggestions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	joined := strings.Join(out.Suggestions, ",")
	if !strings.Contains(joined, "Kubernetes") || !strings.Contains(joined, "Terraform") || len(out.Suggestions) > 12 {
		t.Errorf("suggestions = %v", out.Suggestions)
	}
}
