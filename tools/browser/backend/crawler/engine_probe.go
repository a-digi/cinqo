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
	"encoding/json"
	"log"
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

// EngineTestHandler handles POST /engine-test. Only {"engine":"playwright"}
// is supported; anything else is a 400. One test at a time (409 otherwise).
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
	if body.Engine != "playwright" {
		http.Error(w, `engine must be "playwright"`, http.StatusBadRequest)
		return
	}
	if !playwrightProbeMu.TryLock() {
		http.Error(w, "a Playwright test is already running", http.StatusConflict)
		return
	}
	defer playwrightProbeMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ProbePlaywright())
}
