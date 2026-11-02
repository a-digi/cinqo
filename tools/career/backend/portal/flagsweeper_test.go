package portal

import (
	"context"
	"testing"
	"time"

	"career-tool-backend/db"
)

func settledLink(mod func(*FlagSweepSnapshot)) FlagSweepSnapshot {
	s := FlagSweepSnapshot{PipelineSnapshot: PipelineSnapshot{HasListingInstructions: true}, Settled: true}
	if mod != nil {
		mod(&s)
	}
	return s
}

func TestRepairsForEveryStuckStateAndGuard(t *testing.T) {
	cases := []struct {
		name string
		snap FlagSweepSnapshot
		want FlagRepairs
	}{
		{"listing only, never crawled → listing crawl", settledLink(nil), FlagRepairs{ListingCrawl: true}},
		{"listing crawl already requested", settledLink(func(s *FlagSweepSnapshot) { s.ListingCrawlRequested = true }), FlagRepairs{}},
		{"crawled before, found nothing: not a lost flag", settledLink(func(s *FlagSweepSnapshot) { s.ListingCrawlRuns = 1 }), FlagRepairs{}},
		{"no listing instructions", settledLink(func(s *FlagSweepSnapshot) { s.HasListingInstructions = false }), FlagRepairs{}},
		{"not settled", settledLink(func(s *FlagSweepSnapshot) { s.Settled = false }), FlagRepairs{}},
		{"question pending", settledLink(func(s *FlagSweepSnapshot) { s.QuestionPending = true }), FlagRepairs{}},
		{"listing generating", settledLink(func(s *FlagSweepSnapshot) { s.ListingGenerating = true }), FlagRepairs{}},
		{"detail generating", settledLink(func(s *FlagSweepSnapshot) { s.DetailGenerating, s.Jobs = true, 3 }), FlagRepairs{}},
		{"listing crawling", settledLink(func(s *FlagSweepSnapshot) { s.ListingCrawling = true }), FlagRepairs{}},
		{"detail crawling", settledLink(func(s *FlagSweepSnapshot) { s.DetailCrawling, s.Jobs = true, 3 }), FlagRepairs{}},

		{"jobs saved, no detail instructions, nothing requested → AI request", settledLink(func(s *FlagSweepSnapshot) { s.Jobs, s.ListingCrawlRuns = 3, 1 }), FlagRepairs{DetailAI: true}},
		{"jobs saved, detail AI already requested", settledLink(func(s *FlagSweepSnapshot) { s.Jobs, s.DetailAIRequested = 3, true }), FlagRepairs{}},
		{"two failed runs: still asked", settledLink(func(s *FlagSweepSnapshot) { s.Jobs, s.DetailFailedRuns = 3, 2 }), FlagRepairs{DetailAI: true}},
		{"three failed runs: given up", settledLink(func(s *FlagSweepSnapshot) { s.Jobs, s.DetailFailedRuns = 3, 3 }), FlagRepairs{}},

		{"detail instructions, jobs without description, never visited → detail crawl", settledLink(func(s *FlagSweepSnapshot) {
			s.HasDetailInstructions, s.Jobs, s.JobsWithDescription = true, 4, 1
		}), FlagRepairs{DetailCrawl: true}},
		{"detail crawl already requested", settledLink(func(s *FlagSweepSnapshot) {
			s.HasDetailInstructions, s.Jobs, s.DetailCrawlRequested = true, 4, true
		}), FlagRepairs{}},
		{"detail pages were visited before: no loop", settledLink(func(s *FlagSweepSnapshot) {
			s.HasDetailInstructions, s.Jobs, s.DetailCrawlRuns = true, 4, 1
		}), FlagRepairs{}},
		{"every job has a description", settledLink(func(s *FlagSweepSnapshot) {
			s.HasDetailInstructions, s.Jobs, s.JobsWithDescription = true, 4, 4
		}), FlagRepairs{}},
		{"complete link", settledLink(func(s *FlagSweepSnapshot) {
			s.HasDetailInstructions, s.Jobs, s.JobsWithDescription, s.ListingCrawlRuns, s.DetailCrawlRuns = true, 4, 4, 1, 1
		}), FlagRepairs{}},
	}
	for _, c := range cases {
		if got := RepairsFor(c.snap); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func flagsOf(t *testing.T, id string) (listing, detailAI, detailCrawl int) {
	t.Helper()
	if err := db.JobsDB.QueryRow(`SELECT listing_crawl_requested, job_detail_instructions_ai_requested, job_detail_crawl_requested FROM portal_links WHERE id = ?`, id).
		Scan(&listing, &detailAI, &detailCrawl); err != nil {
		t.Fatal(err)
	}
	return
}

// Real rows: each lost event is repaired once, repeated sweeps change nothing, and a link that changed lately is left alone.
func TestSweepFlagsRepairsEachLostEventOnce(t *testing.T) {
	setupAttemptsDB(t, "a", "b", "c", "d")
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.JobsDB.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	old := "datetime('now', '-1 hour')"
	// a: listing instructions, never crawled (listing_instructions_ready lost)
	exec(`UPDATE portal_links SET crawl_instructions = 'L', updated_at = ` + old + ` WHERE id = 'a'`)
	// b: listing instructions, crawled, jobs saved, no detail instructions (jobs_crawled lost)
	exec(`UPDATE portal_links SET crawl_instructions = 'L', updated_at = ` + old + ` WHERE id = 'b'`)
	exec(`INSERT INTO crawl_runs (id, portal_link_id, status, kind) VALUES ('r1', 'b', 'completed', 'listing')`)
	exec(`INSERT INTO jobs (id, source_url, title, portal_link_id) VALUES ('j1', 'https://j1', 'T', 'b')`)
	// c: both instructions, jobs without description, detail pages never visited
	exec(`UPDATE portal_links SET crawl_instructions = 'L', job_detail_crawl_instructions = 'D', updated_at = ` + old + ` WHERE id = 'c'`)
	exec(`INSERT INTO crawl_runs (id, portal_link_id, status, kind) VALUES ('r2', 'c', 'completed', 'listing')`)
	exec(`INSERT INTO jobs (id, source_url, title, portal_link_id) VALUES ('j2', 'https://j2', 'T', 'c')`)
	// d: like a, but changed a minute ago (an event may still be on its way)
	exec(`UPDATE portal_links SET crawl_instructions = 'L', updated_at = datetime('now', '-1 minutes') WHERE id = 'd'`)

	report, err := SweepFlags()
	if err != nil {
		t.Fatal(err)
	}
	if report != (FlagSweepReport{ListingCrawl: 1, DetailAI: 1, DetailCrawl: 1}) {
		t.Fatalf("report %+v", report)
	}
	if l, ai, dc := flagsOf(t, "a"); l != 1 || ai != 0 || dc != 0 {
		t.Errorf("a: %d %d %d", l, ai, dc)
	}
	if l, ai, dc := flagsOf(t, "b"); l != 0 || ai != 1 || dc != 0 {
		t.Errorf("b: %d %d %d", l, ai, dc)
	}
	if l, ai, dc := flagsOf(t, "c"); l != 0 || ai != 0 || dc != 1 {
		t.Errorf("c: %d %d %d", l, ai, dc)
	}
	if l, ai, dc := flagsOf(t, "d"); l+ai+dc != 0 {
		t.Errorf("d changed lately, must be left alone: %d %d %d", l, ai, dc)
	}

	// An immediate second sweep is a no-op (the raised links changed just now, the flags are set).
	again, err := SweepFlags()
	if err != nil || again != (FlagSweepReport{}) {
		t.Fatalf("second sweep: %+v %v", again, err)
	}
	// After everything settles, only d (now settled) is new: the others still have their flags set, nobody consumed them.
	exec(`UPDATE portal_links SET updated_at = ` + old)
	again, err = SweepFlags()
	if err != nil {
		t.Fatal(err)
	}
	if again != (FlagSweepReport{ListingCrawl: 1}) {
		t.Fatalf("third sweep: %+v", again)
	}

	// Once consumed (cleared) and crawled, a link is not re-requested.
	exec(`UPDATE portal_links SET listing_crawl_requested = 0 WHERE id IN ('a', 'd')`)
	exec(`INSERT INTO crawl_runs (id, portal_link_id, status, kind) VALUES ('r3', 'a', 'completed', 'listing')`)
	exec(`INSERT INTO crawl_runs (id, portal_link_id, status, kind) VALUES ('r4', 'd', 'completed', 'listing')`)
	exec(`UPDATE portal_links SET updated_at = ` + old)
	last, err := SweepFlags()
	if err != nil || last != (FlagSweepReport{}) {
		t.Fatalf("after consumption: %+v %v", last, err)
	}
}

func TestSweepFlagsLeavesRunningWorkAlone(t *testing.T) {
	setupAttemptsDB(t, "a", "b")
	for _, q := range []string{
		`UPDATE portal_links SET crawl_instructions = 'L', updated_at = datetime('now', '-1 hour')`,
		`INSERT INTO instruction_attempts (id, portal_link_id, kind, status, started_at) VALUES ('x', 'a', 'listing', 'running', datetime('now'))`,
		`INSERT INTO crawl_runs (id, portal_link_id, status, kind) VALUES ('y', 'b', 'running', 'listing')`,
	} {
		if _, err := db.JobsDB.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	got, err := SweepFlags()
	if err != nil || got != (FlagSweepReport{}) {
		t.Fatalf("%+v %v", got, err)
	}
}

// A flag that an event raised after the sweep read the link is neither overwritten nor counted (the write names its condition).
func TestSweepFlagsCountsOnlyWhatItRaised(t *testing.T) {
	setupAttemptsDB(t, "a")
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_instructions = 'L', updated_at = datetime('now', '-1 hour') WHERE id = 'a'`); err != nil {
		t.Fatal(err)
	}
	afterFlagSweepRead = func() {
		if _, err := db.JobsDB.Exec(`UPDATE portal_links SET listing_crawl_requested = 1, updated_at = '2020-01-01 00:00:00' WHERE id = 'a'`); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { afterFlagSweepRead = func() {} }()
	got, err := SweepFlags()
	if err != nil || got != (FlagSweepReport{}) {
		t.Fatalf("the late event raised it first, the sweep must not count it: %+v %v", got, err)
	}
	var updated string
	if err := db.JobsDB.QueryRow(`SELECT updated_at FROM portal_links WHERE id = 'a'`).Scan(&updated); err != nil || updated != "2020-01-01 00:00:00" {
		t.Errorf("the sweep must not write a flag that is already raised: %q %v", updated, err)
	}
}

func TestStartFlagSweeperSweepsAndStops(t *testing.T) {
	setupAttemptsDB(t, "a")
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET crawl_instructions = 'L', updated_at = datetime('now', '-1 hour') WHERE id = 'a'`); err != nil {
		t.Fatal(err)
	}
	oldDelay, oldInterval := flagSweepStartDelay, flagSweepInterval
	flagSweepStartDelay, flagSweepInterval = 10*time.Millisecond, time.Hour
	defer func() { flagSweepStartDelay, flagSweepInterval = oldDelay, oldInterval }()
	ctx, cancel := context.WithCancel(context.Background())
	done := StartFlagSweeper(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if l, _, _ := flagsOf(t, "a"); l == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if l, _, _ := flagsOf(t, "a"); l != 1 {
		t.Fatal("the sweeper did not raise the lost flag")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the sweeper did not stop")
	}
}
