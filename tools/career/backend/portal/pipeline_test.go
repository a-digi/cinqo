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

	got, err := PipelineForLinks()
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
	got, err := PipelineForLinks()
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
	got, err := PipelineForLinks()
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("%#v %v", got, err)
	}
}
