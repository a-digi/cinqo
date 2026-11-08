package crawl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/portal"
	"career-tool-backend/portal/linkrule"
)

const (
	listingURL = "https://board.example/jobs"
	pageOneURL = listingURL
	pageTwoURL = "https://board.example/jobs?page=2"
	pageTre    = "https://board.example/jobs?page=3"
)

// titles are letters only: the ingest strips digits from a title, and two titles that differ by digits alone would be one title
var boardTitles = map[string]string{"1": "Backend Developer", "2": "Frontend Developer", "3": "Data Engineer", "4": "QA Engineer", "5": "Product Manager", "6": "Site Reliability Engineer"}

// boardPage is a page of job links: an id with a known title, "!id" for a link without text, "id=Title" for a given title.
func boardPage(jobs ...string) string {
	var parts []string
	for _, j := range jobs {
		switch {
		case strings.HasPrefix(j, "!"):
			parts = append(parts, `{"url":"https://board.example/j/`+j[1:]+`"}`)
		case strings.Contains(j, "="):
			id, title, _ := strings.Cut(j, "=")
			parts = append(parts, `{"url":"https://board.example/j/`+id+`","texts":["`+title+`"]}`)
		default:
			parts = append(parts, `{"url":"https://board.example/j/`+j+`","texts":["`+boardTitles[j]+`"]}`)
		}
	}
	return `{"finalUrl":"","title":"Board","links":[` + strings.Join(parts, ",") + `]}`
}

var boardRule = linkrule.Rule{Host: "board.example", Include: []string{"board.example/j/*"},
	Pagination: &linkrule.Pagination{Kind: linkrule.PagesTemplate, Template: "https://board.example/jobs?page={n}", Second: 2, MaxPages: 5}}

// linkCrawlLink makes a link on the method "links" with boardRule saved.
func linkCrawlLink(t *testing.T) string {
	t.Helper()
	id := linkWithHost(t, listingURL)
	rule := boardRule
	if err := portal.SaveLinkCrawlSettings(id, portal.CrawlMethodLinks, &rule, false); err != nil {
		t.Fatal(err)
	}
	return id
}

func waitRun(t *testing.T, linkID string) (status, summary, errMsg, log string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		run, err := findMostRecentCrawlRun(linkID)
		if err == nil && run.Status != "running" {
			return run.Status, deref(run.ResultSummary), deref(run.ErrorMessage), run.Log
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the run did not finish")
	return
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func jobsOf(t *testing.T, linkID string) []string {
	t.Helper()
	rows, err := db.JobsDB.Query(`SELECT title FROM jobs WHERE portal_link_id = ? ORDER BY title`, linkID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			t.Fatal(err)
		}
		out = append(out, title)
	}
	return out
}

func runRowOf(t *testing.T, linkID string) (saved, updated, skipped, pages int, stopped string) {
	t.Helper()
	run, err := findMostRecentCrawlRun(linkID) // the same notion of "latest" as the screen's
	if err != nil {
		t.Fatal(err)
	}
	if err := db.JobsDB.QueryRow(`SELECT jobs_saved, jobs_updated, jobs_skipped, pages_visited, stopped_reason FROM crawl_runs WHERE id = ?`, run.ID).
		Scan(&saved, &updated, &skipped, &pages, &stopped); err != nil {
		t.Fatal(err)
	}
	return
}

func crawlNow(t *testing.T, linkID string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/portal-links/crawl-now", strings.NewReader(`{"portalLinkId":"`+linkID+`"}`))
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok-7"})
	rec := httptest.NewRecorder()
	CrawlNowHandler(rec, req)
	return rec.Code, rec.Body.String()
}

func threePageBoard(f *fakeCore) {
	f.fetchBodyFor = map[string]string{
		pageOneURL: boardPage("1", "2", "3"),
		pageTwoURL: boardPage("4", "5"),
		pageTre:    boardPage("6"),
	}
}

func TestCrawlNowCrawlsALinkOnTheLinkMethodAcrossItsPages(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "katana")
	threePageBoard(f)

	code, resp := crawlNow(t, id)
	if code != http.StatusAccepted {
		t.Fatalf("%d %s", code, resp)
	}
	var started struct {
		CrawlRunID string `json:"crawlRunId"`
	}
	_ = json.Unmarshal([]byte(resp), &started)
	status, summary, errMsg, log := waitRun(t, id)
	if status != "completed" || errMsg != "" {
		t.Fatalf("%s %s %s", status, summary, errMsg)
	}
	want := []string{"Backend Developer", "Data Engineer", "Frontend Developer", "Product Manager", "QA Engineer", "Site Reliability Engineer"}
	if got := jobsOf(t, id); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("jobs %v", got)
	}
	saved, updated, skipped, pages, stopped := runRowOf(t, id)
	// page 3 still had a job, so page 4 was asked for; the board has none, which ends the run
	if saved != 6 || updated != 0 || skipped != 0 || pages != 4 || stopped != linkrule.StopNoNewJobs {
		t.Errorf("run row: saved=%d updated=%d skipped=%d pages=%d stopped=%q", saved, updated, skipped, pages, stopped)
	}
	if !strings.Contains(summary, "Saved 6 new") || !strings.Contains(summary, "visited 4 page(s)") {
		t.Errorf("summary %q", summary)
	}
	if !strings.Contains(log, "page 1 loaded") || !strings.Contains(log, "page 3 loaded") {
		t.Errorf("log %q", log)
	}
	// every page was asked with the run's id (Stop and phases reach it), the portal's pacing key and the person's token
	pid, _ := portal.GetPortalIDForLink(id)
	if len(f.fetches) != 4 {
		t.Errorf("pages asked for: %d, want 4", len(f.fetches))
	}
	for _, fe := range f.fetches {
		if fe["requestId"] != started.CrawlRunID || fe["rateLimitKey"] != pid {
			t.Errorf("a page was asked with %v, want requestId %s and rateLimitKey %s", fe, started.CrawlRunID, pid)
		}
	}
	for _, a := range f.auths {
		if a != "Bearer tok-7" {
			t.Errorf("auth %q", a)
		}
	}
	// the selector path was never used
	for _, p := range f.paths {
		if strings.HasSuffix(p, "/crawl-paginated") {
			t.Error("a link-crawler run called the selector crawl")
		}
	}
}

func TestCrawlingAgainUpdatesAndNeverDuplicates(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "katana")
	threePageBoard(f)
	for i := 0; i < 2; i++ {
		if code, resp := crawlNow(t, id); code != http.StatusAccepted {
			t.Fatalf("%d %s", code, resp)
		}
		if status, _, errMsg, _ := waitRun(t, id); status != "completed" {
			t.Fatalf("run %d: %s %s", i+1, status, errMsg)
		}
	}
	if got := jobsOf(t, id); len(got) != 6 {
		t.Errorf("jobs after two crawls: %v", got)
	}
	saved, updated, _, _, _ := runRowOf(t, id)
	if saved != 0 || updated != 6 {
		t.Errorf("second run: saved=%d updated=%d, want 0 and 6", saved, updated)
	}
}

// Plan 190: the engine decides the method. With Rod a link that has both a rule and AI instructions is crawled with its AI instructions
// (never with the rule); an engine that cannot be read stops the crawl, never a guess.
func TestRodCrawlsWithTheAIInstructionsNeverWithTheRule(t *testing.T) {
	id := linkCrawlLink(t)
	if err := portal.UpdatePortalLinkCrawlInstructions(id, trialListingA); err != nil {
		t.Fatal(err)
	}
	f := newFakeCore(t, "rod")
	threePageBoard(f)

	code, resp := crawlNow(t, id)
	if code != http.StatusAccepted {
		t.Fatalf("%d %s", code, resp)
	}
	waitRun(t, id) // the scripted core has no selector crawl: the run fails, which is fine here
	sawSelectorCrawl := false
	for _, p := range f.paths {
		if strings.HasSuffix(p, "/fetch-links") {
			t.Error("the link crawler was used although Rod is selected")
		}
		if strings.HasSuffix(p, "/crawl-paginated") {
			sawSelectorCrawl = true
		}
	}
	if !sawSelectorCrawl {
		t.Errorf("the AI instructions were not crawled: %v", f.paths)
	}
	// an unreadable engine closes it
	f.engine = "katana"
	f.settingsStatus = 502
	if code, resp := crawlNow(t, id); code != http.StatusPreconditionFailed || !strings.Contains(resp, "could not be read") {
		t.Errorf("unreadable engine: %d %s", code, resp)
	}
	if got := jobsOf(t, id); len(got) != 0 {
		t.Errorf("jobs were saved: %v", got)
	}
}

// A link that has AI instructions but no rule: Rod crawls it (the selector crawl is called, never the link list); Katana refuses with
// a message that says what to do, and never falls back to the AI instructions.
func TestAnInstructionLinkIsCrawledByRodAndRefusedByKatana(t *testing.T) {
	setupTrialDB(t)
	_, id, _ := trialPortal(t)
	if err := portal.UpdatePortalLinkCrawlInstructions(id, trialListingA); err != nil {
		t.Fatal(err)
	}
	f := newFakeCore(t, "rod")
	code, resp := crawlNow(t, id)
	if code != http.StatusAccepted {
		t.Fatalf("%d %s", code, resp)
	}
	waitRun(t, id)
	sawSelectorCrawl := false
	for _, p := range f.paths {
		if strings.HasSuffix(p, "/fetch-links") {
			t.Error("a link on the instructions method called the link list")
		}
		if strings.HasSuffix(p, "/crawl-paginated") {
			sawSelectorCrawl = true
		}
	}
	if !sawSelectorCrawl {
		t.Errorf("the selector crawl was not called: %v", f.paths)
	}

	f.engine = "katana"
	before := len(f.paths)
	code, resp = crawlNow(t, id)
	if code != http.StatusPreconditionFailed || !strings.Contains(resp, "no rules for the Katana engine") {
		t.Fatalf("Katana without a rule: %d %s", code, resp)
	}
	for _, p := range f.paths[before:] {
		if strings.HasSuffix(p, "/crawl-paginated") || strings.HasSuffix(p, "/fetch-links") {
			t.Errorf("the Browser was asked %s for a link Katana cannot crawl", p)
		}
	}
}

func TestALinkOnTheLinkMethodWithoutARuleIsRefusedNotCrawledWithInstructions(t *testing.T) {
	id := linkCrawlLink(t)
	if err := portal.UpdatePortalLinkCrawlInstructions(id, trialListingA); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET link_rules = 'corrupt' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	f := newFakeCore(t, "katana")
	if code, resp := crawlNow(t, id); code != http.StatusPreconditionFailed || !strings.Contains(resp, "rules") {
		t.Errorf("%d %s", code, resp)
	}
	if _, err := RunListingCrawlNow(id, "tok", "auto_discovery"); err == nil {
		t.Error("auto-discovery started a crawl for a link on the link method without a usable rule")
	}
	for _, p := range f.paths { // only the engine setting may have been read
		if strings.HasSuffix(p, "/crawl-paginated") || strings.HasSuffix(p, "/fetch-links") {
			t.Errorf("the Browser was asked %s", p)
		}
	}
}

func TestAutoDiscoveryFollowsTheEngineRuleUnderKatanaAndInstructionsUnderRod(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "rod")
	threePageBoard(f)
	// Rod: the link has a rule but no AI instructions, so there is nothing to crawl with (the rule is never used under Rod)
	if _, err := RunListingCrawlNow(id, "tok-auto", "auto_discovery"); err == nil || !strings.Contains(err.Error(), "crawl instructions") {
		t.Fatalf("under Rod without AI instructions: %v", err)
	}
	if _, rerr := findMostRecentCrawlRun(id); rerr == nil {
		t.Error("a run was created")
	}
	f.engine = "katana"
	runID, err := RunListingCrawlNow(id, "tok-auto", "auto_discovery")
	if err != nil {
		t.Fatal(err)
	}
	run, rerr := findMostRecentCrawlRun(id)
	if rerr != nil || run.ID != runID || run.Status != "completed" || run.TriggeredBy != "auto_discovery" {
		t.Errorf("%+v %v", run, rerr)
	}
	if got := jobsOf(t, id); len(got) != 6 {
		t.Errorf("jobs %v", got)
	}
	for _, a := range f.auths {
		if a != "Bearer tok-auto" {
			t.Errorf("the cached token must be used, got %q", a)
		}
	}
}

func TestTheEngineIsReadAgainWhenTheRunRuns(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "rod") // switched away between the start and the run: the run reads the engine again
	threePageBoard(f)
	run, err := startCrawlRun(id, "listing", "manual")
	if err != nil {
		t.Fatal(err)
	}
	runCrawlNow(contextForTest(t), run.ID, id, "tok")
	status, _, errMsg, _ := waitRun(t, id)
	if status != "failed" || !strings.Contains(errMsg, "crawl instructions") {
		t.Errorf("%s %q", status, errMsg)
	}
	for _, p := range f.paths {
		if strings.HasSuffix(p, "/fetch-links") {
			t.Error("a page of the link crawler was loaded although Rod is selected")
		}
	}
}

func TestAFirstPageTheSiteStopsFailsTheRunWithTheBrowsersAdvice(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "katana")
	f.fetchStatus = 502
	f.fetchBody = `{"code":"cloudflare_challenge_unresolved","message":"The site asked for a check","advice":"Do not retry; tell the user."}`
	if code, resp := crawlNow(t, id); code != http.StatusAccepted {
		t.Fatalf("%d %s", code, resp)
	}
	status, _, errMsg, _ := waitRun(t, id)
	if status != "failed" || !strings.Contains(errMsg, "The site asked for a check") || !strings.Contains(errMsg, "Do not retry; tell the user.") {
		t.Errorf("%s %q", status, errMsg)
	}
	if got := jobsOf(t, id); len(got) != 0 {
		t.Errorf("jobs %v", got)
	}
}

func TestALaterPageTheSiteStopsKeepsTheJobsOfTheEarlierPages(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "katana")
	threePageBoard(f)
	f.failFor = pageTwoURL
	if code, resp := crawlNow(t, id); code != http.StatusAccepted {
		t.Fatalf("%d %s", code, resp)
	}
	status, summary, errMsg, _ := waitRun(t, id)
	if status != "completed" || errMsg != "" {
		t.Fatalf("%s %s %q", status, summary, errMsg)
	}
	if got := jobsOf(t, id); len(got) != 3 {
		t.Errorf("jobs %v", got)
	}
	if !strings.Contains(summary, "stopped early") || !strings.Contains(summary, "Saved 3 new") {
		t.Errorf("summary %q", summary)
	}
	if _, _, _, pages, stopped := runRowOf(t, id); pages != 1 || stopped != linkrule.StopFetchFailed {
		t.Errorf("pages=%d stopped=%q", pages, stopped)
	}
}

func TestAPageWithoutJobsEndsTheRunWithNothingSavedAndNoError(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "katana")
	f.fetchBodyFor = map[string]string{pageOneURL: `{"finalUrl":"","title":"Empty","links":[{"url":"https://board.example/about","texts":["About"]}]}`}
	if code, resp := crawlNow(t, id); code != http.StatusAccepted {
		t.Fatalf("%d %s", code, resp)
	}
	status, summary, errMsg, _ := waitRun(t, id)
	if status != "completed" || errMsg != "" || !strings.Contains(summary, "Saved 0 new") {
		t.Errorf("%s %q %q", status, summary, errMsg)
	}
	if saved, _, _, pages, stopped := runRowOf(t, id); saved != 0 || pages != 1 || stopped != linkrule.StopNoNewJobs {
		t.Errorf("saved=%d pages=%d stopped=%q", saved, pages, stopped)
	}
}

func TestLinksWithoutTextAreCountedAndNeverBecomeJobs(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "katana")
	f.fetchBodyFor = map[string]string{pageOneURL: boardPage("1", "!2", "3")}
	if code, resp := crawlNow(t, id); code != http.StatusAccepted {
		t.Fatalf("%d %s", code, resp)
	}
	status, summary, _, log := waitRun(t, id)
	if status != "completed" {
		t.Fatal(status)
	}
	if got := jobsOf(t, id); len(got) != 2 {
		t.Errorf("jobs %v", got)
	}
	if _, _, skipped, _, _ := runRowOf(t, id); skipped != 1 {
		t.Errorf("skipped %d", skipped)
	}
	if !strings.Contains(summary, "1 link(s) had no usable text") || !strings.Contains(log, "left out") {
		t.Errorf("summary %q log %q", summary, log)
	}
}

func TestAStoppedRunLeavesItsTerminalRowToTheStopHandlerAndSavesNothing(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "katana")
	threePageBoard(f)
	run, err := startCrawlRun(id, "listing", "manual")
	if err != nil {
		t.Fatal(err)
	}
	ctx := contextForTest(t)
	cancelled, cancel := contextWithCancel(ctx)
	cancel() // the person pressed Stop
	runCrawlNow(cancelled, run.ID, id, "tok")
	if got, _ := findMostRecentCrawlRun(id); got.Status != "running" {
		t.Errorf("a stopped run must leave the terminal row to the cancel handler, status %q", got.Status)
	}
	if got := jobsOf(t, id); len(got) != 0 {
		t.Errorf("a stopped run saved jobs: %v", got)
	}
}

func TestAutoDiscoveryListsLinksByWhatTheEnginesMethodNeeds(t *testing.T) {
	setupTrialDB(t)
	pid, _ := portal.CreatePortal("Board")
	mk := func(url string) string {
		id, err := portal.AddPortalLink(pid, url, "L")
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	withRule := mk("https://board.example/a") // links + rule
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_method = 'links', link_rules = '{"host":"board.example","include":["board.example/j/*"]}' WHERE id = ?`, withRule); err != nil {
		t.Fatal(err)
	}
	ruleOnly := mk("https://board.example/b") // instructions method, a rule stored, no instructions
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET link_rules = '{"host":"board.example","include":["board.example/j/*"]}' WHERE id = ?`, ruleOnly); err != nil {
		t.Fatal(err)
	}
	instrOnly := mk("https://board.example/c") // instructions method with instructions
	if err := portal.UpdatePortalLinkCrawlInstructions(instrOnly, trialListingA); err != nil {
		t.Fatal(err)
	}
	linksNoRule := mk("https://board.example/d") // links method without a rule (corrupt state)
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_method = 'links' WHERE id = ?`, linksNoRule); err != nil {
		t.Fatal(err)
	}
	linksButInstr := mk("https://board.example/e") // links method with instructions but no rule
	if err := portal.UpdatePortalLinkCrawlInstructions(linksButInstr, trialListingA); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_method = 'links' WHERE id = ?`, linksButInstr); err != nil {
		t.Fatal(err)
	}
	// Plan 190: the engine's method decides what a link needs; the stored crawl_method is not read. Katana ("links"): a rule. Rod
	// ("instructions"): AI instructions. "" (the engine could not be read): either.
	for _, id := range []string{withRule, ruleOnly, instrOnly, linksNoRule, linksButInstr} {
		if _, err := db.JobsDB.Exec(`UPDATE portal_links SET auto_discovery_interval_minutes = 30 WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		method string
		want   []string
		not    []string
	}{
		{portal.CrawlMethodLinks, []string{withRule, ruleOnly}, []string{instrOnly, linksNoRule, linksButInstr}},
		{portal.CrawlMethodInstructions, []string{instrOnly, linksButInstr}, []string{withRule, ruleOnly, linksNoRule}},
		{"", []string{withRule, ruleOnly, instrOnly, linksButInstr}, []string{linksNoRule}},
	} {
		// the custom-interval list follows the same rule (every link above has its own interval now, never run: all are due)
		due, err := portal.ListDuePortalLinksWithCustomInterval(c.method)
		if err != nil {
			t.Fatal(err)
		}
		set := map[string]bool{}
		for _, g := range due {
			set[g] = true
		}
		for _, id := range c.want {
			if !set[id] {
				t.Errorf("method %q: %s must be listed, got %v", c.method, id, due)
			}
		}
		for _, id := range c.not {
			if set[id] {
				t.Errorf("method %q: %s must not be listed, got %v", c.method, id, due)
			}
		}
	}
	// the global list (links without an interval of their own)
	for _, id := range []string{withRule, ruleOnly, instrOnly, linksNoRule, linksButInstr} {
		if _, err := db.JobsDB.Exec(`UPDATE portal_links SET auto_discovery_interval_minutes = NULL WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}
	got, err := portal.ListPortalLinksEligibleForAutoDiscovery(portal.CrawlMethodLinks)
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, g := range got {
		set[g] = true
	}
	if !set[withRule] || !set[ruleOnly] || set[instrOnly] || set[linksNoRule] || set[linksButInstr] {
		t.Errorf("global list for Katana: %v", got)
	}
}

func contextForTest(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func contextWithCancel(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// Two postings with the same title are two jobs: the address is the identity of a link-crawled job (it has no company to compare).
func TestTwoPostingsWithTheSameTitleStayTwoJobs(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "katana")
	f.fetchBodyFor = map[string]string{pageOneURL: boardPage("10=Software Engineer", "11=Software Engineer", "12=Software Engineer", "13=Designer")}
	if code, resp := crawlNow(t, id); code != http.StatusAccepted {
		t.Fatalf("%d %s", code, resp)
	}
	status, summary, _, _ := waitRun(t, id)
	if status != "completed" {
		t.Fatal(status)
	}
	got := jobsOf(t, id)
	if len(got) != 4 || !strings.Contains(summary, "Saved 4 new") {
		t.Errorf("jobs %v summary %q", got, summary)
	}
	// and they stay four when crawled again
	if code, _ := crawlNow(t, id); code != http.StatusAccepted {
		t.Fatal(code)
	}
	waitRun(t, id)
	if got := jobsOf(t, id); len(got) != 4 {
		t.Errorf("after a second crawl: %v", got)
	}
}

// A Stop while page 2 loads: the jobs of page 1 are NOT saved (a stopped crawl saves nothing, like the selector crawl) and the terminal
// row is left to the cancel handler.
func TestAStopInTheMiddleOfTheRunSavesNothingOfTheEarlierPages(t *testing.T) {
	id := linkCrawlLink(t)
	f := newFakeCore(t, "katana")
	threePageBoard(f)
	ctx, cancel := contextWithCancel(contextForTest(t))
	defer cancel()
	orig := fetchPageLinks
	n := 0
	fetchPageLinks = func(c context.Context, coreURL, token, pageURL, portalID, requestID string) (pageLinks, error) {
		n++
		if n == 2 {
			cancel() // Stop, while the second page is being loaded
			return pageLinks{}, c.Err()
		}
		return orig(c, coreURL, token, pageURL, portalID, requestID)
	}
	t.Cleanup(func() { fetchPageLinks = orig })
	run, err := startCrawlRun(id, "listing", "manual")
	if err != nil {
		t.Fatal(err)
	}
	runCrawlNow(ctx, run.ID, id, "tok")
	got, _ := findMostRecentCrawlRun(id)
	if got.Status != "running" {
		t.Errorf("the terminal row belongs to the cancel handler, status %q", got.Status)
	}
	if jobs := jobsOf(t, id); len(jobs) != 0 {
		t.Errorf("a stopped run saved jobs: %v", jobs)
	}
	if !strings.Contains(got.Log, "cancelled") {
		t.Errorf("log %q", got.Log)
	}
}

// Plan 197, step 2: the auto-discovery sweep lists a link set to Rod by its AI instructions whatever the Browser's method is.
func TestTheSweepsListALinkSetToRodByItsAIInstructions(t *testing.T) {
	setupTrialDB(t)
	pid, _, _ := trialPortal(t)
	mk := func(url, ai, rule, engine string) string {
		t.Helper()
		id, err := portal.AddPortalLink(pid, url, "L")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_instructions = NULLIF(?, ''), link_rules = NULLIF(?, ''), crawl_engine = ?, auto_discovery_enabled = 1 WHERE id = ?`, ai, rule, engine, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	const rule = `{"host":"x","include":["x/j/#"]}`
	follows := mk("https://x.example/follows", "", rule, "")        // follows the Browser: a rule only
	rodAI := mk("https://x.example/rod-ai", "fields: x", "", "rod") // set to Rod with AI instructions
	rodRule := mk("https://x.example/rod-rule", "", rule, "rod")    // set to Rod with a rule only: nothing it can use
	for _, c := range []struct {
		method    string
		want, not []string
	}{
		{portal.CrawlMethodLinks, []string{follows, rodAI}, []string{rodRule}},
		{portal.CrawlMethodInstructions, []string{rodAI}, []string{follows, rodRule}},
		{"", []string{follows, rodAI}, []string{rodRule}},
	} {
		got, err := portal.ListPortalLinksEligibleForAutoDiscovery(c.method)
		if err != nil {
			t.Fatal(err)
		}
		set := map[string]bool{}
		for _, id := range got {
			set[id] = true
		}
		for _, id := range c.want {
			if !set[id] {
				t.Errorf("method %q: %s must be listed, got %v", c.method, id, got)
			}
		}
		for _, id := range c.not {
			if set[id] {
				t.Errorf("method %q: %s must not be listed, got %v", c.method, id, got)
			}
		}
	}
	// the custom-interval list follows the same rule
	for _, id := range []string{follows, rodAI, rodRule} {
		if _, err := db.JobsDB.Exec(`UPDATE portal_links SET auto_discovery_interval_minutes = 30 WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}
	due, err := portal.ListDuePortalLinksWithCustomInterval(portal.CrawlMethodLinks)
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, id := range due {
		set[id] = true
	}
	if !set[follows] || !set[rodAI] || set[rodRule] {
		t.Errorf("custom interval list under Katana: %v", due)
	}
}
