package jobcleanup

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// fixedNow is 2026-06-30 12:00:00 UTC. save() stores settings at a chosen time so the "last saved" clock is known.
func save(t *testing.T, at time.Time, s Settings) {
	t.Helper()
	old := now
	now = func() time.Time { return at }
	defer func() { now = old }()
	if s.OlderThanDays == 0 {
		s.OlderThanDays = 30
	}
	if s.MinMatchPercent == 0 {
		s.MinMatchPercent = 60
	}
	if s.IntervalHours == 0 {
		s.IntervalHours = 24
	}
	if _, err := UpdateSettings(s); err != nil {
		t.Fatal(err)
	}
}

var (
	savedEarlier = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) // long before fixedNow
	clock        = time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
)

func autoOn() Settings {
	return Settings{Enabled: true, OlderEnabled: true}
}

func TestTheTickDoesNothingWhenTheAutomaticRunIsOff(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "old", old30, "crawled")
	save(t, savedEarlier, Settings{OlderEnabled: true, LowMatchEnabled: true}) // enabled = false
	checkAndRunTick(context.Background())
	if !exists(t, "old") {
		t.Error("deleted while disabled")
	}
	if s, _ := GetSettings(); s.LastRunAt != "" {
		t.Errorf("stamped %q", s.LastRunAt)
	}
}

func TestTheTickDoesNothingWithoutARuleOn(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "old", old30, "crawled")
	save(t, savedEarlier, Settings{Enabled: true})
	checkAndRunTick(context.Background())
	if !exists(t, "old") {
		t.Error("deleted with both rules off")
	}
}

func TestTheTickRunsWhenDue(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "old", old30, "crawled")
	addJob(t, "young", young, "crawled")
	save(t, savedEarlier, autoOn())
	checkAndRunTick(context.Background())
	if exists(t, "old") || !exists(t, "young") {
		t.Error("the due run did not delete exactly the old job")
	}
	s, _ := GetSettings()
	if s.LastRunAt != "2026-06-30 12:00:00" || s.LastRunSummary != "older than 30 days: 1 deleted" {
		t.Errorf("%+v", s)
	}
}

// After a run the next one waits interval_hours: a minute before nothing happens, at the boundary it runs.
func TestTheTickWaitsForTheInterval(t *testing.T) {
	setup(t)
	old := now
	defer func() { now = old }()
	save(t, savedEarlier, Settings{Enabled: true, OlderEnabled: true, IntervalHours: 6})
	run := func(at time.Time) {
		now = func() time.Time { return at }
		checkAndRunTick(context.Background())
	}
	addJob(t, "a", "2026-01-01 00:00:00", "crawled")
	run(clock) // first run: deletes a, stamps 12:00
	if exists(t, "a") {
		t.Fatal("first run did not delete")
	}
	addJob(t, "b", "2026-01-01 00:00:00", "crawled")
	run(clock.Add(6*time.Hour - time.Second))
	if !exists(t, "b") {
		t.Fatal("ran before the interval was over")
	}
	run(clock.Add(6 * time.Hour))
	if exists(t, "b") {
		t.Fatal("did not run at the interval")
	}
}

// Switching the run on (or changing a setting) postpones the first run by a full interval, even if no run ever
// happened.
func TestASaveDelaysTheNextRunByTheInterval(t *testing.T) {
	setup(t)
	old := now
	defer func() { now = old }()
	addJob(t, "old", "2026-01-01 00:00:00", "crawled")
	save(t, clock, Settings{Enabled: true, OlderEnabled: true, IntervalHours: 24})
	now = func() time.Time { return clock.Add(24*time.Hour - time.Minute) }
	checkAndRunTick(context.Background())
	if !exists(t, "old") {
		t.Fatal("ran inside the interval after switching on")
	}
	now = func() time.Time { return clock.Add(24 * time.Hour) }
	checkAndRunTick(context.Background())
	if exists(t, "old") {
		t.Error("did not run after the interval")
	}
}

func TestTheRuleFollowsTheTwoToggles(t *testing.T) {
	cases := []struct {
		s    Settings
		ok   bool
		rule string
	}{
		{Settings{Enabled: true, OlderEnabled: true}, true, RuleOlder},
		{Settings{Enabled: true, LowMatchEnabled: true}, true, RuleLowMatch},
		{Settings{Enabled: true, OlderEnabled: true, LowMatchEnabled: true}, true, RuleBoth},
		{Settings{Enabled: true}, false, ""},
		{Settings{OlderEnabled: true, LowMatchEnabled: true}, false, ""},
	}
	for _, c := range cases {
		c.s.OlderThanDays, c.s.MinMatchPercent, c.s.IncludeManual = 45, 33, true
		o, ok := c.s.automaticOptions()
		if ok != c.ok || o.Rule != c.rule {
			t.Errorf("%+v: %v %q", c.s, ok, o.Rule)
		}
		if ok && (o.OlderThanDays != 45 || o.MinMatchPercent != 33 || !o.IncludeManual) {
			t.Errorf("thresholds not carried: %+v", o)
		}
	}
}

func TestDue(t *testing.T) {
	s := Settings{IntervalHours: 6}
	zero := time.Time{}
	if !s.due(clock, zero) {
		t.Error("never run, never saved: not due")
	}
	s.LastRunAt = "2026-06-30 06:00:00"
	if !s.due(clock, zero) || s.due(clock.Add(-time.Second), zero) {
		t.Error("boundary of the last run")
	}
	if s.due(clock, clock.Add(-time.Hour)) {
		t.Error("a save after the run should postpone")
	}
	if !s.due(clock, clock.Add(-6*time.Hour)) {
		t.Error("an old save should not postpone")
	}
	s.LastRunAt = "garbage"
	if !s.due(clock, zero) {
		t.Error("an unreadable stamp should count as long ago")
	}
}

// A manual run in progress makes the tick skip without waiting or failing; nothing is deleted by the tick.
func TestAManualRunBlocksTheTick(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "old", old30, "crawled")
	save(t, savedEarlier, autoOn())
	runMu.Lock()
	checkAndRunTick(context.Background())
	runMu.Unlock()
	if !exists(t, "old") {
		t.Error("the tick deleted while a run was in progress")
	}
	if s, _ := GetSettings(); s.LastRunAt != "" {
		t.Errorf("a skipped tick stamped the clock: %q", s.LastRunAt)
	}
}

// A cancelled context stops a run before its first batch and says so; nothing is deleted.
func TestACancelledRunStopsBetweenBatches(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "old", old30, "crawled")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := RunCleanupContext(ctx, opts(30, false))
	if err == nil || res.Deleted != 0 || !exists(t, "old") {
		t.Errorf("%+v, %v", res, err)
	}
	if s, _ := GetSettings(); s.LastRunSummary != "older than 30 days: 0 deleted, stopped: context canceled" {
		t.Errorf("summary %q", s.LastRunSummary)
	}
}

// A panic in a tick is survived.
func TestAPanickingTickIsSurvived(t *testing.T) {
	setup(t)
	fixedNow(t)
	save(t, savedEarlier, Settings{Enabled: true, LowMatchEnabled: true})
	old := newScorer
	newScorer = func() (Scorer, error) { panic("boom") }
	defer func() { newScorer = old }()
	checkAndRunTick(context.Background()) // must return
}

// The real goroutine: with a short tick it runs the due cleanup by itself, and it stops when the context is
// cancelled (no further ticks).
func TestTheGoroutineRunsAndStops(t *testing.T) {
	setup(t)
	oldTick := tickInterval
	tickInterval = 10 * time.Millisecond
	defer func() { tickInterval = oldTick }()
	oldNow := now
	now = func() time.Time { return time.Now() } // the real clock: crawled_at below is far in the past
	defer func() { now = oldNow }()

	addJob(t, "old", "2020-01-01 00:00:00", "crawled")
	save(t, time.Now().Add(-48*time.Hour), Settings{Enabled: true, OlderEnabled: true, OlderThanDays: 30})
	ctx, cancel := context.WithCancel(context.Background())
	stopped := StartManager(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for exists(t, "old") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if exists(t, "old") {
		t.Fatal("the goroutine never ran the cleanup")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the goroutine did not stop after cancel")
	}
	addJob(t, "old2", "2020-01-01 00:00:00", "crawled")
	exec(t, `UPDATE job_cleanup_settings SET last_run_at = '2000-01-01 00:00:00', updated_at = '2000-01-01 00:00:00'`)
	time.Sleep(200 * time.Millisecond)
	if !exists(t, "old2") {
		t.Error("the goroutine kept running after cancel")
	}
}

// The ticker never overlaps itself with a long run: counting runs through the scorer while a run holds the lock.
func TestTicksDoNotStackUp(t *testing.T) {
	setup(t)
	fixedNow(t)
	save(t, savedEarlier, Settings{Enabled: true, LowMatchEnabled: true})
	addJob(t, "a", young, "crawled")
	var calls int32
	release := make(chan struct{})
	old := newScorer
	newScorer = func() (Scorer, error) {
		atomic.AddInt32(&calls, 1)
		select {
		case <-release:
		case <-time.After(3 * time.Second): // a second concurrent tick would wait here forever: fail instead
		}
		return &fakeScorer{scores: map[string][]int{"a": {99}}}, nil
	}
	defer func() { newScorer = old }()
	done := make(chan struct{})
	go func() { checkAndRunTick(context.Background()); close(done) }()
	for atomic.LoadInt32(&calls) == 0 {
		time.Sleep(time.Millisecond)
	}
	checkAndRunTick(context.Background()) // second tick while the first holds the lock: skipped at once
	close(release)
	<-done
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("scorer built %d times, want 1", n)
	}
}
