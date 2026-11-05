package crawler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
