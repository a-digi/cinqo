package crawler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"browser-tool-backend/shared"
)

func TestKatanaStatusFollowsChrome(t *testing.T) {
	t.Setenv(shared.ChromePathEnv, "/definitely/not/chrome")
	if st := KatanaStatus(); st.Available || st.Detail == "" {
		t.Errorf("without Chrome: %+v", st)
	}
}

func TestKatanaStatusIsAvailableWithChrome(t *testing.T) {
	if _, err := shared.ChromePath(); err != nil {
		t.Skip("no Chrome on this machine")
	}
	st := KatanaStatus()
	if !st.Available || !strings.Contains(st.Detail, "Katana") || !strings.Contains(st.Detail, "Chrome") {
		t.Errorf("with Chrome: %+v", st)
	}
}

func TestEngineTestHandlerAdmitsOneKatanaTestAtATime(t *testing.T) {
	katanaProbeMu.Lock()
	defer katanaProbeMu.Unlock()
	if rec := postEngineTest(`{"engine":"katana"}`); rec.Code != http.StatusConflict {
		t.Fatalf("a second concurrent Katana test must be a 409, got %d", rec.Code)
	}
}

func TestKatanaTestWithoutChromeSaysSoWithoutStartingAnything(t *testing.T) {
	t.Setenv(shared.ChromePathEnv, "/definitely/not/chrome")
	rec := postEngineTest(`{"engine":"katana"}`)
	var r EngineTestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s %v", rec.Code, rec.Body.String(), err)
	}
	if r.OK || r.Detail == "" || katanaRunning() {
		t.Errorf("result %+v, running=%v", r, katanaRunning())
	}
}

// The real test: Katana's engine starts Chrome, attaches, reads the local page.
// With Rod selected, the Chrome it started is stopped again afterwards.
func TestKatanaTestReadsItsPageAndLeavesNoChromeWhenKatanaIsNotSelected(t *testing.T) {
	requireKatanaChrome(t)
	useBrowserSettings(t, shared.FetchEngineRod)
	rec := httptest.NewRecorder()
	EngineTestHandler(rec, httptest.NewRequest(http.MethodPost, "/engine-test", strings.NewReader(`{"engine":"katana"}`)))
	var r EngineTestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if !r.OK || !strings.Contains(r.Detail, "Katana") {
		t.Fatalf("the test failed: %+v", r)
	}
	if katanaRunning() {
		t.Error("with Rod selected the test must stop the Chrome it started")
	}
}

func TestKatanaTestKeepsItsChromeWhenKatanaIsSelected(t *testing.T) {
	requireKatanaChrome(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	r := ProbeKatana()
	if !r.OK {
		t.Fatalf("the test failed: %+v", r)
	}
	if !katanaRunning() {
		t.Error("with Katana selected its Chrome belongs to the engine and must stay up")
	}
}

// The standing rule: a headless Chrome must never outlive the work that needed it. A "Test Katana" with
// Katana selected leaves the Chrome to the engine, so it must follow the same idle rule as a real fetch:
// closed a moment after the test ends (found live in plan 187's check: it used to stay up until the next fetch).
func TestKatanaTestChromeIsClosedByTheIdleStopWhenKatanaIsSelected(t *testing.T) {
	requireKatanaChrome(t)
	resetEngineTracking(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	shortenIdleStop(t, 300*time.Millisecond)
	if r := ProbeKatana(); !r.OK {
		t.Fatalf("the test failed: %+v", r)
	}
	deadline := time.Now().Add(15 * time.Second)
	for katanaRunning() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if katanaRunning() {
		t.Fatal("the Chrome the Katana test started was still up long after the idle stop should have closed it")
	}
}
