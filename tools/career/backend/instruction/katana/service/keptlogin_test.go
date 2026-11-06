package service_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

const keptSession = "0123456789abcdef0123456789abcdef"

// keptLoginRig is a rig whose link needs a login and holds a kept login (the id only; the cookies live in the Browser tool).
func keptLoginRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	yes := true
	if err := portal.UpdatePortalLinkAutoDiscoveryEnabled(r.linkID, false); err != nil {
		t.Fatal(err)
	}
	if err := portal.UpdatePortalLinkLogin(r.linkID, &yes, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := portal.SetLinkLoginSession(r.linkID, keptSession); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *rig) stateOf() (step, reason string, raw map[string]any) {
	r.t.Helper()
	text, isErr := r.call("katana_get_instruction_state", map[string]any{})
	if isErr {
		r.t.Fatal(text)
	}
	var st struct {
		NextStep string `json:"nextStep"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(text), &st); err != nil {
		r.t.Fatal(err, text)
	}
	_ = json.Unmarshal([]byte(text), &raw)
	return st.NextStep, st.Reason, raw
}

// A login link with a kept login is instructed like any link: the state says so, every page the AI loads carries the kept login, and the
// AI never sees it.
func TestTheJobListRecipeOnALoginLinkWithAKeptLogin(t *testing.T) {
	r := keptLoginRig(t)
	if _, _, err := portal.StartInstructionAttempt(r.linkID, portal.InstructionKindListing, portal.InstructionTriggerManual, "conv"); err != nil {
		t.Fatal(err)
	}
	step, _, raw := r.stateOf()
	if step != "job_list_rule" || raw["loginKept"] != true || raw["login"] != false {
		t.Fatalf("state of a kept-login link: %s %v", step, raw)
	}

	text, isErr := r.call("katana_get_page_links", map[string]any{})
	if isErr || strings.Contains(text, keptSession) {
		t.Fatalf("page links: %v %q", isErr, text)
	}
	var d struct {
		Groups []struct{ Samples []struct{ URL string } } `json:"groups"`
		Next   []string                                   `json:"nextPageCandidates"`
	}
	if err := json.Unmarshal([]byte(text), &d); err != nil {
		t.Fatal(err)
	}
	var picks []string
	for _, g := range d.Groups {
		for _, s := range g.Samples {
			if strings.Contains(s.URL, "/j/") {
				picks = append(picks, s.URL)
			}
		}
	}
	if text, isErr := r.call("katana_propose_link_rule", map[string]any{"jobUrls": picks, "nextPageUrls": d.Next}); isErr {
		t.Fatal(text)
	}
	if text, isErr := r.call("katana_test_link_rule", map[string]any{}); isErr || strings.Contains(text, keptSession) {
		t.Fatalf("test: %v %q", isErr, text)
	}
	if text, isErr := r.call("katana_save_link_rule", map[string]any{}); isErr {
		t.Fatalf("save: %v %q", isErr, text)
	}
	if s, _ := portal.GetLinkCrawlSettings(r.linkID); s.Rules == nil {
		t.Error("the rule of a kept-login link was not saved")
	}

	r.core.mu.Lock()
	defer r.core.mu.Unlock()
	if len(r.core.sessions) < 3 {
		t.Fatalf("expected the page, the test pages: %v", r.core.fetches)
	}
	for i, id := range r.core.sessions {
		if id != keptSession {
			t.Errorf("fetch %d (%s) carried %q, want the kept login", i, r.core.fetches[i], id)
		}
	}
}

// When the site stops accepting the kept login the AI is told to STOP, the link is marked, and nothing is tried again.
func TestAnExpiredKeptLoginStopsTheAIAndIsNotTriedAgain(t *testing.T) {
	r := keptLoginRig(t)
	r.core.mu.Lock()
	r.core.loginExpired = true
	r.core.mu.Unlock()

	text, isErr := r.call("katana_get_page_links", map[string]any{})
	if !isErr || !strings.Contains(text, "STOP") || !strings.Contains(text, "log in again") || strings.Contains(text, keptSession) {
		t.Fatalf("expired answer: %v %q", isErr, text)
	}
	step, reason, raw := r.stateOf()
	if step != "stop_login" || raw["loginExpired"] != true || !strings.Contains(reason, "expired") {
		t.Errorf("state after expiry: %s %q %v", step, reason, raw)
	}
	r.core.mu.Lock()
	n := len(r.core.fetches)
	r.core.mu.Unlock()
	text, isErr = r.call("katana_get_page_links", map[string]any{})
	if !isErr || !strings.Contains(text, "expired") {
		t.Errorf("second call: %v %q", isErr, text)
	}
	r.core.mu.Lock()
	defer r.core.mu.Unlock()
	if len(r.core.fetches) != n {
		t.Error("an expired login must not reach the Browser again")
	}
}

// A login link with no kept login is refused as before, and says what to do.
func TestALoginLinkWithoutAKeptLoginStillStopsTheAI(t *testing.T) {
	r := newRig(t)
	yes := true
	_ = portal.UpdatePortalLinkAutoDiscoveryEnabled(r.linkID, false)
	if err := portal.UpdatePortalLinkLogin(r.linkID, &yes, nil); err != nil {
		t.Fatal(err)
	}
	text, isErr := r.call("katana_get_page_links", map[string]any{})
	if !isErr || !strings.Contains(text, "none is kept") {
		t.Errorf("%v %q", isErr, text)
	}
	if step, _, _ := r.stateOf(); step != "stop_login" {
		t.Errorf("state: %s", step)
	}
	if len(r.core.fetches) != 0 {
		t.Error("the Browser must not be asked")
	}
}

// The job-detail recipe on a login link with a kept login: the page analysis and the selector crawls carry the kept login too.
func TestTheJobDetailRecipeOnALoginLinkWithAKeptLogin(t *testing.T) {
	r := keptLoginRig(t)
	r.core.detail = detailPage
	long := "<p>" + strings.Repeat("A real sentence of the posting. ", 30) + "</p>"
	r.core.extract = extractor(map[string]string{"title": "Posting A1", "company": "Acme", "postedAt": "2026-10-28T09:00:00+00:00", "description": long})
	r.crawledJobs()
	if _, _, err := portal.StartInstructionAttempt(r.linkID, portal.InstructionKindJobDetail, portal.InstructionTriggerManual, "conv"); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"katana_get_job_page_candidates", "katana_test_job_detail_picks", "katana_save_job_detail_picks"} {
		if text, isErr := r.call(tool, map[string]any{}); isErr || strings.Contains(text, keptSession) {
			t.Fatalf("%s: %v %q", tool, isErr, text)
		}
	}
	if stored, _ := portal.GetPortalLinkJobDetailCrawlInstructions(r.linkID); !strings.Contains(stored, "#job-description") {
		t.Errorf("the picks of a kept-login link were not saved: %q", stored)
	}
	r.core.mu.Lock()
	defer r.core.mu.Unlock()
	if len(r.core.detailSessions) == 0 || len(r.core.extractSessions) == 0 {
		t.Fatalf("detail %v extract %v", r.core.detailSessions, r.core.extractSessions)
	}
	for _, id := range append(append([]string{}, r.core.detailSessions...), r.core.extractSessions...) {
		if id != keptSession {
			t.Errorf("a Browser call carried %q instead of the kept login", id)
		}
	}
}

// katana_crawl_jobs on a login link with a kept login: the crawl runs without a window and every page it loads carries the kept login.
func TestTheAICrawlsTheJobsOfALoginLinkWithAKeptLogin(t *testing.T) {
	r := keptLoginRig(t)
	r.call("katana_get_page_links", map[string]any{})
	r.call("katana_propose_link_rule", map[string]any{"jobUrls": []string{"board.example/j/a1", "board.example/j/b2"}})
	r.call("katana_test_link_rule", map[string]any{})
	if text, isErr := r.call("katana_save_link_rule", map[string]any{}); isErr {
		t.Fatal(text)
	}
	r.core.mu.Lock()
	before := len(r.core.sessions)
	r.core.mu.Unlock()

	text, isErr := r.call("katana_crawl_jobs", map[string]any{})
	if isErr || !strings.Contains(text, `"completed"`) || strings.Contains(text, keptSession) {
		t.Fatalf("crawl: %v %q", isErr, text)
	}
	r.core.mu.Lock()
	defer r.core.mu.Unlock()
	if len(r.core.sessions) <= before {
		t.Fatal("the crawl loaded no page")
	}
	for i, id := range r.core.sessions[before:] {
		if id != keptSession {
			t.Errorf("crawl page %d carried %q", i, id)
		}
	}
}

// plan 197, step 1: a link set to use Rod is not read by the Katana AI tools, whatever the Browser selected.
func TestTheKatanaToolsRefuseALinkSetToRod(t *testing.T) {
	r := newRig(t)
	if err := portal.SetLinkCrawlEngine(r.linkID, portal.LinkEngineRod); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"katana_get_page_links", "katana_propose_link_rule", "katana_test_link_rule", "katana_save_link_rule", "katana_crawl_jobs"} {
		args := map[string]any{}
		if tool == "katana_propose_link_rule" {
			args["jobUrls"] = []string{"board.example/j/a1", "board.example/j/b2"}
		}
		text, isErr := r.call(tool, args)
		if !isErr || !strings.Contains(text, "Rod") {
			t.Errorf("%s on a Rod link: %v %q", tool, isErr, text)
		}
	}
	if len(r.core.fetches) != 0 {
		t.Errorf("the Browser must not be asked for a link on Rod: %v", r.core.fetches)
	}
	// back to the Browser's engine: Katana reads it again
	if err := portal.SetLinkCrawlEngine(r.linkID, ""); err != nil {
		t.Fatal(err)
	}
	if text, isErr := r.call("katana_get_page_links", map[string]any{}); isErr {
		t.Errorf("after going back to the Browser: %q", text)
	}
}

// plan 197, step 4: a block is recorded, the AI is told to stop for a while and that Rod is the person's choice, and a read that goes through
// (or an old block) lets it work again.
func TestABlockedPageStopsTheAIForAWhileAndOffersRod(t *testing.T) {
	r := newRig(t)
	r.core.status[listURL] = http.StatusBadGateway
	r.core.mu.Lock()
	r.core.blockCode = "cloudflare_challenge_unresolved"
	r.core.mu.Unlock()
	text, isErr := r.call("katana_get_page_links", map[string]any{})
	if !isErr || !strings.Contains(text, "STOP") || !strings.Contains(text, "Rod can read this link") {
		t.Fatalf("the blocked page: %v %q", isErr, text)
	}
	step, reason, raw := r.stateOf()
	if step != "stop_blocked" || raw["blocked"] != "cloudflare_challenge_unresolved" || !strings.Contains(reason, "Rod") {
		t.Errorf("state: %s %q %v", step, reason, raw)
	}
	// an hour later the AI may try again; the page loads and the block is forgotten
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET engine_block_at = datetime('now', '-2 hours') WHERE id = ?`, r.linkID); err != nil {
		t.Fatal(err)
	}
	if step, _, _ := r.stateOf(); step == "stop_blocked" {
		t.Error("an old block must not stop the AI for ever (it could never clear it)")
	}
	delete(r.core.status, listURL)
	if text, isErr := r.call("katana_get_page_links", map[string]any{}); isErr {
		t.Fatalf("the good read: %q", text)
	}
	if _, recent := portal.EngineBlockRecent(r.linkID, 365*24*time.Hour); recent {
		t.Error("a read that went through must forget the block")
	}
	var code string
	if err := db.JobsDB.QueryRow(`SELECT engine_block_code FROM portal_links WHERE id = ?`, r.linkID).Scan(&code); err != nil || code != "" {
		t.Errorf("block code after a good read: %q %v", code, err)
	}
}
