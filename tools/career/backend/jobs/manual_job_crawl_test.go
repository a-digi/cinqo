package jobs

import (
	"testing"

	"career-tool-backend/db"
)

func addPortalLink(t *testing.T, id, url string) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`INSERT OR IGNORE INTO portals (id, name) VALUES ('p1', 'Portal')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES (?, 'p1', ?)`, id, url); err != nil {
		t.Fatal(err)
	}
}

type jobRow struct {
	Title, Company, Location, Description, PostedAt, PortalLinkID, CrawledAt, Source, Status string
}

func rowOf(t *testing.T, id string) jobRow {
	t.Helper()
	var r jobRow
	var company, location, description, postedAt, link, status *string
	if err := db.JobsDB.QueryRow(
		`SELECT title, company, location, description, posted_at, portal_link_id, crawled_at, source, detail_crawl_status FROM jobs WHERE id = ?`, id,
	).Scan(&r.Title, &company, &location, &description, &postedAt, &link, &r.CrawledAt, &r.Source, &status); err != nil {
		t.Fatal(err)
	}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	r.Company, r.Location, r.Description, r.PostedAt, r.PortalLinkID, r.Status = str(company), str(location), str(description), str(postedAt), str(link), str(status)
	return r
}

func setCrawledAt(t *testing.T, id, value string) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET crawled_at = ? WHERE id = ?`, value, id); err != nil {
		t.Fatal(err)
	}
}

// A crawl that finds the posting a person added by hand links it to the portal
// but never replaces what they typed.
func TestACrawlKeepsAManualJobsTextAndLinksThePortal(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	manual := mustAdd(t, ManualJobInput{
		Title: "My own title", Company: "My company", Location: "My place", Description: "My own notes about the role", PostedAt: "yesterday",
		SourceURL: "https://portal.example/jobs/1",
	})
	setCrawledAt(t, manual.ID, "2026-01-01 10:00:00")

	id, created, duplicate, err := SavePortalJob("l1", "https://portal.example/jobs/1", "Crawled title", "Crawled co", "Crawled place", "Crawled description", "today")
	if err != nil || id != manual.ID || created || duplicate {
		t.Fatalf("id=%q created=%v duplicate=%v err=%v", id, created, duplicate, err)
	}
	got := rowOf(t, manual.ID)
	if got.Title != "My own title" || got.Company != "My company" || got.Location != "My place" || got.Description != "My own notes about the role" || got.PostedAt != "yesterday" {
		t.Errorf("typed text must survive a crawl: %+v", got)
	}
	if got.PortalLinkID != "l1" {
		t.Errorf("the crawl must link the job to its portal: %q", got.PortalLinkID)
	}
	if got.Source != "manual" {
		t.Errorf("a crawl must not turn a manual job into a crawled one: %q", got.Source)
	}
	if got.CrawledAt != "2026-01-01 10:00:00" {
		t.Errorf("the time it was added must not move: %q", got.CrawledAt)
	}
}

func TestACrawlCompletesWhatAPersonLeftEmptyButNothingElse(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	manual := mustAdd(t, ManualJobInput{Title: "Only a title", SourceURL: "https://portal.example/jobs/2"})
	if _, _, _, err := SavePortalJob("l1", "https://portal.example/jobs/2", "Crawled title", "Crawled co", "Crawled place", "Crawled description", "today"); err != nil {
		t.Fatal(err)
	}
	got := rowOf(t, manual.ID)
	if got.Title != "Only a title" {
		t.Errorf("the title is always the person's: %q", got.Title)
	}
	if got.Company != "Crawled co" || got.Location != "Crawled place" || got.Description != "Crawled description" || got.PostedAt != "today" {
		t.Errorf("empty fields must be completed from the crawl: %+v", got)
	}
}

func TestAnAlreadyLinkedManualJobKeepsItsPortalLink(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/a")
	addPortalLink(t, "l2", "https://portal.example/b")
	manual := mustAdd(t, ManualJobInput{Title: "T", SourceURL: "https://portal.example/jobs/3"})
	if _, _, _, err := SavePortalJob("l1", "https://portal.example/jobs/3", "x", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := SavePortalJob("l2", "https://portal.example/jobs/3", "x", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if got := rowOf(t, manual.ID).PortalLinkID; got != "l1" {
		t.Errorf("a manual job already linked stays linked to the first: %q", got)
	}
}

// The behavior for every crawled job is exactly what it was.
func TestACrawledJobIsStillRefreshedInFull(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	id, created, _, err := SavePortalJob("l1", "https://portal.example/jobs/9", "Old title", "Old co", "Old place", "Old description", "old")
	if err != nil || !created {
		t.Fatal(err)
	}
	setCrawledAt(t, id, "2026-01-01 10:00:00")
	if _, created, _, err := SavePortalJob("l1", "https://portal.example/jobs/9", "New title", "New co", "New place", "New description", "new"); err != nil || created {
		t.Fatalf("%v %v", created, err)
	}
	got := rowOf(t, id)
	if got.Title != "New title" || got.Company != "New co" || got.Location != "New place" || got.Description != "New description" || got.PostedAt != "new" || got.Source != "crawled" {
		t.Errorf("a crawled job is refreshed in full: %+v", got)
	}
	if got.CrawledAt == "2026-01-01 10:00:00" {
		t.Error("a crawled job's crawled_at is refreshed")
	}
}

func TestSaveJobFromTheAIKeepsAManualJobsTextToo(t *testing.T) {
	setupJobsDB(t)
	manual := mustAdd(t, ManualJobInput{Title: "Mine", Description: "My notes", SourceURL: "https://example.com/jobs/5"})
	id, created, err := saveJob("https://example.com/jobs/5", "AI title", "AI co", "AI place", "AI description", "ai date")
	if err != nil || created || id != manual.ID {
		t.Fatalf("%q %v %v", id, created, err)
	}
	got := rowOf(t, manual.ID)
	if got.Title != "Mine" || got.Description != "My notes" || got.Company != "AI co" || got.Location != "AI place" || got.Source != "manual" {
		t.Errorf("%+v", got)
	}
	// And a crawled job is still refreshed by it.
	crawled, _, _ := saveJob("https://example.com/jobs/6", "Old", "", "", "", "")
	if _, _, err := saveJob("https://example.com/jobs/6", "New", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if rowOf(t, crawled).Title != "New" {
		t.Error("save_job still refreshes a crawled job")
	}
}

// A different URL with the same title and company is still a cross-portal
// duplicate of the manual job: no second row.
func TestACrawlOfTheSamePostingElsewhereIsADuplicateOfAManualJob(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l1", "https://portal.example/search")
	manual := mustAdd(t, ManualJobInput{Title: "Backend Engineer", Company: "Acme"})
	id, created, duplicate, err := SavePortalJob("l1", "https://other.example/jobs/77", "backend engineer", "ACME", "", "", "")
	if err != nil || created || !duplicate || id != manual.ID {
		t.Fatalf("id=%q created=%v duplicate=%v err=%v", id, created, duplicate, err)
	}
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil || n != 1 {
		t.Errorf("no second row: %d (%v)", n, err)
	}
}

func TestADetailCrawlDoesNotReplaceAManualDescription(t *testing.T) {
	setupJobsDB(t)
	typed := mustAdd(t, ManualJobInput{Title: "With notes", Description: "My notes"})
	blank := mustAdd(t, ManualJobInput{Title: "Without notes"})
	crawledID, _, _ := saveJob("https://example.com/jobs/8", "Crawled", "", "", "Listing preview", "")
	setCrawledAt(t, typed.ID, "2026-01-01 10:00:00")

	for _, id := range []string{typed.ID, blank.ID, crawledID} {
		if updated, err := SaveJobDetailExtraction(id, map[string]string{"description": "Full text from the detail page"}); err != nil || !updated {
			t.Fatalf("%s: %v %v", id, updated, err)
		}
	}
	if got := rowOf(t, typed.ID); got.Description != "My notes" || got.Status != "success" || got.CrawledAt != "2026-01-01 10:00:00" {
		t.Errorf("typed notes must survive: %+v", got)
	}
	if got := rowOf(t, blank.ID); got.Description != "Full text from the detail page" {
		t.Errorf("an empty description is completed: %+v", got)
	}
	if got := rowOf(t, crawledID); got.Description != "Full text from the detail page" {
		t.Errorf("a crawled job's description is replaced as before: %+v", got)
	}
}
