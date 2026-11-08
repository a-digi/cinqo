package crawl

import (
	"encoding/json"
	"reflect"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

func setupCrawlDB(t *testing.T) string {
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
	return "l1"
}

func TestNotFoundOnEveryPage(t *testing.T) {
	page := func(notFound ...string) portal.CrawlResultPage { return portal.CrawlResultPage{NotFound: notFound} }
	cases := []struct {
		name  string
		pages []portal.CrawlResultPage
		want  []string
	}{
		{"no pages", nil, []string{}},
		{"nothing missing", []portal.CrawlResultPage{page(), page()}, []string{}},
		{"missing on the only page", []portal.CrawlResultPage{page("url", "title")}, []string{"title", "url"}},
		{"missing on every page", []portal.CrawlResultPage{page("title"), page("title", "company")}, []string{"title"}},
		{"found on one page is not 'matched nothing'", []portal.CrawlResultPage{page("title"), page()}, []string{}},
		{"a label repeated on one page counts once", []portal.CrawlResultPage{page("title", "title"), page("title")}, []string{"title"}},
	}
	for _, c := range cases {
		got := notFoundOnEveryPage(c.pages)
		if got == nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %#v, want %#v", c.name, got, c.want)
		}
	}
}

func TestBuildListingOutcome(t *testing.T) {
	got := buildListingOutcome(
		portal.IngestCrawlResultsResult{JobsSaved: 3, JobsUpdated: 2, JobsSkipped: 1},
		crawlNowPaginatedResult{PagesVisited: 4, StoppedReason: "no_next_link", Pages: []portal.CrawlResultPage{{NotFound: []string{"company"}}}},
	)
	want := listingOutcome{JobsSaved: 3, JobsUpdated: 2, JobsSkipped: 1, PagesVisited: 4, StoppedReason: "no_next_link", NotFoundFields: []string{"company"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

type outcomeRow struct {
	status                         string
	saved, updated, skipped, pages *int
	stopped, notFound, summary     *string
}

func readOutcome(t *testing.T, id string) outcomeRow {
	t.Helper()
	var r outcomeRow
	if err := db.JobsDB.QueryRow(
		`SELECT status, jobs_saved, jobs_updated, jobs_skipped, pages_visited, stopped_reason, not_found_fields, result_summary FROM crawl_runs WHERE id = ?`, id,
	).Scan(&r.status, &r.saved, &r.updated, &r.skipped, &r.pages, &r.stopped, &r.notFound, &r.summary); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFinishListingCrawlRunStoresTheOutcome(t *testing.T) {
	link := setupCrawlDB(t)
	run, err := startCrawlRun(link, "listing", "auto_discovery")
	if err != nil {
		t.Fatal(err)
	}

	// A zero-job crawl: nothing matched, two selectors found nothing.
	out := listingOutcome{PagesVisited: 1, StoppedReason: "no_next_link", NotFoundFields: []string{"title", "url"}}
	if err := finishListingCrawlRun(run.ID, "Saved 0 new, updated 0, skipped 0 — visited 1 page(s) (no_next_link).", out); err != nil {
		t.Fatal(err)
	}

	r := readOutcome(t, run.ID)
	if r.status != "completed" || r.summary == nil {
		t.Fatalf("status/summary: %+v", r)
	}
	if *r.saved != 0 || *r.updated != 0 || *r.skipped != 0 || *r.pages != 1 || *r.stopped != "no_next_link" {
		t.Errorf("counts: saved=%d updated=%d skipped=%d pages=%d stopped=%q", *r.saved, *r.updated, *r.skipped, *r.pages, *r.stopped)
	}
	var fields []string
	if err := json.Unmarshal([]byte(*r.notFound), &fields); err != nil || !reflect.DeepEqual(fields, []string{"title", "url"}) {
		t.Errorf("not_found_fields = %q (%v)", *r.notFound, err)
	}

	// A normal crawl and an all-skipped crawl keep their own numbers.
	run2, _ := startCrawlRun(link, "listing", "manual")
	finishListingCrawlRun(run2.ID, "x", listingOutcome{JobsSaved: 7, JobsUpdated: 1, PagesVisited: 3, StoppedReason: "max_pages"})
	if r := readOutcome(t, run2.ID); *r.saved != 7 || *r.updated != 1 || *r.pages != 3 || *r.notFound != "[]" {
		t.Errorf("normal crawl: %+v", r)
	}
	run3, _ := startCrawlRun(link, "listing", "manual")
	finishListingCrawlRun(run3.ID, "x", listingOutcome{JobsSkipped: 17, PagesVisited: 1})
	if r := readOutcome(t, run3.ID); *r.saved != 0 || *r.skipped != 17 {
		t.Errorf("all-skipped crawl: %+v", r)
	}
}

func TestOtherFinishPathsLeaveTheOutcomeEmpty(t *testing.T) {
	link := setupCrawlDB(t)

	failed, _ := startCrawlRun(link, "listing", "manual")
	msg := "Cloudflare blocked page 2"
	if err := finishCrawlRun(failed.ID, "failed", nil, &msg); err != nil {
		t.Fatal(err)
	}
	if r := readOutcome(t, failed.ID); r.saved != nil || r.pages != nil || r.notFound != nil {
		t.Errorf("a failed run must have no outcome: %+v", r)
	}

	detail, _ := startCrawlRun(link, "job_detail", "manual")
	summary := "ok"
	if err := finishCrawlRun(detail.ID, "completed", &summary, nil); err != nil {
		t.Fatal(err)
	}
	if r := readOutcome(t, detail.ID); r.saved != nil || r.stopped != nil {
		t.Errorf("a job-detail run must have no listing outcome: %+v", r)
	}
}

func TestFinishListingCrawlRunNeverOverwritesACancelledRun(t *testing.T) {
	link := setupCrawlDB(t)
	run, _ := startCrawlRun(link, "listing", "manual")
	msg := "cancelled by the user"
	if err := finishCrawlRun(run.ID, "cancelled", nil, &msg); err != nil {
		t.Fatal(err)
	}
	if err := finishListingCrawlRun(run.ID, "late result", listingOutcome{JobsSaved: 5}); err != nil {
		t.Fatal(err)
	}
	if r := readOutcome(t, run.ID); r.status != "cancelled" || r.saved != nil {
		t.Errorf("a terminal run must not be rewritten: %+v", r)
	}
}
