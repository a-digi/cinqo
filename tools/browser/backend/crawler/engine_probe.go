// engine_probe.go (step 52) — the "Test Playwright" button's backend:
// POST /engine-test really launches Playwright's Chromium and loads a page,
// so a driver that is installed without its browser (which the cheap
// availability check in engine_status.go cannot see) is caught.
//
// The probe uses its OWN short-lived Playwright instance, launched exactly
// the way the fetch engine launches it (headless Chromium), and stops it
// again — it never touches the shared engine, so testing does not start or
// leave a browser running for a user who has Rod selected. The page it
// loads is a local data: URL, so no network is involved. The result's
// detail is built from fixed phrases, never from raw error or process
// output (the full error goes to the tool's own log).
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

	"github.com/mxschmitt/playwright-go"
)

const (
	probePageTitle = "cinqo-playwright-probe"
	probePageURL   = "data:text/html,<title>" + probePageTitle + "</title><p>ok</p>"
)

// playwrightProbeTimeout bounds the whole test. A first launch of Chromium
// can be slow, so it is generous. A var so a test can shrink it.
var playwrightProbeTimeout = 90 * time.Second

// playwrightProbeMu admits one test at a time: each one starts a Node
// driver and a browser.
var playwrightProbeMu sync.Mutex

// EngineTestRequest is the body of POST /engine-test.
type EngineTestRequest struct {
	Engine string `json:"engine"`
}

// EngineTestResult is the response. OK is whether Playwright launched
// Chromium and loaded the page; Detail is one fixed sentence.
type EngineTestResult struct {
	OK         bool   `json:"ok"`
	Detail     string `json:"detail"`
	DurationMs int64  `json:"durationMs"`
}

// classifyPlaywrightError turns a Playwright start/launch error into one
// of a few fixed sentences. The full error is logged by the caller.
func classifyPlaywrightError(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "please install the driver"):
		return "Playwright's driver is not installed"
	case strings.Contains(msg, "executable doesn't exist"),
		strings.Contains(msg, "playwright install"),
		strings.Contains(msg, "looks like playwright"):
		return "Playwright's driver is installed but its Chromium is not (run the Playwright browser installer)"
	default:
		return "Playwright could not start Chromium (see the Browser tool's log for details)"
	}
}

// ProbePlaywright runs the test described in the file comment. Never
// returns an error: every outcome is an EngineTestResult.
func ProbePlaywright() EngineTestResult {
	start := time.Now()
	finish := func(r EngineTestResult) EngineTestResult {
		r.DurationMs = time.Since(start).Milliseconds()
		return r
	}

	// The cheap check first: no point launching anything when the driver
	// itself is missing or the wrong version, and its message is clearer.
	if st := PlaywrightStatus(); !st.Available {
		return finish(EngineTestResult{Detail: st.Detail})
	}

	var mu sync.Mutex
	var pw *playwright.Playwright
	var browser playwright.Browser
	cleanup := func() {
		mu.Lock()
		defer mu.Unlock()
		if browser != nil {
			_ = browser.Close()
			browser = nil
		}
		if pw != nil {
			_ = pw.Stop()
			pw = nil
		}
	}

	done := make(chan EngineTestResult, 1)
	go func() {
		fail := func(stage string, err error) {
			log.Printf("playwright test: %s failed: %v", stage, err)
			done <- EngineTestResult{Detail: classifyPlaywrightError(err)}
		}
		inst, err := playwright.Run()
		if err != nil {
			fail("start", err)
			return
		}
		mu.Lock()
		pw = inst
		mu.Unlock()

		b, err := inst.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)})
		if err != nil {
			fail("launch", err)
			return
		}
		mu.Lock()
		browser = b
		mu.Unlock()

		page, err := b.NewPage()
		if err != nil {
			fail("new page", err)
			return
		}
		timeoutMs := float64(15 * time.Second / time.Millisecond)
		if _, err := page.Goto(probePageURL, playwright.PageGotoOptions{Timeout: &timeoutMs}); err != nil {
			fail("load page", err)
			return
		}
		title, err := page.Title()
		if err != nil || title != probePageTitle {
			log.Printf("playwright test: unexpected page state (title %q, err %v)", title, err)
			done <- EngineTestResult{Detail: "Playwright started Chromium but the test page did not load as expected"}
			return
		}
		done <- EngineTestResult{OK: true, Detail: "Playwright launched Chromium and loaded a test page"}
	}()

	select {
	case r := <-done:
		cleanup()
		return finish(r)
	case <-time.After(playwrightProbeTimeout):
		cleanup() // stopping the driver also unblocks the goroutine above
		return finish(EngineTestResult{Detail: "Playwright did not finish the test in time"})
	}
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
