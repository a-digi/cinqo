package portal

import (
	"fmt"
	"testing"

	"career-tool-backend/db"
)

// base is a link that is complete: both instructions, jobs with descriptions, nothing requested or running.
func complete() PipelineSnapshot {
	return PipelineSnapshot{HasListingInstructions: true, HasDetailInstructions: true, Jobs: 5, JobsWithDescription: 5}
}

func with(s PipelineSnapshot, change func(*PipelineSnapshot)) PipelineSnapshot {
	change(&s)
	return s
}

// Every stage, and every conflict between two facts, in the order the table decides them.
func TestStageOfEveryStageAndConflict(t *testing.T) {
	cases := []struct {
		name string
		snap PipelineSnapshot
		want PipelineStage
	}{
		{"nothing at all", PipelineSnapshot{}, PipelineStage{StageNoListing, WaitUser}},
		{"complete", complete(), PipelineStage{StageComplete, WaitNothing}},
		{"complete with no jobs yet is ready to crawl", with(complete(), func(s *PipelineSnapshot) { s.Jobs, s.JobsWithDescription = 0, 0 }), PipelineStage{StageListingReady, WaitUser}},
		{"listing only, never crawled", PipelineSnapshot{HasListingInstructions: true}, PipelineStage{StageListingReady, WaitUser}},
		{"listing crawl requested, waiting for the page", PipelineSnapshot{HasListingInstructions: true, ListingCrawlRequested: true}, PipelineStage{StageListingCrawlWaiting, WaitPage}},
		{"listing crawling", PipelineSnapshot{HasListingInstructions: true, ListingCrawling: true}, PipelineStage{StageListingCrawling, WaitCrawl}},
		{"listing generating", PipelineSnapshot{ListingGenerating: true}, PipelineStage{StageListingGenerating, WaitAI}},
		{"question pending", PipelineSnapshot{QuestionPending: true}, PipelineStage{StageListingQuestion, WaitUser}},
		{"jobs saved, detail instructions requested", PipelineSnapshot{HasListingInstructions: true, Jobs: 3, DetailAIRequested: true}, PipelineStage{StageDetailRequested, WaitPage}},
		{"jobs saved, nothing requested, no failure", PipelineSnapshot{HasListingInstructions: true, Jobs: 3}, PipelineStage{StageDetailMissing, WaitUser}},
		{"jobs saved, detail generation failed", PipelineSnapshot{HasListingInstructions: true, Jobs: 3, DetailFailedRuns: 2}, PipelineStage{StageDetailFailed, WaitUser}},
		{"jobs saved, detail generation failed once", PipelineSnapshot{HasListingInstructions: true, Jobs: 3, DetailFailedRuns: 1}, PipelineStage{StageDetailFailed, WaitUser}},
		{"detail instructions generating", PipelineSnapshot{HasListingInstructions: true, Jobs: 3, DetailGenerating: true}, PipelineStage{StageDetailGenerating, WaitAI}},
		{"detail crawl requested, waiting for the page", with(complete(), func(s *PipelineSnapshot) { s.DetailCrawlRequested = true }), PipelineStage{StageDetailCrawlWaiting, WaitPage}},
		{"detail crawling", with(complete(), func(s *PipelineSnapshot) { s.DetailCrawling = true }), PipelineStage{StageDetailCrawling, WaitCrawl}},
		{"detail crawl pending: some jobs without a description", with(complete(), func(s *PipelineSnapshot) { s.JobsWithDescription = 2 }), PipelineStage{StageDetailCrawlPending, WaitUser}},

		// conflicts: which fact wins
		{"a question wins over a running generation", PipelineSnapshot{QuestionPending: true, ListingGenerating: true, HasListingInstructions: true}, PipelineStage{StageListingQuestion, WaitUser}},
		{"a running listing generation wins over stored instructions", PipelineSnapshot{ListingGenerating: true, HasListingInstructions: true, Jobs: 4}, PipelineStage{StageListingGenerating, WaitAI}},
		{"a flag without listing instructions is still no instructions", PipelineSnapshot{ListingCrawlRequested: true, DetailAIRequested: true}, PipelineStage{StageNoListing, WaitUser}},
		{"a running listing crawl wins over a waiting flag", PipelineSnapshot{HasListingInstructions: true, ListingCrawling: true, ListingCrawlRequested: true}, PipelineStage{StageListingCrawling, WaitCrawl}},
		{"a running crawl wins over a detail request", PipelineSnapshot{HasListingInstructions: true, Jobs: 2, ListingCrawling: true, DetailAIRequested: true}, PipelineStage{StageListingCrawling, WaitCrawl}},
		{"detail generation wins over a detail crawl flag", with(complete(), func(s *PipelineSnapshot) { s.DetailGenerating, s.DetailCrawlRequested = true, true }), PipelineStage{StageDetailGenerating, WaitAI}},
		{"a running detail crawl wins over the detail crawl flag", with(complete(), func(s *PipelineSnapshot) { s.DetailCrawling, s.DetailCrawlRequested = true, true }), PipelineStage{StageDetailCrawling, WaitCrawl}},
		{"a listing crawl flag wins over the detail steps", with(complete(), func(s *PipelineSnapshot) { s.ListingCrawlRequested, s.DetailCrawlRequested = true, true }), PipelineStage{StageListingCrawlWaiting, WaitPage}},
		{"the detail request flag wins over a failed streak", PipelineSnapshot{HasListingInstructions: true, Jobs: 3, DetailAIRequested: true, DetailFailedRuns: 5}, PipelineStage{StageDetailRequested, WaitPage}},
		{"flag set but detail instructions present: the flag is for the crawl, not the AI", with(complete(), func(s *PipelineSnapshot) { s.DetailAIRequested = true }), PipelineStage{StageComplete, WaitNothing}},
		{"detail instructions present, a failed streak does not matter", with(complete(), func(s *PipelineSnapshot) { s.DetailFailedRuns = 4 }), PipelineStage{StageComplete, WaitNothing}},
		{"detail instructions but no jobs: the listing step comes first", PipelineSnapshot{HasListingInstructions: true, HasDetailInstructions: true}, PipelineStage{StageListingReady, WaitUser}},
		{"all jobs described but no detail instructions is still missing", PipelineSnapshot{HasListingInstructions: true, Jobs: 3, JobsWithDescription: 3}, PipelineStage{StageDetailMissing, WaitUser}},
		{"detail crawl flag wins over pending descriptions", with(complete(), func(s *PipelineSnapshot) { s.JobsWithDescription, s.DetailCrawlRequested = 0, true }), PipelineStage{StageDetailCrawlWaiting, WaitPage}},
	}
	for _, c := range cases {
		if got := StageOf(c.snap); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

// The loader reads real rows: instructions, flags, running attempts and crawls, jobs.
func TestPipelineForLinksReadsEveryStageFromTheDatabase(t *testing.T) {
	setupAttemptsDB(t)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.JobsDB.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	link := func(id, listing, detail string, listingReq, detailAIReq, detailCrawlReq int, question string) {
		exec(`INSERT INTO portal_links (id, portal_id, url, crawl_instructions, job_detail_crawl_instructions, listing_crawl_requested,
		        job_detail_instructions_ai_requested, job_detail_crawl_requested, jobs_question_asked_at, created_at)
		      VALUES (?, 'p1', ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), ?)`,
			id, "https://x/"+id, listing, detail, listingReq, detailAIReq, detailCrawlReq, question, fmt.Sprintf("2026-01-01 10:00:%02d", int(id[0]-'a')))
	}
	job := func(id, link, description string) {
		exec(`INSERT INTO jobs (id, source_url, title, portal_link_id, description) VALUES (?, ?, 'T', ?, NULLIF(?, ''))`, id, "https://j/"+id, link, description)
	}
	// a: no instructions, b: ready, c: crawl waiting, d: crawling, e: detail requested, f: detail generating, g: detail failed,
	// h: detail crawl waiting, i: complete, j: question, k: listing generating, l: detail crawling, m: pending descriptions
	link("a", "", "", 0, 0, 0, "")
	link("b", "L", "", 0, 0, 0, "")
	link("c", "L", "", 1, 0, 0, "")
	link("d", "L", "", 0, 0, 0, "")
	exec(`INSERT INTO crawl_runs (id, portal_link_id, status, kind, started_at) VALUES ('r1', 'd', 'running', 'listing', '2026-01-02 09:00:00')`)
	link("e", "L", "", 0, 1, 0, "")
	job("je", "e", "")
	link("f", "L", "", 0, 0, 0, "")
	job("jf", "f", "")
	exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, started_at) VALUES ('a1', 'f', 'job_detail', 'running', datetime('now'))`)
	link("g", "L", "", 0, 0, 0, "")
	job("jg", "g", "")
	exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, started_at, finished_at, failure_reason) VALUES ('a2', 'g', 'job_detail', 'failed', '2026-01-01 00:00:00', '2026-01-01 00:01:00', 'not_saved'), ('a3', 'g', 'job_detail', 'failed', '2026-01-02 00:00:00', '2026-01-02 00:01:00', 'error')`)
	link("h", "L", "D", 0, 0, 1, "")
	job("jh", "h", "text")
	link("i", "L", "D", 0, 0, 0, "")
	job("ji1", "i", "text")
	job("ji2", "i", "more text")
	link("j", "L", "", 0, 0, 0, "2026-01-02T10:00:00Z")
	link("k", "", "", 0, 0, 0, "")
	exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, started_at) VALUES ('a4', 'k', 'listing', 'running', datetime('now'))`)
	link("l", "L", "D", 0, 0, 0, "")
	job("jl", "l", "text")
	exec(`INSERT INTO crawl_runs (id, portal_link_id, status, kind, started_at) VALUES ('r2', 'l', 'running', 'job_detail', '2026-01-02 09:30:00')`)
	link("n", "   ", "", 0, 0, 0, "")  // whitespace only is no instructions
	link("o", "L", "   ", 0, 0, 0, "") // whitespace only is no job-detail instructions either
	job("jo", "o", "text")
	link("m", "L", "D", 0, 0, 0, "")
	job("jm1", "m", "text")
	job("jm2", "m", "")
	job("jm3", "m", "   ")

	got, err := PipelineForLinks("")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]PipelineLink{}
	for _, l := range got {
		by[l.PortalLinkID] = l
	}
	want := map[string][2]string{
		"a": {StageNoListing, WaitUser}, "b": {StageListingReady, WaitUser}, "c": {StageListingCrawlWaiting, WaitPage},
		"d": {StageListingCrawling, WaitCrawl}, "e": {StageDetailRequested, WaitPage}, "f": {StageDetailGenerating, WaitAI},
		"g": {StageDetailFailed, WaitUser}, "h": {StageDetailCrawlWaiting, WaitPage}, "i": {StageComplete, WaitNothing},
		"j": {StageListingQuestion, WaitUser}, "k": {StageListingGenerating, WaitAI}, "l": {StageDetailCrawling, WaitCrawl},
		"m": {StageDetailCrawlPending, WaitUser}, "n": {StageNoListing, WaitUser}, "o": {StageDetailMissing, WaitUser},
	}
	if len(got) != len(want) {
		t.Errorf("%d links, want %d", len(got), len(want))
	}
	for id, w := range want {
		if l := by[id]; l.Stage != w[0] || l.WaitingFor != w[1] {
			t.Errorf("link %s: %s/%s, want %s/%s", id, l.Stage, l.WaitingFor, w[0], w[1])
		}
	}
	if d := by["d"].Since; d != "2026-01-02 09:00:00" {
		t.Errorf("the running listing crawl's start: %q", d)
	}
	if d := by["l"].Since; d != "2026-01-02 09:30:00" {
		t.Errorf("the running detail crawl's start: %q", d)
	}
	if by["f"].Since == "" || by["k"].Since == "" {
		t.Errorf("a running generation says when it started: f=%q k=%q", by["f"].Since, by["k"].Since)
	}
	for _, id := range []string{"a", "b", "c", "e", "g", "h", "i", "j", "m"} {
		if by[id].Since != "" {
			t.Errorf("link %s is not running, since must be empty: %q", id, by[id].Since)
		}
	}
	if d := by["m"].Detail; d.Jobs != 3 || d.JobsWithDescription != 1 {
		t.Errorf("m: a blank description does not count: %+v", d)
	}
	if d := by["g"].Detail; d.DetailFailedRuns != 2 || d.Jobs != 1 {
		t.Errorf("g: %+v", d)
	}
	if d := by["i"].Detail; d.Jobs != 2 || d.JobsWithDescription != 2 || d.DetailFailedRuns != 0 {
		t.Errorf("i: %+v", d)
	}
	// oldest link first: the links were created in the order a, b, c, ...
	for i := 1; i < len(got); i++ {
		if got[i-1].PortalLinkID > got[i].PortalLinkID {
			t.Errorf("order: %s comes before %s", got[i-1].PortalLinkID, got[i].PortalLinkID)
		}
	}
}

// A running attempt that is older than the stale threshold is not counted as running, and reading never heals (writes) it.
func TestPipelineIgnoresAStaleRunningAttemptWithoutWritingAnything(t *testing.T) {
	setupAttemptsDB(t, "l1")
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_instructions = 'L' WHERE id = 'l1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, started_at) VALUES ('old', 'l1', 'job_detail', 'running', datetime('now', '-3 hours'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, portal_link_id) VALUES ('j', 'https://j', 'T', 'l1')`); err != nil {
		t.Fatal(err)
	}
	got, err := PipelineForLinks("")
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	if got[0].Stage == StageDetailGenerating {
		t.Errorf("a stale attempt must not show as generating: %+v", got[0])
	}
	var status string
	if err := db.JobsDB.QueryRow(`SELECT status FROM instruction_attempts WHERE id = 'old'`).Scan(&status); err != nil || status != "running" {
		t.Errorf("reading the pipeline must not touch the attempt: %q %v", status, err)
	}
}

func TestPipelineForNoLinksIsAnEmptyList(t *testing.T) {
	setupAttemptsDB(t)
	got, err := PipelineForLinks("")
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("%#v %v", got, err)
	}
}

// Plan 190: under Katana a link without job-detail instructions is "missing" them (to be picked), never "requested" from the AI or
// "failed"; under Rod and for a login link the AI's request and failures still decide.
func TestPipelineUnderKatanaIgnoresTheAIsJobDetailRequestAndFailures(t *testing.T) {
	setupAttemptsDB(t)
	add := func(id string, login int) {
		t.Helper()
		if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url, link_rules, requires_login, job_detail_instructions_ai_requested, crawl_instructions, created_at)
		      VALUES (?, 'p1', ?, '{"host":"x","include":["x/j/#"]}', ?, 1, 'fields: x', ?)`,
			id, "https://x/"+id, login, fmt.Sprintf("2026-01-01 10:00:%02d", int(id[0]-'a'))); err != nil {
			t.Fatal(err)
		}
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, portal_link_id) VALUES (?, ?, 'T', ?)`, "j"+id, "https://j/"+id, id); err != nil {
			t.Fatal(err)
		}
	}
	add("a", 0) // a normal link
	add("b", 1) // a login link
	stage := func(method, id string) string {
		got, err := PipelineForLinks(method)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range got {
			if l.PortalLinkID == id {
				return l.Stage
			}
		}
		return ""
	}
	if got := stage(CrawlMethodLinks, "a"); got != StageDetailMissing {
		t.Errorf("Katana: %q, want %q", got, StageDetailMissing)
	}
	if got := stage(CrawlMethodInstructions, "a"); got != StageDetailRequested {
		t.Errorf("Rod: %q, want %q", got, StageDetailRequested)
	}
	if got := stage(CrawlMethodLinks, "b"); got != StageDetailRequested {
		t.Errorf("a login link under Katana still uses the AI: %q, want %q", got, StageDetailRequested)
	}
}

// Plan 190: the stage's "has listing instructions" follows the method the Browser's selected engine means: a rule under Katana
// ("links"), AI instructions under Rod ("instructions"), either when no engine could be read (""), and a login link always its AI instructions.
func TestPipelineJudgesTheJobListByTheEnginesMethod(t *testing.T) {
	setupAttemptsDB(t)
	add := func(id, ai, rule string, login int) {
		t.Helper()
		if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url, crawl_instructions, link_rules, requires_login, created_at)
		      VALUES (?, 'p1', ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
			id, "https://x/"+id, ai, rule, login, fmt.Sprintf("2026-01-01 10:00:%02d", int(id[0]-'a'))); err != nil {
			t.Fatal(err)
		}
	}
	const rule = `{"host":"x","include":["x/j/#"]}`
	add("a", "", rule, 0)        // a rule only (the Dua Pune case)
	add("b", "fields: x", "", 0) // AI instructions only
	add("c", "fields: x", rule, 0)
	add("d", "", rule, 1) // a login link with a rule only: its AI instructions are what count
	add("e", "fields: x", "", 1)
	for _, c := range []struct {
		method string
		noList []string // the links whose stage is "no listing instructions yet"
	}{
		{CrawlMethodLinks, []string{"b", "d", "e"}[:2]}, // Katana: b has no rule; d is a login link without AI instructions
		{CrawlMethodInstructions, []string{"a", "d"}},   // Rod: a has no AI instructions; d the same
		{"", []string{"d"}},                             // either: only the login link without AI instructions has nothing
	} {
		got, err := PipelineForLinks(c.method)
		if err != nil {
			t.Fatal(err)
		}
		no := map[string]bool{}
		for _, l := range got {
			if l.Stage == StageNoListing {
				no[l.PortalLinkID] = true
			}
		}
		want := map[string]bool{}
		for _, id := range c.noList {
			want[id] = true
		}
		if len(no) != len(want) {
			t.Errorf("method %q: no-listing links %v, want %v", c.method, no, want)
			continue
		}
		for id := range want {
			if !no[id] {
				t.Errorf("method %q: link %s should have no listing instructions, got %v", c.method, id, no)
			}
		}
	}
}

// Plan 195, step 4: under Katana a link with a kept login is judged like any other link (a rule is its listing instructions); under another
// reading, or after the login expired, it stays a login link.
func TestPipelineJudgesAKeptLoginLinkByTheEngine(t *testing.T) {
	loginSetup(t)
	link := newLinkOn(t, "https://x.example/jobs")
	yes := true
	if err := UpdatePortalLinkAutoDiscoveryEnabled(link, false); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePortalLinkLogin(link, &yes, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := SetLinkLoginSession(link, "0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET link_rules = '{"host":"x.example","include":["x.example/*"]}' WHERE id = ?`, link); err != nil {
		t.Fatal(err)
	}
	has := func(method string) bool {
		rows, _, _, err := loadPipelineRows(method)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.id == link {
				return r.snap.HasListingInstructions
			}
		}
		t.Fatal("link not found")
		return false
	}
	if !has(CrawlMethodLinks) {
		t.Error("Katana + kept login: the rule is the listing instructions")
	}
	if has(CrawlMethodInstructions) {
		t.Error("another reading: a login link counts its AI instructions only")
	}
	if err := MarkLoginSessionExpired(link); err != nil {
		t.Fatal(err)
	}
	if has(CrawlMethodLinks) {
		t.Error("an expired login is a login link again")
	}
}

// Plan 197, step 2: a link set to use Rod is judged by its AI instructions whatever the Browser's method is, in the same database as links
// that follow the Browser.
func TestPipelineJudgesALinkSetToRodByItsAIInstructionsUnderKatana(t *testing.T) {
	setupAttemptsDB(t)
	add := func(id, ai, rule, engine string) {
		t.Helper()
		if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url, crawl_instructions, link_rules, crawl_engine, created_at)
		      VALUES (?, 'p1', ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
			id, "https://x/"+id, ai, rule, engine, fmt.Sprintf("2026-01-01 10:00:%02d", int(id[0]-'a'))); err != nil {
			t.Fatal(err)
		}
	}
	const rule = `{"host":"x","include":["x/j/#"]}`
	add("a", "", rule, "")             // follows the Browser: a rule only
	add("b", "fields: x", rule, "rod") // set to Rod: has AI instructions (and a rule it does not use)
	add("c", "", rule, "rod")          // set to Rod: a rule only = nothing it can use
	add("d", "fields: x", "", "")      // follows the Browser: AI instructions only
	noListing := func(method string) map[string]bool {
		got, err := PipelineForLinks(method)
		if err != nil {
			t.Fatal(err)
		}
		no := map[string]bool{}
		for _, l := range got {
			if l.Stage == StageNoListing {
				no[l.PortalLinkID] = true
			}
		}
		return no
	}
	// Katana selected: a and (the rule counts) fine; d has no rule; b is Rod with AI instructions: fine; c is Rod with only a rule: nothing
	if no := noListing(CrawlMethodLinks); len(no) != 2 || !no["c"] || !no["d"] {
		t.Errorf("Katana: %v, want c and d", no)
	}
	// Rod selected: a has no AI instructions; c is Rod with only a rule
	if no := noListing(CrawlMethodInstructions); len(no) != 2 || !no["a"] || !no["c"] {
		t.Errorf("Rod: %v, want a and c", no)
	}
	// no engine reading: a link set to Rod still needs AI instructions, the others take either
	if no := noListing(""); len(no) != 1 || !no["c"] {
		t.Errorf("unknown: %v, want c", no)
	}
}

// The method of each row (it decides whether the AI's job-detail request counts: under Katana it says nothing about a Katana link, but it does
// for a link set to Rod, whose job details are written by the AI).
func TestEachPipelineRowCarriesTheMethodOfItsOwnEngine(t *testing.T) {
	setupAttemptsDB(t)
	for _, c := range []struct{ id, engine string }{{"a", ""}, {"b", "rod"}} {
		if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url, crawl_instructions, link_rules, crawl_engine, created_at)
		      VALUES (?, 'p1', ?, 'fields: x', ?, ?, '2026-01-01 10:00:00')`, c.id, "https://x/"+c.id, `{"host":"x","include":["x/j/#"]}`, c.engine); err != nil {
			t.Fatal(err)
		}
	}
	for browser, want := range map[string]map[string]string{
		CrawlMethodLinks:        {"a": CrawlMethodLinks, "b": CrawlMethodInstructions},
		CrawlMethodInstructions: {"a": CrawlMethodInstructions, "b": CrawlMethodInstructions},
		"":                      {"a": "", "b": CrawlMethodInstructions},
	} {
		rows, _, _, err := loadPipelineRows(browser)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.method != want[r.id] {
				t.Errorf("Browser method %q: link %s reads with %q, want %q", browser, r.id, r.method, want[r.id])
			}
		}
	}
}
