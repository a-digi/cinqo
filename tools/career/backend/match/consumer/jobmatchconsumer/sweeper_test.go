package jobmatchconsumer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func useUnmatched(t *testing.T, fn func(limit int) ([]string, error)) {
	t.Helper()
	old, oldStale := unmatchedJobIDs, staleMatchJobIDs
	unmatchedJobIDs = fn
	staleMatchJobIDs = func(int) ([]string, error) { return nil, nil }
	t.Cleanup(func() { unmatchedJobIDs, staleMatchJobIDs = old, oldStale })
}

func drainQueue(c *Consumer) []string {
	var out []string
	for {
		select {
		case id := <-c.queue:
			out = append(out, id)
		default:
			return out
		}
	}
}

func TestASweepQueuesTheUnmatchedJobsInOrder(t *testing.T) {
	var asked int
	useUnmatched(t, func(limit int) ([]string, error) { asked = limit; return []string{"new", "mid", "old"}, nil })
	c := New(10)
	n, err := c.Sweep()
	if err != nil || n != 3 || !slices.Equal(drainQueue(c), []string{"new", "mid", "old"}) {
		t.Fatalf("queued %d, %v", n, err)
	}
	if asked != 200 || SweepLimit != 200 {
		t.Errorf("asked for %d, SweepLimit %d, want 200", asked, SweepLimit)
	}
}

// A queue without room stops the sweep where it is: what fit is queued, the rest waits for the next sweep.
func TestASweepStopsWhenTheQueueIsFull(t *testing.T) {
	useUnmatched(t, func(int) ([]string, error) { return []string{"a", "b", "c", "d"}, nil })
	c := New(2)
	n, err := c.Sweep()
	if err != nil || n != 2 || !slices.Equal(drainQueue(c), []string{"a", "b"}) {
		t.Fatalf("queued %d, %v", n, err)
	}
	// an already full queue takes nothing
	c = New(1)
	c.queue <- "event"
	if n, err := c.Sweep(); err != nil || n != 0 {
		t.Errorf("full queue: %d, %v", n, err)
	}
}

func TestASweepWithNothingToDoQueuesNothing(t *testing.T) {
	useUnmatched(t, func(int) ([]string, error) { return nil, nil })
	c := New(10)
	if n, err := c.Sweep(); err != nil || n != 0 || len(c.queue) != 0 {
		t.Errorf("%d, %v", n, err)
	}
}

func TestASweepThatCannotLookReportsItAndQueuesNothing(t *testing.T) {
	useUnmatched(t, func(int) ([]string, error) { return []string{"a"}, errors.New("db gone") })
	c := New(10)
	if n, err := c.Sweep(); err == nil || n != 0 || len(c.queue) != 0 {
		t.Errorf("%d, %v", n, err)
	}
}

// The timer: the first sweep waits for the start delay, the next ones follow at the interval, a failing or panicking one
// does not stop the sweeper, and cancelling stops it.
func TestTheSweeperRunsOnATimerAndStops(t *testing.T) {
	oldDelay, oldInterval := sweepStartDelay, sweepInterval
	sweepStartDelay, sweepInterval = 150*time.Millisecond, 30*time.Millisecond
	defer func() { sweepStartDelay, sweepInterval = oldDelay, oldInterval }()

	var calls int32
	useUnmatched(t, func(int) ([]string, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			panic("boom")
		case 2:
			return nil, errors.New("temporary")
		}
		return []string{fmt.Sprintf("job-%d", atomic.LoadInt32(&calls))}, nil
	})
	c := New(100)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := c.StartSweeper(ctx)

	time.Sleep(60 * time.Millisecond)
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Errorf("a sweep ran %d time(s) before the start delay", n)
	}
	waitFor(t, "four sweeps, two of them queued", func() bool { return atomic.LoadInt32(&calls) >= 4 && len(c.queue) >= 2 })
	if len(c.queue) < 2 {
		t.Errorf("sweeps after the panic and the error did not queue: %d waiting", len(c.queue))
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the sweeper did not stop")
	}
	after := atomic.LoadInt32(&calls)
	time.Sleep(120 * time.Millisecond)
	if atomic.LoadInt32(&calls) != after {
		t.Error("a stopped sweeper kept sweeping")
	}
}

func TestACancelledSweeperNeverSweepsIfStoppedBeforeTheDelay(t *testing.T) {
	oldDelay := sweepStartDelay
	sweepStartDelay = 100 * time.Millisecond
	defer func() { sweepStartDelay = oldDelay }()
	var calls int32
	useUnmatched(t, func(int) ([]string, error) { atomic.AddInt32(&calls, 1); return nil, nil })
	ctx, cancel := context.WithCancel(context.Background())
	stopped := New(10).StartSweeper(ctx)
	cancel()
	<-stopped
	time.Sleep(150 * time.Millisecond)
	if atomic.LoadInt32(&calls) != 0 {
		t.Error("swept after being cancelled")
	}
}

// The whole way with the real tables and the real matcher: jobs that never had an event, one with an AI result and one
// that nobody can judge yet; the sweep and the worker match exactly the ones that can be matched, nothing with a row is
// touched, and once a profile can judge, the next sweep matches the rest.
func TestTheSweepMatchesWhatWasMissedAndLeavesTheRest(t *testing.T) {
	setup(t)
	desc := "Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."
	for i, id := range []string{"missed1", "missed2", "hasAI"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company, location, description, crawled_at) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?, ?)`,
			id, "https://x.example/"+id, desc, fmt.Sprintf("2026-06-0%d 00:00:00", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('hasAI', 'p', 5, 'completed', 'ai')`); err != nil {
		t.Fatal(err)
	}
	c := New(50)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	defer func() { cancel(); <-done }()

	matches := func() map[string]string {
		out := map[string]string{}
		rows, err := db.JobsDB.Query(`SELECT job_id, kind FROM job_matches`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, kind string
			_ = rows.Scan(&id, &kind)
			out[id] = kind
		}
		return out
	}

	// no profile yet: the sweep queues the two missed jobs, the matcher cannot judge them, nothing is stored
	if n, err := c.Sweep(); err != nil || n != 2 {
		t.Fatalf("first sweep queued %d, %v", n, err)
	}
	time.Sleep(150 * time.Millisecond)
	if got := matches(); len(got) != 1 || got["hasAI"] != "ai" {
		t.Fatalf("with no profile: %v", got)
	}

	// a profile that can judge appears: the next sweep matches both, and the AI result is untouched
	pid, err := profile.CreateProfile("Fit", "Test")
	if err != nil {
		t.Fatal(err)
	}
	per, err := persona.CreatePersona(pid, "Fit persona", "")
	if err != nil {
		t.Fatal(err)
	}
	h, titles, loc := "Backend Engineer", "Software Engineer, Backend Developer", "Hamburg"
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: per, Headline: &h, DesiredTitles: &titles, Location: &loc, DesiredLocations: &loc}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Go", "Docker", "Kubernetes", "PostgreSQL"} {
		if err := persona.AddCareerSkill(per, s); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := c.Sweep(); err != nil || n != 2 {
		t.Fatalf("second sweep queued %d, %v", n, err)
	}
	waitFor(t, "the two matches", func() bool { return len(matches()) == 3 })
	got := matches()
	if got["missed1"] != "manual" || got["missed2"] != "manual" || got["hasAI"] != "ai" {
		t.Errorf("%v", got)
	}
	// converged: nothing left to offer
	if n, err := c.Sweep(); err != nil || n != 0 {
		t.Errorf("third sweep queued %d, %v", n, err)
	}
}

// ---- the catch-up for matches older than their job's text (step 164) ----

func useStale(t *testing.T, unmatched, stale func(limit int) ([]string, error)) {
	t.Helper()
	old, oldStale := unmatchedJobIDs, staleMatchJobIDs
	unmatchedJobIDs, staleMatchJobIDs = unmatched, stale
	t.Cleanup(func() { unmatchedJobIDs, staleMatchJobIDs = old, oldStale })
}

func drainChanged(c *Consumer) []string {
	var out []string
	for {
		select {
		case id := <-c.changed:
			out = append(out, id)
		default:
			return out
		}
	}
}

func TestASweepQueuesUnmatchedJobsAsNewAndStaleOnesAsChanged(t *testing.T) {
	var askedStale int
	useStale(t,
		func(int) ([]string, error) { return []string{"u1", "u2"}, nil },
		func(limit int) ([]string, error) { askedStale = limit; return []string{"s1", "s2", "s3"}, nil })
	c := New(10)
	n, err := c.Sweep()
	if err != nil || n != 5 {
		t.Fatalf("queued %d, %v", n, err)
	}
	if got := drainQueue(c); !slices.Equal(got, []string{"u1", "u2"}) {
		t.Errorf("new queue %v", got)
	}
	if got := drainChanged(c); !slices.Equal(got, []string{"s1", "s2", "s3"}) {
		t.Errorf("changed queue %v", got)
	}
	if askedStale != SweepLimit {
		t.Errorf("asked %d, want %d", askedStale, SweepLimit)
	}
}

func TestAFailingStaleLookupKeepsTheUnmatchedOnes(t *testing.T) {
	useStale(t,
		func(int) ([]string, error) { return []string{"u1"}, nil },
		func(int) ([]string, error) { return nil, errors.New("db gone") })
	c := New(10)
	if n, err := c.Sweep(); err != nil || n != 1 || len(c.queue) != 1 || len(c.changed) != 0 {
		t.Fatalf("%d, %v", n, err)
	}
}

func TestAFullChangedQueueStopsTheSweep(t *testing.T) {
	useStale(t,
		func(int) ([]string, error) { return nil, nil },
		func(int) ([]string, error) { return []string{"s1", "s2", "s3"}, nil })
	c := New(2)
	n, err := c.Sweep()
	if err != nil || n != 2 || !slices.Equal(drainChanged(c), []string{"s1", "s2"}) {
		t.Fatalf("queued %d, %v", n, err)
	}
}

// With the real lookups and the real re-matcher: a lost event is repaired by one sweep and the job is not offered again.
func TestALostEventIsRepairedBySweepingOnce(t *testing.T) {
	setup(t)
	profile, _ := seedProfileForSweep(t)
	execSQL(t, `INSERT INTO jobs (id, source_url, title, company, location, description, description_changed_at) VALUES ('j1', 'https://x/1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?, '2026-01-02 10:00:00')`, sweepJobDesc)
	execSQL(t, `INSERT INTO job_matches (job_id, profile_id, score, status, kind, updated_at) VALUES ('j1', ?, 1, 'completed', 'manual', '2026-01-02 09:00:00')`, profile)

	c := New(10)
	n, err := c.Sweep()
	if err != nil || n != 1 || len(c.changed) != 1 {
		t.Fatalf("the stale match must be offered once: %d %v", n, err)
	}
	c.process(batch{changed: drainChanged(c)}) // what the worker does with it, with the real re-matcher
	var score int
	var at string
	if err := db.JobsDB.QueryRow(`SELECT score, updated_at FROM job_matches WHERE job_id = 'j1'`).Scan(&score, &at); err != nil {
		t.Fatal(err)
	}
	if score == 1 || at <= "2026-01-02 10:00:00" {
		t.Errorf("the match must be made again from the text: score %d at %s", score, at)
	}
	if n, err := c.Sweep(); err != nil || n != 0 {
		t.Errorf("once repaired it is not offered again: %d %v", n, err)
	}
}

func execSQL(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

const sweepJobDesc = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
	"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."

// seedProfileForSweep makes a profile with one persona that fits sweepJobDesc.
func seedProfileForSweep(t *testing.T) (profileID, personaID string) {
	t.Helper()
	profileID, err := profile.CreateProfile("Fit", "Test")
	if err != nil {
		t.Fatal(err)
	}
	personaID, err = persona.CreatePersona(profileID, "Fit persona", "")
	if err != nil {
		t.Fatal(err)
	}
	h, titles, loc := "Backend Engineer", "Software Engineer, Backend Developer", "Hamburg"
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: personaID, Headline: &h, DesiredTitles: &titles, Location: &loc, DesiredLocations: &loc}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Go", "Docker", "Kubernetes", "PostgreSQL"} {
		if err := persona.AddCareerSkill(personaID, s); err != nil {
			t.Fatal(err)
		}
	}
	return profileID, personaID
}
