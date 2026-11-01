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
	old := unmatchedJobIDs
	unmatchedJobIDs = fn
	t.Cleanup(func() { unmatchedJobIDs = old })
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
	waitFor(t, "four sweeps", func() bool { return atomic.LoadInt32(&calls) >= 4 })
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
