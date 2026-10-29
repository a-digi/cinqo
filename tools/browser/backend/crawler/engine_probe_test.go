package crawler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClassifyPlaywrightError(t *testing.T) {
	cases := map[string]string{
		"please install the driver (v1.62.1) first":                             "driver is not installed",
		"could not launch: Executable doesn't exist at /x/chromium-1234/chrome": "Chromium is not",
		"Looks like Playwright was just installed or updated.":                  "Chromium is not",
		"BrowserType.launch: Please run playwright install to download":         "Chromium is not",
		"some totally different failure with /secret/path and SECRET-OUTPUT":    "could not start Chromium",
	}
	for in, want := range cases {
		got := classifyPlaywrightError(errors.New(in))
		if !strings.Contains(got, want) {
			t.Errorf("%q → %q, want it to mention %q", in, got, want)
		}
		if strings.Contains(got, "SECRET") || strings.Contains(got, "/secret") || strings.Contains(got, "/x/chromium") {
			t.Errorf("the detail must never echo the raw error: %q", got)
		}
	}
}

func postEngineTest(body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	EngineTestHandler(rec, httptest.NewRequest(http.MethodPost, "/engine-test", strings.NewReader(body)))
	return rec
}

func TestEngineTestHandlerValidatesItsRequest(t *testing.T) {
	for _, body := range []string{`not json`, `{}`, `{"engine":""}`, `{"engine":"rod"}`, `{"engine":"Playwright"}`, `{"engine":"playwright; rm -rf /"}`} {
		if rec := postEngineTest(body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", body, rec.Code)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		EngineTestHandler(rec, httptest.NewRequest(method, "/engine-test", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", method, rec.Code)
		}
	}
}

func TestEngineTestHandlerAdmitsOneTestAtATime(t *testing.T) {
	playwrightProbeMu.Lock() // a test is "running"
	defer playwrightProbeMu.Unlock()
	if rec := postEngineTest(`{"engine":"playwright"}`); rec.Code != http.StatusConflict {
		t.Fatalf("a second concurrent test must be a 409, got %d", rec.Code)
	}
}

// With no usable driver the test fails fast with the status message and
// launches nothing.
func TestProbeReportsAMissingDriverWithoutLaunchingAnything(t *testing.T) {
	fakeDriver(t, `echo "Version 0.0.1 SECRET-PROCESS-OUTPUT"`) // wrong version
	start := time.Now()
	res := ProbePlaywright()
	if res.OK || !strings.Contains(res.Detail, "not the version") || strings.Contains(res.Detail, "SECRET") {
		t.Fatalf("%+v", res)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("a missing driver must be reported without launching anything, took %s", time.Since(start))
	}
	if pwInst != nil || pwBrowser != nil {
		t.Errorf("the probe must never start the shared Playwright engine")
	}
}

func requireRealPlaywright(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping a real Playwright launch in -short mode")
	}
	if st := PlaywrightStatus(); !st.Available {
		t.Skipf("Playwright is not installed here: %s", st.Detail)
	}
}

func TestProbeLaunchesRealChromiumAndLeavesTheSharedEngineAlone(t *testing.T) {
	requireRealPlaywright(t)
	res := ProbePlaywright()
	if !res.OK {
		t.Fatalf("a working Playwright must pass: %+v", res)
	}
	if res.DurationMs <= 0 || !strings.Contains(res.Detail, "launched Chromium") {
		t.Errorf("%+v", res)
	}
	if pwInst != nil || pwBrowser != nil {
		t.Errorf("the probe uses its own short-lived instance; the shared engine must stay unstarted")
	}

	// And through the handler: the JSON shape the page reads.
	rec := postEngineTest(`{"engine":"playwright"}`)
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK ||
		body["ok"] != true || body["detail"] == "" || body["durationMs"] == nil {
		t.Fatalf("%d %s (%v)", rec.Code, rec.Body.String(), err)
	}
}

// The case the cheap availability check cannot see: the driver is installed
// but Playwright's Chromium is not. Simulated for real by pointing the
// browser lookup at an empty directory.
func TestProbeCatchesAMissingChromiumForReal(t *testing.T) {
	requireRealPlaywright(t)
	t.Setenv("PLAYWRIGHT_BROWSERS_PATH", t.TempDir())
	res := ProbePlaywright()
	if res.OK || !strings.Contains(res.Detail, "Chromium is not") {
		t.Fatalf("a missing browser must be reported as such: %+v", res)
	}
}

func TestProbeGivesUpWhenItTakesTooLong(t *testing.T) {
	requireRealPlaywright(t)
	orig := playwrightProbeTimeout
	playwrightProbeTimeout = 50 * time.Millisecond
	t.Cleanup(func() { playwrightProbeTimeout = orig })

	start := time.Now()
	res := ProbePlaywright()
	if res.OK || !strings.Contains(res.Detail, "in time") {
		t.Fatalf("%+v", res)
	}
	if time.Since(start) > 15*time.Second {
		t.Errorf("a timed-out test must return promptly, took %s", time.Since(start))
	}
}

func TestManifestDeclaresTheEngineTestRouteWithItsScope(t *testing.T) {
	raw := readManifest(t)
	for _, r := range raw {
		if r["path_suffix"] == "engine-test" {
			if r["method"] != "POST" || r["required_scope"] != "tool:browser:crawl" {
				t.Errorf("route: %+v", r)
			}
			return
		}
	}
	t.Error("manifest.json does not declare the engine-test route")
}
