package personahandlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/handler/handlertest"
	"career-tool-backend/match/personamatch"
	"career-tool-backend/match/personamatchconsumer"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func matchesOf(t *testing.T, jobID string) personaMatchesAnswer {
	t.Helper()
	rec := handlertest.BoardCall(t, JobPersonaMatchesHandler, "GET", "/job-persona-matches?jobId="+jobID, "")
	handlertest.WantStatus(t, rec, 200)
	var a personaMatchesAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	return a
}

func shortList(a personaMatchesAnswer) []string {
	out := []string{}
	for _, m := range a.Matches {
		out = append(out, fmt.Sprintf("%s(%s):%d", m.PersonaName, m.ProfileName, m.Score))
	}
	return out
}

func TestTheJobsPersonaMatchesAreListedBestFirstWithNames(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t) // profiles "Ada Lovelace" and "Grace Hopper"
	a, _ := persona.CreatePersona(p1, "Alpha", "")
	b, _ := persona.CreatePersona(p1, "Beta", "")
	x, _ := persona.CreatePersona(p2, "Xi", "")
	if err := personamatch.Replace("j1", []personamatch.Row{
		{PersonaID: a, ProfileID: p1, Score: 40}, {PersonaID: b, ProfileID: p1, Score: 85}, {PersonaID: x, ProfileID: p2, Score: 60},
	}); err != nil {
		t.Fatal(err)
	}
	got := matchesOf(t, "j1")
	if want := []string{"Beta(Ada Lovelace):85", "Xi(Grace Hopper):60", "Alpha(Ada Lovelace):40"}; !reflect.DeepEqual(shortList(got), want) {
		t.Errorf("matches = %v, want %v", shortList(got), want)
	}
	if got.Best != b || got.Matches[0].ProfileID != p1 || got.Matches[1].ProfileID != p2 || got.Matches[0].PersonaID != b {
		t.Errorf("best %q, first %+v", got.Best, got.Matches[0])
	}
}

func TestAJobWithoutScoresAnswersAnEmptyListAndNoBest(t *testing.T) {
	handlertest.BoardFixture(t)
	rec := handlertest.BoardCall(t, JobPersonaMatchesHandler, "GET", "/job-persona-matches?jobId=j1", "")
	handlertest.WantStatus(t, rec, 200)
	if !strings.Contains(rec.Body.String(), `"matches":[]`) || !strings.Contains(rec.Body.String(), `"best":""`) {
		t.Errorf("body = %s, want an empty array (not null) and an empty best", rec.Body.String())
	}
}

func TestPersonaMatchesGetRefusals(t *testing.T) {
	handlertest.BoardFixture(t)
	handlertest.WantStatus(t, handlertest.BoardCall(t, JobPersonaMatchesHandler, "GET", "/job-persona-matches", ""), 400)
	handlertest.WantStatus(t, handlertest.BoardCall(t, JobPersonaMatchesHandler, "GET", "/job-persona-matches?jobId=", ""), 400)
	handlertest.WantStatus(t, handlertest.BoardCall(t, JobPersonaMatchesHandler, "GET", "/job-persona-matches?jobId=nope", ""), 404)
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		handlertest.WantStatus(t, handlertest.BoardCall(t, JobPersonaMatchesHandler, m, "/job-persona-matches?jobId=j1", ""), 405)
	}
}

// A deleted persona's leftover row is not shown, and names are read as they are now.
func TestADeletedPersonasRowIsSkippedAndNamesAreCurrent(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	a, _ := persona.CreatePersona(p1, "Old name", "")
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: a, ProfileID: p1, Score: 50}, {PersonaID: "ghost", ProfileID: p1, Score: 99}})
	if got := shortList(matchesOf(t, "j1")); !reflect.DeepEqual(got, []string{"Old name(Ada Lovelace):50"}) {
		t.Errorf("with a ghost: %v", got)
	}
	if err := persona.UpdatePersona(a, handlertest.StrPtr("New name"), nil); err != nil {
		t.Fatal(err)
	}
	if got := shortList(matchesOf(t, "j1")); !reflect.DeepEqual(got, []string{"New name(Ada Lovelace):50"}) {
		t.Errorf("after a rename: %v", got)
	}
}

func TestAProfileWithoutANameIsLabelledByItsId(t *testing.T) {
	handlertest.BoardFixture(t)
	nameless, err := profile.CreateProfile("", "")
	if err != nil {
		t.Skipf("a nameless profile cannot be created here: %v", err)
	}
	per, _ := persona.CreatePersona(nameless, "Solo", "")
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: per, ProfileID: nameless, Score: 10}})
	got := matchesOf(t, "j1")
	if len(got.Matches) != 1 || got.Matches[0].ProfileName != "Profile "+nameless[:8] {
		t.Errorf("profile name = %q", got.Matches[0].ProfileName)
	}
}

// Equal scores follow the order the profiles and then their personas were created, not the order of ids.
func TestEqualScoresFollowTheCreationOrderOfProfilesThenPersonas(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	a, _ := persona.CreatePersona(p1, "A", "")
	b, _ := persona.CreatePersona(p1, "B", "")
	x, _ := persona.CreatePersona(p2, "X", "")
	set := func(q string, args ...any) {
		if _, err := db.CareerDB.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	set(`UPDATE profiles SET created_at = '2026-01-01 10:00:00' WHERE id = ?`, p2) // Grace's profile is the older one
	set(`UPDATE profiles SET created_at = '2026-02-01 10:00:00' WHERE id = ?`, p1)
	set(`UPDATE personas SET created_at = '2026-03-02 10:00:00' WHERE id = ?`, a) // within Ada's profile, B is older than A
	set(`UPDATE personas SET created_at = '2026-03-01 10:00:00' WHERE id = ?`, b)
	set(`UPDATE personas SET created_at = '2026-04-01 10:00:00' WHERE id = ?`, x)
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: a, ProfileID: p1, Score: 70}, {PersonaID: b, ProfileID: p1, Score: 70}, {PersonaID: x, ProfileID: p2, Score: 70}})
	got := matchesOf(t, "j1")
	if want := []string{"X(Grace Hopper):70", "B(Ada Lovelace):70", "A(Ada Lovelace):70"}; !reflect.DeepEqual(shortList(got), want) || got.Best != x {
		t.Errorf("order = %v (best %s), want %v", shortList(got), got.Best, want)
	}
}

func TestBuildPersonaMatchesOfNothing(t *testing.T) {
	got := buildPersonaMatches(nil, []matchPersona{{ID: "a"}})
	if got.Matches == nil || len(got.Matches) != 0 || got.Best != "" {
		t.Errorf("no rows: %+v", got)
	}
	got = buildPersonaMatches([]personamatch.Row{{PersonaID: "gone", Score: 5}}, []matchPersona{{ID: "a"}})
	if len(got.Matches) != 0 || got.Best != "" {
		t.Errorf("only a deleted persona: %+v", got)
	}
}

func TestAPersonaListFailureIsA500(t *testing.T) {
	handlertest.BoardFixture(t)
	old := listMatchPersonas
	listMatchPersonas = func() ([]matchPersona, error) { return nil, errors.New("career.db gone") }
	t.Cleanup(func() { listMatchPersonas = old })
	handlertest.WantStatus(t, handlertest.BoardCall(t, JobPersonaMatchesHandler, "GET", "/job-persona-matches?jobId=j1", ""), 500)
}

// ---- recalculate, with the real scorer ----

func recalc(t *testing.T, body string) (int, personaMatchesAnswer) {
	t.Helper()
	rec := handlertest.BoardCall(t, JobPersonaMatchesRecalculateHandler, "POST", "/job-persona-matches/recalculate", body)
	var a personaMatchesAnswer
	if rec.Code == 200 {
		if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
			t.Fatalf("%v: %s", err, rec.Body.String())
		}
	}
	return rec.Code, a
}

func TestRecalculateScoresTheJobForEveryPersonaAndReplacesTheRows(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	fit := handlertest.SeedScoringPersona(t, p1, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	other := handlertest.SeedScoringPersona(t, p2, "Other", []string{"Cobol", "Fortran"})
	bare, _ := persona.CreatePersona(p2, "Bare", "") // nothing to compare: no row
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET title = 'Backend Engineer (m/w/d)', location = 'Hamburg', description = ? WHERE id = 'j1'`, handlertest.RecalcDesc); err != nil {
		t.Fatal(err)
	}
	// stale rows: a wrong score for Fit, and a row of the persona that cannot judge
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: fit, ProfileID: p1, Score: 1}, {PersonaID: bare, ProfileID: p2, Score: 77}})

	code, got := recalc(t, `{"jobId":"j1"}`)
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	if len(got.Matches) != 2 || got.Best != fit || got.Matches[0].PersonaID != fit || got.Matches[0].Score < 60 || got.Matches[1].PersonaID != other || got.Matches[1].Score >= 60 {
		t.Errorf("matches = %v (best %s)", shortList(got), got.Best)
	}
	rows, _ := personamatch.ForJob("j1")
	if len(rows) != 2 {
		t.Errorf("stored rows = %d, want 2 (the stale one of the persona that cannot judge is gone)", len(rows))
	}
	// the GET shows the same
	if again := matchesOf(t, "j1"); !reflect.DeepEqual(shortList(again), shortList(got)) {
		t.Errorf("GET %v, recalculate %v", shortList(again), shortList(got))
	}
	// a second recalculation changes nothing
	if code, second := recalc(t, `{"jobId":"j1"}`); code != 200 || !reflect.DeepEqual(shortList(second), shortList(got)) {
		t.Errorf("second run: %d %v", code, shortList(second))
	}
}

func TestRecalculateWithNoPersonasAnswersAnEmptyList(t *testing.T) {
	handlertest.BoardFixture(t)
	code, got := recalc(t, `{"jobId":"j1"}`)
	if code != 200 || len(got.Matches) != 0 || got.Best != "" {
		t.Errorf("%d %+v", code, got)
	}
}

func TestRecalculateRefusals(t *testing.T) {
	handlertest.BoardFixture(t)
	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"not json":      {`nope`, 400},
		"empty body":    {``, 400},
		"no job id":     {`{}`, 400},
		"empty job id":  {`{"jobId":""}`, 400},
		"job id number": {`{"jobId":5}`, 400},
		"unknown job":   {`{"jobId":"nope"}`, 404},
	} {
		if code, _ := recalc(t, tc.body); code != tc.want {
			t.Errorf("%s: status %d, want %d", name, code, tc.want)
		}
	}
	for _, m := range []string{"GET", "PUT", "DELETE"} {
		handlertest.WantStatus(t, handlertest.BoardCall(t, JobPersonaMatchesRecalculateHandler, m, "/job-persona-matches/recalculate", `{"jobId":"j1"}`), 405)
	}
}

// A failing recalculation answers 500 and leaves the earlier rows alone.
func TestAFailedRecalculationIsA500AndKeepsTheEarlierRows(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	a, _ := persona.CreatePersona(p1, "Alpha", "")
	_ = personamatch.Replace("j1", []personamatch.Row{{PersonaID: a, ProfileID: p1, Score: 33}})
	old := recalculatePersonaMatches
	t.Cleanup(func() { recalculatePersonaMatches = old })

	recalculatePersonaMatches = func([]string) ([]personamatchconsumer.Outcome, error) { return nil, errors.New("personas unreadable") }
	if code, _ := recalc(t, `{"jobId":"j1"}`); code != 500 {
		t.Errorf("a batch error: status %d, want 500", code)
	}
	recalculatePersonaMatches = func(ids []string) ([]personamatchconsumer.Outcome, error) {
		return []personamatchconsumer.Outcome{{JobID: ids[0], Status: personamatchconsumer.Failed, Err: errors.New("could not score")}}, nil
	}
	if code, _ := recalc(t, `{"jobId":"j1"}`); code != 500 {
		t.Errorf("a failed job: status %d, want 500", code)
	}
	if got := shortList(matchesOf(t, "j1")); !reflect.DeepEqual(got, []string{"Alpha(Ada Lovelace):33"}) {
		t.Errorf("rows after the failures = %v", got)
	}
}

// Both routes are declared with the jobs scope and routed in main.go.
func TestPersonaMatchRoutesAreDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../../../manifest.json")
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
	seen := map[string]string{}
	for _, r := range m.Routes {
		if strings.HasPrefix(r.PathSuffix, "job-persona-matches") {
			seen[r.Method+" "+r.PathSuffix] = r.RequiredScope
		}
	}
	want := map[string]string{"GET job-persona-matches": "tool:career:jobs", "POST job-persona-matches/recalculate": "tool:career:jobs"}
	if !reflect.DeepEqual(seen, want) {
		t.Errorf("declared = %v, want %v", seen, want)
	}
	mainSrc, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{`"/job-persona-matches", personahandlers.JobPersonaMatchesHandler`, `"/job-persona-matches/recalculate", personahandlers.JobPersonaMatchesRecalculateHandler`} {
		if !strings.Contains(string(mainSrc), line) {
			t.Errorf("main.go lacks %s", line)
		}
	}
}
