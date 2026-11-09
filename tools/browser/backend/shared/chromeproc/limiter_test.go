package chromeproc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTheLimitIsFiveAndTheEnvironmentCanOnlyLowerIt(t *testing.T) {
	for env, want := range map[string]int{"": 5, "3": 3, "1": 1, " 2 ": 2, "5": 5, "6": 5, "100": 5, "0": 5, "-1": 5, "x": 5} {
		if got := limitFrom(env); got != want {
			t.Errorf("%q: got %d, want %d", env, got, want)
		}
	}
	if MaxSessions != 5 || QueueWait != 60*time.Second || StuckAfter != 15*time.Minute {
		t.Error("the plan's numbers: 5 sessions, a 60 s queue, a 15 minute stuck limit")
	}
	if NewLimiter(0, nil).Status().Limit != 1 || NewLimiter(-3, nil).Status().Limit != 1 {
		t.Error("a limiter always has at least one slot")
	}
}

func TestOnlyFiveRunAtOnceAndTheSixthWaitsForAFreeSlot(t *testing.T) {
	l := NewLimiter(5, nil)
	var rel []func()
	for i := 0; i < 5; i++ {
		r, err := l.Acquire(context.Background(), "a", time.Second, nil)
		if err != nil {
			t.Fatal(err)
		}
		rel = append(rel, r)
	}
	if st := l.Status(); st.Running != 5 || st.Waiting != 0 || st.Limit != 5 {
		t.Fatalf("%+v", st)
	}
	got := make(chan error, 1)
	go func() {
		r, err := l.Acquire(context.Background(), "sixth", 5*time.Second, nil)
		if err == nil {
			defer r()
		}
		got <- err
	}()
	deadline := time.Now().Add(time.Second)
	for l.Status().Waiting != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if st := l.Status(); st.Waiting != 1 || st.Running != 5 {
		t.Fatalf("the sixth must wait: %+v", st)
	}
	select {
	case <-got:
		t.Fatal("the sixth must not run while five are running")
	case <-time.After(60 * time.Millisecond):
	}
	rel[0]()
	if err := <-got; err != nil {
		t.Errorf("a freed slot must go to the waiter: %v", err)
	}
	for _, r := range rel[1:] {
		r()
	}
	if st := l.Status(); st.Running != 0 || st.Waiting != 0 {
		t.Errorf("%+v", st)
	}
}

func TestTheQueueIsFirstComeFirstServed(t *testing.T) {
	l := NewLimiter(1, nil)
	hold, _ := l.Acquire(context.Background(), "holder", time.Second, nil)
	var mu sync.Mutex
	var order []int
	var wg sync.WaitGroup
	for i := 1; i <= 4; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := l.Acquire(context.Background(), "w", 5*time.Second, nil)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			r()
		}()
		for l.Status().Waiting != i { // enter the line one by one, in order
			time.Sleep(2 * time.Millisecond)
		}
	}
	hold()
	wg.Wait()
	for i, v := range order {
		if v != i+1 {
			t.Fatalf("order %v", order)
		}
	}
}

func TestANewcomerNeverJumpsTheQueue(t *testing.T) {
	l := NewLimiter(1, nil)
	hold, _ := l.Acquire(context.Background(), "holder", time.Second, nil)
	first := make(chan func(), 1)
	go func() {
		r, _ := l.Acquire(context.Background(), "queued", 5*time.Second, nil)
		first <- r
	}()
	for l.Status().Waiting != 1 {
		time.Sleep(2 * time.Millisecond)
	}
	hold() // the slot goes to the queued one at once; a newcomer arriving now must wait behind it
	r := <-first
	if _, err := l.Acquire(context.Background(), "newcomer", 30*time.Millisecond, nil); !errors.Is(err, ErrTooManySessions) {
		t.Errorf("the newcomer must not get the slot: %v", err)
	}
	r()
}

func TestAWaitThatRunsOutFailsWithTooManySessionsAndLeavesTheLine(t *testing.T) {
	l := NewLimiter(1, nil)
	hold, _ := l.Acquire(context.Background(), "holder", time.Second, nil)
	defer hold()
	start := time.Now()
	_, err := l.Acquire(context.Background(), "late", 80*time.Millisecond, nil)
	if !errors.Is(err, ErrTooManySessions) || time.Since(start) < 80*time.Millisecond {
		t.Errorf("%v after %s", err, time.Since(start))
	}
	if st := l.Status(); st.Waiting != 0 || st.Running != 1 {
		t.Errorf("%+v", st)
	}
}

func TestACancelledWaiterLeavesTheLineAtOnce(t *testing.T) {
	l := NewLimiter(1, nil)
	hold, _ := l.Acquire(context.Background(), "holder", time.Second, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := l.Acquire(ctx, "x", time.Hour, nil)
		done <- err
	}()
	for l.Status().Waiting != 1 {
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("%v", err)
	}
	if l.Status().Waiting != 0 {
		t.Error("it must have left the line")
	}
	hold()
	if st := l.Status(); st.Running != 0 {
		t.Errorf("no slot may be lost to a waiter that left: %+v", st)
	}
	// a slot handed over at the very moment the waiter gives up is passed on, never leaked
	r, err := l.Acquire(context.Background(), "after", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	r()
}

func TestReleaseIsIdempotent(t *testing.T) {
	l := NewLimiter(2, nil)
	a, _ := l.Acquire(context.Background(), "a", time.Second, nil)
	b, _ := l.Acquire(context.Background(), "b", time.Second, nil)
	a()
	a()
	a()
	if st := l.Status(); st.Running != 1 {
		t.Errorf("releasing one session three times frees one slot: %+v", st)
	}
	b()
	if l.Status().Running != 0 {
		t.Error("both released")
	}
}

func TestAStuckSessionIsCancelledAndItsSlotGivenToTheNextInLine(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	l := NewLimiter(1, func() time.Time { return now })
	var cancelled atomic.Bool
	stuck, _ := l.Acquire(context.Background(), "stuck", time.Second, func() { cancelled.Store(true) })
	got := make(chan error, 1)
	go func() {
		r, err := l.Acquire(context.Background(), "next", 5*time.Second, nil)
		if err == nil {
			r()
		}
		got <- err
	}()
	for l.Status().Waiting != 1 {
		time.Sleep(2 * time.Millisecond)
	}
	now = now.Add(14*time.Minute + 59*time.Second)
	if n := l.ReapStuck(StuckAfter); n != 0 || cancelled.Load() {
		t.Fatalf("just under the limit: %d", n)
	}
	now = now.Add(time.Second)
	if n := l.ReapStuck(StuckAfter); n != 1 || !cancelled.Load() {
		t.Fatalf("at the limit: %d cancelled=%v", n, cancelled.Load())
	}
	if err := <-got; err != nil {
		t.Errorf("the next in line must get the freed slot: %v", err)
	}
	if l.StuckReleased() != 1 {
		t.Errorf("%d", l.StuckReleased())
	}
	stuck() // the stuck request finally ends: its late release must not free a slot twice
	if st := l.Status(); st.Running != 0 {
		t.Errorf("%+v", st)
	}
	r, err := l.Acquire(context.Background(), "again", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	r2, err2 := l.Acquire(context.Background(), "again2", 20*time.Millisecond, nil)
	if err2 == nil {
		r2()
		t.Error("the late release of a reaped session must not have opened a second slot")
	}
	r()
}

func TestNeverMoreThanFiveRunInParallelUnderLoad(t *testing.T) {
	l := NewLimiter(5, nil)
	var inside, peak atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := l.Acquire(context.Background(), "x", 10*time.Second, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer r()
			n := inside.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(3 * time.Millisecond)
			inside.Add(-1)
		}()
	}
	wg.Wait()
	if peak.Load() > 5 || peak.Load() < 2 {
		t.Errorf("peak %d", peak.Load())
	}
	if st := l.Status(); st.Running != 0 || st.Waiting != 0 {
		t.Errorf("%+v", st)
	}
}

func TestTheMiddlewareRunsTheHandlerAndGivesTheSlotBackEvenOnAPanic(t *testing.T) {
	l := NewLimiter(1, nil)
	ok := limitWith(l, time.Second, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("done")) })
	rec := httptest.NewRecorder()
	ok(rec, httptest.NewRequest(http.MethodPost, "/crawl", nil))
	if rec.Body.String() != "done" || l.Status().Running != 0 {
		t.Errorf("%q %+v", rec.Body.String(), l.Status())
	}
	boom := limitWith(l, time.Second, func(http.ResponseWriter, *http.Request) { panic("boom") })
	func() {
		defer func() { _ = recover() }()
		boom(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/crawl", nil))
	}()
	if l.Status().Running != 0 {
		t.Error("a panicking handler must still give its slot back")
	}
}

func TestTheMiddlewareAnswers503WithAClearMessageWhenNoSlotIsFree(t *testing.T) {
	l := NewLimiter(2, nil)
	r1, _ := l.Acquire(context.Background(), "a", time.Second, nil)
	r2, _ := l.Acquire(context.Background(), "b", time.Second, nil)
	defer r1()
	defer r2()
	called := false
	h := limitWith(l, 50*time.Millisecond, func(http.ResponseWriter, *http.Request) { called = true })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/crawl", nil))
	if called {
		t.Error("the handler must not run without a slot")
	}
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != "5" || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "too_many_sessions" || body["limit"] != float64(2) || body["running"] != float64(2) || !strings.Contains(body["message"].(string), "2 of 2 crawl sessions are running") {
		t.Errorf("%v", body)
	}
}

func TestAClientThatLeavesWhileWaitingGetsNoAnswerAndLeavesTheLine(t *testing.T) {
	l := NewLimiter(1, nil)
	hold, _ := l.Acquire(context.Background(), "a", time.Second, nil)
	defer hold()
	ctx, cancel := context.WithCancel(context.Background())
	h := limitWith(l, time.Hour, func(http.ResponseWriter, *http.Request) {})
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h(rec, httptest.NewRequest(http.MethodPost, "/crawl", nil).WithContext(ctx))
		close(done)
	}()
	for l.Status().Waiting != 1 {
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	<-done
	if rec.Body.Len() != 0 || l.Status().Waiting != 0 {
		t.Errorf("%q %+v", rec.Body.String(), l.Status())
	}
}

func TestAStuckRequestHasItsContextCancelled(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	l := NewLimiter(1, func() time.Time { return now })
	started, finished := make(chan struct{}), make(chan error, 1)
	h := limitWith(l, time.Second, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		finished <- r.Context().Err()
	})
	go h(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/crawl", nil))
	<-started
	now = now.Add(StuckAfter)
	if l.ReapStuck(StuckAfter) != 1 {
		t.Fatal("not reaped")
	}
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the stuck request must have its context cancelled")
	}
}

func TestTheStatusCarriesTheSessionNumbersAndTheWatchdogReapsStuckSessions(t *testing.T) {
	old := sessions
	t.Cleanup(func() { sessions = old })
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	sessions = NewLimiter(5, func() time.Time { return now })
	r1, _ := sessions.Acquire(context.Background(), "a", time.Second, nil)
	defer r1()
	st := New("", newFake(), nil).Snapshot()
	if st.Limit != 5 || st.Running != 1 || st.Waiting != 0 {
		t.Errorf("%+v", st)
	}
	now = now.Add(StuckAfter)
	w := newWDWorld(t)
	res := w.r.WatchdogOnce(quick)
	if res.StuckSessions != 1 || !res.acted() {
		t.Errorf("%+v", res)
	}
	if got := w.r.Snapshot().Watchdog.StuckSessions; got != 1 {
		t.Errorf("%d", got)
	}
	if Sessions().Running != 0 {
		t.Error("the stuck session's slot must be free")
	}
}
