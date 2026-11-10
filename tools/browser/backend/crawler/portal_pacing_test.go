package crawler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func resetPacing(t *testing.T) {
	t.Helper()
	portalPacingMu.Lock()
	portalPacingLast = map[string]time.Time{}
	portalPacingAlarm = map[string]bool{}
	portalPacingMu.Unlock()
}

func rateLimited() error { return &crawlError{Code: codeRateLimited} }

func TestThePacingGapsAreThreeAndTwentySeconds(t *testing.T) {
	if normalPortalGap != 3*time.Second || alarmPortalGap != 20*time.Second {
		t.Fatalf("%v %v", normalPortalGap, alarmPortalGap)
	}
}

func TestTheFirstReadOfAKeyStartsAtOnceAndTheNextThreeSecondsLater(t *testing.T) {
	resetPacing(t)
	if got := reservePacingSlot("k", t0); !got.Equal(t0) {
		t.Errorf("first: %v", got)
	}
	// asked 1 s later: waits until 3 s after the first
	if got := reservePacingSlot("k", t0.Add(time.Second)); !got.Equal(t0.Add(3 * time.Second)) {
		t.Errorf("second: %v", got)
	}
	// asked long after: no wait
	late := t0.Add(time.Hour)
	if got := reservePacingSlot("k", late); !got.Equal(late) {
		t.Errorf("late: %v", got)
	}
}

func TestParallelReadsOfOneKeyAreSpacedThreeSecondsApart(t *testing.T) {
	resetPacing(t)
	var got []time.Time
	for i := 0; i < 4; i++ {
		got = append(got, reservePacingSlot("k", t0))
	}
	for i, g := range got {
		if want := t0.Add(time.Duration(i) * normalPortalGap); !g.Equal(want) {
			t.Errorf("read %d: %v, want %v", i, g, want)
		}
	}
}

func TestARateLimitRaisesTheAlarmAndTheGapIsTwentySeconds(t *testing.T) {
	resetPacing(t)
	reservePacingSlot("k", t0)
	notePacingOutcome("k", rateLimited())
	if !pacingAlarmRaised("k") {
		t.Fatal("the alarm must be raised")
	}
	if got := reservePacingSlot("k", t0.Add(time.Second)); !got.Equal(t0.Add(20 * time.Second)) {
		t.Errorf("with the alarm: %v", got)
	}
	// a further rate limit does not lengthen it (no doubling)
	notePacingOutcome("k", rateLimited())
	if got := reservePacingSlot("k", t0.Add(21*time.Second)); !got.Equal(t0.Add(40 * time.Second)) {
		t.Errorf("a second rate limit: %v", got)
	}
}

func TestTheFirstReadWithoutARateLimitRemovesTheAlarmAtOnce(t *testing.T) {
	resetPacing(t)
	reservePacingSlot("k", t0)
	notePacingOutcome("k", rateLimited())
	notePacingOutcome("k", nil)
	if pacingAlarmRaised("k") {
		t.Fatal("one clean read must remove the alarm")
	}
	if got := reservePacingSlot("k", t0.Add(time.Second)); !got.Equal(t0.Add(3 * time.Second)) {
		t.Errorf("after the alarm: %v", got)
	}
}

func TestAnyOtherErrorChangesNothing(t *testing.T) {
	resetPacing(t)
	for _, err := range []error{errors.New("boom"), &crawlError{Code: codeCloudflareUnresolved}, &crawlError{Code: codeCloudflareBlocked}, context.Canceled} {
		notePacingOutcome("k", err)
		if pacingAlarmRaised("k") {
			t.Errorf("%v raised the alarm", err)
		}
	}
	notePacingOutcome("k", rateLimited())
	for _, err := range []error{errors.New("boom"), &crawlError{Code: codeCloudflareUnresolved}, context.Canceled} {
		notePacingOutcome("k", err)
		if !pacingAlarmRaised("k") {
			t.Errorf("%v removed the alarm", err)
		}
	}
}

func TestKeysAreIndependentAndNoKeyIsNeverPaced(t *testing.T) {
	resetPacing(t)
	reservePacingSlot("a", t0)
	notePacingOutcome("a", rateLimited())
	reservePacingSlot("b", t0)
	if pacingAlarmRaised("b") {
		t.Error("b must not be alarmed by a")
	}
	if got := reservePacingSlot("b", t0.Add(time.Second)); !got.Equal(t0.Add(3 * time.Second)) {
		t.Errorf("b: %v", got)
	}
	notePacingOutcome("", rateLimited())
	if pacingAlarmRaised("") {
		t.Error("no key, no alarm")
	}
	start := time.Now()
	for i := 0; i < 3; i++ {
		awaitPortalCrawlPacing(context.Background(), "")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Error("an AI call (no key) must never wait")
	}
	portalPacingMu.Lock()
	_, kept := portalPacingLast[""]
	portalPacingMu.Unlock()
	if kept {
		t.Error("no key must reserve nothing")
	}
}

func TestRetryAfterIsTwentyWhenTheSiteGaveNoneAndTheAlarmIsRaised(t *testing.T) {
	resetPacing(t)
	err := &crawlError{Code: codeRateLimited}
	notePacingOutcome("k", err)
	withPacingRetry("k", err)
	if err.RetryAfterSeconds != 20 {
		t.Errorf("%d", err.RetryAfterSeconds)
	}
	site := &crawlError{Code: codeRateLimited, RetryAfterSeconds: 45}
	notePacingOutcome("k", site)
	withPacingRetry("k", site)
	if site.RetryAfterSeconds != 45 {
		t.Errorf("the site's own value wins: %d", site.RetryAfterSeconds)
	}
	other := &crawlError{Code: codeCloudflareUnresolved}
	withPacingRetry("k", other)
	if other.RetryAfterSeconds != 0 {
		t.Errorf("only a rate limit gets it: %d", other.RetryAfterSeconds)
	}
	withPacingRetry("", err2())
	resetPacing(t)
	clean := &crawlError{Code: codeRateLimited}
	withPacingRetry("k", clean)
	if clean.RetryAfterSeconds != 0 {
		t.Errorf("no alarm, no advice: %d", clean.RetryAfterSeconds)
	}
}

func err2() error { return &crawlError{Code: codeRateLimited} }

func TestAStopInterruptsAPacingWaitAndKeepsTheReservation(t *testing.T) {
	resetPacing(t)
	awaitPortalCrawlPacing(context.Background(), "k") // first: at once
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { awaitPortalCrawlPacing(ctx, "k"); close(done) }() // would wait 3 s
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("a cancel must end the wait at once")
	}
	portalPacingMu.Lock()
	last := portalPacingLast["k"]
	portalPacingMu.Unlock()
	if !last.After(time.Now()) {
		t.Error("the reservation must be kept")
	}
}

func TestTheRealWaitIsAboutThreeSeconds(t *testing.T) {
	resetPacing(t)
	awaitPortalCrawlPacing(context.Background(), "k")
	start := time.Now()
	awaitPortalCrawlPacing(context.Background(), "k")
	if d := time.Since(start); d < 2800*time.Millisecond || d > 3600*time.Millisecond {
		t.Errorf("waited %v", d)
	}
}

func TestReservationsUnderConcurrencyAreAllDistinct(t *testing.T) {
	resetPacing(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[time.Time]bool{}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := reservePacingSlot("k", t0)
			mu.Lock()
			seen[s] = true
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(seen) != 20 {
		t.Errorf("%d distinct slots, want 20", len(seen))
	}
}
