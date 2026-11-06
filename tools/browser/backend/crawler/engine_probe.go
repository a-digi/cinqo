// engine_probe.go (step 52, plan 184, plan 187) — the "Test Katana" button's
// backend: POST /engine-test really runs the Katana engine on a tiny local
// page, so a Chrome that is found but cannot be driven (which the cheap
// availability check in engine_status.go cannot see) is caught. The result's
// detail is built from fixed phrases, never from raw error or process output
// (the full error goes to the tool's own log). Katana is the only engine with
// a test; the Playwright engine this file once tested was removed in plan 187.
package crawler

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// EngineTestRequest is the body of POST /engine-test.
type EngineTestRequest struct {
	Engine string `json:"engine"`
}

// EngineTestResult is the response. OK is whether the engine started and read
// the test page; Detail is one fixed sentence.
type EngineTestResult struct {
	OK         bool   `json:"ok"`
	Detail     string `json:"detail"`
	DurationMs int64  `json:"durationMs"`
}

// katanaProbeTimeout bounds the Katana test (a first start of Chrome can be
// slow). A var so a test can shrink it.
var katanaProbeTimeout = 60 * time.Second

// katanaProbeMu admits one Katana test at a time.
var katanaProbeMu sync.Mutex

// ProbeKatana (plan 184) really runs the Katana engine: it serves one tiny
// page from a throw-away server on 127.0.0.1 (no network, nothing public
// touched), fetches it through fetchWithKatana — the engine's own Chrome,
// attach and read path — and checks the title and the text came back. When
// Katana is not the selected engine its Chrome is stopped again, so a test
// never leaves a browser running for someone who uses Rod. Never returns an
// error: every outcome is an EngineTestResult with a fixed sentence (the
// detail goes to the log).
func ProbeKatana() EngineTestResult {
	start := time.Now()
	finish := func(r EngineTestResult) EngineTestResult {
		r.DurationMs = time.Since(start).Milliseconds()
		return r
	}
	if st := KatanaStatus(); !st.Available {
		return finish(EngineTestResult{Detail: st.Detail})
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Printf("katana test: could not open the local test page: %v", err)
		return finish(EngineTestResult{Detail: "Katana's test page could not be served locally"})
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><html><head><title>" + katanaProbeTitle + "</title></head><body><p>" + katanaProbeText + "</p></body></html>"))
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), katanaProbeTimeout)
	defer cancel()
	resp, err := fetchWithKatana(ctx, "http://"+ln.Addr().String()+"/", "", nil, nil, nil, 0, nil)
	if selectedFetchEngine() != fetchEngineKatana {
		StopKatanaEngine()
	}
	switch {
	case ctx.Err() != nil:
		return finish(EngineTestResult{Detail: "Katana did not finish the test in time"})
	case err != nil:
		log.Printf("katana test: fetch failed: %v", err)
		return finish(EngineTestResult{Detail: "Katana could not start or read the test page (see the Browser tool's log for details)"})
	case resp.Title != katanaProbeTitle || !strings.Contains(resp.HTML, katanaProbeText):
		log.Printf("katana test: unexpected page (title %q)", resp.Title)
		return finish(EngineTestResult{Detail: "Katana started Chrome but the test page did not come back as expected"})
	}
	return finish(EngineTestResult{OK: true, Detail: "Katana attached to Chrome and read a test page"})
}

const (
	katanaProbeTitle = "cinqo-katana-probe"
	katanaProbeText  = "katana probe ok"
)

// EngineTestHandler handles POST /engine-test. {"engine":"katana"} is the only
// engine with a test; anything else (a removed engine's name included) is a 400.
// One test at a time (409 otherwise).
func EngineTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body EngineTestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Engine != "katana" {
		http.Error(w, `engine must be "katana"`, http.StatusBadRequest)
		return
	}
	if !katanaProbeMu.TryLock() {
		http.Error(w, "a Katana test is already running", http.StatusConflict)
		return
	}
	defer katanaProbeMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ProbeKatana())
}
