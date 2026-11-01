package jobrecrawlconsumer

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"career-tool-backend/jobrecrawl"
)

func setRow(t *testing.T, id, state string, age time.Duration) {
	t.Helper()
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES (?, 'b', ?, ?)`, id, time.Now().UTC().Add(-age).Format(time.RFC3339), state)
}

func TestSweepTakesOnlyOldQueuedRequests(t *testing.T) {
	setup(t)
	for _, id := range []string{"old-queued", "new-queued", "old-running", "old-done", "old-cancelled"} {
		addJob(t, id, "crawled", "x")
	}
	setRow(t, "old-queued", "queued", time.Hour)
	setRow(t, "new-queued", "queued", time.Second)
	setRow(t, "old-running", "running", time.Hour)
	setRow(t, "old-done", "done", time.Hour)
	setRow(t, "old-cancelled", "cancelled", time.Hour)
	c := New(10)
	n, err := c.Sweep()
	if err != nil || n != 1 {
		t.Fatalf("queued %d: %v", n, err)
	}
	if id := <-c.queue; id != "old-queued" {
		t.Errorf("queued %q", id)
	}
}

func TestSweepDoesNotQueueAnIdTwice(t *testing.T) {
	setup(t)
	addJob(t, "a", "crawled", "x")
	setRow(t, "a", "queued", time.Hour)
	c := New(10)
	if n, _ := c.Sweep(); n != 1 {
		t.Fatalf("first sweep %d", n)
	}
	if n, _ := c.Sweep(); n != 0 || len(c.queue) != 1 {
		t.Errorf("the second sweep queued %d (queue %d)", n, len(c.queue))
	}
}

func TestSweepStopsWhenTheQueueIsFullAndTheNextOneContinues(t *testing.T) {
	setup(t)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("j%d", i)
		addJob(t, id, "crawled", "x")
		setRow(t, id, "queued", time.Hour-time.Duration(i)*time.Minute)
	}
	c := New(3)
	n, err := c.Sweep()
	if err != nil || n != 3 {
		t.Fatalf("queued %d: %v", n, err)
	}
	for i := 0; i < 3; i++ {
		id := <-c.queue
		c.take(id)
	}
	if n, _ := c.Sweep(); n != 3 { // the three taken ones are queued rows still, and the two that did not fit
		t.Errorf("next sweep %d", n)
	}
}

func TestSweepIsLimitedAndOldestFirst(t *testing.T) {
	setup(t)
	oldDue := dueQueued
	var gotAge time.Duration
	var gotLimit int
	dueQueued = func(age time.Duration, limit int) ([]string, error) {
		gotAge, gotLimit = age, limit
		return []string{"x"}, nil
	}
	t.Cleanup(func() { dueQueued = oldDue })
	c := New(10)
	if _, err := c.Sweep(); err != nil {
		t.Fatal(err)
	}
	if gotAge != sweepAge || gotLimit != SweepLimit {
		t.Errorf("age %v limit %d", gotAge, gotLimit)
	}
	// and the table query itself returns the oldest first
	for i, age := range []time.Duration{time.Hour, 3 * time.Hour, 2 * time.Hour} {
		id := fmt.Sprintf("o%d", i)
		addJob(t, id, "crawled", "x")
		setRow(t, id, "queued", age)
	}
	ids, err := jobrecrawl.DueQueued(time.Minute, 2)
	if err != nil || len(ids) != 2 || ids[0] != "o1" || ids[1] != "o2" {
		t.Errorf("%v %v", ids, err)
	}
}

func TestSweepErrorIsReturned(t *testing.T) {
	oldDue := dueQueued
	dueQueued = func(time.Duration, int) ([]string, error) { return nil, errors.New("db gone") }
	t.Cleanup(func() { dueQueued = oldDue })
	if _, err := New(1).Sweep(); err == nil {
		t.Error("expected the error")
	}
}

// The whole catch-up: the events never arrived, the sweeper finds the waiting requests and the worker crawls them.
func TestTheSweeperAndTheWorkerCatchUpMissedEvents(t *testing.T) {
	fb := setup(t)
	for _, id := range []string{"a", "b"} {
		fb.pages[addJob(t, id, "crawled", "old")] = "new " + id
		setRow(t, id, "queued", time.Hour)
	}
	oldDelay, oldInterval := sweepStartDelay, sweepInterval
	sweepStartDelay, sweepInterval = 20*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { sweepStartDelay, sweepInterval = oldDelay, oldInterval })

	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	worker := c.Start(ctx)
	sweeper := c.StartSweeper(ctx)
	waitFor(t, "both crawled", func() bool { return requestOf(t, "a").state == "done" && requestOf(t, "b").state == "done" })
	cancel()
	<-worker
	<-sweeper
	for _, id := range []string{"a", "b"} {
		if got := requestOf(t, id); got.outcome != jobrecrawl.OutcomeRefreshed || descriptionOf(t, id) != "new "+id {
			t.Errorf("%s: %+v", id, got)
		}
	}
	if fb.callCount() != 2 {
		t.Errorf("each job crawled once: %d", fb.callCount())
	}
}

// A job that waits for a session is offered again by a later sweep and crawled when a session appears.
func TestAWaitingJobIsCrawledOnceASessionAppears(t *testing.T) {
	fb := setup(t)
	fb.pages[addJob(t, "a", "crawled", "old")] = "new"
	setRow(t, "a", "queued", time.Hour)
	have := false
	cachedToken = func() (string, bool) {
		if have {
			return "tok", true
		}
		return "", false
	}
	oldDelay, oldInterval := sweepStartDelay, sweepInterval
	sweepStartDelay, sweepInterval = 20*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { sweepStartDelay, sweepInterval = oldDelay, oldInterval })

	c := New(10)
	ctx, cancel := context.WithCancel(context.Background())
	worker := c.Start(ctx)
	sweeper := c.StartSweeper(ctx)
	waitFor(t, "waiting", func() bool { r := requestOf(t, "a"); return r.state == "queued" && r.reason == WaitingForSession })
	if fb.callCount() != 0 {
		t.Error("no crawl yet")
	}
	have = true
	waitFor(t, "crawled", func() bool { return requestOf(t, "a").state == "done" })
	cancel()
	<-worker
	<-sweeper
	if requestOf(t, "a").outcome != jobrecrawl.OutcomeRefreshed {
		t.Errorf("%+v", requestOf(t, "a"))
	}
}

func TestASweepThatPanicsDoesNotStopTheSweeper(t *testing.T) {
	oldDue := dueQueued
	var calls atomic.Int32
	dueQueued = func(time.Duration, int) ([]string, error) {
		if calls.Add(1) == 1 {
			panic("boom")
		}
		return nil, nil
	}
	oldDelay, oldInterval := sweepStartDelay, sweepInterval
	sweepStartDelay, sweepInterval = 10*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { dueQueued, sweepStartDelay, sweepInterval = oldDue, oldDelay, oldInterval })
	ctx, cancel := context.WithCancel(context.Background())
	done := New(1).StartSweeper(ctx)
	waitFor(t, "a second sweep", func() bool { return calls.Load() >= 2 })
	cancel()
	<-done
}
