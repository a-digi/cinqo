package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"career-tool-backend/db"
)

func jrCall(method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	jobRecruitersHandler(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

func jrFixture(t *testing.T) {
	t.Helper()
	setupHandlerDB(t)
	for _, q := range []string{
		`INSERT INTO companies (id, name) VALUES ('c1', 'Acme'), ('c2', 'Beta Agency')`,
		`INSERT INTO recruiters (id, first_name, last_name, email) VALUES ('r1', 'Ada', 'Lovelace', 'ada@acme.test'), ('r2', 'Bob', 'Marley', '')`,
		`INSERT INTO recruiter_companies (recruiter_id, company_id) VALUES ('r1', 'c1'), ('r2', 'c2')`,
		`INSERT INTO jobs (id, source_url, title, company, company_id, location) VALUES ('j1', 'https://e.com/1', 'Engineer', 'Acme', 'c1', 'Hamburg'), ('j2', 'https://e.com/2', 'Plain job', NULL, NULL, NULL)`,
	} {
		if _, err := db.JobsDB.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func jrJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("%d %q: %v", rec.Code, rec.Body.String(), err)
	}
	return m
}

func TestJobRecruiterRoutesEndToEnd(t *testing.T) {
	jrFixture(t)

	// nothing mapped yet: both lists are empty arrays, not null
	rec := jrCall("GET", "/job-recruiters?jobId=j1", "")
	wantStatus(t, rec, 200)
	if got := jrJSON(t, rec)["recruiters"]; got == nil || len(got.([]any)) != 0 {
		t.Fatalf("recruiters = %v", got)
	}
	rec = jrCall("GET", "/job-recruiters?recruiterId=r1", "")
	wantStatus(t, rec, 200)
	if got := jrJSON(t, rec)["jobs"]; got == nil || len(got.([]any)) != 0 {
		t.Fatalf("jobs = %v", got)
	}

	// map: created, and the answer is the job's refreshed list
	rec = jrCall("POST", "/job-recruiters", `{"jobId":"j1","recruiterId":"r1"}`)
	wantStatus(t, rec, 200)
	body := jrJSON(t, rec)
	list := body["recruiters"].([]any)
	if body["mapped"] != true || len(list) != 1 {
		t.Fatalf("first map: %s", rec.Body.String())
	}
	one := list[0].(map[string]any)
	if one["id"] != "r1" || one["firstName"] != "Ada" || one["lastName"] != "Lovelace" || one["email"] != "ada@acme.test" || one["companyId"] != "c1" || one["companyName"] != "Acme" || one["mappedAt"] == "" {
		t.Errorf("recruiter fields: %v", one)
	}

	// a repeat is not an error and says so
	rec = jrCall("POST", "/job-recruiters", `{"jobId":"j1","recruiterId":"r1"}`)
	wantStatus(t, rec, 200)
	if body := jrJSON(t, rec); body["mapped"] != false || len(body["recruiters"].([]any)) != 1 {
		t.Errorf("repeat: %s", rec.Body.String())
	}

	// a second recruiter on the job and the same recruiter on a second job
	wantStatus(t, jrCall("POST", "/job-recruiters", `{"jobId":"j1","recruiterId":"r2"}`), 200)
	wantStatus(t, jrCall("POST", "/job-recruiters", `{"jobId":"j2","recruiterId":"r1"}`), 200)

	rec = jrCall("GET", "/job-recruiters?jobId=j1", "")
	wantStatus(t, rec, 200)
	if n := len(jrJSON(t, rec)["recruiters"].([]any)); n != 2 {
		t.Errorf("j1 has %d recruiters, want 2", n)
	}
	rec = jrCall("GET", "/job-recruiters?recruiterId=r1", "")
	wantStatus(t, rec, 200)
	jobsOf := jrJSON(t, rec)["jobs"].([]any)
	if len(jobsOf) != 2 {
		t.Fatalf("r1 has %d jobs, want 2: %s", len(jobsOf), rec.Body.String())
	}
	byID := map[string]map[string]any{}
	for _, j := range jobsOf {
		m := j.(map[string]any)
		byID[m["id"].(string)] = m
	}
	if byID["j1"]["title"] != "Engineer" || byID["j1"]["company"] != "Acme" || byID["j1"]["companyId"] != "c1" || byID["j1"]["location"] != "Hamburg" || byID["j2"]["title"] != "Plain job" {
		t.Errorf("job fields: %v", byID)
	}

	// unmap twice: 204 both times, and only that mapping goes
	wantStatus(t, jrCall("DELETE", "/job-recruiters?jobId=j1&recruiterId=r1", ""), 204)
	wantStatus(t, jrCall("DELETE", "/job-recruiters?jobId=j1&recruiterId=r1", ""), 204)
	rec = jrCall("GET", "/job-recruiters?jobId=j1", "")
	if list := jrJSON(t, rec)["recruiters"].([]any); len(list) != 1 || list[0].(map[string]any)["id"] != "r2" {
		t.Errorf("after unmap: %s", rec.Body.String())
	}
	rec = jrCall("GET", "/job-recruiters?recruiterId=r1", "")
	if list := jrJSON(t, rec)["jobs"].([]any); len(list) != 1 || list[0].(map[string]any)["id"] != "j2" {
		t.Errorf("r1's other job must stay: %s", rec.Body.String())
	}
}

func TestJobRecruiterRoutesRefuseBadInput(t *testing.T) {
	jrFixture(t)
	for name, tc := range map[string]struct {
		method, target, body string
		want                 int
	}{
		"GET with neither parameter":     {"GET", "/job-recruiters", "", 400},
		"GET with both parameters":       {"GET", "/job-recruiters?jobId=j1&recruiterId=r1", "", 400},
		"GET unknown job":                {"GET", "/job-recruiters?jobId=nope", "", 404},
		"GET unknown recruiter":          {"GET", "/job-recruiters?recruiterId=nope", "", 404},
		"POST unknown job":               {"POST", "/job-recruiters", `{"jobId":"nope","recruiterId":"r1"}`, 404},
		"POST unknown recruiter":         {"POST", "/job-recruiters", `{"jobId":"j1","recruiterId":"nope"}`, 404},
		"POST without a job id":          {"POST", "/job-recruiters", `{"recruiterId":"r1"}`, 400},
		"POST without a recruiter id":    {"POST", "/job-recruiters", `{"jobId":"j1"}`, 400},
		"POST an empty body":             {"POST", "/job-recruiters", ``, 400},
		"POST text that is not JSON":     {"POST", "/job-recruiters", `jobId=j1`, 400},
		"POST ids of the wrong type":     {"POST", "/job-recruiters", `{"jobId":1,"recruiterId":2}`, 400},
		"DELETE without parameters":      {"DELETE", "/job-recruiters", "", 400},
		"DELETE without the recruiter":   {"DELETE", "/job-recruiters?jobId=j1", "", 400},
		"DELETE without the job":         {"DELETE", "/job-recruiters?recruiterId=r1", "", 400},
		"PUT is not allowed":             {"PUT", "/job-recruiters", `{}`, 405},
		"PATCH is not allowed":           {"PATCH", "/job-recruiters", `{}`, 405},
		"SQL-looking text is just an id": {"GET", "/job-recruiters?jobId=%27%20OR%201%3D1%20--", "", 404},
	} {
		if rec := jrCall(tc.method, tc.target, tc.body); rec.Code != tc.want {
			t.Errorf("%s: %d %q, want %d", name, rec.Code, strings.TrimSpace(rec.Body.String()), tc.want)
		}
	}
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_recruiters`).Scan(&n); err != nil || n != 0 {
		t.Errorf("a refused call wrote %d rows (%v)", n, err)
	}
	// a body over the limit is refused, not read
	big := `{"jobId":"j1","recruiterId":"r1","pad":"` + strings.Repeat("x", maxJobRecruiterBodyBytes) + `"}`
	if rec := jrCall("POST", "/job-recruiters", big); rec.Code != http.StatusBadRequest {
		t.Errorf("a body over the limit: %d", rec.Code)
	}
}

func TestDeletingTheJobOrRecruiterRemovesItFromTheRoute(t *testing.T) {
	jrFixture(t)
	wantStatus(t, jrCall("POST", "/job-recruiters", `{"jobId":"j1","recruiterId":"r1"}`), 200)
	wantStatus(t, jrCall("POST", "/job-recruiters", `{"jobId":"j1","recruiterId":"r2"}`), 200)
	if _, err := db.JobsDB.Exec(`DELETE FROM recruiters WHERE id = 'r1'`); err != nil {
		t.Fatal(err)
	}
	rec := jrCall("GET", "/job-recruiters?jobId=j1", "")
	if list := jrJSON(t, rec)["recruiters"].([]any); len(list) != 1 || list[0].(map[string]any)["id"] != "r2" {
		t.Errorf("after deleting r1: %s", rec.Body.String())
	}
	if _, err := db.JobsDB.Exec(`DELETE FROM jobs WHERE id = 'j1'`); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, jrCall("GET", "/job-recruiters?jobId=j1", ""), 404)
	rec = jrCall("GET", "/job-recruiters?recruiterId=r2", "")
	if list := jrJSON(t, rec)["jobs"].([]any); len(list) != 0 {
		t.Errorf("after deleting j1: %s", rec.Body.String())
	}
}

// The core only passes a call to a route the manifest declares, with the scope the user's token must hold; main.go must serve the path.
func TestTheManifestDeclaresTheJobRecruiterRoutesWithTheJobsScope(t *testing.T) {
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []struct {
			Method        string `json:"method"`
			PathSuffix    string `json:"path_suffix"`
			RequiredScope string `json:"required_scope"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	have := map[string]string{}
	for _, r := range m.Routes {
		if r.PathSuffix == "job-recruiters" {
			have[r.Method] = r.RequiredScope
		}
	}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		if have[method] != "tool:career:jobs" {
			t.Errorf("manifest route %s job-recruiters has scope %q, want tool:career:jobs", method, have[method])
		}
	}
	if len(have) != 3 {
		t.Errorf("the manifest declares %v for job-recruiters, want exactly GET, POST and DELETE", have)
	}
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `http.HandleFunc("/job-recruiters", jobRecruitersHandler)`) {
		t.Error("main.go does not serve /job-recruiters")
	}
}
