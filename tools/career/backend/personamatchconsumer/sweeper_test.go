package personamatchconsumer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"career-tool-backend/personamatch"
)

func useWithout(t *testing.T, fn func(limit int) ([]string, error)) {
	t.Helper()
	old := jobsWithoutRows
	jobsWithoutRows = fn
	t.Cleanup(func() { jobsWithoutRows = old })
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

func TestASweepQueuesTheJobsWithoutRowsInOrder(t *testing.T) {
	var asked int
	useWithout(t, func(limit int) ([]string, error) { asked = limit; return []string{"new", "mid", "old"}, nil })
	c := New(10)
	n, err := c.Sweep()
	if err != nil || n != 3 || !slices.Equal(drainQueue(c), []string{"new", "mid", "old"}) {
		t.Fatalf("queued %d, %v", n, err)
	}
	if asked != 500 || SweepLimit != 500 {
		t.Errorf("asked for %d, SweepLimit %d, want 500", asked, SweepLimit)
	}
}

func TestASweepStopsWhenTheQueueIsFull(t *testing.T) {
	useWithout(t, func(int) ([]string, error) { return []string{"a", "b", "c", "d"}, nil })
	c := New(2)
	n, err := c.Sweep()
	if err != nil || n != 2 || !slices.Equal(drainQueue(c), []string{"a", "b"}) {
		t.Fatalf("queued %d, %v", n, err)
	}
	c = New(1)
	c.queue <- "event"
	if n, err := c.Sweep(); err != nil || n != 0 {
		t.Errorf("full queue: %d, %v", n, err)
	}
}

func TestASweepWithNothingToDoQueuesNothing(t *testing.T) {
	useWithout(t, func(int) ([]string, error) { return nil, nil })
	c := New(10)
	if n, err := c.Sweep(); err != nil || n != 0 || len(c.queue) != 0 {
		t.Errorf("%d, %v", n, err)
	}
}

func TestASweepThatCannotLookReportsItAndQueuesNothing(t *testing.T) {
	useWithout(t, func(int) ([]string, error) { return []string{"a"}, errors.New("db gone") })
	c := New(10)
	if n, err := c.Sweep(); err == nil || n != 0 || len(c.queue) != 0 {
		t.Errorf("%d, %v", n, err)
	}
}

// With the real lookup: only jobs without any row are offered, newest first.
func TestTheRealSweepOffersOnlyJobsWithoutRows(t *testing.T) {
	setup(t)
	exec(t, `INSERT INTO jobs (id, source_url, title, crawled_at) VALUES ('old', 'https://x/1', 'T', '2026-01-01 10:00:00')`)
	exec(t, `INSERT INTO jobs (id, source_url, title, crawled_at) VALUES ('new', 'https://x/2', 'T', '2026-06-01 10:00:00')`)
	exec(t, `INSERT INTO jobs (id, source_url, title, crawled_at) VALUES ('done', 'https://x/3', 'T', '2026-07-01 10:00:00')`)
	if err := personamatch.Replace("done", []personamatch.Row{{PersonaID: "pa", ProfileID: "A", Score: 40}}); err != nil {
		t.Fatal(err)
	}
	c := New(10)
	n, err := c.Sweep()
	if err != nil || n != 2 || !slices.Equal(drainQueue(c), []string{"new", "old"}) {
		t.Fatalf("queued %d, %v", n, err)
	}
}

func TestTheSweeperRunsOnATimerAndStops(t *testing.T) {
	oldDelay, oldInterval := sweepStartDelay, sweepInterval
	sweepStartDelay, sweepInterval = 150*time.Millisecond, 30*time.Millisecond
	defer func() { sweepStartDelay, sweepInterval = oldDelay, oldInterval }()

	var calls int32
	useWithout(t, func(int) ([]string, error) {
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
	// a sweep counts as called before it has queued, so wait for the queue instead of checking at once
	waitFor(t, "the sweeps after the panic and the error to queue", func() bool { return len(c.queue) >= 2 })
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

func TestTheStartDelayAndIntervalAreThirtySecondsAndTenMinutes(t *testing.T) {
	if sweepStartDelay != 30*time.Second || sweepInterval != 10*time.Minute {
		t.Errorf("delay %v, interval %v", sweepStartDelay, sweepInterval)
	}
}
