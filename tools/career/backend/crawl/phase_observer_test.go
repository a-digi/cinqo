package crawl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestThePhaseObserverIsToldEveryChangeOfAPageLoad(t *testing.T) {
	setupTrialDB(t) // the poller also writes the phase of a crawl run (a recrawl has none: nothing to update)
	var mu sync.Mutex
	answers := []string{`{"phase":"queued_for_tab","message":"waiting for a free browser tab"}`, `{"phase":"navigating","message":"navigating to https://x"}`, `{"phase":"navigating","message":"navigating to https://x"}`, `{"phase":"extracting","message":"extracting page 1"}`}
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Query().Get("requestId") != "recrawl-1" {
			http.NotFound(w, r)
			return
		}
		if n == 0 { // the first look: the Browser knows nothing yet (it waits for the platform's pacing)
			n++
			http.NotFound(w, r)
			return
		}
		i := n - 1
		n++
		if i >= len(answers) {
			i = len(answers) - 1
		}
		_, _ = w.Write([]byte(answers[i]))
	}))
	defer srv.Close()
	old := crawlStatusPollInterval
	crawlStatusPollInterval = 15 * time.Millisecond
	defer func() { crawlStatusPollInterval = old }()

	type seen struct{ phase, message string }
	var got []seen
	var gmu sync.Mutex
	ctx := WithPhaseObserver(context.Background(), func(phase, message string) {
		gmu.Lock()
		got = append(got, seen{phase, message})
		gmu.Unlock()
	})
	stop := pollBrowserPhase(ctx, srv.URL, "recrawl-1", "tok")
	time.Sleep(250 * time.Millisecond)
	stop()

	gmu.Lock()
	defer gmu.Unlock()
	want := []seen{{"queued_for_tab", "waiting for a free browser tab"}, {"navigating", "navigating to https://x"}, {"extracting", "extracting page 1"}}
	if len(got) != len(want) {
		t.Fatalf("observed %v, want %v (the 404 and the repeated phase are not changes)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("change %d: %v, want %v", i, got[i], want[i])
		}
	}
}

func TestWithoutAnObserverThePollerStillWorks(t *testing.T) {
	PhaseObserver(context.Background())("navigating", "x") // does nothing, never panics
	if PhaseObserver(WithPhaseObserver(context.Background(), nil)) == nil {
		t.Error("a nil observer must read as a no-op function")
	}
}
