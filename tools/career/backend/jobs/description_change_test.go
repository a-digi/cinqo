package jobs

import (
	"strings"
	"testing"

	"career-tool-backend/db"
)

// Step 164: every write that really changes an existing job's description announces it, once, after the write.

func changedEvents(c *capturedEvents) []map[string]any { return c.ofTopic(DescriptionChangedTopic) }

func stampOf(t *testing.T, id string) (changedAt *string) {
	t.Helper()
	if err := db.JobsDB.QueryRow(`SELECT description_changed_at FROM jobs WHERE id = ?`, id).Scan(&changedAt); err != nil {
		t.Fatal(err)
	}
	return changedAt
}

func seedCrawledJob(t *testing.T, id, description string) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, description, source) VALUES (?, ?, 'A job', ?, 'crawled')`, id, "https://x/"+id, description); err != nil {
		t.Fatal(err)
	}
}

func TestADetailCrawlThatFillsInTheFullTextAnnouncesTheChange(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	seedCrawledJob(t, "j1", "short snippet")

	updated, err := SaveJobDetailExtraction("j1", map[string]string{"description": "The full text of the posting, much longer than the snippet."})
	if err != nil || !updated {
		t.Fatalf("save: %v %v", updated, err)
	}
	got := changedEvents(events)
	if len(got) != 1 || payloadOf(got[0])["job_id"] != "j1" || payloadOf(got[0])["source"] != DescriptionSourceDetailCrawl || len(payloadOf(got[0])) != 2 {
		t.Fatalf("events: %v", got)
	}
	if stampOf(t, "j1") == nil {
		t.Error("description_changed_at must be set")
	}
	var status, text string
	_ = db.JobsDB.QueryRow(`SELECT detail_crawl_status, description FROM jobs WHERE id = 'j1'`).Scan(&status, &text)
	if status != "success" || !strings.HasPrefix(text, "The full text") {
		t.Errorf("the save itself must be unchanged: %q %q", status, text)
	}
}

func TestTheEventComesAfterTheWrite(t *testing.T) {
	setupJobsDB(t)
	seedCrawledJob(t, "j1", "old")
	var seen string
	restore := SetDescriptionChangedPublisher(func(jobID, source string) {
		_ = db.JobsDB.QueryRow(`SELECT description FROM jobs WHERE id = ?`, jobID).Scan(&seen)
	})
	defer restore()
	if _, err := SaveJobDetailExtraction("j1", map[string]string{"description": "brand new text"}); err != nil {
		t.Fatal(err)
	}
	if seen != "brand new text" {
		t.Errorf("a consumer must read the new text when the event goes out, it read %q", seen)
	}
}

func TestTheSameTextAgainAnnouncesNothing(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	seedCrawledJob(t, "j1", "same text")
	if _, err := SaveJobDetailExtraction("j1", map[string]string{"description": "same text"}); err != nil {
		t.Fatal(err)
	}
	if n := len(changedEvents(events)); n != 0 || stampOf(t, "j1") != nil {
		t.Errorf("no change: %d events, stamp %v", n, stampOf(t, "j1"))
	}
	// the first real change sets the stamp, a repeat of that text does not move it
	if _, err := SaveJobDetailExtraction("j1", map[string]string{"description": "other text"}); err != nil {
		t.Fatal(err)
	}
	first := stampOf(t, "j1")
	if first == nil || len(changedEvents(events)) != 1 {
		t.Fatalf("one change expected: %v %d", first, len(changedEvents(events)))
	}
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET description_changed_at = '2000-01-01 00:00:00' WHERE id = 'j1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveJobDetailExtraction("j1", map[string]string{"description": "other text"}); err != nil {
		t.Fatal(err)
	}
	if got := stampOf(t, "j1"); got == nil || *got != "2000-01-01 00:00:00" || len(changedEvents(events)) != 1 {
		t.Errorf("a repeat must not touch the stamp or send again: %v %d", got, len(changedEvents(events)))
	}
}

func TestAStoredTextThatOnlyDiffersByMarkupIsNotAChange(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	seedCrawledJob(t, "j1", "<p>Hello   <b>world</b></p>") // a legacy row from before only text was kept (step 147)
	if _, err := SaveJobDetailExtraction("j1", map[string]string{"description": "<p>Hello   <b>world</b></p>"}); err != nil {
		t.Fatal(err)
	}
	if n := len(changedEvents(events)); n != 0 {
		t.Errorf("the same text in other clothes must not announce a change: %d", n)
	}
}

func TestAManualJobKeepsItsTypedTextAndAnnouncesNothing(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	j := mustAdd(t, ManualJobInput{Title: "Typed", Description: "what the person wrote"})
	if _, err := SaveJobDetailExtraction(j.ID, map[string]string{"description": "what the page says"}); err != nil {
		t.Fatal(err)
	}
	var text string
	_ = db.JobsDB.QueryRow(`SELECT description FROM jobs WHERE id = ?`, j.ID).Scan(&text)
	if text != "what the person wrote" || len(changedEvents(events)) != 0 || stampOf(t, j.ID) != nil {
		t.Errorf("typed text is never replaced and never announced: %q %d", text, len(changedEvents(events)))
	}
}

func TestAManualJobWithoutATextGetsItFilledInAndAnnouncesIt(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	j := mustAdd(t, ManualJobInput{Title: "Typed"})
	if _, err := SaveJobDetailExtraction(j.ID, map[string]string{"description": "from the page"}); err != nil {
		t.Fatal(err)
	}
	if got := changedEvents(events); len(got) != 1 || payloadOf(got[0])["job_id"] != j.ID {
		t.Errorf("a filled-in description is a change: %v", got)
	}
}

func TestAnEmptyExtractionIsAFailureNotAChange(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	seedCrawledJob(t, "j1", "good old text")
	if updated, err := SaveJobDetailExtraction("j1", map[string]string{"description": "   "}); err != nil || updated {
		t.Fatalf("%v %v", updated, err)
	}
	if n := len(changedEvents(events)); n != 0 || stampOf(t, "j1") != nil {
		t.Errorf("a failed crawl changes nothing: %d", n)
	}
}

func TestAnUnknownJobIsStillAnError(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	if _, err := SaveJobDetailExtraction("nope", map[string]string{"description": "x"}); err == nil {
		t.Error("an unknown job must still be an error")
	}
	if len(changedEvents(events)) != 0 {
		t.Error("nothing announced for an unknown job")
	}
}

// ---- listing refreshes ----

func addLink(t *testing.T) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`INSERT INTO portals (id, name) VALUES ('p1', 'P')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://x.example/list')`); err != nil {
		t.Fatal(err)
	}
}

func TestAListingRefreshThatChangesTheTextAnnouncesIt(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	addLink(t)
	id, created, _, err := SavePortalJob("l1", "https://x.example/job/1", "Engineer", "Acme", "Hamburg", "first snippet", "")
	if err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	if n := len(changedEvents(events)); n != 0 {
		t.Fatalf("a NEW job is announced by job.found only, not as a change: %d", n)
	}
	// a refresh with the same text: nothing
	if _, _, _, err := SavePortalJob("l1", "https://x.example/job/1", "Engineer", "Acme", "Hamburg", "first snippet", ""); err != nil {
		t.Fatal(err)
	}
	if n := len(changedEvents(events)); n != 0 {
		t.Fatalf("same text: %d", n)
	}
	// a refresh with other text: one event with the listing source
	if _, _, _, err := SavePortalJob("l1", "https://x.example/job/1", "Engineer", "Acme", "Hamburg", "a new snippet", ""); err != nil {
		t.Fatal(err)
	}
	got := changedEvents(events)
	if len(got) != 1 || payloadOf(got[0])["job_id"] != id || payloadOf(got[0])["source"] != DescriptionSourceListingCrawl {
		t.Fatalf("events: %v", got)
	}
	if stampOf(t, id) == nil {
		t.Error("the stamp must be set")
	}
}

func TestSaveJobRefreshAnnouncesToo(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	id, created, err := saveJob("https://x.example/job/2", "Engineer", "Acme", "Hamburg", "one", "")
	if err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	if _, _, err := saveJob("https://x.example/job/2", "Engineer", "Acme", "Hamburg", "two", ""); err != nil {
		t.Fatal(err)
	}
	if got := changedEvents(events); len(got) != 1 || payloadOf(got[0])["job_id"] != id || payloadOf(got[0])["source"] != DescriptionSourceListingCrawl {
		t.Errorf("events: %v", got)
	}
	if stampOf(t, id) == nil {
		t.Error("the stamp must be set by this path too")
	}
}

// The values of the event's source field are part of the contract with its consumers' logs; pin them literally.
func TestTheSourceNamesAreFixed(t *testing.T) {
	if DescriptionSourceDetailCrawl != "detail_crawl" || DescriptionSourceListingCrawl != "listing_crawl" {
		t.Errorf("%q %q", DescriptionSourceDetailCrawl, DescriptionSourceListingCrawl)
	}
	if DescriptionChangedTopic != "career.job.description_changed" {
		t.Errorf("%q", DescriptionChangedTopic)
	}
}

func TestAListingRefreshNeverAnnouncesOverAFullTextFromADetailCrawl(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	addLink(t)
	id, _, _, _ := SavePortalJob("l1", "https://x.example/job/1", "Engineer", "Acme", "Hamburg", "snippet", "")
	if _, err := SaveJobDetailExtraction(id, map[string]string{"description": "the full text"}); err != nil {
		t.Fatal(err)
	}
	before := len(changedEvents(events)) // the detail crawl's own announcement
	if before != 1 {
		t.Fatalf("detail crawl: %d", before)
	}
	// the next listing crawl brings the snippet again: the matches were made from the full text, so no new announcement
	if _, _, _, err := SavePortalJob("l1", "https://x.example/job/1", "Engineer", "Acme", "Hamburg", "snippet", ""); err != nil {
		t.Fatal(err)
	}
	if n := len(changedEvents(events)); n != before {
		t.Errorf("a listing snippet over a detail-crawled text must not announce: %d", n)
	}
}

func TestAListingRefreshKeepsATypedManualTextSilently(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	addLink(t)
	j := mustAdd(t, ManualJobInput{Title: "Typed", SourceURL: "https://x.example/job/9", Description: "typed"})
	if _, _, _, err := SavePortalJob("l1", "https://x.example/job/9", "Typed", "Acme", "Hamburg", "crawled snippet", ""); err != nil {
		t.Fatal(err)
	}
	var text string
	_ = db.JobsDB.QueryRow(`SELECT description FROM jobs WHERE id = ?`, j.ID).Scan(&text)
	if text != "typed" || len(changedEvents(events)) != 0 || stampOf(t, j.ID) != nil {
		t.Errorf("%q %d", text, len(changedEvents(events)))
	}
}

func TestACrossPortalDuplicateWritesAndAnnouncesNothing(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	addLink(t)
	if _, _, _, err := SavePortalJob("l1", "https://x.example/job/1", "Same Title", "Same Co", "Hamburg", "text one", ""); err != nil {
		t.Fatal(err)
	}
	_, _, duplicate, err := SavePortalJob("l1", "https://y.example/other/1", "Same Title", "Same Co", "Hamburg", "text two", "")
	if err != nil || !duplicate {
		t.Fatalf("%v %v", duplicate, err)
	}
	if n := len(changedEvents(events)); n != 0 {
		t.Errorf("a duplicate is not a change: %d", n)
	}
}

func TestTheOneOffDescriptionCleanupAnnouncesNothing(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	seedCrawledJob(t, "j1", "<p>messy <b>markup</b></p>")
	if _, err := CleanStoredDescriptions(); err != nil {
		t.Fatal(err)
	}
	if n := len(changedEvents(events)); n != 0 {
		t.Errorf("the one-off cleanup is not a change of content: %d", n)
	}
}
