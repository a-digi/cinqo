package jobs

import (
	"testing"

	"career-tool-backend/db"
)

// Step 148: a job can be recrawled when it has a posting URL and a portal link with job detail crawl instructions.
func TestCanRecrawl(t *testing.T) {
	setupJobsDB(t)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.JobsDB.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO portals (id, name) VALUES ('p', 'Portal')`)
	exec(`INSERT INTO portal_links (id, portal_id, url, job_detail_crawl_instructions) VALUES
		('with', 'p', 'https://a.example', 'fields: []'),
		('null', 'p', 'https://b.example', NULL),
		('empty', 'p', 'https://c.example', ''),
		('blank', 'p', 'https://d.example', ?)`, "  \n\t\r\n ")
	add := func(id, url, link, source string) {
		t.Helper()
		var l any
		if link != "" {
			l = link
		}
		exec(`INSERT INTO jobs (id, source_url, title, portal_link_id, source, crawled_at) VALUES (?, ?, ?, ?, ?, datetime('now'))`, id, url, id, l, source)
	}
	add("yes", "https://a.example/1", "with", "crawled")
	add("manual-yes", "https://a.example/2", "with", "manual")
	add("no-instructions-null", "https://b.example/1", "null", "crawled")
	add("no-instructions-empty", "https://c.example/1", "empty", "crawled")
	add("no-instructions-blank", "https://d.example/1", "blank", "crawled")
	add("no-link", "https://e.example/1", "", "crawled")
	add("manual-no-link", "https://e.example/2", "", "manual")
	add("no-url", "", "with", "manual")

	want := map[string]bool{
		"yes": true, "manual-yes": true, "no-instructions-null": false, "no-instructions-empty": false, "no-instructions-blank": false,
		"no-link": false, "manual-no-link": false, "no-url": false,
	}
	list, err := SearchJobs("", "", nil, "", "", 100, 0)
	if err != nil || len(list.Jobs) != len(want) {
		t.Fatalf("%d jobs: %v", len(list.Jobs), err)
	}
	for _, j := range list.Jobs {
		if j.CanRecrawl != want[j.ID] {
			t.Errorf("list: %s canRecrawl=%v, want %v", j.ID, j.CanRecrawl, want[j.ID])
		}
	}
	for id, w := range want {
		one, err := GetJobByID(id)
		if err != nil {
			t.Fatal(err)
		}
		if one.CanRecrawl != w {
			t.Errorf("get: %s canRecrawl=%v, want %v", id, one.CanRecrawl, w)
		}
	}
}

// Setting the instructions later turns the flag on for jobs already there; it is derived on every read, never stored.
func TestCanRecrawlFollowsTheInstructions(t *testing.T) {
	setupJobsDB(t)
	addPortalLink(t, "l", "https://a.example/search")
	id, _, err := saveJob("https://a.example/jobs/1", "T", "", "", "text", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`UPDATE jobs SET portal_link_id = 'l' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	before, _ := GetJobByID(id)
	if before.CanRecrawl {
		t.Error("no instructions yet")
	}
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET job_detail_crawl_instructions = 'fields: []' WHERE id = 'l'`); err != nil {
		t.Fatal(err)
	}
	after, _ := GetJobByID(id)
	if !after.CanRecrawl {
		t.Error("instructions are set now")
	}
}
