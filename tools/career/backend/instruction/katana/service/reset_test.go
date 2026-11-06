package service_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/instruction/katana/service"
	"career-tool-backend/portal"
)

const detailYAML = "fields:\n  - label: description\n    selector: .body\n"

func stateOf(t *testing.T, r *rig) (step string, raw string) {
	t.Helper()
	text, isErr := r.call("katana_get_instruction_state", map[string]any{})
	if isErr {
		t.Fatal(text)
	}
	var s struct {
		NextStep string `json:"nextStep"`
	}
	if err := json.Unmarshal([]byte(text), &s); err != nil {
		t.Fatal(err, text)
	}
	return s.NextStep, text
}

func TestTheStateFollowsTheWorkAndStartOverBeginsAgain(t *testing.T) {
	r := newRig(t)
	if step, _ := stateOf(t, r); step != service.StepJobListRule {
		t.Fatalf("a new link: %s", step)
	}

	// the whole job list recipe, then the state says the jobs are next, and what this session did
	r.call("katana_get_page_links", map[string]any{})
	r.call("katana_propose_link_rule", map[string]any{"jobUrls": []string{"board.example/j/a1", "board.example/j/b2"}})
	if step, raw := stateOf(t, r); step != service.StepJobListRule || !strings.Contains(raw, `"proposalReady":true`) {
		t.Fatalf("after a proposal: %s", raw)
	}
	r.call("katana_test_link_rule", map[string]any{})
	if _, raw := stateOf(t, r); !strings.Contains(raw, `"ruleTested":true`) {
		t.Fatalf("after the test: %s", raw)
	}
	r.call("katana_save_link_rule", map[string]any{})
	if step, raw := stateOf(t, r); step != service.StepCrawlJobs || !strings.Contains(raw, `"saved":true`) {
		t.Fatalf("rule saved: %s", raw)
	}

	// jobs crawled, then the job details saved
	items := []map[string]any{{"title": "Backend Developer", "url": "https://board.example/j/a1"}}
	if _, err := portal.IngestLinkCrawlResults(r.linkID, []portal.CrawlResultPage{{URL: listURL, Items: items}}); err != nil {
		t.Fatal(err)
	}
	if step, _ := stateOf(t, r); step != service.StepJobDetails {
		t.Fatalf("jobs crawled: %s", step)
	}
	if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(r.linkID, detailYAML); err != nil {
		t.Fatal(err)
	}
	if step, _ := stateOf(t, r); step != service.StepDone {
		t.Fatalf("both saved: %s", step)
	}

	// Rod's own instructions and a few attempts, flags and conversations that Start over must treat correctly
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_instructions = 'rod-yaml', listing_crawl_requested = 1, job_detail_instructions_ai_requested = 1, job_detail_crawl_requested = 1, instructions_ai_error = 'x', job_detail_instructions_ai_error = 'y' WHERE id = ?`, r.linkID); err != nil {
		t.Fatal(err)
	}
	for _, a := range []struct{ kind, conv string }{{"listing", "conv-listing"}, {"job_detail", "conv-detail"}} {
		if _, err := db.JobsDB.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, conversation_id, finished_at) VALUES (?, ?, ?, 'failed', ?, datetime('now'))`, "att-"+a.kind, r.linkID, a.kind, a.conv); err != nil {
			t.Fatal(err)
		}
	}

	// job details only: the rule stays, the listing attempt stays
	res, err := service.StartOver(r.linkID, service.ScopeJobDetails)
	if err != nil || res.RuleCleared || res.AttemptsRemoved != 1 || strings.Join(res.ConversationIDs, ",") != "conv-detail" {
		t.Fatalf("job details only: %+v %v", res, err)
	}
	if step, _ := stateOf(t, r); step != service.StepJobDetails {
		t.Fatalf("after resetting the job details: %s", step)
	}
	var attempts int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM instruction_attempts WHERE portal_link_id = ?`, r.linkID).Scan(&attempts)
	if attempts != 1 {
		t.Errorf("the listing attempt was removed: %d", attempts)
	}

	// everything: the rule goes, the AI starts from the beginning; jobs and Rod's instructions stay
	res, err = service.StartOver(r.linkID, service.ScopeAll)
	if err != nil || !res.RuleCleared || res.AttemptsRemoved != 1 || strings.Join(res.ConversationIDs, ",") != "conv-listing" {
		t.Fatalf("all: %+v %v", res, err)
	}
	if step, _ := stateOf(t, r); step != service.StepJobListRule {
		t.Fatalf("after starting over: %s", step)
	}
	var rod, flags, jobs int
	var errText *string
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM portal_links WHERE id = ? AND crawl_instructions = 'rod-yaml'`, r.linkID).Scan(&rod)
	_ = db.JobsDB.QueryRow(`SELECT listing_crawl_requested + job_detail_instructions_ai_requested + job_detail_crawl_requested FROM portal_links WHERE id = ?`, r.linkID).Scan(&flags)
	_ = db.JobsDB.QueryRow(`SELECT instructions_ai_error FROM portal_links WHERE id = ?`, r.linkID).Scan(&errText)
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE portal_link_id = ?`, r.linkID).Scan(&jobs)
	if rod != 1 {
		t.Error("Start over deleted the Rod instructions")
	}
	if flags != 0 || errText != nil {
		t.Errorf("flags %d error %v", flags, errText)
	}
	if jobs != 1 {
		t.Errorf("Start over deleted crawled jobs: %d", jobs)
	}
	if s, _ := portal.GetLinkCrawlSettings(r.linkID); s.Rules != nil {
		t.Error("the rule is still there")
	}
	// the AI session forgot its proposal, so the save needs a new test
	if text, isErr := r.call("katana_save_link_rule", map[string]any{}); !isErr {
		t.Errorf("the old proposal survived a reset: %s", text)
	}
}

func TestStartOverIsRefusedWhileSomethingRuns(t *testing.T) {
	r := newRig(t)
	if _, err := db.JobsDB.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status) VALUES ('run1', ?, 'listing', 'running')`, r.linkID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartOver(r.linkID, service.ScopeAll); err != portal.ErrGenerationRunning {
		t.Errorf("a running generation: %v", err)
	}
	_, _ = db.JobsDB.Exec(`DELETE FROM instruction_attempts`)
	if _, err := db.JobsDB.Exec(`INSERT INTO crawl_runs (id, portal_link_id, status) VALUES ('c1', ?, 'running')`, r.linkID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartOver(r.linkID, service.ScopeAll); err != service.ErrCrawlRunning {
		t.Errorf("a running crawl: %v", err)
	}
	if _, err := service.StartOver(r.linkID, "everything"); err != service.ErrBadScope {
		t.Errorf("a bad scope: %v", err)
	}
}

func TestTheStartOverRoute(t *testing.T) {
	r := newRig(t)
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/portal-links/katana/start-over", strings.NewReader(body))
		rec := httptest.NewRecorder()
		service.StartOverHandler(rec, req)
		return rec
	}
	if rec := post(`{"portalLinkId":"` + r.linkID + `","scope":"all"}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"conversationIds":[]`) {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if rec := post(`{"portalLinkId":"` + r.linkID + `","scope":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad scope: %d", rec.Code)
	}
	if rec := post(`{"portalLinkId":"nope","scope":"all"}`); rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Errorf("unknown link: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/portal-links/katana/start-over", nil)
	rec := httptest.NewRecorder()
	service.StartOverHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}
