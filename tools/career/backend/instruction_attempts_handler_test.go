package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

func setupHandlerDB(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
	if _, err := db.JobsDB.Exec(`INSERT INTO portals (id, name) VALUES ('p1', 'P')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://x')`); err != nil {
		t.Fatal(err)
	}
}

func postJSON(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response %q is not JSON: %v", rec.Body.String(), err)
	}
	return out
}

func TestInstructionAttemptStartAndFinishFlow(t *testing.T) {
	setupHandlerDB(t)

	rec := postJSON(instructionAttemptStartHandler, `{"portalLinkId":"l1","kind":"listing","conversationId":"c1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	first := decode(t, rec)
	if first["started"] != true || first["attemptId"] == "" {
		t.Fatalf("start response: %v", first)
	}
	var trig string
	if err := db.JobsDB.QueryRow(`SELECT triggered_by FROM instruction_attempts`).Scan(&trig); err != nil || trig != "manual" {
		t.Fatalf("triggeredBy must default to manual, got %q (%v)", trig, err)
	}

	again := decode(t, postJSON(instructionAttemptStartHandler, `{"portalLinkId":"l1","kind":"listing","triggeredBy":"auto"}`))
	if again["started"] != false || again["attemptId"] != first["attemptId"] {
		t.Fatalf("second start must be idempotent: %v", again)
	}

	rec = postJSON(instructionAttemptFinishHandler, `{"portalLinkId":"l1","kind":"listing","error":"it broke"}`)
	if rec.Code != http.StatusOK || decode(t, rec)["status"] != "failed" {
		t.Fatalf("finish without save: %d %s", rec.Code, rec.Body.String())
	}
	if got := decode(t, postJSON(instructionAttemptFinishHandler, `{"portalLinkId":"l1","kind":"listing"}`))["status"]; got != "none" {
		t.Fatalf("second finish must be none, got %v", got)
	}
}

func TestInstructionAttemptFinishSucceedsOnlyAfterServerSideSave(t *testing.T) {
	setupHandlerDB(t)
	postJSON(instructionAttemptStartHandler, `{"portalLinkId":"l1","kind":"job_detail","triggeredBy":"auto"}`)

	if err := portal.MarkInstructionSaved("l1", portal.InstructionKindJobDetail); err != nil {
		t.Fatal(err)
	}
	// A client cannot claim success or failure itself: it only sends an error text.
	got := decode(t, postJSON(instructionAttemptFinishHandler, `{"portalLinkId":"l1","kind":"job_detail","error":"ignored"}`))["status"]
	if got != "succeeded" {
		t.Fatalf("want succeeded, got %v", got)
	}
}

func TestInstructionAttemptHandlersRejectBadInput(t *testing.T) {
	setupHandlerDB(t)

	cases := []struct {
		name    string
		handler http.HandlerFunc
		body    string
		want    int
	}{
		{"start: no link id", instructionAttemptStartHandler, `{"kind":"listing"}`, http.StatusBadRequest},
		{"start: not JSON", instructionAttemptStartHandler, `nope`, http.StatusBadRequest},
		{"start: bad kind", instructionAttemptStartHandler, `{"portalLinkId":"l1","kind":"x"}`, http.StatusBadRequest},
		{"start: missing kind", instructionAttemptStartHandler, `{"portalLinkId":"l1"}`, http.StatusBadRequest},
		{"start: bad trigger", instructionAttemptStartHandler, `{"portalLinkId":"l1","kind":"listing","triggeredBy":"robot"}`, http.StatusBadRequest},
		{"start: unknown link", instructionAttemptStartHandler, `{"portalLinkId":"zzz","kind":"listing"}`, http.StatusBadRequest},
		{"finish: no link id", instructionAttemptFinishHandler, `{"kind":"listing"}`, http.StatusBadRequest},
		{"finish: bad kind", instructionAttemptFinishHandler, `{"portalLinkId":"l1","kind":"x"}`, http.StatusBadRequest},
		{"finish: unknown link", instructionAttemptFinishHandler, `{"portalLinkId":"zzz","kind":"listing"}`, http.StatusBadRequest},
		{"start: oversized body", instructionAttemptStartHandler, `{"portalLinkId":"l1","kind":"listing","conversationId":"` + strings.Repeat("a", 70<<10) + `"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		if rec := postJSON(c.handler, c.body); rec.Code != c.want {
			t.Errorf("%s: got %d (%s), want %d", c.name, rec.Code, strings.TrimSpace(rec.Body.String()), c.want)
		}
	}

	for name, h := range map[string]http.HandlerFunc{"start": instructionAttemptStartHandler, "finish": instructionAttemptFinishHandler} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s GET: got %d, want 405", name, rec.Code)
		}
	}

	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM instruction_attempts`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rejected requests must not create attempts, found %d (%v)", n, err)
	}
}

// TestManifestDeclaresInstructionAttemptRoutes guards the scope gate:
// every non-public route must be declared with a required scope.
func TestManifestDeclaresInstructionAttemptRoutes(t *testing.T) {
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Routes []map[string]string `json:"routes"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"portal-links/instruction-attempts/start", "portal-links/instruction-attempts/finish"} {
		found := false
		for _, r := range parsed.Routes {
			if r["path_suffix"] == suffix {
				found = true
				if r["method"] != "POST" || r["required_scope"] != "tool:career:portals" {
					t.Errorf("%s: method=%q scope=%q", suffix, r["method"], r["required_scope"])
				}
			}
		}
		if !found {
			t.Errorf("manifest.json does not declare %s", suffix)
		}
	}
}

func putPortalLink(body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	portalLinksHandler(rec, req)
	return rec
}

func TestPutPortalLinkEmptyCrawlAcknowledgement(t *testing.T) {
	setupHandlerDB(t)
	if _, err := db.JobsDB.Exec(`INSERT INTO crawl_runs (id, portal_link_id, kind, status, finished_at, triggered_by, jobs_saved, jobs_updated, jobs_skipped, pages_visited, stopped_reason, not_found_fields)
		VALUES ('r1', 'l1', 'listing', 'completed', '2026-01-01T00:00:00Z', 'manual', 0, 0, 0, 1, 'no_next_link', '[]')`); err != nil {
		t.Fatal(err)
	}

	ack := func() string {
		portals, err := portal.ListPortals()
		if err != nil || len(portals) == 0 || len(portals[0].Links) == 0 || portals[0].Links[0].LastListingCrawl == nil {
			t.Fatalf("no summary: %v", err)
		}
		return portals[0].Links[0].LastListingCrawl.AcknowledgedAt
	}

	if rec := putPortalLink(`{"id":"l1","emptyCrawlAcknowledged":true}`); rec.Code != http.StatusOK {
		t.Fatalf("acknowledge: %d %s", rec.Code, rec.Body.String())
	}
	if ack() == "" {
		t.Fatal("acknowledging must set the timestamp")
	}
	// The response is the refreshed list, carrying the summary.
	var resp struct {
		Portals []struct {
			Links []struct {
				LastListingCrawl *struct {
					AcknowledgedAt string `json:"acknowledgedAt"`
				} `json:"lastListingCrawl"`
			} `json:"links"`
		} `json:"portals"`
	}
	rec := putPortalLink(`{"id":"l1","emptyCrawlAcknowledged":true}`)
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Portals[0].Links[0].LastListingCrawl == nil || resp.Portals[0].Links[0].LastListingCrawl.AcknowledgedAt == "" {
		t.Fatalf("response must carry the summary: %s (%v)", rec.Body.String(), err)
	}

	if rec := putPortalLink(`{"id":"l1","emptyCrawlAcknowledged":false}`); rec.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body.String())
	}
	if ack() != "" {
		t.Fatal("false must clear the acknowledgement")
	}

	if rec := putPortalLink(`{"id":"zzz","emptyCrawlAcknowledged":true}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown link: %d", rec.Code)
	}
	if rec := putPortalLink(`{"id":"l1"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a PUT with nothing to update must still be rejected: %d", rec.Code)
	}
}
