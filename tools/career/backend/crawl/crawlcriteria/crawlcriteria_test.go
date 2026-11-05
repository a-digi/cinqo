package crawlcriteria

import (
	jobmatch "career-tool-backend/match"
	"errors"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

// fakeScorer gives the scores it is told, for the edges of the rule.
type fakeScorer struct {
	scores []int
	err    error
	seen   []jobs.Candidate
}

func (f *fakeScorer) ScoreCandidate(c jobs.Candidate) ([]int, error) {
	f.seen = append(f.seen, c)
	return f.scores, f.err
}

func match(t *testing.T, percent int, scores ...int) jobs.Verdict {
	t.Helper()
	m, err := NewMinProfileMatch(percent, &fakeScorer{scores: scores})
	if err != nil {
		t.Fatal(err)
	}
	v, err := m.Match(jobs.Candidate{Title: "Job"})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestTheJobIsKeptWhenAtLeastOneProfileReachesTheLimit(t *testing.T) {
	cases := []struct {
		name    string
		percent int
		scores  []int
		keep    bool
	}{
		{"one profile above", 30, []int{55}, true},
		{"one profile exactly at the limit", 30, []int{30}, true},
		{"one profile one under the limit", 30, []int{29}, false},
		{"one profile far under", 30, []int{3}, false},
		{"the best of several decides", 30, []int{5, 41, 12}, true},
		{"all under", 30, []int{5, 29, 12}, false},
		{"only the first is above", 30, []int{80, 1, 2}, true},
		{"only the last is above", 30, []int{1, 2, 80}, true},
		{"a zero score under any limit", 1, []int{0}, false},
		{"a zero score at the lowest limit of 1 is under", 1, []int{0, 0}, false},
		{"100 needed and reached", 100, []int{100}, true},
		{"100 needed and 99 is not enough", 100, []int{99, 98}, false},
	}
	for _, c := range cases {
		if got := match(t, c.percent, c.scores...); got.Keep != c.keep {
			t.Errorf("%s: keep=%v, want %v (%s)", c.name, got.Keep, c.keep, got.Reason)
		}
	}
}

func TestAJobNoProfileCouldJudgeIsKept(t *testing.T) {
	v := match(t, 30) // no scores at all
	if !v.Keep || !strings.Contains(v.Reason, "no profile could judge") {
		t.Errorf("%+v", v)
	}
}

func TestTheReasonsAreInPlainWords(t *testing.T) {
	if v := match(t, 30, 22, 14); v.Keep || v.Reason != "best profile match 22% is under the limit of 30%" {
		t.Errorf("%+v", v)
	}
	if v := match(t, 30, 22, 41); !v.Keep || v.Reason != "best profile match 41% reaches the limit of 30%" {
		t.Errorf("%+v", v)
	}
}

// A scorer that fails is an error, not a verdict: the crawler's chain turns that into "keep the job".
func TestAScorerErrorIsReturnedNotTurnedIntoARefusal(t *testing.T) {
	boom := errors.New("scoring data unreadable")
	m, _ := NewMinProfileMatch(30, &fakeScorer{err: boom})
	v, err := m.Match(jobs.Candidate{Title: "Job"})
	if !errors.Is(err, boom) || v.Keep {
		t.Errorf("verdict %+v, err %v", v, err)
	}
	// and the chain of the crawler keeps the job
	if refused := jobs.NewCriteriaFrom(m).Check(jobs.Candidate{Title: "Job"}); refused != nil {
		t.Errorf("the chain must keep a job whose matcher failed: %+v", refused)
	}
}

func TestTheScorerIsGivenTheCandidate(t *testing.T) {
	f := &fakeScorer{scores: []int{50}}
	m, _ := NewMinProfileMatch(30, f)
	c := jobs.Candidate{SourceURL: "https://x/1", Title: "T", Company: "C", Location: "L", Description: "D", PostedAt: "P", PortalLinkID: "l1"}
	if _, err := m.Match(c); err != nil || len(f.seen) != 1 || f.seen[0] != c {
		t.Errorf("seen %+v err %v", f.seen, err)
	}
}

func TestTheNameAndTheInterface(t *testing.T) {
	m, _ := NewMinProfileMatch(30, &fakeScorer{})
	var asMatcher jobs.CriteriaMatcher = m
	if asMatcher.Name() != "min-profile-match" || MinProfileMatchName != "min-profile-match" {
		t.Errorf("name %q", asMatcher.Name())
	}
}

func TestTheLimitMustBeOneToHundredAndAScorerIsRequired(t *testing.T) {
	for _, p := range []int{-5, 0, 101, 1000} {
		if _, err := NewMinProfileMatch(p, &fakeScorer{}); !errors.Is(err, ErrInvalidPercent) {
			t.Errorf("%d: %v", p, err)
		}
	}
	for _, p := range []int{1, 30, 100} {
		if _, err := NewMinProfileMatch(p, &fakeScorer{}); err != nil {
			t.Errorf("%d: %v", p, err)
		}
	}
	if _, err := NewMinProfileMatch(30, nil); err == nil {
		t.Error("a missing scorer is refused")
	}
}

// ---- with the real engine and real personas ----

func str(s string) *string { return &s }

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
}

func seedProfile(t *testing.T, name string, skills []string) {
	t.Helper()
	profileID, err := profile.CreateProfile(name, "Test")
	if err != nil {
		t.Fatal(err)
	}
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
}

const goJobDesc = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
	"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."

const salesJobDesc = "Wir suchen einen Vertriebsleiter für den Außendienst. Du führst Kundengespräche, betreust Key Accounts und verkaufst " +
	"Lizenzen an Einkäufer. Verkaufserfahrung von mindestens 5 Jahren ist erforderlich."

var (
	fitting   = jobs.Candidate{Title: "Backend Engineer (m/w/d)", Company: "Acme", Location: "Hamburg", Description: goJobDesc}
	unfitting = jobs.Candidate{Title: "Sales Manager", Company: "Acme", Location: "Munich", Description: salesJobDesc}
)

func TestTheRealEngineKeepsAFittingJobAndRefusesAnUnfittingOne(t *testing.T) {
	setup(t)
	seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	m, err := NewMinProfileMatchFromData(30)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := m.Match(fitting); err != nil || !v.Keep {
		t.Errorf("a fitting job: %+v %v", v, err)
	}
	if v, err := m.Match(unfitting); err != nil || v.Keep {
		t.Errorf("an unfitting job: %+v %v", v, err)
	}
}

// At least ONE profile above the limit is enough: a second profile that does not fit changes nothing.
func TestOneFittingProfileAmongSeveralIsEnough(t *testing.T) {
	setup(t)
	seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	seedProfile(t, "Other", []string{"Cobol", "Fortran"})
	m, err := NewMinProfileMatchFromData(30)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := m.Match(fitting); !v.Keep {
		t.Errorf("one profile fits: %+v", v)
	}
	if v, _ := m.Match(unfitting); v.Keep {
		t.Errorf("no profile fits a sales job: %+v", v)
	}
}

// The edge with the real numbers: a limit equal to the best score keeps the job, one above refuses it.
func TestTheLimitEdgeWithTheRealScore(t *testing.T) {
	setup(t)
	seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	scorer, err := jobmatch.NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	scores, err := scorer.ScoreCandidate(fitting)
	if err != nil || len(scores) != 1 || scores[0] < 2 || scores[0] > 99 {
		t.Fatalf("scores %v %v", scores, err)
	}
	best := scores[0]
	atEdge, _ := NewMinProfileMatch(best, scorer)
	if v, _ := atEdge.Match(fitting); !v.Keep {
		t.Errorf("a limit of %d with a score of %d keeps the job: %+v", best, best, v)
	}
	above, _ := NewMinProfileMatch(best+1, scorer)
	if v, _ := above.Match(fitting); v.Keep {
		t.Errorf("a limit of %d with a score of %d refuses the job: %+v", best+1, best, v)
	}
}

// Nothing to judge it with: no profile at all, or a profile without a persona: every job is kept.
func TestWithoutAnythingToJudgeWithEveryJobIsKept(t *testing.T) {
	setup(t)
	m, err := NewMinProfileMatchFromData(100)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := m.Match(unfitting); !v.Keep {
		t.Errorf("no profile: %+v", v)
	}
	if _, err := profile.CreateProfile("Bare", "Profile"); err != nil {
		t.Fatal(err)
	}
	m2, _ := NewMinProfileMatchFromData(100)
	if v, _ := m2.Match(unfitting); !v.Keep {
		t.Errorf("a profile without a persona: %+v", v)
	}
}

// The whole thing the crawler does: the matcher in the chain, through the real funnel. A refused new job is not inserted, a kept one is,
// and one already saved is refreshed even though the matcher would now refuse it.
func TestThroughTheFunnelOfTheCrawler(t *testing.T) {
	setup(t)
	seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	if _, err := db.JobsDB.Exec(`INSERT INTO portals (id, name) VALUES ('p1', 'Portal')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://portal.example/search')`); err != nil {
		t.Fatal(err)
	}
	m, err := NewMinProfileMatchFromData(30)
	if err != nil {
		t.Fatal(err)
	}
	chain := jobs.NewCriteriaFrom(m)

	if _, created, _, err := jobs.SavePortalJobWith(chain, "l1", "https://portal.example/jobs/fit", fitting.Title, fitting.Company, fitting.Location, fitting.Description, ""); err != nil || !created {
		t.Fatalf("the fitting job: created=%v err=%v", created, err)
	}
	_, _, _, err = jobs.SavePortalJobWith(chain, "l1", "https://portal.example/jobs/sales", unfitting.Title, unfitting.Company, unfitting.Location, unfitting.Description, "")
	var refused *jobs.FilteredOutError
	if !errors.As(err, &refused) || refused.Matcher != MinProfileMatchName || !strings.Contains(refused.Reason, "under the limit of 30%") {
		t.Fatalf("the unfitting job must be refused with the reason: %v", err)
	}
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil || n != 1 {
		t.Errorf("%d jobs, want only the fitting one (%v)", n, err)
	}
	// the fitting job comes back with a text that would be refused: it is refreshed, not hidden
	if _, created, _, err := jobs.SavePortalJobWith(chain, "l1", "https://portal.example/jobs/fit", unfitting.Title, unfitting.Company, unfitting.Location, unfitting.Description, ""); err != nil || created {
		t.Errorf("a refresh: created=%v err=%v", created, err)
	}
}
