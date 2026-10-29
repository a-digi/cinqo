package portal

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

var runCounter int

// addListingRun inserts one finished listing run for a link; saved < 0
// inserts a run from before step 90 (no structured data).
func addListingRun(t *testing.T, link, status string, saved, updated, skipped int, finishedAt string) {
	t.Helper()
	runCounter++
	id := fmt.Sprintf("run-%d", runCounter)
	var err error
	if saved < 0 {
		_, err = db.JobsDB.Exec(`INSERT INTO crawl_runs (id, portal_link_id, kind, status, finished_at) VALUES (?, ?, 'listing', ?, ?)`, id, link, status, finishedAt)
	} else {
		_, err = db.JobsDB.Exec(
			`INSERT INTO crawl_runs (id, portal_link_id, kind, status, finished_at, triggered_by, jobs_saved, jobs_updated, jobs_skipped, pages_visited, stopped_reason, not_found_fields)
			 VALUES (?, ?, 'listing', ?, ?, 'auto_discovery', ?, ?, ?, 2, 'no_next_link', '["title"]')`,
			id, link, status, finishedAt, saved, updated, skipped)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func stamp(minutesAgo int) string {
	return time.Now().UTC().Add(-time.Duration(minutesAgo) * time.Minute).Format(time.RFC3339)
}

func lastCrawl(t *testing.T, link string) (LastListingCrawl, bool) {
	t.Helper()
	m, err := LastListingCrawlByLink()
	if err != nil {
		t.Fatal(err)
	}
	c, ok := m[link]
	return c, ok
}

func TestLastListingCrawlSummarisesTheLatestRun(t *testing.T) {
	setupAttemptsDB(t, "l1")
	addListingRun(t, "l1", "completed", 5, 1, 0, stamp(60))
	addListingRun(t, "l1", "completed", 0, 0, 0, stamp(10))

	c, ok := lastCrawl(t, "l1")
	if !ok {
		t.Fatal("expected a summary")
	}
	if c.JobsSaved != 0 || c.JobsUpdated != 0 || c.PagesVisited != 2 || c.StoppedReason != "no_next_link" || c.TriggeredBy != "auto_discovery" {
		t.Errorf("latest run: %+v", c)
	}
	if len(c.NotFoundFields) != 1 || c.NotFoundFields[0] != "title" {
		t.Errorf("notFoundFields: %v", c.NotFoundFields)
	}
	if c.ZeroStreak != 1 {
		t.Errorf("streak = %d, want 1", c.ZeroStreak)
	}
	if c.LastJobsAt == "" {
		t.Errorf("lastJobsAt must name the earlier run that found jobs")
	}
}

func TestZeroStreakCountsOnlyRunsSinceTheLastRunWithJobs(t *testing.T) {
	setupAttemptsDB(t, "l1")
	addListingRun(t, "l1", "completed", 0, 0, 0, stamp(50))
	addListingRun(t, "l1", "completed", 0, 0, 0, stamp(40))
	addListingRun(t, "l1", "completed", 0, 3, 0, stamp(30)) // only updated jobs: still found jobs
	addListingRun(t, "l1", "completed", 0, 0, 0, stamp(20))
	addListingRun(t, "l1", "completed", 0, 0, 4, stamp(10)) // all skipped: no usable job

	c, _ := lastCrawl(t, "l1")
	if c.ZeroStreak != 2 {
		t.Errorf("streak = %d, want 2 (the runs after the one that updated jobs)", c.ZeroStreak)
	}
	if c.JobsSkipped != 4 {
		t.Errorf("skipped = %d, want 4", c.JobsSkipped)
	}

	// A run that finds jobs ends the streak.
	addListingRun(t, "l1", "completed", 2, 0, 0, stamp(1))
	if c, _ := lastCrawl(t, "l1"); c.ZeroStreak != 0 || c.JobsSaved != 2 {
		t.Errorf("after a run with jobs: %+v", c)
	}
}

func TestOnlyCompletedListingRunsWithDataCount(t *testing.T) {
	setupAttemptsDB(t, "l1", "l2", "l3")

	// l1: a failed run after a completed one must not become "the latest".
	addListingRun(t, "l1", "completed", 3, 0, 0, stamp(30))
	addListingRun(t, "l1", "failed", 0, 0, 0, stamp(5))
	if c, ok := lastCrawl(t, "l1"); !ok || c.JobsSaved != 3 || c.ZeroStreak != 0 {
		t.Errorf("a failed run must be ignored: %+v ok=%v", c, ok)
	}

	// l2: only a run from before step 90 → no entry at all.
	addListingRun(t, "l2", "completed", -1, 0, 0, stamp(30))
	if _, ok := lastCrawl(t, "l2"); ok {
		t.Errorf("a run without data must not produce a summary")
	}

	// l3: a job-detail run is not a listing run.
	if _, err := db.JobsDB.Exec(`INSERT INTO crawl_runs (id, portal_link_id, kind, status, finished_at, jobs_saved) VALUES ('detail', 'l3', 'job_detail', 'completed', ?, 0)`, stamp(5)); err != nil {
		t.Fatal(err)
	}
	if _, ok := lastCrawl(t, "l3"); ok {
		t.Errorf("a job-detail run must not produce a listing summary")
	}
}

func TestJobsFromThisLinkAndNeverHadJobs(t *testing.T) {
	setupAttemptsDB(t, "l1", "l2")
	for i := 0; i < 3; i++ {
		if _, _, _, err := jobs.SavePortalJob("l1", fmt.Sprintf("https://x/l1/%d", i), fmt.Sprintf("Job %d", i), "", "", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	addListingRun(t, "l1", "completed", 0, 0, 0, stamp(5))
	addListingRun(t, "l2", "completed", 0, 0, 0, stamp(5))

	if c, _ := lastCrawl(t, "l1"); c.JobsFromThisLink != 3 {
		t.Errorf("l1 jobsFromThisLink = %d, want 3", c.JobsFromThisLink)
	}
	c2, _ := lastCrawl(t, "l2")
	if c2.JobsFromThisLink != 0 || c2.LastJobsAt != "" {
		t.Errorf("a link that never produced a job: %+v", c2)
	}
}

func TestAcknowledgementIsRFC3339AndClearable(t *testing.T) {
	setupAttemptsDB(t, "l1")
	addListingRun(t, "l1", "completed", 0, 0, 0, stamp(5))

	if err := SetEmptyCrawlAcknowledged("l1", true); err != nil {
		t.Fatal(err)
	}
	c, _ := lastCrawl(t, "l1")
	ack, err := time.Parse(time.RFC3339, c.AcknowledgedAt)
	if err != nil {
		t.Fatalf("acknowledgedAt %q is not RFC3339: %v", c.AcknowledgedAt, err)
	}
	finished, _ := time.Parse(time.RFC3339, c.FinishedAt)
	if !ack.After(finished) {
		t.Errorf("the acknowledgement (%s) must be later than the run it acknowledges (%s)", c.AcknowledgedAt, c.FinishedAt)
	}

	if err := SetEmptyCrawlAcknowledged("l1", false); err != nil {
		t.Fatal(err)
	}
	if c, _ := lastCrawl(t, "l1"); c.AcknowledgedAt != "" {
		t.Errorf("cleared acknowledgement still present: %q", c.AcknowledgedAt)
	}
	if err := SetEmptyCrawlAcknowledged("nope", true); !errors.Is(err, ErrUnknownPortalLink) {
		t.Errorf("unknown link: %v", err)
	}
}

func TestListPortalsIncludesLastListingCrawlOnlyWhenThereIsData(t *testing.T) {
	setupAttemptsDB(t, "l1", "l2")
	addListingRun(t, "l1", "completed", 0, 0, 0, stamp(5))

	portals, err := ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]portalLink{}
	for _, l := range portals[0].Links {
		byID[l.ID] = l
	}
	if byID["l1"].LastListingCrawl == nil || byID["l1"].LastListingCrawl.ZeroStreak != 1 {
		t.Errorf("l1: %+v", byID["l1"].LastListingCrawl)
	}
	if byID["l2"].LastListingCrawl != nil {
		t.Errorf("a link with no crawl data must have no lastListingCrawl")
	}

	for id, wantKey := range map[string]bool{"l1": true, "l2": false} {
		raw, _ := json.Marshal(byID[id])
		var decoded map[string]any
		_ = json.Unmarshal(raw, &decoded)
		if _, has := decoded["lastListingCrawl"]; has != wantKey {
			t.Errorf("%s: lastListingCrawl key present = %v, want %v", id, has, wantKey)
		}
	}
	if l1 := byID["l1"].LastListingCrawl; l1 != nil {
		raw, _ := json.Marshal(l1)
		var decoded map[string]any
		_ = json.Unmarshal(raw, &decoded)
		for _, key := range []string{"finishedAt", "triggeredBy", "jobsSaved", "jobsUpdated", "jobsSkipped", "pagesVisited", "stoppedReason", "notFoundFields", "zeroStreak", "jobsFromThisLink"} {
			if _, ok := decoded[key]; !ok {
				t.Errorf("summary JSON is missing %q", key)
			}
		}
	}
}
