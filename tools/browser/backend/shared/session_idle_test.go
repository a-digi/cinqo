package shared

import (
	"context"
	"testing"
	"time"
)

// runningSession fakes a started shared browser: a live Ctx, nothing real.
func runningSession(t *testing.T) {
	t.Helper()
	Mu.Lock()
	savedCtx, savedCancel := Ctx, primaryCancel
	Ctx, primaryCancel = context.Background(), nil
	Mu.Unlock()
	savedIdle := SessionIdleStop
	SessionIdleStop = time.Second
	t.Cleanup(func() {
		Mu.Lock()
		Ctx, primaryCancel = savedCtx, savedCancel
		Mu.Unlock()
		SessionIdleStop = savedIdle
	})
}

func running() bool {
	Mu.Lock()
	defer Mu.Unlock()
	return Ctx != nil
}

func TestReapIdleSession_ClosesAfterTheIdleTime(t *testing.T) {
	runningSession(t)
	touchSessionActivity()
	if reapIdleSession(time.Now()) {
		t.Fatal("closed a browser that was just used")
	}
	if !reapIdleSession(time.Now().Add(2 * time.Second)) {
		t.Fatal("did not close a browser idle for longer than SessionIdleStop")
	}
	if running() {
		t.Fatal("the session is still set after it was closed")
	}
}

func TestReapIdleSession_NothingRunningIsLeftAlone(t *testing.T) {
	Mu.Lock()
	saved := Ctx
	Ctx = nil
	Mu.Unlock()
	t.Cleanup(func() { Mu.Lock(); Ctx = saved; Mu.Unlock() })
	if reapIdleSession(time.Now().Add(time.Hour)) {
		t.Fatal("reported closing a browser that was not running")
	}
}

func TestReapIdleSession_NeverClosesUnderAWorkerTab(t *testing.T) {
	runningSession(t)
	touchSessionActivity()
	slots := workerSlotPool()
	slots <- struct{}{}
	defer func() { <-slots }()
	if reapIdleSession(time.Now().Add(time.Hour)) {
		t.Fatal("closed the browser while a worker tab held a slot")
	}
	if !running() {
		t.Fatal("session was torn down under a worker tab")
	}
}

func TestReapIdleSession_NeverClosesWhilePrimaryTabIsInUse(t *testing.T) {
	runningSession(t)
	lastSessionActivity.Store(time.Now().Add(-time.Hour).UnixNano()) // long idle on record
	Mu.Lock()
	closed := reapIdleSession(time.Now().Add(time.Hour))
	Mu.Unlock()
	if closed {
		t.Fatal("closed the browser while the primary tab (Mu) was held")
	}
	// Seeing it busy counts as activity, so the idle time restarts.
	if reapIdleSession(time.Now()) {
		t.Fatal("closed right after the primary tab was released")
	}
}

func TestEnsureAndWorkerAcquireCountAsActivity(t *testing.T) {
	lastSessionActivity.Store(0)
	touchSessionActivity()
	if time.Since(time.Unix(0, lastSessionActivity.Load())) > time.Second {
		t.Fatal("touchSessionActivity did not record now")
	}
}
