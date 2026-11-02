package profilejobmatchconsumer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

// Step 164: the catch-up for lost description_changed events, and the marks that tell the sweeper what this consumer has seen.

var (
	markedMu sync.Mutex
	marked   [][]string
)

// useMarks replaces the recording of checked jobs with a recorder, so a test with a fake batch needs no database.
func useMarks(t *testing.T) {
	t.Helper()
	markedMu.Lock()
	marked = nil
	markedMu.Unlock()
	old := markChecked
	markChecked = func(ids []string) error {
		markedMu.Lock()
		defer markedMu.Unlock()
		marked = append(marked, append([]string(nil), ids...))
		return nil
	}
	t.Cleanup(func() { markChecked = old })
}

func markedNow() [][]string {
	markedMu.Lock()
	defer markedMu.Unlock()
	return append([][]string(nil), marked...)
}

func useToCheck(t *testing.T, fn func(limit int) ([]string, error)) {
	t.Helper()
	old := jobsToCheck
	jobsToCheck = fn
	t.Cleanup(func() { jobsToCheck = old })
}

func drain(c *Consumer) []string {
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

func TestASweepQueuesTheJobsWhoseTextChanged(t *testing.T) {
	var asked int
	useToCheck(t, func(limit int) ([]string, error) { asked = limit; return []string{"a", "b", "c"}, nil })
	c := New(10)
	n, err := c.Sweep()
	if err != nil || n != 3 || !slices.Equal(drain(c), []string{"a", "b", "c"}) {
		t.Fatalf("queued %d, %v", n, err)
	}
	if asked != 200 || SweepLimit != 200 {
		t.Errorf("asked %d, SweepLimit %d, want 200", asked, SweepLimit)
	}
}

func TestASweepStopsWhenTheQueueIsFullAndReportsALookupFailure(t *testing.T) {
	useToCheck(t, func(int) ([]string, error) { return []string{"a", "b", "c"}, nil })
	c := New(2)
	if n, err := c.Sweep(); err != nil || n != 2 || !slices.Equal(drain(c), []string{"a", "b"}) {
		t.Fatalf("%d %v", n, err)
	}
	useToCheck(t, func(int) ([]string, error) { return []string{"a"}, errors.New("db gone") })
	c = New(2)
	if n, err := c.Sweep(); err == nil || n != 0 || len(c.queue) != 0 {
		t.Errorf("%d %v", n, err)
	}
}

func TestTheSweeperRunsOnATimerSurvivesAPanicAndStops(t *testing.T) {
	oldDelay, oldInterval := sweepStartDelay, sweepInterval
	sweepStartDelay, sweepInterval = 150*time.Millisecond, 30*time.Millisecond
	defer func() { sweepStartDelay, sweepInterval = oldDelay, oldInterval }()
	var calls int32
	useToCheck(t, func(int) ([]string, error) {
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
	waitFor(t, "the sweeps after the panic and the error to queue", func() bool { return len(c.queue) >= 2 })
	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the sweeper did not stop")
	}
}

// ---- the marks ----

func TestAWorkerPassMarksEveryJobItLookedAt(t *testing.T) {
	watch(t, func(ids []string) ([]Outcome, error) {
		return []Outcome{{JobID: "a", ProfileID: "p", Status: Added}, {JobID: "b", ProfileID: "p", Status: Below}}, nil
	})
	c := New(10)
	c.process([]string{"a", "b", "c"}) // c has no outcome at all: a profile could not judge it, or no rule exists
	if got := markedNow(); len(got) != 1 || !slices.Equal(got[0], []string{"a", "b", "c"}) {
		t.Errorf("marked %v", got)
	}
}

func TestAJobThatFailedIsNotMarkedSoTheSweeperOffersItAgain(t *testing.T) {
	watch(t, func(ids []string) ([]Outcome, error) {
		return []Outcome{{JobID: "a", ProfileID: "p", Status: Added}, {JobID: "b", ProfileID: "p", Status: Unjudged, Err: errors.New("lookup failed")}}, nil
	})
	c := New(10)
	c.process([]string{"a", "b"})
	if got := markedNow(); len(got) != 1 || !slices.Equal(got[0], []string{"a"}) {
		t.Errorf("marked %v, want only a", got)
	}
}

func TestABatchThatCannotStartMarksNothing(t *testing.T) {
	watch(t, func([]string) ([]Outcome, error) { return nil, errors.New("rules unreadable") })
	c := New(10)
	c.process([]string{"a", "b"})
	if got := markedNow(); len(got) != 0 {
		t.Errorf("marked %v", got)
	}
}

func TestAFailingMarkDoesNotStopTheWorker(t *testing.T) {
	b := watch(t, nil)
	old := markChecked
	markChecked = func([]string) error { return errors.New("db locked") }
	t.Cleanup(func() { markChecked = old })
	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)
	c.queue <- "a"
	waitFor(t, "first pass", func() bool { return len(b.sizes()) == 1 })
	c.queue <- "b"
	waitFor(t, "second pass", func() bool { return len(b.sizes()) == 2 })
	cancel()
	<-done
}

// With the real batch and the real marks: a job whose text changed is offered until the consumer has looked at it, then it is not.
func TestARealPassEndsTheOfferOfAChangedJob(t *testing.T) {
	setup(t)
	addJob(t, "j1")
	addJob(t, "j2")
	exec(t, `UPDATE jobs SET description_changed_at = '2026-01-02 10:00:00' WHERE id IN ('j1', 'j2')`)
	if got, _ := jobs.JobsNeedingRulesCheck(10); len(got) != 2 {
		t.Fatalf("offered before the pass: %v", got)
	}
	c := New(10)
	c.process([]string{"j1"}) // no rule anywhere: nothing to do, and that is a look too
	got, _ := jobs.JobsNeedingRulesCheck(10)
	if !slices.Equal(got, []string{"j2"}) {
		t.Errorf("only the job that was not looked at is still offered: %v", got)
	}
	var at *string
	_ = db.JobsDB.QueryRow(`SELECT rules_checked_at FROM jobs WHERE id = 'j1'`).Scan(&at)
	if at == nil || *at <= "2026-01-02 10:00:00" {
		t.Errorf("the mark: %v", at)
	}
}

// A lost event, end to end with a real profile, rule and scorer: the sweep finds the job, the worker puts it on the board, the
// job is not offered again.
func TestALostEventIsRepairedByTheSweepAndTheJobReachesTheBoard(t *testing.T) {
	setup(t)
	fit := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	sFit := customStatus(t, fit, "To check")
	rule(t, fit, 60, sFit)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description, description_changed_at) VALUES ('j1', 'https://x.example/j1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?, '2026-01-02 10:00:00')`, jobDesc)

	c := New(10)
	n, err := c.Sweep()
	if err != nil || n != 1 {
		t.Fatalf("the job must be offered: %d %v", n, err)
	}
	c.process(drain(c))
	if got := onBoard(t, fit, "j1"); got != sFit {
		t.Errorf("the job must reach the board as %q, got %q", sFit, got)
	}
	if n, err := c.Sweep(); err != nil || n != 0 {
		t.Errorf("once looked at it is not offered again: %d %v", n, err)
	}
}

// main.go starts the sweeper next to the worker.
func TestMainStartsTheSweeper(t *testing.T) {
	raw, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "profilejobmatchconsumer.StartSweeper(context.Background())") {
		t.Error("main.go does not start the profile rules sweeper")
	}
}
