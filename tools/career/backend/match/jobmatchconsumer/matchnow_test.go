package jobmatchconsumer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/match"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func ids(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("j%04d", i)
	}
	return out
}

func outcomesOf(status string, list []string) []match.AutoMatchOutcome {
	var out []match.AutoMatchOutcome
	for _, id := range list {
		out = append(out, match.AutoMatchOutcome{JobID: id, Status: status})
	}
	return out
}

// Every unmatched job is counted in exactly one bucket: matched, not judged yet (or gone), failed, or matched meanwhile.
func TestMatchNowCountsEveryJobOnce(t *testing.T) {
	useUnmatched(t, func(int) ([]string, error) { return ids(6), nil })
	watch(t, func(batch []string) ([]match.AutoMatchOutcome, error) {
		return []match.AutoMatchOutcome{
			{JobID: batch[0], Status: match.AutoMatched}, {JobID: batch[1], Status: match.AutoMatched},
			{JobID: batch[2], Status: match.AutoUnjudged}, {JobID: batch[3], Status: match.AutoUnknownJob},
			{JobID: batch[4], Status: match.AutoFailed}, {JobID: batch[5], Status: match.AutoSkippedExisting},
		}, nil
	})
	res, err := MatchNow()
	want := MatchNowResult{Unmatched: 6, Matched: 2, Unjudged: 2, Failed: 1, SkippedExisting: 1}
	if err != nil || res != want {
		t.Errorf("%+v, %v; want %+v", res, err, want)
	}
	if res.Matched+res.Unjudged+res.Failed+res.SkippedExisting != res.Unmatched {
		t.Error("a job was counted twice or not at all")
	}
}

func TestMatchNowWithNothingToDo(t *testing.T) {
	useUnmatched(t, func(int) ([]string, error) { return nil, nil })
	b := watch(t, nil)
	res, err := MatchNow()
	if err != nil || res != (MatchNowResult{}) || len(b.sizes()) != 0 {
		t.Errorf("%+v, %v, batches %v", res, err, b.sizes())
	}
}

// 120 jobs go through in batches of 50, 50 and 20; the cap is 1000 and one more than that is asked for to know there is more.
func TestMatchNowWorksInBatchesAndCapsAtTheLimit(t *testing.T) {
	var asked int
	useUnmatched(t, func(limit int) ([]string, error) { asked = limit; return ids(120), nil })
	b := watch(t, func(batch []string) ([]match.AutoMatchOutcome, error) {
		return outcomesOf(match.AutoMatched, batch), nil
	})
	res, err := MatchNow()
	if err != nil || res.Matched != 120 || res.Unmatched != 120 || res.More {
		t.Fatalf("%+v, %v", res, err)
	}
	if s := b.sizes(); len(s) != 3 || s[0] != 50 || s[1] != 50 || s[2] != 20 {
		t.Errorf("batches %v", s)
	}
	if asked != 1001 || MatchNowLimit != 1000 {
		t.Errorf("asked for %d with limit %d, want 1001 and 1000", asked, MatchNowLimit)
	}

	useUnmatched(t, func(int) ([]string, error) { return ids(1001), nil })
	res, err = MatchNow()
	if err != nil || res.Unmatched != 1000 || res.Matched != 1000 || !res.More {
		t.Errorf("over the cap: %+v, %v", res, err)
	}
	if exactly := func() bool {
		useUnmatched(t, func(int) ([]string, error) { return ids(1000), nil })
		r, _ := MatchNow()
		return r.More
	}(); exactly {
		t.Error("exactly 1000 jobs is not 'more'")
	}
}

// A failure that stops the run is returned with what was counted before it; a lookup failure counts nothing.
func TestMatchNowReportsAFailureWithTheCountsSoFar(t *testing.T) {
	useUnmatched(t, func(int) ([]string, error) { return ids(120), nil })
	calls := 0
	watch(t, func(batch []string) ([]match.AutoMatchOutcome, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("profiles unreadable")
		}
		return outcomesOf(match.AutoMatched, batch), nil
	})
	res, err := MatchNow()
	if err == nil || res.Matched != 50 || res.Unmatched != 120 || calls != 2 {
		t.Errorf("%+v, %v, calls %d", res, err, calls)
	}
	useUnmatched(t, func(int) ([]string, error) { return nil, errors.New("db gone") })
	if res, err := MatchNow(); err == nil || res != (MatchNowResult{}) {
		t.Errorf("lookup failure: %+v, %v", res, err)
	}
}

func TestMatchNowRefusesASecondPressWhileOneRuns(t *testing.T) {
	useUnmatched(t, func(int) ([]string, error) { return ids(2), nil })
	started, release := make(chan struct{}), make(chan struct{})
	watch(t, func(batch []string) ([]match.AutoMatchOutcome, error) {
		close(started)
		<-release
		return outcomesOf(match.AutoMatched, batch), nil
	})
	done := make(chan MatchNowResult)
	go func() { r, _ := MatchNow(); done <- r }()
	<-started
	if _, err := MatchNow(); !errors.Is(err, ErrBusy) {
		t.Errorf("second press: %v", err)
	}
	close(release)
	if r := <-done; r.Matched != 2 {
		t.Errorf("first press: %+v", r)
	}
	// and the lock is released afterwards
	useUnmatched(t, func(int) ([]string, error) { return nil, nil })
	if _, err := MatchNow(); err != nil {
		t.Errorf("after the first press: %v", err)
	}
}

func TestTheMatchNowRoute(t *testing.T) {
	useUnmatched(t, func(int) ([]string, error) { return ids(3), nil })
	watch(t, func(batch []string) ([]match.AutoMatchOutcome, error) {
		return append(outcomesOf(match.AutoMatched, batch[:2]), outcomesOf(match.AutoUnjudged, batch[2:])...), nil
	})
	rec := httptest.NewRecorder()
	MatchNowHandler(rec, httptest.NewRequest(http.MethodPost, "/jobs/match/unmatched", nil))
	body := rec.Body.String()
	for _, want := range []string{`"unmatched":3`, `"matched":2`, `"unjudged":1`, `"failed":0`, `"skippedExisting":0`, `"more":false`} {
		if rec.Code != 200 || !strings.Contains(body, want) {
			t.Errorf("%d %s (missing %s)", rec.Code, body, want)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		MatchNowHandler(rec, httptest.NewRequest(m, "/jobs/match/unmatched", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
	// a failure is a 500, a press while another runs is a 409
	useUnmatched(t, func(int) ([]string, error) { return nil, errors.New("db gone") })
	rec = httptest.NewRecorder()
	MatchNowHandler(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("failure: %d", rec.Code)
	}
	matchNowMu.Lock()
	rec = httptest.NewRecorder()
	MatchNowHandler(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
	matchNowMu.Unlock()
	if rec.Code != http.StatusConflict {
		t.Errorf("busy: %d", rec.Code)
	}
}

// With the real tables and the real matcher: a press with no profile can judge nothing; once a profile can, a press
// matches the jobs, the job with an existing match is not offered, and a second press finds nothing to do.
func TestMatchNowWithTheRealMatcher(t *testing.T) {
	setup(t)
	desc := "Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."
	for _, id := range []string{"a", "b", "c"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company, location, description) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, id, "https://x.example/"+id, desc); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('c', 'p', 5, 'completed', 'ai')`); err != nil {
		t.Fatal(err)
	}
	res, err := MatchNow()
	if err != nil || res != (MatchNowResult{Unmatched: 2, Unjudged: 2}) {
		t.Fatalf("no profile: %+v, %v", res, err)
	}
	pid, _ := profile.CreateProfile("Fit", "Test")
	per, _ := persona.CreatePersona(pid, "Fit persona", "")
	h, titles, loc := "Backend Engineer", "Software Engineer, Backend Developer", "Hamburg"
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: per, Headline: &h, DesiredTitles: &titles, Location: &loc, DesiredLocations: &loc}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Go", "Docker", "Kubernetes", "PostgreSQL"} {
		if err := persona.AddCareerSkill(per, s); err != nil {
			t.Fatal(err)
		}
	}
	res, err = MatchNow()
	if err != nil || res != (MatchNowResult{Unmatched: 2, Matched: 2}) {
		t.Fatalf("with a profile: %+v, %v", res, err)
	}
	var kind string
	_ = db.JobsDB.QueryRow(`SELECT kind FROM job_matches WHERE job_id = 'c'`).Scan(&kind)
	if kind != "ai" {
		t.Errorf("the AI match was replaced: %s", kind)
	}
	if res, err := MatchNow(); err != nil || res != (MatchNowResult{}) {
		t.Errorf("second press: %+v, %v", res, err)
	}
	b, _ := json.Marshal(MatchNowResult{})
	for _, k := range []string{"unmatched", "matched", "unjudged", "failed", "skippedExisting", "more"} {
		if !strings.Contains(string(b), `"`+k+`"`) {
			t.Errorf("json name %s missing in %s", k, b)
		}
	}
}
