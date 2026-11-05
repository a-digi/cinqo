package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/jobs/jobboard"
	"career-tool-backend/jobs/jobmatchconsumer"
	"career-tool-backend/persona"
	"career-tool-backend/persona/personamatchconsumer"
	"career-tool-backend/profile"
	"career-tool-backend/profile/profilejobmatchconsumer"
	"career-tool-backend/profile/profilerules"
)

// Step 164, step 5: the whole chain, with the real consumers, workers, scorers and database, and a stand-in for core that
// delivers an event to the three listeners or drops it. A job is found with a snippet; its full text arrives later through a
// detail crawl (the real save); the persona scores, the job's match and the profile's board must all follow, whether the
// events arrive or are lost and the sweepers repair it. See plan/ai/tools/career/step-164-job-description-changed-rematch.md.

const (
	chainSnippet = "Backend Engineer in Hamburg. Wir arbeiten mit Cobol und Fortran auf dem Mainframe. Mindestens 3 Jahre Berufserfahrung."
	chainFull    = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
		"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."
)

// chain is one test's world: a profile that fits chainFull and not chainSnippet, with one rule (60%), and the stand-in for core.
type chain struct {
	t        *testing.T
	profile  string
	persona  string
	status   string
	mu       sync.Mutex
	lost     map[string]bool // which listeners miss the description_changed event: "persona", "rules", "match"
	received map[string]int  // events delivered, per listener
}

func newChain(t *testing.T) *chain {
	t.Helper()
	setupHandlerDB(t)
	c := &chain{t: t, lost: map[string]bool{}, received: map[string]int{}}
	var err error
	if c.profile, err = profile.CreateProfile("Fit", "Test"); err != nil {
		t.Fatal(err)
	}
	if c.persona, err = persona.CreatePersona(c.profile, "Fit persona", ""); err != nil {
		t.Fatal(err)
	}
	h, titles, loc := "Backend Engineer", "Software Engineer, Backend Developer", "Hamburg"
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: c.persona, Headline: &h, DesiredTitles: &titles, Location: &loc, DesiredLocations: &loc}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Go", "Docker", "Kubernetes", "PostgreSQL"} {
		if err := persona.AddCareerSkill(c.persona, s); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := persona.AddCareerExperience(c.persona, "Acme", "Backend Developer", "2019-03", "", "Go services with Docker"); err != nil {
		t.Fatal(err)
	}
	st, err := jobboard.CreateStatus(c.profile, "To check")
	if err != nil {
		t.Fatal(err)
	}
	c.status = st.ID
	if _, err := profilerules.Create(c.profile, 60, c.status); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	dones := []<-chan struct{}{personamatchconsumer.Start(ctx), profilejobmatchconsumer.Start(ctx), jobmatchconsumer.Start(ctx)}
	t.Cleanup(func() {
		cancel()
		for _, d := range dones {
			<-d
		}
	})
	restore := jobs.SetDescriptionChangedPublisher(c.deliverChanged)
	t.Cleanup(restore)
	return c
}

func send(h http.HandlerFunc, topic, jobID, extra string) int {
	body := fmt.Sprintf(`{"id":"e","topic":%q,"payload":{"job_id":%q%s}}`, topic, jobID, extra)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	return rec.Code
}

// deliverChanged is what core does with "career.job.description_changed": hand it to every listener that does not miss it.
func (c *chain) deliverChanged(jobID, source string) {
	listeners := map[string]http.HandlerFunc{
		"persona": personamatchconsumer.EventHandler,
		"rules":   profilejobmatchconsumer.EventHandler,
		"match":   jobmatchconsumer.ChangedEventHandler,
	}
	for name, h := range listeners {
		c.mu.Lock()
		lost := c.lost[name]
		if !lost {
			c.received[name]++
		}
		c.mu.Unlock()
		if lost {
			continue
		}
		if code := send(h, "career.job.description_changed", jobID, fmt.Sprintf(`,"source":%q`, source)); code != http.StatusAccepted {
			c.t.Errorf("listener %s answered %d", name, code)
		}
	}
}

// deliverFound is what core does with "career.job.found".
func (c *chain) deliverFound(jobID string) {
	for name, h := range map[string]http.HandlerFunc{"persona": personamatchconsumer.EventHandler, "rules": profilejobmatchconsumer.EventHandler, "match": jobmatchconsumer.EventHandler} {
		if code := send(h, "career.job.found", jobID, `,"title":"T","company":"Acme","source":"crawled"`); code != http.StatusAccepted {
			c.t.Errorf("listener %s answered %d to found", name, code)
		}
	}
}

func (c *chain) loseFor(listeners ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lost = map[string]bool{}
	for _, l := range listeners {
		c.lost[l] = true
	}
}

func (c *chain) deliveredTo(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.received[name]
}

func chainExec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func chainWait(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(15 * time.Millisecond)
	}
}

// addJob inserts a job with a text and lets the three consumers meet it through the found event, then waits until all three
// have looked at it.
func (c *chain) addJob(id, text string) {
	c.t.Helper()
	chainExec(c.t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`,
		id, "https://x.example/"+id, text)
	c.deliverFound(id)
	c.waitSeen(id)
}

// waitSeen waits until the persona row, the job's match and the rules check of the job exist.
func (c *chain) waitSeen(id string) {
	c.t.Helper()
	chainWait(c.t, "the three consumers to look at "+id, func() bool {
		s := c.state(id)
		return s.persona >= 0 && s.match >= 0 && s.rulesChecked
	})
}

type jobState struct {
	persona      int // the persona's score, -1 without a row
	match        int // the job's match score, -1 without a row
	matchKind    string
	board        string // the status id on the profile's board, "" when absent
	rulesChecked bool
}

func (c *chain) state(id string) jobState {
	s := jobState{persona: -1, match: -1}
	if err := db.JobsDB.QueryRow(`SELECT score FROM job_persona_matches WHERE job_id = ? AND persona_id = ?`, id, c.persona).Scan(&s.persona); err != nil && err != sql.ErrNoRows {
		c.t.Fatal(err)
	}
	if err := db.JobsDB.QueryRow(`SELECT score, kind FROM job_matches WHERE job_id = ?`, id).Scan(&s.match, &s.matchKind); err != nil && err != sql.ErrNoRows {
		c.t.Fatal(err)
	}
	if err := db.JobsDB.QueryRow(`SELECT status_id FROM job_board_entries WHERE profile_id = ? AND job_id = ?`, c.profile, id).Scan(&s.board); err != nil && err != sql.ErrNoRows {
		c.t.Fatal(err)
	}
	var checked sql.NullString
	if err := db.JobsDB.QueryRow(`SELECT rules_checked_at FROM jobs WHERE id = ?`, id).Scan(&checked); err != nil {
		c.t.Fatal(err)
	}
	s.rulesChecked = checked.Valid
	return s
}

// detailCrawl is the real save a detail crawl (or a recrawl) ends with.
func (c *chain) detailCrawl(id, text string) {
	c.t.Helper()
	if updated, err := jobs.SaveJobDetailExtraction(id, map[string]string{"description": text}); err != nil || !updated {
		c.t.Fatalf("SaveJobDetailExtraction: %v %v", updated, err)
	}
}

// sweepAll runs one sweep of each of the three consumers and returns how many ids each queued.
func (c *chain) sweepAll() (persona, rules, match int) {
	c.t.Helper()
	var err error
	if persona, err = personamatchconsumer.Default.Sweep(); err != nil {
		c.t.Fatal(err)
	}
	if rules, err = profilejobmatchconsumer.Default.Sweep(); err != nil {
		c.t.Fatal(err)
	}
	if match, err = jobmatchconsumer.Default.Sweep(); err != nil {
		c.t.Fatal(err)
	}
	return
}

// settle lets time pass past the second of the change (a match made in that very second is offered once more, on purpose), sweeps,
// and repeats until a sweep finds nothing. It fails if that takes more than three rounds: the repair must converge.
func (c *chain) settle() {
	c.t.Helper()
	for round := 1; round <= 3; round++ {
		time.Sleep(1100 * time.Millisecond)
		p, r, m := c.sweepAll()
		if p == 0 && r == 0 && m == 0 {
			return
		}
		time.Sleep(400 * time.Millisecond) // let the workers handle what was queued
	}
	c.t.Fatal("the sweeps did not converge within three rounds")
}

func (c *chain) wantFits(id string) jobState {
	c.t.Helper()
	chainWait(c.t, id+" to follow its full text", func() bool {
		s := c.state(id)
		return s.persona >= 60 && s.match >= 60 && s.board == c.status
	})
	return c.state(id)
}

func (c *chain) wantStillOnTheSnippet(id string) {
	c.t.Helper()
	time.Sleep(400 * time.Millisecond)
	s := c.state(id)
	if s.persona >= 60 || s.match >= 60 || s.board != "" {
		c.t.Fatalf("%s moved although its events were lost: %+v", id, s)
	}
}

// ---------------------------------------------------------------------------------------------------------------

func TestTheFixtureIsWhatTheChainTestsAssume(t *testing.T) {
	c := newChain(t)
	c.addJob("j1", chainSnippet)
	s := c.state("j1")
	if s.persona < 0 || s.persona >= 60 || s.match < 0 || s.match >= 60 || s.board != "" || s.matchKind != "manual" {
		t.Fatalf("with the snippet the job must be judged, fit below 60 and not be on the board: %+v", s)
	}
}

// The text arrives and every event is delivered: all three stores follow.
func TestAChangedTextReachesThePersonaScoreTheMatchAndTheBoard(t *testing.T) {
	c := newChain(t)
	c.addJob("j1", chainSnippet)
	before := c.state("j1")
	c.detailCrawl("j1", chainFull)
	after := c.wantFits("j1")
	if after.persona <= before.persona || after.match <= before.match || after.matchKind != "manual" {
		t.Errorf("the scores must rise: %+v -> %+v", before, after)
	}
	for _, l := range []string{"persona", "rules", "match"} {
		if c.deliveredTo(l) != 1 {
			t.Errorf("listener %s got %d events, want exactly 1 (one write, one event)", l, c.deliveredTo(l))
		}
	}
	// the delivered events leave a same-second mark: one more handling at the next sweep, then nothing
	c.settle()
	if again := c.state("j1"); again.persona != after.persona || again.match != after.match || again.board != after.board {
		t.Errorf("the sweeps must not change a repaired job: %+v vs %+v", after, again)
	}
}

// Every event is lost: nothing moves; the sweepers repair all three to the same state a delivered event gives, and then stop offering.
func TestEveryEventLostIsRepairedBySweeping(t *testing.T) {
	c := newChain(t)
	c.addJob("delivered", chainSnippet)
	c.addJob("lost", chainSnippet)
	c.detailCrawl("delivered", chainFull)
	want := c.wantFits("delivered")

	c.loseFor("persona", "rules", "match")
	c.detailCrawl("lost", chainFull)
	c.wantStillOnTheSnippet("lost")

	time.Sleep(1100 * time.Millisecond)
	p, r, m := c.sweepAll()
	if p < 1 || r < 1 || m < 1 {
		t.Fatalf("each sweeper must offer the lost job: persona %d, rules %d, match %d", p, r, m)
	}
	got := c.wantFits("lost")
	if got.persona != want.persona || got.match != want.match || got.board != want.board || got.matchKind != want.matchKind {
		t.Errorf("a repaired job must end like a delivered one: %+v vs %+v", got, want)
	}
	c.settle()
}

// Only one listener missed its event: only that store is repaired, the others were already right and the sweepers do not redo them
// beyond the one same-second pass.
func TestOneListenerMissingItsEventIsRepairedAloneEachInTurn(t *testing.T) {
	for _, lost := range []string{"persona", "rules", "match"} {
		t.Run(lost, func(t *testing.T) {
			c := newChain(t)
			c.addJob("j1", chainSnippet)
			c.loseFor(lost)
			c.detailCrawl("j1", chainFull)
			chainWait(t, "the listeners that got the event", func() bool {
				s := c.state("j1")
				ok := true
				if lost != "persona" {
					ok = ok && s.persona >= 60
				}
				if lost != "match" {
					ok = ok && s.match >= 60
				}
				if lost != "rules" {
					ok = ok && s.board == c.status
				}
				return ok
			})
			s := c.state("j1")
			if (lost == "persona" && s.persona >= 60) || (lost == "match" && s.match >= 60) || (lost == "rules" && s.board != "") {
				t.Fatalf("the store of the listener that missed the event must still be on the snippet: %+v", s)
			}
			time.Sleep(1100 * time.Millisecond)
			c.sweepAll()
			c.wantFits("j1")
			c.settle()
		})
	}
}

// An AI result is never replaced, by the event or by the sweeper; the other stores still follow.
func TestAnAIResultSurvivesTheChangeAndTheSweeps(t *testing.T) {
	c := newChain(t)
	chainExec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('ai', 'https://x.example/ai', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, chainSnippet)
	chainExec(t, `INSERT INTO job_matches (job_id, profile_id, score, status, kind, conversation_id, updated_at) VALUES ('ai', ?, 7, 'completed', 'ai', 'conv-1', '2000-01-01 00:00:00')`, c.profile)
	chainExec(t, `INSERT INTO job_match_skills (job_id, skill) VALUES ('ai', 'by the AI')`)
	c.deliverFound("ai")
	chainWait(t, "the persona and rules look", func() bool { s := c.state("ai"); return s.persona >= 0 && s.rulesChecked })

	c.detailCrawl("ai", chainFull)
	chainWait(t, "persona and board to follow", func() bool { s := c.state("ai"); return s.persona >= 60 && s.board == c.status })
	c.loseFor("persona", "rules", "match") // and a second change that nobody hears of
	c.detailCrawl("ai", chainSnippet)
	time.Sleep(1100 * time.Millisecond)
	c.sweepAll()
	c.settle()

	var score int
	var kind, conv string
	if err := db.JobsDB.QueryRow(`SELECT score, kind, conversation_id FROM job_matches WHERE job_id = 'ai'`).Scan(&score, &kind, &conv); err != nil {
		t.Fatal(err)
	}
	var skill string
	_ = db.JobsDB.QueryRow(`SELECT skill FROM job_match_skills WHERE job_id = 'ai'`).Scan(&skill)
	if score != 7 || kind != "ai" || conv != "conv-1" || skill != "by the AI" {
		t.Errorf("the AI result must be untouched: %d %s %s %s", score, kind, conv, skill)
	}
}

// A hand-typed text is never replaced, so nothing is announced, nothing is stale, nothing moves.
func TestATypedTextChangesNothingAndNothingIsOffered(t *testing.T) {
	c := newChain(t)
	c.addJob("typed", chainSnippet)
	chainExec(t, `UPDATE jobs SET source = 'manual' WHERE id = 'typed'`)
	before := c.state("typed")
	c.detailCrawl("typed", chainFull)
	time.Sleep(400 * time.Millisecond)
	if after := c.state("typed"); after != before {
		t.Errorf("%+v -> %+v", before, after)
	}
	for _, l := range []string{"persona", "rules", "match"} {
		if c.deliveredTo(l) != 0 {
			t.Errorf("%s got an event for a text that did not change", l)
		}
	}
	time.Sleep(1100 * time.Millisecond)
	if p, r, m := c.sweepAll(); p != 0 || r != 0 || m != 0 {
		t.Errorf("nothing is stale: %d %d %d", p, r, m)
	}
}

// A listing refresh that brings the snippet back over a detail-crawled text is not announced; the matches made from the full text stay.
func TestAListingSnippetOverAFullTextIsNotAChange(t *testing.T) {
	c := newChain(t)
	chainExec(t, `INSERT INTO portals (id, name) VALUES ('p9', 'P')`)
	chainExec(t, `INSERT INTO portal_links (id, portal_id, url) VALUES ('l9', 'p9', 'https://x.example/list')`)
	id, created, _, err := jobs.SavePortalJob("l9", "https://x.example/job/9", "Backend Engineer (m/w/d)", "Acme", "Hamburg", chainSnippet, "")
	if err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	c.deliverFound(id)
	c.waitSeen(id)
	c.detailCrawl(id, chainFull)
	full := c.wantFits(id)
	got := c.deliveredTo("match")
	if _, _, _, err := jobs.SavePortalJob("l9", "https://x.example/job/9", "Backend Engineer (m/w/d)", "Acme", "Hamburg", chainSnippet, ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if c.deliveredTo("match") != got || c.state(id) != full {
		t.Errorf("a snippet over a full text must not announce or move anything: %d events, %+v vs %+v", c.deliveredTo("match")-got, c.state(id), full)
	}
}

// A job already on the board is not moved, one the person took off is not put back, by the event or by the sweeper.
func TestABoardIsOnlyEverAddedTo(t *testing.T) {
	c := newChain(t)
	other, err := jobboard.CreateStatus(c.profile, "Maybe")
	if err != nil {
		t.Fatal(err)
	}
	c.addJob("kept", chainSnippet)
	c.addJob("gone", chainSnippet)
	chainExec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES (?, 'kept', ?)`, c.profile, other.ID)
	chainExec(t, `INSERT INTO job_board_removals (profile_id, job_id) VALUES (?, 'gone')`, c.profile)
	c.detailCrawl("kept", chainFull)
	c.detailCrawl("gone", chainFull)
	chainWait(t, "scores to follow", func() bool { return c.state("kept").persona >= 60 && c.state("gone").persona >= 60 })
	c.settle()
	if got := c.state("kept").board; got != other.ID {
		t.Errorf("a job on a board is not moved: %q, want %q", got, other.ID)
	}
	if got := c.state("gone").board; got != "" {
		t.Errorf("a removed job is not put back: %q", got)
	}
}

// Duplicate deliveries and a found event after the change all end in the same state, with one board entry and one match row.
func TestRepeatedAndMixedDeliveriesEndInOneState(t *testing.T) {
	c := newChain(t)
	c.addJob("j1", chainSnippet)
	c.detailCrawl("j1", chainFull)
	want := c.wantFits("j1")
	for i := 0; i < 3; i++ {
		c.deliverChanged("j1", "detail_crawl")
	}
	c.deliverFound("j1")
	time.Sleep(600 * time.Millisecond)
	if got := c.state("j1"); got != want {
		t.Errorf("%+v vs %+v", got, want)
	}
	var boards, matches int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_board_entries WHERE job_id = 'j1'`).Scan(&boards)
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_matches WHERE job_id = 'j1'`).Scan(&matches)
	if boards != 1 || matches != 1 {
		t.Errorf("one board entry and one match row expected, got %d and %d", boards, matches)
	}
}

// A detail crawl of many jobs at once, with every event lost: one round of sweeps repairs them all.
func TestAWholeCrawlOfLostEventsIsRepairedByOneRoundOfSweeps(t *testing.T) {
	c := newChain(t)
	const n = 60
	for i := 0; i < n; i++ {
		c.addJob(fmt.Sprintf("b%02d", i), chainSnippet)
	}
	c.loseFor("persona", "rules", "match")
	for i := 0; i < n; i++ {
		c.detailCrawl(fmt.Sprintf("b%02d", i), chainFull)
	}
	time.Sleep(1100 * time.Millisecond)
	if p, r, m := c.sweepAll(); p != n || r != n || m != n {
		t.Fatalf("every job must be offered once by each sweeper: %d %d %d, want %d", p, r, m, n)
	}
	for i := 0; i < n; i++ {
		c.wantFits(fmt.Sprintf("b%02d", i))
	}
	c.settle()
}

// An event for a job that is gone must not stop the workers: the next job is still handled.
func TestAnEventForAJobThatIsGoneDoesNotStopTheWorkers(t *testing.T) {
	c := newChain(t)
	c.addJob("doomed", chainSnippet)
	c.addJob("next", chainSnippet)
	c.detailCrawl("doomed", chainFull)
	chainExec(t, `DELETE FROM jobs WHERE id = 'doomed'`)
	c.deliverChanged("doomed", "detail_crawl")
	c.detailCrawl("next", chainFull)
	c.wantFits("next")
}
