package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
	"career-tool-backend/jobs/deletedjobs"
)

func remember(t *testing.T, url string) {
	t.Helper()
	if _, err := deletedjobs.Remember(deletedjobs.Entry{SourceURL: url, Title: "gone", Reason: deletedjobs.ReasonDeleted}); err != nil {
		t.Fatal(err)
	}
}

func jobCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func portalLink(t *testing.T) {
	t.Helper()
	exec(t, `INSERT INTO portals (id, name) VALUES ('p1', 'Alpha')`)
	exec(t, `INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://portal.example/list')`)
}

const goneURL = "https://www.portal.example/jobs/Backend-Engineer--1234567-inline.html"

// The same URL written in any equivalent way is ignored by a crawl: nothing is inserted and no event is sent.
func TestACrawlIgnoresADeletedJobInEverySpelling(t *testing.T) {
	setupJobsDB(t)
	portalLink(t)
	events := captureEvents(t)
	remember(t, goneURL)
	for _, u := range []string{
		goneURL,
		"http://www.portal.example/jobs/Backend-Engineer--1234567-inline.html",
		"https://portal.example/jobs/Backend-Engineer--1234567-inline.html",
		"https://PORTAL.example/jobs/Backend-Engineer--1234567-inline.html/",
		goneURL + "?utm_source=newsletter&gclid=x#apply",
		"https://portal.example:443/jobs/Backend-Engineer--1234567-inline.html",
	} {
		id, created, duplicate, err := SavePortalJob("l1", u, "Backend Engineer", "Acme", "Berlin", "d", "today")
		if !errors.Is(err, ErrDeletedBefore) || id != "" || created || duplicate {
			t.Errorf("%s: id=%q created=%v duplicate=%v err=%v", u, id, created, duplicate, err)
		}
		if _, created, err := saveJob(u, "Backend Engineer", "Acme", "Berlin", "d", "today"); !errors.Is(err, ErrDeletedBefore) || created {
			t.Errorf("saveJob %s: created=%v err=%v", u, created, err)
		}
	}
	if jobCount(t) != 0 {
		t.Errorf("%d jobs were inserted", jobCount(t))
	}
	if events.count() != 0 {
		t.Errorf("%d events were sent for ignored jobs", events.count())
	}
}

// A job that is not remembered still comes in, and a title that matches a remembered job but has another URL is a
// different job (only the URL decides for now).
func TestOnlyTheURLDecides(t *testing.T) {
	setupJobsDB(t)
	portalLink(t)
	remember(t, goneURL)
	for _, u := range []string{
		"https://www.portal.example/jobs/Backend-Engineer--7654321-inline.html",      // another id
		"https://www.portal.example/jobs/Backend-Engineer--1234567-inline.html?id=2", // an identifying parameter
		"https://other.example/jobs/Backend-Engineer--1234567-inline.html",           // another host
	} {
		_, created, _, err := SavePortalJob("l1", u, "Backend Engineer", "Acme"+u, "Berlin", "d", "today")
		if err != nil || !created {
			t.Errorf("%s: created=%v err=%v", u, created, err)
		}
	}
	if _, created, err := saveJob("https://www.portal.example/other", "Backend Engineer", "", "", "", ""); err != nil || !created {
		t.Errorf("saveJob: %v %v", created, err)
	}
}

// A live job is always refreshed, even if its URL is in the memory (an entry written before the filter existed, or by
// a delete that failed): the memory is only asked about a URL no row has.
func TestALiveJobIsRefreshedEvenIfItsURLIsRemembered(t *testing.T) {
	setupJobsDB(t)
	portalLink(t)
	id, created, _, err := SavePortalJob("l1", goneURL, "Old title", "Acme", "Berlin", "d", "today")
	if err != nil || !created {
		t.Fatal(err)
	}
	remember(t, goneURL)
	id2, created, _, err := SavePortalJob("l1", goneURL, "New title", "Acme", "Berlin", "d2", "today")
	if err != nil || created || id2 != id {
		t.Fatalf("id=%q created=%v err=%v", id2, created, err)
	}
	var title string
	_ = db.JobsDB.QueryRow(`SELECT title FROM jobs WHERE id = ?`, id).Scan(&title)
	if title != "New title" {
		t.Errorf("title %q: the live job was not refreshed", title)
	}
	id3, created, err := saveJob(goneURL, "Newest", "", "", "", "")
	if err != nil || created || id3 != id {
		t.Errorf("saveJob: id=%q created=%v err=%v", id3, created, err)
	}
}

// The ignore beats the cross-portal duplicate check, so a crawl reports "ignored" rather than "duplicate".
func TestIgnoredComesBeforeDuplicate(t *testing.T) {
	setupJobsDB(t)
	portalLink(t)
	if _, _, _, err := SavePortalJob("l1", "https://portal.example/jobs/live", "Same title", "Same company", "", "", ""); err != nil {
		t.Fatal(err)
	}
	remember(t, "https://other.example/jobs/gone")
	_, _, duplicate, err := SavePortalJob("l1", "https://other.example/jobs/gone", "Same title", "Same company", "", "", "")
	if !errors.Is(err, ErrDeletedBefore) || duplicate {
		t.Errorf("duplicate=%v err=%v", duplicate, err)
	}
	// a URL that is not remembered keeps the old answer
	if _, _, duplicate, err := SavePortalJob("l1", "https://other.example/jobs/new", "Same title", "Same company", "", "", ""); err != nil || !duplicate {
		t.Errorf("not remembered: duplicate=%v err=%v", duplicate, err)
	}
}

// A relative URL from a portal is made absolute against the portal link first, and that is what is compared.
func TestARelativeURLIsComparedAfterItIsMadeAbsolute(t *testing.T) {
	setupJobsDB(t)
	portalLink(t)
	remember(t, "https://portal.example/jobs/9")
	if _, _, _, err := SavePortalJob("l1", "/jobs/9", "x", "y", "", "", ""); !errors.Is(err, ErrDeletedBefore) {
		t.Errorf("err = %v", err)
	}
	if _, created, _, err := SavePortalJob("l1", "/jobs/10", "x", "z", "", "", ""); err != nil || !created {
		t.Errorf("another path: %v %v", created, err)
	}
}

// The memory is a safety net: unreadable or missing, the crawl saves as it always did.
func TestACrawlSavesWhenTheMemoryIsMissingOrBroken(t *testing.T) {
	setupJobsDB(t)
	portalLink(t)
	remember(t, goneURL)
	good := db.DeletedJobsDB
	db.DeletedJobsDB = nil
	if _, created, _, err := SavePortalJob("l1", goneURL, "Backend", "Acme", "", "", ""); err != nil || !created {
		t.Fatalf("no memory: %v %v", created, err)
	}
	db.DeletedJobsDB = good
	_ = good.Close()
	if _, created, err := saveJob("https://x.example/a", "Alpha", "", "", "", ""); err != nil || !created {
		t.Fatalf("closed memory: %v %v", created, err)
	}
}

// The whole loop: a crawled job is deleted, the crawl brings it again and is turned away; adding it by hand clears the
// entry, and a later crawl refreshes the hand-added job instead of being ignored.
func TestDeleteCrawlAddByHandCrawl(t *testing.T) {
	setupJobsDB(t)
	portalLink(t)
	id, _, _, err := SavePortalJob("l1", goneURL, "Backend Engineer", "Acme", "Berlin", "d", "today")
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteJob(id); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := SavePortalJob("l1", goneURL+"?utm_medium=mail", "Backend Engineer", "Acme", "Berlin", "d", "today"); !errors.Is(err, ErrDeletedBefore) {
		t.Fatalf("after delete: %v", err)
	}
	if jobCount(t) != 0 {
		t.Fatal("the job came back")
	}
	added, err := AddManualJob(ManualJobInput{Title: "Backend Engineer", Company: "Acme", SourceURL: goneURL})
	if err != nil {
		t.Fatal(err)
	}
	if has, _ := deletedjobs.Has(goneURL); has {
		t.Error("adding it by hand did not clear the entry")
	}
	_, created, _, err := SavePortalJob("l1", goneURL, "Crawled title", "Acme", "Berlin", "d", "today")
	if err != nil || created {
		t.Errorf("a later crawl: created=%v err=%v (the hand-added job should be refreshed)", created, err)
	}
	if jobCount(t) != 1 || added.ID == "" {
		t.Errorf("%d jobs", jobCount(t))
	}
}

// The entry is cleared whichever way the person spelled the URL they typed in.
func TestAddingByHandClearsTheEntryWhateverTheSpelling(t *testing.T) {
	setupJobsDB(t)
	remember(t, goneURL)
	if _, err := AddManualJob(ManualJobInput{Title: "Typed", SourceURL: "http://portal.example/jobs/Backend-Engineer--1234567-inline.html/?utm_source=x"}); err != nil {
		t.Fatal(err)
	}
	if has, _ := deletedjobs.Has(goneURL); has {
		t.Error("the entry survived")
	}
}

// Adding by hand without a URL, or a URL nobody deleted, leaves the memory alone.
func TestAddingByHandLeavesOtherEntriesAlone(t *testing.T) {
	setupJobsDB(t)
	remember(t, "https://a.example/1")
	remember(t, "https://b.example/2")
	if _, err := AddManualJob(ManualJobInput{Title: "No url"}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddManualJob(ManualJobInput{Title: "Other url", SourceURL: "https://c.example/3"}); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"https://a.example/1", "https://b.example/2"} {
		if has, _ := deletedjobs.Has(u); !has {
			t.Errorf("%s was forgotten", u)
		}
	}
	if _, err := AddManualJob(ManualJobInput{Title: "Back", SourceURL: "https://a.example/1"}); err != nil {
		t.Fatal(err)
	}
	if has, _ := deletedjobs.Has("https://a.example/1"); has {
		t.Error("a.example/1 should be forgotten")
	}
	if has, _ := deletedjobs.Has("https://b.example/2"); !has {
		t.Error("b.example/2 was forgotten")
	}
}

func mcpSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterSaveJob(server)
	RegisterSavePortalJob(server)
	RegisterSavePortalJobs(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func callTool(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || res.IsError {
		t.Fatalf("%s: %v %+v", name, err, res)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// What the AI is told: ignored:true for a deleted job (and not an error), ignored counted apart in the batch.
func TestTheAIToolsReportIgnored(t *testing.T) {
	setupJobsDB(t)
	portalLink(t)
	remember(t, goneURL)
	s := mcpSession(t)

	one := callTool(t, s, "save_portal_job", map[string]any{"portalLinkId": "l1", "sourceUrl": goneURL, "title": "Backend", "company": "Acme"})
	if one["ignored"] != true || one["created"] != false || one["duplicate"] != false || one["id"] != "" {
		t.Errorf("save_portal_job: %v", one)
	}
	fresh := callTool(t, s, "save_portal_job", map[string]any{"portalLinkId": "l1", "sourceUrl": "https://portal.example/jobs/new", "title": "Backend", "company": "Acme"})
	if fresh["created"] != true || fresh["ignored"] != nil {
		t.Errorf("a normal save must not mention ignored: %v", fresh)
	}
	saved := callTool(t, s, "save_job", map[string]any{"sourceUrl": goneURL, "title": "Backend"})
	if saved["ignored"] != true || saved["created"] != false {
		t.Errorf("save_job: %v", saved)
	}
	batch := callTool(t, s, "save_portal_jobs", map[string]any{"portalLinkId": "l1", "jobs": []map[string]any{
		{"sourceUrl": goneURL, "title": "A", "company": "X"},
		{"sourceUrl": goneURL + "/", "title": "B", "company": "Y"},
		{"sourceUrl": "https://portal.example/jobs/c", "title": "C", "company": "Z"},
		{"sourceUrl": "https://portal.example/jobs/new", "title": "N", "company": "Acme"},
		{"sourceUrl": "", "title": "no url", "company": "Q"},
	}})
	if batch["ignored"] != float64(2) || batch["saved"] != float64(1) || batch["updated"] != float64(1) || batch["skipped"] != float64(1) || batch["jobsProcessed"] != float64(5) || batch["duplicates"] != float64(0) {
		t.Errorf("save_portal_jobs: %v", batch)
	}
}
