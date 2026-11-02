package profilejobmatchconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobboard"
	"career-tool-backend/jobs"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
	"career-tool-backend/profilerules"
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

func newProfile(t *testing.T, name string) string {
	t.Helper()
	id, err := profile.CreateProfile(name, "Test")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func customStatus(t *testing.T, profileID, name string) string {
	t.Helper()
	s, err := jobboard.CreateStatus(profileID, name)
	if err != nil {
		t.Fatal(err)
	}
	return s.ID
}

func rule(t *testing.T, profileID string, percent int, statusID string) {
	t.Helper()
	if _, err := profilerules.Create(profileID, percent, statusID); err != nil {
		t.Fatal(err)
	}
}

// onBoard is the job's status id on a profile's board, "" when it is not on it.
func onBoard(t *testing.T, profileID, jobID string) string {
	t.Helper()
	var s string
	err := db.JobsDB.QueryRow(`SELECT status_id FROM job_board_entries WHERE profile_id = ? AND job_id = ?`, profileID, jobID).Scan(&s)
	if err != nil {
		return ""
	}
	return s
}

func entries(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_board_entries`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// fake is a scorer with fixed scores: by[jobID][profileID], an error per job, and a hook run while scoring a job.
type fake struct {
	by    map[string]map[string]int
	errs  map[string]error
	hook  func(jobID string)
	asked []string
}

func (f *fake) ScoreByProfile(jobID string) (map[string]int, error) {
	f.asked = append(f.asked, jobID)
	if f.hook != nil {
		f.hook(jobID)
	}
	if err := f.errs[jobID]; err != nil {
		return nil, err
	}
	m, ok := f.by[jobID]
	if !ok {
		return nil, jobs.ErrUnknownJob
	}
	return m, nil
}

// useFake replaces the batch's scorer and records which profiles it was asked to score and how often it was built.
func useFake(t *testing.T, f *fake) (builtFor *[]map[string]bool) {
	t.Helper()
	var built []map[string]bool
	old := newScorer
	newScorer = func(only map[string]bool) (scorer, error) {
		built = append(built, only)
		return f, nil
	}
	t.Cleanup(func() { newScorer = old })
	return &built
}

func outcomeFor(outs []Outcome, jobID, profileID string) (Outcome, bool) {
	for _, o := range outs {
		if o.JobID == jobID && o.ProfileID == profileID {
			return o, true
		}
	}
	return Outcome{}, false
}

// ---- the rules decide ----

// With the highest percentage first, a job gets the status of the highest percentage it reaches.
func TestWithTheHighestPercentageFirstTheHighestReachedDecides(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	check, top := customStatus(t, p, "To check"), customStatus(t, p, "Top pick")
	rule(t, p, 85, top) // the order they are made in is the order they are tried in
	rule(t, p, 60, check)
	for _, id := range []string{"j50", "j59", "j60", "j70", "j84", "j85", "j90"} {
		addJob(t, id)
	}
	f := &fake{by: map[string]map[string]int{
		"j50": {p: 50}, "j59": {p: 59}, "j60": {p: 60}, "j70": {p: 70}, "j84": {p: 84}, "j85": {p: 85}, "j90": {p: 90},
	}}
	useFake(t, f)

	outs, err := ProcessBatch([]string{"j50", "j59", "j60", "j70", "j84", "j85", "j90"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"j50": "", "j59": "", "j60": check, "j70": check, "j84": check, "j85": top, "j90": top}
	for job, status := range want {
		if got := onBoard(t, p, job); got != status {
			t.Errorf("%s: on board with %q, want %q", job, got, status)
		}
	}
	if o, _ := outcomeFor(outs, "j50", p); o.Status != Below || o.Score != 50 {
		t.Errorf("j50 outcome = %+v", o)
	}
	if o, _ := outcomeFor(outs, "j90", p); o.Status != Added || o.StatusID != top || o.Score != 90 {
		t.Errorf("j90 outcome = %+v", o)
	}
}

// The position decides, not the percentage: with 60% first, a job that fits both goes to the 60% rule.
func TestTheFirstFittingRuleWinsEvenWhenALaterOneHasAHigherPercentage(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	check, top := customStatus(t, p, "To check"), customStatus(t, p, "Top pick")
	rule(t, p, 60, check) // first
	rule(t, p, 85, top)   // second: never wins, every job that reaches 85% reaches 60% too
	for _, id := range []string{"j50", "j70", "j90"} {
		addJob(t, id)
	}
	useFake(t, &fake{by: map[string]map[string]int{"j50": {p: 50}, "j70": {p: 70}, "j90": {p: 90}}})
	if _, err := ProcessBatch([]string{"j50", "j70", "j90"}); err != nil {
		t.Fatal(err)
	}
	for job, want := range map[string]string{"j50": "", "j70": check, "j90": check} {
		if got := onBoard(t, p, job); got != want {
			t.Errorf("%s: on board with %q, want %q (the first fitting rule decides)", job, got, want)
		}
	}
}

// A rule that does not fit is skipped and the next one that fits decides: 90% first, 50% second, a 70% job goes to 50%.
func TestARuleThatDoesNotFitIsSkipped(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	top, low := customStatus(t, p, "Top"), customStatus(t, p, "Low")
	rule(t, p, 90, top)
	rule(t, p, 50, low)
	for _, id := range []string{"j40", "j70", "j95"} {
		addJob(t, id)
	}
	useFake(t, &fake{by: map[string]map[string]int{"j40": {p: 40}, "j70": {p: 70}, "j95": {p: 95}}})
	if _, err := ProcessBatch([]string{"j40", "j70", "j95"}); err != nil {
		t.Fatal(err)
	}
	for job, want := range map[string]string{"j40": "", "j70": low, "j95": top} {
		if got := onBoard(t, p, job); got != want {
			t.Errorf("%s: on board with %q, want %q", job, got, want)
		}
	}
}

// Reordering the rules changes what the next job gets, with the same rules and the same score.
func TestReorderingTheRulesChangesTheStatusTheNextJobGets(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	check, top := customStatus(t, p, "To check"), customStatus(t, p, "Top pick")
	rule(t, p, 60, check)
	rule(t, p, 85, top)
	addJob(t, "j1")
	addJob(t, "j2")
	useFake(t, &fake{by: map[string]map[string]int{"j1": {p: 90}, "j2": {p: 90}}})

	if _, err := ProcessBatch([]string{"j1"}); err != nil {
		t.Fatal(err)
	}
	if got := onBoard(t, p, "j1"); got != check {
		t.Fatalf("before the reorder: %q, want %q", got, check)
	}
	rules, _ := profilerules.List(p)
	if _, err := profilerules.Reorder(p, []string{rules[1].ID, rules[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := ProcessBatch([]string{"j2"}); err != nil {
		t.Fatal(err)
	}
	if got := onBoard(t, p, "j2"); got != top {
		t.Errorf("after the reorder: %q, want %q", got, top)
	}
	if got := onBoard(t, p, "j1"); got != check {
		t.Errorf("a card that was added before the reorder must not move: %q", got)
	}
}

func TestEachProfileUsesItsOwnRulesAndScore(t *testing.T) {
	setup(t)
	a, b, c := newProfile(t, "Ana"), newProfile(t, "Ben"), newProfile(t, "Cy")
	sa, sb := customStatus(t, a, "A status"), customStatus(t, b, "B status")
	rule(t, a, 60, sa)
	rule(t, b, 80, sb)
	addJob(t, "j1")
	f := &fake{by: map[string]map[string]int{"j1": {a: 65, b: 65, c: 99}}}
	built := useFake(t, f)

	if _, err := ProcessBatch([]string{"j1"}); err != nil {
		t.Fatal(err)
	}
	if onBoard(t, a, "j1") != sa {
		t.Error("Ana (65 >= 60) must get her status")
	}
	if onBoard(t, b, "j1") != "" {
		t.Error("Ben (65 < 80) must not get the job")
	}
	if onBoard(t, c, "j1") != "" {
		t.Error("Cy has no rules and must not get the job whatever the score")
	}
	if len(*built) != 1 || !(*built)[0][a] || !(*built)[0][b] || (*built)[0][c] || len((*built)[0]) != 2 {
		t.Errorf("the scorer must be built for exactly the profiles with rules, once: %v", *built)
	}
}

func TestAJobAlreadyOnTheBoardIsLeftAlone(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	auto, mine := customStatus(t, p, "Auto"), customStatus(t, p, "Mine")
	rule(t, p, 50, auto)
	addJob(t, "j1")
	if _, err := jobboard.SetEntry(p, "j1", mine); err != nil {
		t.Fatal(err)
	}
	var before string
	_ = db.JobsDB.QueryRow(`SELECT updated_at FROM job_board_entries WHERE job_id = 'j1'`).Scan(&before)
	useFake(t, &fake{by: map[string]map[string]int{"j1": {p: 90}}})

	outs, err := ProcessBatch([]string{"j1"})
	if err != nil {
		t.Fatal(err)
	}
	var after string
	_ = db.JobsDB.QueryRow(`SELECT updated_at FROM job_board_entries WHERE job_id = 'j1'`).Scan(&after)
	if onBoard(t, p, "j1") != mine || after != before {
		t.Errorf("the card moved: status %q, updated_at %q -> %q", onBoard(t, p, "j1"), before, after)
	}
	if o, _ := outcomeFor(outs, "j1", p); o.Status != OnBoard {
		t.Errorf("outcome = %+v", o)
	}
}

func TestADeliveryTwiceChangesNothingTheSecondTime(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	s := customStatus(t, p, "Auto")
	rule(t, p, 50, s)
	addJob(t, "j1")
	useFake(t, &fake{by: map[string]map[string]int{"j1": {p: 70}}})

	if outs, _ := ProcessBatch([]string{"j1"}); len(outs) != 1 || outs[0].Status != Added {
		t.Fatalf("first = %+v", outs)
	}
	outs, err := ProcessBatch([]string{"j1"})
	if err != nil || len(outs) != 1 || outs[0].Status != OnBoard {
		t.Errorf("second = %+v, %v", outs, err)
	}
	if entries(t) != 1 {
		t.Errorf("entries = %d, want 1", entries(t))
	}
}

func TestAnIdInTheBatchTwiceIsHandledOnce(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 50, jobboard.NotReviewedID)
	addJob(t, "j1")
	f := &fake{by: map[string]map[string]int{"j1": {p: 70}}}
	useFake(t, f)
	outs, _ := ProcessBatch([]string{"j1", "", "j1", "j1"})
	if len(outs) != 1 || len(f.asked) != 1 {
		t.Errorf("outcomes=%d asked=%v", len(outs), f.asked)
	}
}

func TestWithoutAnyRuleNothingIsReadOrScored(t *testing.T) {
	setup(t)
	newProfile(t, "Ana")
	addJob(t, "j1")
	built := useFake(t, &fake{by: map[string]map[string]int{"j1": {}}})
	outs, err := ProcessBatch([]string{"j1"})
	if err != nil || len(outs) != 0 || len(*built) != 0 || entries(t) != 0 {
		t.Errorf("outs=%v err=%v built=%d entries=%d", outs, err, len(*built), entries(t))
	}
}

func TestAnEmptyBatchDoesNothing(t *testing.T) {
	setup(t)
	built := useFake(t, &fake{})
	if outs, err := ProcessBatch(nil); err != nil || outs != nil || len(*built) != 0 {
		t.Errorf("outs=%v err=%v built=%d", outs, err, len(*built))
	}
	if outs, err := ProcessBatch([]string{"", ""}); err != nil || outs != nil || len(*built) != 0 {
		t.Errorf("blank ids: outs=%v err=%v built=%d", outs, err, len(*built))
	}
}

func TestAProfileThatCannotJudgeTheJobGetsNothing(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 1, jobboard.NotReviewedID)
	addJob(t, "j1")
	useFake(t, &fake{by: map[string]map[string]int{"j1": {}}}) // the profile is absent from the scores
	outs, err := ProcessBatch([]string{"j1"})
	if err != nil || len(outs) != 0 || entries(t) != 0 {
		t.Errorf("outs=%v err=%v entries=%d", outs, err, entries(t))
	}
}

func TestAJobThatNoLongerExistsIsSkippedAndTheRestGoOn(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 50, jobboard.NotReviewedID)
	addJob(t, "j2")
	useFake(t, &fake{by: map[string]map[string]int{"j2": {p: 80}}}) // "gone" is unknown to the fake
	outs, err := ProcessBatch([]string{"gone", "j2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || outs[0].JobID != "j2" || onBoard(t, p, "j2") != jobboard.NotReviewedID {
		t.Errorf("outs=%+v", outs)
	}
}

func TestAScoringFailureForOneJobDoesNotStopTheOthers(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 50, jobboard.NotReviewedID)
	addJob(t, "bad")
	addJob(t, "good")
	useFake(t, &fake{
		by:   map[string]map[string]int{"good": {p: 80}},
		errs: map[string]error{"bad": errors.New("boom")},
	})
	outs, err := ProcessBatch([]string{"bad", "good"})
	if err != nil {
		t.Fatal(err)
	}
	if o, _ := outcomeFor(outs, "bad", ""); o.Status != Unjudged || o.Err == nil {
		t.Errorf("bad outcome = %+v", o)
	}
	if onBoard(t, p, "good") != jobboard.NotReviewedID || onBoard(t, p, "bad") != "" {
		t.Error("the good job must be added and the bad one not")
	}
}

// A job deleted between its scoring and the insert is an Unjudged outcome, never an entry for a missing job or a panic.
func TestAJobDeletedWhileBeingScoredIsNotAdded(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 50, jobboard.NotReviewedID)
	addJob(t, "j1")
	useFake(t, &fake{
		by:   map[string]map[string]int{"j1": {p: 90}},
		hook: func(id string) { exec(t, `DELETE FROM jobs WHERE id = ?`, id) },
	})
	outs, err := ProcessBatch([]string{"j1"})
	if err != nil {
		t.Fatal(err)
	}
	if o, _ := outcomeFor(outs, "j1", p); o.Status != Unjudged || !errors.Is(o.Err, jobboard.ErrUnknownJob) {
		t.Errorf("outcome = %+v", o)
	}
	if entries(t) != 0 {
		t.Error("an entry for a deleted job")
	}
}

func TestARuleOfADeletedStatusIsGoneWithIt(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	gone := customStatus(t, p, "Gone")
	rule(t, p, 50, gone)
	if err := jobboard.DeleteStatus(gone); err != nil {
		t.Fatal(err)
	}
	addJob(t, "j1")
	built := useFake(t, &fake{by: map[string]map[string]int{"j1": {p: 90}}})
	outs, _ := ProcessBatch([]string{"j1"})
	if len(outs) != 0 || entries(t) != 0 || len(*built) != 0 {
		t.Errorf("outs=%v entries=%d built=%d: no rule is left, so nothing happens", outs, entries(t), len(*built))
	}
}

func TestAScorerThatCannotBeBuiltFailsTheBatchAndAddsNothing(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 50, jobboard.NotReviewedID)
	addJob(t, "j1")
	old := newScorer
	newScorer = func(map[string]bool) (scorer, error) { return nil, errors.New("no profiles") }
	t.Cleanup(func() { newScorer = old })
	if outs, err := ProcessBatch([]string{"j1"}); err == nil || outs != nil || entries(t) != 0 {
		t.Errorf("outs=%v err=%v entries=%d", outs, err, entries(t))
	}
}

func TestAnUnreadableRuleTableFailsTheBatch(t *testing.T) {
	setup(t)
	useFake(t, &fake{})
	exec(t, `DROP TABLE profile_match_rules`)
	if outs, err := ProcessBatch([]string{"j1"}); err == nil || outs != nil {
		t.Errorf("outs=%v err=%v, want an error", outs, err)
	}
}

// ---- real scoring, end to end ----

func str(s string) *string { return &s }

func seedProfile(t *testing.T, name string, skills []string) string {
	t.Helper()
	profileID := newProfile(t, name)
	personaID, err := persona.CreatePersona(profileID, name+" persona", "")
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
	if _, err := persona.AddCareerExperience(personaID, "Acme", "Backend Developer", "2019-03", "", "Go services with Docker"); err != nil {
		t.Fatal(err)
	}
	return profileID
}

const jobDesc = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
	"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."

// With the real scorer: the fitting profile (above 60) gets the job in its status, the other (below 60) does not, though it
// has the same rule; a profile without rules is never touched.
func TestTheRealScorerPutsTheJobOnTheFittingProfilesBoardOnly(t *testing.T) {
	setup(t)
	fit := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	other := seedProfile(t, "Other", []string{"Cobol", "Fortran"})
	bare := seedProfile(t, "Bare", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"}) // fits too, but has no rules
	sFit, sOther := customStatus(t, fit, "To check"), customStatus(t, other, "To check")
	rule(t, fit, 60, sFit)
	rule(t, other, 60, sOther)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('j1', 'https://x.example/j1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, jobDesc)

	outs, err := ProcessBatch([]string{"j1"})
	if err != nil {
		t.Fatal(err)
	}
	if onBoard(t, fit, "j1") != sFit {
		t.Errorf("the fitting profile's board: %q, want %q (outcomes %+v)", onBoard(t, fit, "j1"), sFit, outs)
	}
	if onBoard(t, other, "j1") != "" || onBoard(t, bare, "j1") != "" {
		t.Error("the profile that does not fit, and the one without rules, must not get the job")
	}
	if o, _ := outcomeFor(outs, "j1", other); o.Status != Below {
		t.Errorf("other's outcome = %+v, want below", o)
	}
	var matches int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_matches`).Scan(&matches)
	if matches != 0 {
		t.Error("this consumer must store no job_matches row")
	}
}

// ---- the route and the worker ----

func send(h func(http.ResponseWriter, *http.Request), method, body string) int {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, "/events/job-found-profile-rules", strings.NewReader(body)))
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
	useMarks(t)
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
		if n > BatchSize {
			t.Errorf("a batch of %d, the limit is %d", n, BatchSize)
		}
	}
	if len(b.sizes()) < 3 {
		t.Errorf("batches %v: 120 ids need at least three", b.sizes())
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
		return []Outcome{{JobID: ids[0], Err: errors.New("one job failed")}}, nil
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

// The consumer is declared next to the other job.found listener and routed in main.go.
func TestTheConsumerIsDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	m := string(raw)
	if !strings.Contains(m, `"topic": "career.job.found",
      "path_suffix": "events/job-found-profile-rules"`) {
		t.Error("manifest.json has no career.job.found -> events/job-found-profile-rules listener")
	}
	if !strings.Contains(m, `"path_suffix": "events/job-found"`) {
		t.Error("the JobMatchConsumer's own listener must stay")
	}
	mainSrc, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(mainSrc)
	if !strings.Contains(src, `"/events/job-found-profile-rules", profilejobmatchconsumer.EventHandler`) {
		t.Error("main.go does not route /events/job-found-profile-rules")
	}
	if !strings.Contains(src, `profilejobmatchconsumer.Start(context.Background())`) {
		t.Error("main.go does not start the worker")
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

// The outcomes of one job come in the order of the profile ids, so the result never depends on map order.
func TestOutcomesOfAJobComeInProfileIdOrder(t *testing.T) {
	setup(t)
	ids := []string{newProfile(t, "A"), newProfile(t, "B"), newProfile(t, "C"), newProfile(t, "D")}
	scores := map[string]int{}
	for _, id := range ids {
		rule(t, id, 10, jobboard.NotReviewedID)
		scores[id] = 50
	}
	addJob(t, "j1")
	useFake(t, &fake{by: map[string]map[string]int{"j1": scores}})
	for run := 0; run < 20; run++ {
		outs, err := ProcessBatch([]string{"j1"})
		if err != nil || len(outs) != 4 {
			t.Fatalf("%v %v", outs, err)
		}
		for i := 1; i < len(outs); i++ {
			if outs[i-1].ProfileID >= outs[i].ProfileID {
				t.Fatalf("run %d: outcomes not in profile id order: %s before %s", run, outs[i-1].ProfileID, outs[i].ProfileID)
			}
		}
	}
}

// ---- removals (step 144) ----

// A job the person took off this board is not put back by the consumer (also when its event is delivered again).
func TestAJobThePersonRemovedIsNotAddedAgain(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	s := customStatus(t, p, "Auto")
	rule(t, p, 50, s)
	addJob(t, "j1")
	useFake(t, &fake{by: map[string]map[string]int{"j1": {p: 80}}})
	if outs, _ := ProcessBatch([]string{"j1"}); len(outs) != 1 || outs[0].Status != Added {
		t.Fatalf("first delivery: %+v", outs)
	}
	if err := jobboard.RemoveEntry(p, "j1"); err != nil { // the person takes the card off
		t.Fatal(err)
	}
	outs, err := ProcessBatch([]string{"j1"}) // the same event again
	if err != nil || len(outs) != 1 {
		t.Fatalf("%+v %v", outs, err)
	}
	if outs[0].Status != Removed || outs[0].StatusID != s || onBoard(t, p, "j1") != "" {
		t.Errorf("outcome %+v, on board %q: a removed job must stay off", outs[0], onBoard(t, p, "j1"))
	}
}

func TestARemovalOnOneProfileDoesNotStopAnotherProfilesAdd(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	sa, sb := customStatus(t, a, "A"), customStatus(t, b, "B")
	rule(t, a, 50, sa)
	rule(t, b, 50, sb)
	addJob(t, "j1")
	if _, err := jobboard.SetEntry(a, "j1", sa); err != nil {
		t.Fatal(err)
	}
	_ = jobboard.RemoveEntry(a, "j1") // Ana removed it
	useFake(t, &fake{by: map[string]map[string]int{"j1": {a: 80, b: 80}}})
	outs, _ := ProcessBatch([]string{"j1"})
	if o, _ := outcomeFor(outs, "j1", a); o.Status != Removed {
		t.Errorf("Ana: %+v", o)
	}
	if o, _ := outcomeFor(outs, "j1", b); o.Status != Added || onBoard(t, b, "j1") != sb {
		t.Errorf("Ben: %+v, on board %q", o, onBoard(t, b, "j1"))
	}
}

// A job the person removed and then put back by hand is just a card on the board: the consumer leaves it alone.
func TestAJobPutBackByHandIsOnTheBoardForTheConsumer(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	s := customStatus(t, p, "Auto")
	mine := customStatus(t, p, "Mine")
	rule(t, p, 50, s)
	addJob(t, "j1")
	_, _ = jobboard.SetEntry(p, "j1", s)
	_ = jobboard.RemoveEntry(p, "j1")
	_, _ = jobboard.SetEntry(p, "j1", mine) // the person puts it back, in their own status
	useFake(t, &fake{by: map[string]map[string]int{"j1": {p: 90}}})
	outs, _ := ProcessBatch([]string{"j1"})
	if len(outs) != 1 || outs[0].Status != OnBoard || onBoard(t, p, "j1") != mine {
		t.Errorf("%+v, on board %q", outs, onBoard(t, p, "j1"))
	}
}

// ---- a job whose text changed (step 164) ----

func changedEvent(jobID, source string) string {
	return fmt.Sprintf(`{"id":"e2","topic":"career.job.description_changed","payload":{"job_id":%q,"source":%q}}`, jobID, source)
}

// The route takes the description-changed event as it is really sent ({job_id, source}); a payload without a job id is refused.
func TestTheRouteQueuesADescriptionChangedEventToo(t *testing.T) {
	c := New(10)
	for _, source := range []string{"detail_crawl", "listing_crawl", ""} {
		if code := send(c.Handler, http.MethodPost, changedEvent("job-"+source, source)); code != http.StatusAccepted {
			t.Fatalf("source %q: status %d", source, code)
		}
	}
	if len(c.queue) != 3 {
		t.Errorf("queued %d, want 3", len(c.queue))
	}
	if code := send(c.Handler, http.MethodPost, `{"id":"e3","topic":"career.job.description_changed","payload":{"source":"detail_crawl"}}`); code != http.StatusBadRequest {
		t.Errorf("no job id: %d", code)
	}
}

// The case this step exists for: a job found with a short snippet fits nothing; the detail crawl fills in the full text; the
// event brings it to the board of the profile it now fits.
func TestAJobWhoseFullTextNowFitsARuleGetsOntoTheBoard(t *testing.T) {
	setup(t)
	fit := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	sFit := customStatus(t, fit, "To check")
	rule(t, fit, 60, sFit)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('j1', 'https://x.example/j1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', 'short snippet')`)

	if _, err := ProcessBatch([]string{"j1"}); err != nil { // the job.found run, with the snippet
		t.Fatal(err)
	}
	if got := onBoard(t, fit, "j1"); got != "" {
		t.Fatalf("with the snippet the job must fit nothing, it is on the board as %q", got)
	}

	exec(t, `UPDATE jobs SET description = ?, description_changed_at = datetime('now') WHERE id = 'j1'`, jobDesc) // the detail crawl
	outs, err := ProcessBatch([]string{"j1"})                                                                     // what the description-changed event queues
	if err != nil {
		t.Fatal(err)
	}
	if got := onBoard(t, fit, "j1"); got != sFit {
		t.Errorf("with the full text the job must reach the board as %q, got %q (%+v)", sFit, got, outs)
	}
}

// A text change never moves a job that is already on a board, even where a different rule would now apply, and never puts
// back a job the person took off.
func TestATextChangeNeverMovesOrRestoresAnything(t *testing.T) {
	setup(t)
	p := seedProfile(t, "P", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	high := customStatus(t, p, "Great fit")
	low := customStatus(t, p, "Maybe")
	rule(t, p, 90, high)
	rule(t, p, 20, low)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('kept', 'https://x.example/kept', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, jobDesc)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('gone', 'https://x.example/gone', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, jobDesc)
	exec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES (?, 'kept', ?)`, p, low) // the person (or an earlier run) put it in "Maybe"
	exec(t, `INSERT INTO job_board_removals (profile_id, job_id) VALUES (?, 'gone')`, p)

	exec(t, `UPDATE jobs SET description = description || ' Zusätzlich: Go, Docker, Kubernetes.' WHERE id IN ('kept', 'gone')`)
	if _, err := ProcessBatch([]string{"kept", "gone"}); err != nil {
		t.Fatal(err)
	}
	if got := onBoard(t, p, "kept"); got != low {
		t.Errorf("a job on a board stays where it is: %q, want %q", got, low)
	}
	if got := onBoard(t, p, "gone"); got != "" {
		t.Errorf("a job the person took off stays off: %q", got)
	}
}

// Two deliveries of the same change, and the found event and the change event for one job in a row, add the job once.
func TestRepeatedAndMixedEventsAddTheJobOnce(t *testing.T) {
	setup(t)
	fit := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	sFit := customStatus(t, fit, "To check")
	rule(t, fit, 60, sFit)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('j1', 'https://x.example/j1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, jobDesc)
	if _, err := ProcessBatch([]string{"j1", "j1", "j1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ProcessBatch([]string{"j1"}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_board_entries WHERE job_id = 'j1'`).Scan(&n)
	if n != 1 || onBoard(t, fit, "j1") != sFit {
		t.Errorf("entries: %d", n)
	}
}

// Through the route and the worker: a queued description-changed event reaches the batch the worker handles.
func TestADescriptionChangedEventReachesTheWorker(t *testing.T) {
	b := watch(t, nil)
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	defer func() { cancel(); <-done }()
	if code := send(c.Handler, http.MethodPost, changedEvent("job-9", "detail_crawl")); code != http.StatusAccepted {
		t.Fatalf("status %d", code)
	}
	waitFor(t, "the batch", func() bool { return len(b.sizes()) == 1 })
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.list[0]) != 1 || b.list[0][0] != "job-9" {
		t.Errorf("batch %v", b.list)
	}
}

// The consumer is declared for the second topic, routed, and its first listener is untouched.
func TestTheDescriptionChangedListenerIsDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Listeners []struct {
			Topic      string `json:"topic"`
			PathSuffix string `json:"path_suffix"`
		} `json:"event_listeners"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	count := func(topic, path string) int {
		n := 0
		for _, l := range m.Listeners {
			if l.Topic == topic && l.PathSuffix == path {
				n++
			}
		}
		return n
	}
	if count("career.job.description_changed", "events/job-description-changed-profile-rules") != 1 {
		t.Error("manifest.json must declare career.job.description_changed -> events/job-description-changed-profile-rules exactly once")
	}
	for _, keep := range [][2]string{
		{"career.job.found", "events/job-found-profile-rules"},
		{"career.job.description_changed", "events/job-description-changed-persona-matches"},
		{"career.job.found", "events/job-found-persona-matches"},
	} {
		if count(keep[0], keep[1]) != 1 {
			t.Errorf("the listener %v must stay", keep)
		}
	}
	mainSrc, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), `"/events/job-description-changed-profile-rules", profilejobmatchconsumer.EventHandler`) {
		t.Error("main.go does not route /events/job-description-changed-profile-rules")
	}
}
