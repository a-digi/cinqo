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

func listViaHandler(t *testing.T) []domainView {
	t.Helper()
	rec := httptest.NewRecorder()
	DomainsHandler(rec, httptest.NewRequest(http.MethodGet, "/domains", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /domains: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Domains []domainView `json:"domains"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Domains
}

func clearNow(domain string) (*httptest.ResponseRecorder, map[string]any) {
	rec := httptest.NewRecorder()
	DomainsClearHandler(rec, httptest.NewRequest(http.MethodPost, "/domains/clear", strings.NewReader(`{"domain":"`+domain+`"}`)))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func find(domains []domainView, name string) *domainView {
	for i := range domains {
		if domains[i].Domain == name {
			return &domains[i]
		}
	}
	return nil
}

func TestNormalizeDomain(t *testing.T) {
	for in, want := range map[string]string{
		"example.com":                    "example.com",
		"  Example.COM ":                 "example.com",
		"www.example.com":                "example.com",
		"https://www.example.com/a?b":    "example.com",
		"http://jobs.example.com:8080/x": "jobs.example.com",
		"":                               "",
		"   ":                            "",
	} {
		if got := normalizeDomain(in); got != want {
			t.Errorf("normalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTheListMergesKnownDomainsKeptClearancesAndOpenRequests(t *testing.T) {
	stub := newHumanStub(t)
	if err := recordCloudflareDomain("known.example", "cf-mitigated: challenge (HTTP 403)"); err != nil {
		t.Fatal(err)
	}
	valid := shared.Clearance{Domain: "cleared.example", Name: shared.ClearanceCookieName, Value: "SECRET-VALUE", Path: "/", ExpiresAt: time.Now().Add(time.Hour), UserAgent: "UA", Source: "human"}
	if err := shared.StoreClearance(valid); err != nil {
		t.Fatal(err)
	}
	// An expired one is shown as expired (the store refuses expired rows, so age it by hand).
	expired := valid
	expired.Domain = "old.example"
	if err := shared.StoreClearance(expired); err != nil {
		t.Fatal(err)
	}
	if _, err := shared.DB.Exec(`UPDATE domain_clearance SET expires_at = ? WHERE domain = 'old.example'`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	humanAssistedError("https://asked.example/", "", unresolved())
	waitState(t, "asked.example", humanWaiting)
	stub.waitLaunches(t, 1)

	rec := httptest.NewRecorder()
	DomainsHandler(rec, httptest.NewRequest(http.MethodGet, "/domains", nil))
	if strings.Contains(rec.Body.String(), "SECRET-VALUE") {
		t.Fatal("a cookie value must never be returned")
	}
	domains := listViaHandler(t)

	if d := find(domains, "known.example"); d == nil || d.Reason == "" || d.Clearance.State != "none" || d.Request != nil {
		t.Errorf("known: %+v", d)
	}
	if d := find(domains, "cleared.example"); d == nil || d.Clearance.State != "valid" || d.Clearance.ExpiresAt == "" || d.Clearance.Source != "human" {
		t.Errorf("cleared: %+v", d)
	}
	if d := find(domains, "old.example"); d == nil || d.Clearance.State != "expired" {
		t.Errorf("expired: %+v", d)
	}
	d := find(domains, "asked.example")
	if d == nil || d.Request == nil || d.Request.State != "waiting" || !strings.HasPrefix(d.Request.RequestID, "human-asked.example-") {
		t.Fatalf("asked: %+v", d)
	}
	if domains[0].Domain != "asked.example" {
		t.Errorf("what is waiting for a person must come first: %s", domains[0].Domain)
	}
	stub.release <- nil
}

func TestAFinishedRequestShowsItsStateButNoCancelId(t *testing.T) {
	stub := newHumanStub(t)
	humanAssistedError("https://done.example/", "", unresolved())
	waitState(t, "done.example", humanWaiting)
	stub.waitLaunches(t, 1)
	stub.release <- &crawlError{Code: codeCloudflareUnresolved}
	waitState(t, "done.example", humanGaveUp)
	d := find(listViaHandler(t), "done.example")
	if d == nil || d.Request == nil || d.Request.State != "gave_up" || d.Request.RequestID != "" || d.Request.FinishedAt == "" || d.Request.Detail == "" {
		t.Fatalf("%+v", d)
	}
}

func TestClearNowOpensTheWindowIgnoringTheSettingAndTheCooldown(t *testing.T) {
	stub := newHumanStub(t)
	// The agent-facing setting is off, and a request just ended: neither blocks the user.
	s, _ := shared.LoadBrowserSettings()
	s.HumanAssist = false
	if err := shared.SaveBrowserSettings(s); err != nil {
		t.Fatal(err)
	}
	rec, body := clearNow("93.184.216.34")
	if rec.Code != http.StatusOK || body["state"] != "waiting" || body["alreadyWaiting"] != false || !strings.HasPrefix(body["requestId"].(string), "human-93.184.216.34-") {
		t.Fatalf("%d %v", rec.Code, body)
	}
	waitState(t, "93.184.216.34", humanWaiting)
	stub.waitLaunches(t, 1)
	if got := stub.launches[0]; got != "https://93.184.216.34/" {
		t.Errorf("the window opens the site's own page: %q", got)
	}

	// Pressing it again while the window is open reuses the request.
	rec2, body2 := clearNow("93.184.216.34")
	if rec2.Code != http.StatusOK || body2["alreadyWaiting"] != true || body2["requestId"] != body["requestId"] {
		t.Errorf("second press: %v", body2)
	}
	if stub.launched() != 1 {
		t.Errorf("a second window was opened: %d", stub.launched())
	}
	stub.release <- nil
	waitState(t, "93.184.216.34", humanCleared)

	// After it ended (cleared or not) the user can press it again at once.
	_, body3 := clearNow("93.184.216.34")
	if body3["alreadyWaiting"] != false {
		t.Errorf("after it ended: %v", body3)
	}
	stub.waitLaunches(t, 2)
	stub.release <- nil
}

func TestClearNowRefusesBadInput(t *testing.T) {
	newHumanStub(t)
	for name, domain := range map[string]string{
		"empty":                        "",
		"spaces":                       "   ",
		"loopback":                     "127.0.0.1",
		"private":                      "10.0.0.5",
		"link-local":                   "169.254.169.254",
		"pasted URL to a private host": "http://192.168.1.1/admin",
	} {
		if rec, _ := clearNow(domain); rec.Code != http.StatusBadRequest {
			t.Errorf("%s (%q): status %d, want 400", name, domain, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	DomainsClearHandler(rec, httptest.NewRequest(http.MethodPost, "/domains/clear", strings.NewReader("nope")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad body: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	DomainsClearHandler(rec, httptest.NewRequest(http.MethodGet, "/domains/clear", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}

func TestForgetRemovesTheClearanceTheRecordAndStopsAWaitingWindow(t *testing.T) {
	stub := newHumanStub(t)
	_ = recordCloudflareDomain("93.184.216.34", "x")
	_ = shared.StoreClearance(shared.Clearance{Domain: "93.184.216.34", Name: shared.ClearanceCookieName, Value: "V", Path: "/", ExpiresAt: time.Now().Add(time.Hour), UserAgent: "UA", Source: "human"})
	_ = shared.StoreClearance(shared.Clearance{Domain: "keep.example", Name: shared.ClearanceCookieName, Value: "V", Path: "/", ExpiresAt: time.Now().Add(time.Hour), UserAgent: "UA", Source: "human"})
	clearNow("93.184.216.34")
	waitState(t, "93.184.216.34", humanWaiting)
	stub.waitLaunches(t, 1)

	rec := httptest.NewRecorder()
	DomainsHandler(rec, httptest.NewRequest(http.MethodDelete, "/domains?domain=https://93.184.216.34/some/page", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE: %d %s", rec.Code, rec.Body.String())
	}
	domains := listViaHandler(t)
	if find(domains, "93.184.216.34") != nil {
		t.Errorf("a forgotten domain must be gone from the list: %+v", find(domains, "93.184.216.34"))
	}
	if find(domains, "keep.example") == nil {
		t.Error("other domains must stay")
	}
	// The waiting window was cancelled: the launcher's context ended.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && humanRequestSummary("93.184.216.34") != "" {
		time.Sleep(10 * time.Millisecond)
	}
	if humanRequestSummary("93.184.216.34") != "" {
		t.Error("the request must be gone")
	}
}

func TestDomainsHandlerRejectsBadRequests(t *testing.T) {
	newHumanStub(t)
	rec := httptest.NewRecorder()
	DomainsHandler(rec, httptest.NewRequest(http.MethodDelete, "/domains", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("DELETE without a domain: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	DomainsHandler(rec, httptest.NewRequest(http.MethodPost, "/domains", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}

func TestManifestDeclaresTheDomainsRoutesWithTheirScope(t *testing.T) {
	want := map[string]bool{"GET domains": false, "DELETE domains": false, "POST domains/clear": false}
	for _, r := range readManifest(t) {
		key := r["method"] + " " + r["path_suffix"]
		if _, ok := want[key]; ok {
			if r["required_scope"] != "tool:browser:crawl" {
				t.Errorf("%s: scope %q", key, r["required_scope"])
			}
			want[key] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("manifest.json does not declare %s", k)
		}
	}
}
