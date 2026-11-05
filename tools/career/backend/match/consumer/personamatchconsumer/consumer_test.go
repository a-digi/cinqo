package personamatchconsumer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/match/personamatch"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
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

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func addJob(t *testing.T, id string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title, company) VALUES (?, ?, ?, 'Acme')`, id, "https://x.example/"+id, "Job "+id)
}

func scores(t *testing.T, jobID string) []string {
	t.Helper()
	rows, err := personamatch.ForJob(jobID)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s/%s:%d", r.ProfileID, r.PersonaID, r.Score))
	}
	return out
}

func tableRows(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// fake is a scorer with fixed rows per job, an error per job, and a hook run while scoring a job.
type fake struct {
	has   bool
	rows  map[string][]personamatch.Row
	errs  map[string]error
	hook  func(jobID string)
	asked []string
}

func (f *fake) HasPersonas() bool { return f.has }

func (f *fake) Rows(jobID string) ([]personamatch.Row, error) {
	f.asked = append(f.asked, jobID)
	if f.hook != nil {
		f.hook(jobID)
	}
	if err := f.errs[jobID]; err != nil {
		return nil, err
	}
	rows, ok := f.rows[jobID]
	if !ok {
		return nil, jobs.ErrUnknownJob
	}
	return rows, nil
}

// useFake replaces the batch's scorer and counts how often it was built.
func useFake(t *testing.T, f *fake) *int {
	t.Helper()
	built := 0
	old := newScorer
	newScorer = func() (scorer, error) { built++; return f, nil }
	t.Cleanup(func() { newScorer = old })
	return &built
}

func outcomeOf(outs []Outcome, jobID string) (Outcome, bool) {
	for _, o := range outs {
		if o.JobID == jobID {
			return o, true
		}
	}
	return Outcome{}, false
}

func row(persona, profile string, score int) personamatch.Row {
	return personamatch.Row{PersonaID: persona, ProfileID: profile, Score: score, EngineVersion: "v"}
}

// ---- the batch ----

func TestABatchWritesEachJobsRows(t *testing.T) {
	setup(t)
	addJob(t, "j1")
	addJob(t, "j2")
	useFake(t, &fake{has: true, rows: map[string][]personamatch.Row{
		"j1": {row("pa", "A", 40), row("pb", "B", 90)},
		"j2": {row("pa", "A", 10)},
	}})
	outs, err := ProcessBatch([]string{"j1", "j2"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(scores(t, "j1"), []string{"B/pb:90", "A/pa:40"}) || !reflect.DeepEqual(scores(t, "j2"), []string{"A/pa:10"}) {
		t.Errorf("j1 %v, j2 %v", scores(t, "j1"), scores(t, "j2"))
	}
	if o, _ := outcomeOf(outs, "j1"); o.Status != Scored || o.Personas != 2 {
		t.Errorf("j1 outcome = %+v", o)
	}
}

// A second delivery brings the rows up to date instead of adding more: the old set is replaced.
func TestARedeliveryReplacesTheRows(t *testing.T) {
	setup(t)
	addJob(t, "j1")
	f := &fake{has: true, rows: map[string][]personamatch.Row{"j1": {row("pa", "A", 40), row("pb", "A", 50)}}}
	useFake(t, f)
	if _, err := ProcessBatch([]string{"j1"}); err != nil {
		t.Fatal(err)
	}
	f.rows["j1"] = []personamatch.Row{row("pa", "A", 77)} // pb can no longer be judged
	if _, err := ProcessBatch([]string{"j1"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(scores(t, "j1"), []string{"A/pa:77"}) {
		t.Errorf("rows = %v", scores(t, "j1"))
	}
}

func TestAJobNoPersonaCanJudgeGetsNoRowsAndIsUnjudged(t *testing.T) {
	setup(t)
	addJob(t, "j1")
	useFake(t, &fake{has: true, rows: map[string][]personamatch.Row{"j1": {}}})
	outs, err := ProcessBatch([]string{"j1"})
	if err != nil {
		t.Fatal(err)
	}
	if o, _ := outcomeOf(outs, "j1"); o.Status != Unjudged || o.Personas != 0 {
		t.Errorf("outcome = %+v", o)
	}
	if tableRows(t, "job_persona_matches") != 0 {
		t.Error("rows for an unjudged job")
	}
}

func TestWithoutAnyPersonaNothingIsScoredOrWritten(t *testing.T) {
	setup(t)
	addJob(t, "j1")
	f := &fake{has: false, rows: map[string][]personamatch.Row{"j1": {row("pa", "A", 40)}}}
	built := useFake(t, f)
	outs, err := ProcessBatch([]string{"j1"})
	if err != nil || len(outs) != 0 || len(f.asked) != 0 || tableRows(t, "job_persona_matches") != 0 {
		t.Errorf("outs=%v err=%v asked=%v", outs, err, f.asked)
	}
	if *built != 1 {
		t.Errorf("the scorer is built once per batch, built %d", *built)
	}
}

func TestAnEmptyBatchBuildsNothing(t *testing.T) {
	setup(t)
	f := &fake{has: true}
	built := useFake(t, f)
	if outs, err := ProcessBatch(nil); err != nil || outs != nil || *built != 0 {
		t.Errorf("outs=%v err=%v built=%d", outs, err, *built)
	}
	if outs, err := ProcessBatch([]string{"", ""}); err != nil || outs != nil || *built != 0 {
		t.Errorf("blank ids: outs=%v err=%v built=%d", outs, err, *built)
	}
}

func TestAnIdInTheBatchTwiceIsHandledOnce(t *testing.T) {
	setup(t)
	addJob(t, "j1")
	f := &fake{has: true, rows: map[string][]personamatch.Row{"j1": {row("pa", "A", 40)}}}
	useFake(t, f)
	outs, _ := ProcessBatch([]string{"j1", "", "j1", "j1"})
	if len(outs) != 1 || len(f.asked) != 1 {
		t.Errorf("outcomes=%d asked=%v", len(outs), f.asked)
	}
}

func TestAJobThatNoLongerExistsIsSkippedAndTheRestGoOn(t *testing.T) {
	setup(t)
	addJob(t, "j2")
	useFake(t, &fake{has: true, rows: map[string][]personamatch.Row{"j2": {row("pa", "A", 40)}}}) // "gone" is unknown to the fake
	outs, err := ProcessBatch([]string{"gone", "j2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || outs[0].JobID != "j2" || len(scores(t, "j2")) != 1 {
		t.Errorf("outs=%+v", outs)
	}
}

// A job deleted between its scoring and the write is skipped: no rows, no outcome, no panic.
func TestAJobDeletedWhileBeingScoredLeavesNothing(t *testing.T) {
	setup(t)
	addJob(t, "j1")
	addJob(t, "j2")
	useFake(t, &fake{
		has:  true,
		rows: map[string][]personamatch.Row{"j1": {row("pa", "A", 40)}, "j2": {row("pa", "A", 50)}},
		hook: func(id string) {
			if id == "j1" {
				exec(t, `DELETE FROM jobs WHERE id = 'j1'`)
			}
		},
	})
	outs, err := ProcessBatch([]string{"j1", "j2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := outcomeOf(outs, "j1"); ok {
		t.Error("a deleted job must have no outcome")
	}
	if tableRows(t, "job_persona_matches") != 1 || len(scores(t, "j2")) != 1 {
		t.Error("only j2 may have rows")
	}
}

func TestAScoringFailureForOneJobDoesNotStopTheOthers(t *testing.T) {
	setup(t)
	addJob(t, "bad")
	addJob(t, "good")
	useFake(t, &fake{
		has:  true,
		rows: map[string][]personamatch.Row{"good": {row("pa", "A", 40)}},
		errs: map[string]error{"bad": errors.New("boom")},
	})
	outs, err := ProcessBatch([]string{"bad", "good"})
	if err != nil {
		t.Fatal(err)
	}
	if o, _ := outcomeOf(outs, "bad"); o.Status != Failed || o.Err == nil {
		t.Errorf("bad outcome = %+v", o)
	}
	if o, _ := outcomeOf(outs, "good"); o.Status != Scored {
		t.Errorf("good outcome = %+v", o)
	}
	if len(scores(t, "bad")) != 0 || len(scores(t, "good")) != 1 {
		t.Error("the bad job must have no rows and the good one its row")
	}
}

// A score the table refuses fails that job only and leaves its earlier rows alone.
func TestAStoreFailureIsThatJobsFailureAndKeepsItsEarlierRows(t *testing.T) {
	setup(t)
	addJob(t, "j1")
	addJob(t, "j2")
	_ = personamatch.Replace("j1", []personamatch.Row{row("pa", "A", 33)})
	useFake(t, &fake{has: true, rows: map[string][]personamatch.Row{
		"j1": {row("pa", "A", 200)}, // outside 0 to 100
		"j2": {row("pa", "A", 44)},
	}})
	outs, err := ProcessBatch([]string{"j1", "j2"})
	if err != nil {
		t.Fatal(err)
	}
	if o, _ := outcomeOf(outs, "j1"); o.Status != Failed {
		t.Errorf("j1 outcome = %+v", o)
	}
	if !reflect.DeepEqual(scores(t, "j1"), []string{"A/pa:33"}) || !reflect.DeepEqual(scores(t, "j2"), []string{"A/pa:44"}) {
		t.Errorf("j1 %v, j2 %v", scores(t, "j1"), scores(t, "j2"))
	}
}

func TestAScorerThatCannotBeBuiltFailsTheBatch(t *testing.T) {
	setup(t)
	addJob(t, "j1")
	old := newScorer
	newScorer = func() (scorer, error) { return nil, errors.New("no personas") }
	t.Cleanup(func() { newScorer = old })
	if outs, err := ProcessBatch([]string{"j1"}); err == nil || outs != nil || tableRows(t, "job_persona_matches") != 0 {
		t.Errorf("outs=%v err=%v", outs, err)
	}
}

// ---- real scoring, end to end ----

func str(s string) *string { return &s }

func seedProfile(t *testing.T, name string, skills []string) (profileID, personaID string) {
	t.Helper()
	profileID, err := profile.CreateProfile(name, "Test")
	if err != nil {
		t.Fatal(err)
	}
	personaID = seedPersona(t, profileID, name+" persona", skills)
	return profileID, personaID
}

func seedPersona(t *testing.T, profileID, name string, skills []string) string {
	t.Helper()
	personaID, err := persona.CreatePersona(profileID, name, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{
		PersonaID: personaID, Headline: str("Backend Engineer"), DesiredTitles: str("Software Engineer, Backend Developer"),
		Location: str("Hamburg"), DesiredLocations: str("Hamburg, Remote"),
	}); err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if err := persona.AddCareerSkill(personaID, s); err != nil {
			t.Fatal(err)
		}
	}
	return personaID
}

const jobDesc = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
	"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."

// With the real scorer: every persona of every profile gets a row with its profile's id, the fitting ones score clearly
// above the others, a second run changes nothing, and the job's own match row is not touched.
func TestTheRealScorerRatesEveryPersonaOfEveryProfile(t *testing.T) {
	setup(t)
	fit, fitPersona := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	weakPersona := seedPersona(t, fit, "Weak", []string{"Cobol", "Fortran"})
	other, otherPersona := seedProfile(t, "Other", []string{"Cobol", "Fortran"})
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('j1', 'https://x.example/j1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, jobDesc)

	outs, err := ProcessBatch([]string{"j1"})
	if err != nil {
		t.Fatal(err)
	}
	if o, _ := outcomeOf(outs, "j1"); o.Status != Scored || o.Personas != 3 {
		t.Fatalf("outcome = %+v", o)
	}
	rows, _ := personamatch.ForJob("j1")
	by := map[string]personamatch.Row{}
	for _, r := range rows {
		by[r.PersonaID] = r
	}
	if by[fitPersona].ProfileID != fit || by[weakPersona].ProfileID != fit || by[otherPersona].ProfileID != other {
		t.Errorf("profile ids wrong: %+v", by)
	}
	if by[fitPersona].Score < 60 || by[weakPersona].Score >= 60 || by[otherPersona].Score >= 60 || rows[0].PersonaID != fitPersona {
		t.Errorf("scores %+v: the fitting persona must score clearly higher and come first", rows)
	}
	before := scores(t, "j1")
	if _, err := ProcessBatch([]string{"j1"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, scores(t, "j1")) || tableRows(t, "job_persona_matches") != 3 {
		t.Errorf("a second run changed the rows: %v -> %v", before, scores(t, "j1"))
	}
	if tableRows(t, "job_matches") != 0 {
		t.Error("this consumer must not write job_matches")
	}
}

// ---- the route and the worker ----

func send(h func(http.ResponseWriter, *http.Request), method, body string) int {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, "/events/job-found-persona-matches", strings.NewReader(body)))
	return rec.Code
}

func event(jobID string) string {
	return fmt.Sprintf(`{"id":"e1","topic":"career.job.found","payload":{"job_id":%q,"title":"T","company":"","source":"manual"}}`, jobID)
}

func TestTheRouteQueuesAValidEventAndAnswersAtOnce(t *testing.T) {
	c := New(10)
	if code := send(c.Handler, http.MethodPost, event("job-1")); code != http.StatusAccepted {
		t.Fatalf("status %d", code)
	}
	if len(c.queue) != 1 || <-c.queue != "job-1" {
		t.Error("the job id was not queued")
	}
}

func TestTheRouteRefusesWhatItCannotUse(t *testing.T) {
	c := New(10)
	for name, tc := range map[string]struct {
		method, body string
		want         int
	}{
		"GET":            {http.MethodGet, "", http.StatusMethodNotAllowed},
		"PUT":            {http.MethodPut, event("a"), http.StatusMethodNotAllowed},
		"not json":       {http.MethodPost, "nope", http.StatusBadRequest},
		"empty body":     {http.MethodPost, "", http.StatusBadRequest},
		"no payload":     {http.MethodPost, `{"topic":"career.job.found"}`, http.StatusBadRequest},
		"empty job id":   {http.MethodPost, `{"payload":{"job_id":""}}`, http.StatusBadRequest},
		"missing job id": {http.MethodPost, `{"payload":{"title":"t"}}`, http.StatusBadRequest},
		"job id number":  {http.MethodPost, `{"payload":{"job_id":5}}`, http.StatusBadRequest},
	} {
		if code := send(c.Handler, tc.method, tc.body); code != tc.want {
			t.Errorf("%s: status %d, want %d", name, code, tc.want)
		}
	}
	if len(c.queue) != 0 {
		t.Errorf("%d ids were queued by refused events", len(c.queue))
	}
}

func TestAFullQueueAnswers503AndKeepsWhatItHas(t *testing.T) {
	c := New(2)
	for _, id := range []string{"a", "b"} {
		if code := send(c.Handler, http.MethodPost, event(id)); code != http.StatusAccepted {
			t.Fatalf("%s: %d", id, code)
		}
	}
	if code := send(c.Handler, http.MethodPost, event("c")); code != http.StatusServiceUnavailable {
		t.Errorf("full queue: status %d, want 503", code)
	}
	if len(c.queue) != 2 {
		t.Errorf("queue holds %d, want 2", len(c.queue))
	}
}

type batches struct {
	mu   sync.Mutex
	list [][]string
}

func (b *batches) sizes() []int {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []int
	for _, l := range b.list {
		out = append(out, len(l))
	}
	return out
}

func watch(t *testing.T, fn func(ids []string) ([]Outcome, error)) *batches {
	t.Helper()
	b := &batches{}
	old := run
	run = func(ids []string) ([]Outcome, error) {
		b.mu.Lock()
		b.list = append(b.list, append([]string(nil), ids...))
		b.mu.Unlock()
		if fn != nil {
			return fn(ids)
		}
		return nil, nil
	}
	t.Cleanup(func() { run = old })
	return b
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTheWorkerHandlesQueuedIdsInBatchesOfAtMostFifty(t *testing.T) {
	b := watch(t, nil)
	c := New(200)
	for i := 0; i < 120; i++ {
		c.queue <- fmt.Sprint("job-", i)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	waitFor(t, "all ids", func() bool {
		total := 0
		for _, n := range b.sizes() {
			total += n
		}
		return total == 120
	})
	cancel()
	<-done
	for _, n := range b.sizes() {
		if n > 50 {
			t.Errorf("a batch of %d, the limit is 50", n)
		}
	}
	if len(b.sizes()) < 3 {
		t.Errorf("batches %v: 120 ids need at least three", b.sizes())
	}
}

// The batch is fifty, written out (not the constant, so changing the constant is noticed).
func TestABatchIsExactlyFiftyIds(t *testing.T) {
	b := watch(t, nil)
	c := New(200)
	for i := 0; i < 51; i++ {
		c.queue <- fmt.Sprint("job-", i)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	waitFor(t, "both batches", func() bool { return len(b.sizes()) >= 2 })
	cancel()
	<-done
	if got := b.sizes(); got[0] != 50 || got[1] != 1 {
		t.Errorf("batches = %v, want [50 1]", got)
	}
}

func TestTheWorkerSurvivesAFailingAndAPanickingBatch(t *testing.T) {
	calls := 0
	b := watch(t, func(ids []string) ([]Outcome, error) {
		calls++
		switch calls {
		case 1:
			return nil, errors.New("boom")
		case 2:
			panic("kaboom")
		}
		return []Outcome{{JobID: ids[0], Status: Failed, Err: errors.New("one job failed")}}, nil
	})
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	for i := 0; i < 3; i++ {
		c.queue <- fmt.Sprint("job-", i)
		n := i + 1
		waitFor(t, "batch", func() bool { return len(b.sizes()) >= n })
	}
	cancel()
	<-done
	if len(b.sizes()) != 3 {
		t.Errorf("batches = %d, want 3: the worker must go on after a failure and a panic", len(b.sizes()))
	}
}

func TestTheWorkerStopsWhenItsContextIsCancelled(t *testing.T) {
	watch(t, nil)
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the worker did not stop")
	}
}

// The consumer is declared next to the other job.found listeners and routed in main.go; the worker and sweeper start.
func TestTheConsumerIsDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../../../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	m := string(raw)
	if !strings.Contains(m, `"topic": "career.job.found",
      "path_suffix": "events/job-found-persona-matches"`) {
		t.Error("manifest.json has no career.job.found -> events/job-found-persona-matches listener")
	}
	for _, other := range []string{`"path_suffix": "events/job-found"`, `"path_suffix": "events/job-found-profile-rules"`} {
		if !strings.Contains(m, other) {
			t.Errorf("the other listener %s must stay", other)
		}
	}
	mainSrc, err := os.ReadFile("../../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(mainSrc)
	for _, want := range []string{
		`"/events/job-found-persona-matches", personamatchconsumer.EventHandler`,
		`personamatchconsumer.Start(context.Background())`,
		`personamatchconsumer.StartSweeper(context.Background())`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("main.go lacks %s", want)
		}
	}
}

// With the real scorer and no persona at all: no outcome, nothing written, whatever the job.
func TestTheRealScorerWithoutPersonasDoesNothing(t *testing.T) {
	setup(t)
	if _, err := profile.CreateProfile("No", "Personas"); err != nil {
		t.Fatal(err)
	}
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('j1', 'https://x.example/j1', 'Backend Engineer', 'Acme', 'Hamburg', ?)`, jobDesc)
	outs, err := ProcessBatch([]string{"j1"})
	if err != nil || len(outs) != 0 || tableRows(t, "job_persona_matches") != 0 {
		t.Errorf("outs=%v err=%v", outs, err)
	}
}

// The route also serves "career.job.description_changed" (step 148): the payload is the same {"job_id": "…"}, and the topic is not
// looked at, so the job is queued and its scores are made again from the new text.
func TestADescriptionChangedEventQueuesTheJob(t *testing.T) {
	c := New(10)
	body := `{"topic":"career.job.description_changed","payload":{"job_id":"job-7"}}`
	rec := httptest.NewRecorder()
	c.Handler(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	select {
	case id := <-c.queue:
		if id != "job-7" {
			t.Errorf("queued %q", id)
		}
	default:
		t.Error("nothing queued")
	}
}

// The whole path of step 148's last step with the real scorer: a job scored from a text that does not fit, its description changes
// (what a recrawl does), the "career.job.description_changed" event reaches the running worker through the route, and the job's
// rows are replaced by scores made from the new text.
func TestAChangedDescriptionIsScoredAgainThroughTheEvent(t *testing.T) {
	setup(t)
	_, fitPersona := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('j1', 'https://x.example/j1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', 'Wir suchen Verstärkung für das Büro.')`)
	if _, err := ProcessBatch([]string{"j1"}); err != nil {
		t.Fatal(err)
	}
	score := func() int {
		rows, err := personamatch.ForJob("j1")
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.PersonaID == fitPersona {
				return r.Score
			}
		}
		return -1
	}
	old := score()
	if old < 0 || old >= 60 {
		t.Fatalf("the old text should not fit well: %d", old)
	}

	exec(t, `UPDATE jobs SET description = ? WHERE id = 'j1'`, jobDesc)
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	body := `{"topic":"career.job.description_changed","payload":{"job_id":"j1"}}`
	rec := httptest.NewRecorder()
	c.Handler(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("%d", rec.Code)
	}
	deadline := time.Now().Add(10 * time.Second)
	for score() == old && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if now := score(); now < 60 || now <= old {
		t.Errorf("the score did not follow the new text: %d -> %d", old, now)
	}
}
