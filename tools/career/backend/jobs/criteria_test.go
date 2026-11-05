package jobs

import (
	"errors"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs/deletedjobs"
)

// fakeMatcher is a CriteriaMatcher that answers as told and records what it was asked, in a log shared by all the matchers of a test.
type fakeMatcher struct {
	name   string
	keep   bool
	reason string
	err    error
	panics bool
	log    *[]string
	seen   []Candidate
}

func (m *fakeMatcher) Name() string { return m.name }

func (m *fakeMatcher) Match(c Candidate) (Verdict, error) {
	*m.log = append(*m.log, m.name)
	m.seen = append(m.seen, c)
	if m.panics {
		panic("matcher blew up")
	}
	if m.err != nil {
		return Verdict{}, m.err
	}
	return Verdict{Keep: m.keep, Reason: m.reason}, nil
}

func matcher(log *[]string, name string, keep bool) *fakeMatcher {
	return &fakeMatcher{name: name, keep: keep, reason: name + " says no", log: log}
}

func jobsInTable(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWithoutMatchersEveryNewJobIsInserted(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	for name, c := range map[string]*Criteria{"nil chain": nil, "empty chain": NewCriteriaFrom(), "no source": NewCriteria()} {
		_, created, _, err := SavePortalJobWith(c, "l1", "https://portal.example/jobs/"+strings.ReplaceAll(name, " ", "-"), "T "+name, "Acme "+name, "", "d", "")
		if err != nil || !created {
			t.Errorf("%s: created=%v err=%v", name, created, err)
		}
	}
}

func TestARefusedNewJobIsNotInsertedAndSendsNoEvent(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	var log []string
	_, created, duplicate, err := SavePortalJobWith(NewCriteriaFrom(matcher(&log, "strict", false)), "l1", "https://portal.example/jobs/1", "Job", "Acme", "Berlin", "text", "today")
	if !errors.Is(err, ErrFilteredOut) || created || duplicate {
		t.Fatalf("created=%v duplicate=%v err=%v", created, duplicate, err)
	}
	var refused *FilteredOutError
	if !errors.As(err, &refused) || refused.Matcher != "strict" || refused.Reason != "strict says no" {
		t.Errorf("the error must say who refused and why: %+v", refused)
	}
	if jobsInTable(t) != 0 || events.count() != 0 {
		t.Errorf("nothing may be written or announced: %d jobs, %d events", jobsInTable(t), events.count())
	}
}

func TestAKeptNewJobIsInsertedAndAnnounced(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	var log []string
	_, created, _, err := SavePortalJobWith(NewCriteriaFrom(matcher(&log, "lenient", true)), "l1", "https://portal.example/jobs/1", "Job", "Acme", "", "text", "")
	if err != nil || !created || jobsInTable(t) != 1 {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if len(events.ofTopic("career.job.found")) != 1 {
		t.Error("the new job is announced")
	}
}

func TestMatchersAreAskedInOrderAndTheFirstRefusalEndsTheCheck(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	var log []string
	chain := NewCriteriaFrom(matcher(&log, "a", true), matcher(&log, "b", false), matcher(&log, "c", false))
	_, _, _, err := SavePortalJobWith(chain, "l1", "https://portal.example/jobs/1", "Job", "Acme", "", "text", "")
	var refused *FilteredOutError
	if !errors.As(err, &refused) || refused.Matcher != "b" {
		t.Fatalf("b is the first to refuse: %v", err)
	}
	if strings.Join(log, ",") != "a,b" {
		t.Errorf("asked %v: c must not be asked after b refused", log)
	}
}

func TestEveryMatcherMustKeepTheJob(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	var log []string
	chain := NewCriteriaFrom(matcher(&log, "a", true), matcher(&log, "b", true), matcher(&log, "c", true))
	if _, created, _, err := SavePortalJobWith(chain, "l1", "https://portal.example/jobs/1", "Job", "Acme", "", "text", ""); err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if strings.Join(log, ",") != "a,b,c" {
		t.Errorf("all three are asked: %v", log)
	}
}

// A matcher that fails never loses a job: the error is logged, the job counts as kept by that matcher and the next one is still asked.
func TestAFailingMatcherKeepsTheJobAndTheNextOneIsStillAsked(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	var log []string
	broken := matcher(&log, "broken", false)
	broken.err = errors.New("scoring data unreadable")
	panicky := matcher(&log, "panicky", false)
	panicky.panics = true

	if _, created, _, err := SavePortalJobWith(NewCriteriaFrom(broken, panicky), "l1", "https://portal.example/jobs/1", "Job 1", "Acme", "", "t", ""); err != nil || !created {
		t.Fatalf("an error and a panic keep the job: created=%v err=%v", created, err)
	}
	log = nil
	strict := matcher(&log, "strict", false)
	_, _, _, err := SavePortalJobWith(NewCriteriaFrom(broken, strict), "l1", "https://portal.example/jobs/2", "Job 2", "Acme", "", "t", "")
	if !errors.Is(err, ErrFilteredOut) || strings.Join(log, ",") != "broken,strict" {
		t.Errorf("a later matcher can still refuse: err=%v asked=%v", err, log)
	}
}

// The matchers are only asked about a real insert: a job that exists is refreshed, a cross-portal duplicate is reported, a job deleted
// before is turned away, whatever the matchers would say.
func TestMatchersAreNotAskedAboutAnythingThatIsNotAnInsert(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	if _, created, _, err := SavePortalJobWith(nil, "l1", "https://portal.example/jobs/1", "Job", "Acme", "", "old text", ""); err != nil || !created {
		t.Fatal(err)
	}
	var log []string
	refuser := NewCriteriaFrom(matcher(&log, "strict", false))

	// the same URL again: refreshed
	id, created, duplicate, err := SavePortalJobWith(refuser, "l1", "https://portal.example/jobs/1", "Job", "Acme", "", "new text", "")
	if err != nil || created || duplicate || id == "" {
		t.Fatalf("a refresh: id=%q created=%v duplicate=%v err=%v", id, created, duplicate, err)
	}
	if got := descriptionOf(t, id); got != "new text" {
		t.Errorf("the refresh went through: %q", got)
	}
	// another URL, the same title and company: reported as a duplicate
	if _, created, duplicate, err := SavePortalJobWith(refuser, "l1", "https://other.example/jobs/9", "Job", "Acme", "", "x", ""); err != nil || created || !duplicate {
		t.Errorf("a duplicate: created=%v duplicate=%v err=%v", created, duplicate, err)
	}
	// a URL deleted before
	if _, err := deletedjobs.Remember(deletedjobs.Entry{SourceURL: "https://portal.example/jobs/gone", Title: "x", Reason: deletedjobs.ReasonDeleted}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := SavePortalJobWith(refuser, "l1", "https://portal.example/jobs/gone", "Gone", "Acme", "", "x", ""); !errors.Is(err, ErrDeletedBefore) {
		t.Errorf("deleted before: %v", err)
	}
	if len(log) != 0 {
		t.Errorf("the matchers were asked %v times about jobs that were not inserts", log)
	}
}

func TestAJobAddedByHandIsNeverAskedAbout(t *testing.T) {
	setupJobsDB(t)
	var log []string
	UseCriteria(sourceOf(matcher(&log, "strict", false)))
	t.Cleanup(func() { UseCriteria(nil) })
	if _, err := AddManualJob(ManualJobInput{Title: "Mine", Company: "Me"}); err != nil {
		t.Fatal(err)
	}
	if len(log) != 0 || jobsInTable(t) != 1 {
		t.Errorf("asked %v, %d jobs", log, jobsInTable(t))
	}
}

func TestSaveJobAsksTheCriteriaToo(t *testing.T) {
	setupJobsDB(t)
	var log []string
	_, created, err := saveJobWith(NewCriteriaFrom(matcher(&log, "strict", false)), "https://x.example/jobs/1", "Job", "Acme", "", "text", "")
	if !errors.Is(err, ErrFilteredOut) || created || jobsInTable(t) != 0 {
		t.Fatalf("refused: created=%v err=%v", created, err)
	}
	id, created, err := saveJobWith(NewCriteriaFrom(matcher(&log, "lenient", true)), "https://x.example/jobs/1", "Job", "Acme", "", "text", "")
	if err != nil || !created || id == "" {
		t.Fatalf("kept: %v", err)
	}
	// once saved, a refresh is not refused
	if _, created, err := saveJobWith(NewCriteriaFrom(matcher(&log, "strict", false)), "https://x.example/jobs/1", "Job", "Acme", "", "newer", ""); err != nil || created {
		t.Errorf("a refresh: created=%v err=%v", created, err)
	}
}

// What a matcher is given: the candidate as the crawl found it, the description already plain text, the link of the portal.
func TestTheMatcherIsGivenTheCandidate(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	var log []string
	m := matcher(&log, "spy", true)
	_, _, _, err := SavePortalJobWith(NewCriteriaFrom(m), "l1", "/jobs/42", "Backend Engineer", "Acme", "Hamburg", "<style>.a{top:0}</style>Real text", "2 days ago")
	if err != nil || len(m.seen) != 1 {
		t.Fatalf("%v %d", err, len(m.seen))
	}
	want := Candidate{SourceURL: "https://portal.example/jobs/42", Title: "Backend Engineer", Company: "Acme", Location: "Hamburg", Description: "Real text", PostedAt: "2 days ago", PortalLinkID: "l1"}
	if m.seen[0] != want {
		t.Errorf("got  %+v\nwant %+v", m.seen[0], want)
	}
	var log2 []string
	m2 := matcher(&log2, "spy2", true)
	if _, _, err := saveJobWith(NewCriteriaFrom(m2), "https://x.example/jobs/7", "T", "C", "L", "<b>bold</b> words", "p"); err != nil {
		t.Fatal(err)
	}
	if got := m2.seen[0]; got.PortalLinkID != "" || got.Description != "bold words" || got.SourceURL != "https://x.example/jobs/7" {
		t.Errorf("saveJob's candidate: %+v", got)
	}
}

// ---- the source ----

type fakeSource struct {
	ms    []CriteriaMatcher
	err   error
	calls int
}

func (s *fakeSource) Matchers() ([]CriteriaMatcher, error) {
	s.calls++
	return s.ms, s.err
}

func sourceOf(ms ...CriteriaMatcher) *fakeSource { return &fakeSource{ms: ms} }

func TestTheSourceIsAskedOncePerChainNotPerJob(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	var log []string
	src := sourceOf(matcher(&log, "lenient", true))
	UseCriteria(src)
	t.Cleanup(func() { UseCriteria(nil) })
	chain := NewCriteria()
	for i := 0; i < 5; i++ {
		if _, _, _, err := SavePortalJobWith(chain, "l1", "https://portal.example/jobs/"+string(rune('a'+i)), "Job "+string(rune('a'+i)), "Acme", "", "t", ""); err != nil {
			t.Fatal(err)
		}
	}
	if src.calls != 1 || len(log) != 5 {
		t.Errorf("source asked %d times, matcher %d times", src.calls, len(log))
	}
	// the plain save builds its own chain, one per call
	if _, _, _, err := SavePortalJob("l1", "https://portal.example/jobs/z", "Job z", "Acme", "", "t", ""); err != nil || src.calls != 2 {
		t.Errorf("err=%v calls=%d", err, src.calls)
	}
}

func TestASourceThatFailsKeepsEveryJob(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	var log []string
	// the source answers with a matcher that would refuse AND an error: the error wins, nothing is asked, every job is kept
	UseCriteria(&fakeSource{ms: []CriteriaMatcher{matcher(&log, "strict", false)}, err: errors.New("settings unreadable")})
	t.Cleanup(func() { UseCriteria(nil) })
	if _, created, _, err := SavePortalJob("l1", "https://portal.example/jobs/1", "Job", "Acme", "", "t", ""); err != nil || !created {
		t.Errorf("created=%v err=%v", created, err)
	}
	if len(log) != 0 {
		t.Errorf("a failed source must not leave matchers behind: asked %v", log)
	}
}

func TestRemovingTheSourceKeepsEverythingAgain(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	var log []string
	UseCriteria(sourceOf(matcher(&log, "strict", false)))
	if _, _, _, err := SavePortalJob("l1", "https://portal.example/jobs/1", "Job 1", "Acme", "", "t", ""); !errors.Is(err, ErrFilteredOut) {
		t.Fatalf("refused while injected: %v", err)
	}
	UseCriteria(nil)
	if _, created, _, err := SavePortalJob("l1", "https://portal.example/jobs/1", "Job 1", "Acme", "", "t", ""); err != nil || !created {
		t.Errorf("created=%v err=%v", created, err)
	}
}

func TestFilteredOutErrorReadsWell(t *testing.T) {
	err := error(&FilteredOutError{Matcher: "min-profile-match", Reason: "best match 22% is under 30%"})
	if !errors.Is(err, ErrFilteredOut) || errors.Is(err, ErrDeletedBefore) {
		t.Error("it is ErrFilteredOut and nothing else")
	}
	if !strings.Contains(err.Error(), "min-profile-match") || !strings.Contains(err.Error(), "22%") {
		t.Errorf("message: %q", err.Error())
	}
}

// ---- the log of refused jobs ----

func TestTheLogIsToldAboutEveryRefusedJob(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	var told []string
	UseFilteredOutLog(func(c Candidate, e *FilteredOutError) { told = append(told, c.Title+"|"+e.Matcher+"|"+e.Reason) })
	t.Cleanup(func() { UseFilteredOutLog(nil) })
	var log []string
	strict := NewCriteriaFrom(matcher(&log, "strict", false))
	if _, _, _, err := SavePortalJobWith(strict, "l1", "https://portal.example/jobs/1", "Job 1", "Acme", "", "t", ""); !errors.Is(err, ErrFilteredOut) {
		t.Fatal(err)
	}
	if _, _, err := saveJobWith(strict, "https://x.example/jobs/2", "Job 2", "Acme", "", "t", ""); !errors.Is(err, ErrFilteredOut) {
		t.Fatal(err)
	}
	// a kept job, a refresh and a manual job are not told
	if _, _, _, err := SavePortalJobWith(NewCriteriaFrom(matcher(&log, "lenient", true)), "l1", "https://portal.example/jobs/3", "Job 3", "Acme", "", "t", ""); err != nil {
		t.Fatal(err)
	}
	want := []string{"Job 1|strict|strict says no", "Job 2|strict|strict says no"}
	if strings.Join(told, ";") != strings.Join(want, ";") {
		t.Errorf("told %v, want %v", told, want)
	}
}

// A log that panics never fails the crawl: the refusal still comes back.
func TestALogThatPanicsDoesNotBreakTheCrawl(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	UseFilteredOutLog(func(Candidate, *FilteredOutError) { panic("log blew up") })
	t.Cleanup(func() { UseFilteredOutLog(nil) })
	var log []string
	_, _, _, err := SavePortalJobWith(NewCriteriaFrom(matcher(&log, "strict", false)), "l1", "https://portal.example/jobs/1", "Job", "Acme", "", "t", "")
	if !errors.Is(err, ErrFilteredOut) {
		t.Errorf("err %v", err)
	}
}
