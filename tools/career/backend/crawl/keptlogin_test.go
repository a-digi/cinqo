package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/portal"
	"career-tool-backend/portal/linkrule"
)

const keptSession = "0123456789abcdef0123456789abcdef"

// keptLoginLink is a link that needs a login and holds a kept login (the id only; the cookies live in the Browser tool).
func keptLoginLink(t *testing.T) string {
	t.Helper()
	id, _ := loginLinkSetup(t, true, "")
	if _, err := portal.SetLinkLoginSession(id, keptSession); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAKeptLoginMakesALoginLinkUsableByTheLinkCrawler(t *testing.T) {
	// without a kept login: refused before the Browser is asked, as before
	plain, _ := loginLinkSetup(t, true, "")
	f := newFakeCore(t, "katana")
	if code, _ := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+plain+`"}`, true); code != http.StatusBadRequest || f.fetchCount() != 0 {
		t.Fatalf("no kept login: %d fetches=%d", code, f.fetchCount())
	}

	id := keptLoginLink(t)
	f = newFakeCore(t, "katana")
	code, body := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true)
	if code != http.StatusOK {
		t.Fatalf("with a kept login: %d %s", code, body)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.fetches) != 1 || f.fetches[0]["loginSessionId"] != keptSession {
		t.Errorf("the Browser must be asked with the kept login: %v", f.fetches)
	}
	if strings.Contains(body, keptSession) {
		t.Error("the answer to the page must never carry the session id")
	}
}

func TestPagesOfALinkWithoutALoginNeverCarryASession(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "katana")
	if code, _ := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true); code != 200 {
		t.Fatal(code)
	}
	if _, has := f.fetches[0]["loginSessionId"]; has {
		t.Errorf("a link that needs no login must not name a session: %v", f.fetches[0])
	}
}

func TestLoginExpiredFromTheBrowserMarksTheLinkAndStopsTryingTheSession(t *testing.T) {
	id := keptLoginLink(t)
	f := newFakeCore(t, "katana")
	f.failFor = "https://shop.example/new-search"
	// the Browser tool says the site no longer accepts the kept login
	f.mu.Lock()
	f.failBody = `{"code":"login_expired","message":"The kept login no longer works","reason":"the page shows a login form"}`
	f.mu.Unlock()
	code, _ := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true)
	if code == http.StatusOK {
		t.Fatal("an expired login must not look like a success")
	}
	if got := linkLoginLabel(t, id); got != portal.LoginSessionExpired {
		t.Errorf("the link must show the expired login, got %q", got)
	}
	n := f.fetchCount()
	if code, _ := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true); code != http.StatusBadRequest || f.fetchCount() != n {
		t.Errorf("an expired login must not be tried again: %d, fetches %d -> %d", code, n, f.fetchCount())
	}
	// logging in again and keeping the login makes it usable again
	if _, err := portal.SetLinkLoginSession(id, "fedcba9876543210fedcba9876543210"); err != nil {
		t.Fatal(err)
	}
	if got := linkLoginLabel(t, id); got != portal.LoginSessionValid {
		t.Errorf("a new kept login must reset the state, got %q", got)
	}
}

func linkLoginLabel(t *testing.T, linkID string) string {
	t.Helper()
	portals, err := portal.ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range portals {
		for _, l := range p.Links {
			if l.ID == linkID {
				return l.LoginSession
			}
		}
	}
	t.Fatal("link not listed")
	return ""
}

func TestLinkCrawlSelectedForALoginLinkNeedsAKeptLoginAndKatana(t *testing.T) {
	ctx := context.Background()
	id := keptLoginLink(t)
	rule := linkrule.Rule{Host: "shop.example", Include: []string{"shop.example/jobs/*"}}
	if err := portal.SaveLinkCrawlSettings(id, portal.CrawlMethodLinks, &rule, false); err != nil {
		t.Fatalf("a rule for a login link with a kept login: %v", err)
	}
	stubEngine(t, "katana")
	if on, err := linkCrawlSelected(ctx, "http://core", "tok", id); err != nil || !on {
		t.Errorf("Katana + kept login + rule: %v %v", on, err)
	}
	stubEngine(t, "rod")
	if on, err := linkCrawlSelected(ctx, "http://core", "tok", id); err != nil || on {
		t.Errorf("Rod keeps the instructions and the window: %v %v", on, err)
	}
	stubEngine(t, "katana")
	if err := portal.MarkLoginSessionExpired(id); err != nil {
		t.Fatal(err)
	}
	if on, err := linkCrawlSelected(ctx, "http://core", "tok", id); err != nil || on {
		t.Errorf("an expired login is not used: %v %v", on, err)
	}
	if err := checkLinkCrawlStart(ctx, "http://core", "tok", id); !errors.Is(err, portal.ErrLinkRulesLogin) {
		t.Errorf("an expired login must not start the link crawler: %v", err)
	}

	// a login link without any kept login cannot store a rule
	plain, _ := loginLinkSetup(t, true, "")
	if err := portal.SaveLinkCrawlSettings(plain, portal.CrawlMethodLinks, &rule, false); !errors.Is(err, portal.ErrLinkRulesLogin) {
		t.Errorf("no kept login: %v", err)
	}
}

func stubEngine(t *testing.T, engine string) {
	t.Helper()
	orig := readBrowserEngine
	readBrowserEngine = func(context.Context, string, string) (string, error) { return engine, nil }
	t.Cleanup(func() { readBrowserEngine = orig })
}

// --- job details with a kept login (plan 195, step 4) ---

func login502(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message, "reason": "the page shows a login form"})
}

func TestJobDetailRunWithAKeptLoginOnKatanaNeverOpensAWindowAndSendsTheSession(t *testing.T) {
	fastLogin(t)
	linkID := keptLoginLink(t)
	targets := seedJobs(t, linkID, 2)
	sb := &scriptedBrowser{engine: "katana", crawl: func(_ int, w http.ResponseWriter) { writeJSON(w, detailPage(strings.Repeat("d", 150))) }}
	startScripted(t, sb)
	runID := startRun(t, linkID, "job_detail")
	runJobDetailCrawlNow(context.Background(), runID, linkID, targets, loginToken)

	if sb.count("open") != 0 || sb.count("status") != 0 || sb.count("crawl") != 2 {
		t.Fatalf("no window may open: %v", sb.seen())
	}
	for i, c := range sb.crawls {
		if c["loginSessionId"] != keptSession || c["useLoginWindow"] == true {
			t.Errorf("crawl %d body: %v", i, c)
		}
	}
	if r := readRun(t, runID); r.status != "completed" {
		t.Errorf("run = %+v", r)
	}
}

func TestJobDetailRunWithAKeptLoginOnRodStillUsesTheWindow(t *testing.T) {
	fastLogin(t)
	linkID := keptLoginLink(t)
	targets := seedJobs(t, linkID, 1)
	sb := &scriptedBrowser{
		engine: "rod",
		status: func(int) (int, string, string) { return 0, "confirmed", "" },
		crawl:  func(_ int, w http.ResponseWriter) { writeJSON(w, detailPage(strings.Repeat("d", 150))) },
	}
	startScripted(t, sb)
	runID := startRun(t, linkID, "job_detail")
	runJobDetailCrawlNow(context.Background(), runID, linkID, targets, loginToken)
	if sb.count("open") != 1 {
		t.Fatalf("Rod keeps the window: %v", sb.seen())
	}
	if _, has := sb.crawls[0]["loginSessionId"]; has {
		t.Errorf("Rod must never be sent a session: %v", sb.crawls[0])
	}
}

func TestAnExpiredKeptLoginStopsTheJobDetailRunAndMarksTheLink(t *testing.T) {
	fastLogin(t)
	linkID := keptLoginLink(t)
	targets := seedJobs(t, linkID, 3)
	sb := &scriptedBrowser{engine: "katana", crawl: func(_ int, w http.ResponseWriter) {
		login502(w, "login_expired", "The kept login no longer works: log in again (and keep the login) to crawl this link")
	}}
	startScripted(t, sb)
	runID := startRun(t, linkID, "job_detail")
	runJobDetailCrawlNow(context.Background(), runID, linkID, targets, loginToken)

	if sb.count("crawl") != 1 {
		t.Errorf("every other job would meet the same login page: %v", sb.seen())
	}
	r := readRun(t, runID)
	if r.status != "failed" || !strings.Contains(r.errMsg, "kept login no longer works") || strings.Contains(strings.ToLower(r.errMsg), "cloudflare") {
		t.Errorf("run = %+v", r)
	}
	if got := linkLoginLabel(t, linkID); got != portal.LoginSessionExpired {
		t.Errorf("the link must show the expired login: %q", got)
	}
	// and the job was not marked as a failed detail crawl of its own
	var failed int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE portal_link_id = ? AND detail_crawl_status = 'failed'`, linkID).Scan(&failed); err != nil || failed != 0 {
		t.Errorf("%d job(s) were marked failed for a login problem (%v)", failed, err)
	}
}

func TestSingleJobRecrawlOfAKeptLoginLinkIsAllowedOnKatanaOnly(t *testing.T) {
	linkID := keptLoginLink(t)
	sb := &scriptedBrowser{engine: "katana", crawl: func(_ int, w http.ResponseWriter) { writeJSON(w, detailPage(strings.Repeat("d", 150))) }}
	startScripted(t, sb)
	targets := seedJobs(t, linkID, 1)
	_, err := RecrawlOneJob(context.Background(), jobs.RecrawlTarget{ID: targets[0].ID, SourceURL: targets[0].SourceURL, PortalLinkID: linkID}, loginToken)
	if err != nil {
		t.Fatalf("katana + kept login: %v", err)
	}
	if sb.count("crawl") != 1 || sb.crawls[0]["loginSessionId"] != keptSession {
		t.Errorf("the session must be sent: %v %v", sb.seen(), sb.crawls)
	}

	rod := &scriptedBrowser{engine: "rod", crawl: func(_ int, w http.ResponseWriter) { writeJSON(w, detailPage("x")) }}
	startScripted(t, rod)
	_, err = RecrawlOneJob(context.Background(), jobs.RecrawlTarget{ID: "j", SourceURL: "https://shop.example/job/1", PortalLinkID: linkID}, loginToken)
	var np *NotPossibleError
	if !errors.As(err, &np) || rod.count("crawl") != 0 {
		t.Errorf("Rod must refuse a login link: %v %v", err, rod.seen())
	}
}
