package portal

import (
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/deletedjobs"
)

func rememberURL(t *testing.T, url string) {
	t.Helper()
	if _, err := deletedjobs.Remember(deletedjobs.Entry{SourceURL: url, Title: "gone", Reason: deletedjobs.ReasonDeleted}); err != nil {
		t.Fatal(err)
	}
}

func liveJobs(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A crawl that finds jobs the user deleted before does not add them, counts them as ignored, and ALSO counts them as
// updated, so the zero-jobs flag (saved + updated) does not mistake a link whose jobs were all deleted for a broken one.
// Both result shapes (grouped items and parallel arrays) behave the same.
func TestIngestIgnoresDeletedJobsInBothShapes(t *testing.T) {
	setupAttemptsDB(t, "l1")
	rememberURL(t, "https://jobs.example/a")
	rememberURL(t, "https://jobs.example/b")

	pages := []CrawlResultPage{
		{URL: "https://jobs.example/list?page=1", Items: []map[string]any{
			{"title": "Job A", "url": "https://www.jobs.example/a/?utm_source=x", "company": "Acme"}, // deleted, another spelling
			{"title": "Job C", "url": "https://jobs.example/c", "company": "Acme"},                   // new
			{"title": "", "url": "https://jobs.example/skipme"},                                      // skipped, no title
		}},
		{URL: "https://jobs.example/list?page=2", Results: map[string]any{
			"title":   []any{"Job B", "Job D"},
			"url":     []any{"https://jobs.example/b", "https://jobs.example/d"},
			"company": []any{"Globex", "Initech"},
		}},
	}
	res, err := IngestCrawlResults("l1", pages)
	if err != nil {
		t.Fatal(err)
	}
	if res.JobsSaved != 2 || res.JobsIgnored != 2 || res.JobsUpdated != 2 || res.JobsSkipped != 1 {
		t.Errorf("%+v, want saved 2, ignored 2, updated 2 (the ignored ones), skipped 1", res)
	}
	if liveJobs(t) != 2 {
		t.Errorf("%d jobs, want only C and D", liveJobs(t))
	}

	// the same crawl again: C and D are live and refreshed, A and B are still turned away
	res, err = IngestCrawlResults("l1", pages)
	if err != nil {
		t.Fatal(err)
	}
	if res.JobsSaved != 0 || res.JobsIgnored != 2 || res.JobsUpdated != 4 || liveJobs(t) != 2 {
		t.Errorf("second crawl: %+v, %d jobs", res, liveJobs(t))
	}
}

// A crawl whose every job was deleted before still counts as having found jobs (saved + updated is not zero).
func TestACrawlOfOnlyDeletedJobsIsNotAnEmptyCrawl(t *testing.T) {
	setupAttemptsDB(t, "l1")
	rememberURL(t, "https://jobs.example/a")
	res, err := IngestCrawlResults("l1", []CrawlResultPage{{URL: "https://jobs.example/list", Items: []map[string]any{
		{"title": "Job A", "url": "https://jobs.example/a"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.JobsSaved+res.JobsUpdated == 0 || res.JobsIgnored != 1 || liveJobs(t) != 0 {
		t.Errorf("%+v, %d jobs: the crawl found a job, so it must not look empty", res, liveJobs(t))
	}
}

// Nothing remembered: the counters are exactly what they were.
func TestIngestWithoutMemoryIsUnchanged(t *testing.T) {
	setupAttemptsDB(t, "l1")
	res, err := IngestCrawlResults("l1", []CrawlResultPage{{URL: "https://jobs.example/list", Items: []map[string]any{
		{"title": "Job A", "url": "https://jobs.example/a"}, {"title": "Job B", "url": "https://jobs.example/b"},
	}}})
	if err != nil || res != (IngestCrawlResultsResult{JobsSaved: 2}) {
		t.Errorf("%+v, %v", res, err)
	}
}
