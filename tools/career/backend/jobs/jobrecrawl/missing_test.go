package jobrecrawl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestFindMissingNamesTheJobsThatCanBeCrawledAndWhyTheOthersCannot(t *testing.T) {
	setup(t) // portal link l1 with job detail instructions
	exec(t, `INSERT INTO portal_links (id, portal_id, url) VALUES ('noinstr', 'p1', 'https://portal.example/b')`)
	exec(t, `INSERT INTO portal_links (id, portal_id, url, job_detail_crawl_instructions, requires_login) VALUES ('login', 'p1', 'https://portal.example/c', 'fields: []', 1)`)

	// crawlable, in the order they were crawled
	for _, id := range []string{"a", "b", "c"} {
		addJob(t, id)
	}
	exec(t, `UPDATE jobs SET crawled_at = '2026-01-01 00:00:03' WHERE id = 'a'`)
	exec(t, `UPDATE jobs SET crawled_at = '2026-01-01 00:00:01' WHERE id = 'b'`)
	exec(t, `UPDATE jobs SET crawled_at = '2026-01-01 00:00:02' WHERE id = 'c'`)
	// a failed detail crawl is tried again; a successful one never is
	addJob(t, "failed")
	exec(t, `UPDATE jobs SET detail_crawl_status = 'failed', crawled_at = '2026-01-01 00:00:09' WHERE id = 'failed'`)
	addJob(t, "done")
	exec(t, `UPDATE jobs SET detail_crawl_status = 'success' WHERE id = 'done'`)
	// cannot be crawled, each for its own reason
	addManual(t, "byhand", "", "https://by.hand/1") // no portal
	addManual(t, "nourl", "l1", "")                 // no posting address
	exec(t, `INSERT INTO jobs (id, source_url, title, portal_link_id, source, crawled_at) VALUES ('x1', 'https://portal.example/x1', 'x', 'noinstr', 'crawled', datetime('now'))`)
	exec(t, `INSERT INTO jobs (id, source_url, title, portal_link_id, source, crawled_at) VALUES ('l1job', 'https://portal.example/l1', 'x', 'login', 'crawled', datetime('now'))`)
	// already queued: not queued twice
	addJob(t, "queued")
	exec(t, `INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state) VALUES ('queued', 'b1', datetime('now'), 'queued')`)

	got, err := FindMissing()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"b", "c", "a", "failed"}; !reflect.DeepEqual(got.JobIDs, want) {
		t.Errorf("crawlable jobs %v, want %v", got.JobIDs, want)
	}
	if got.Total != 9 || got.NoPortal != 2 || got.NoInstructions != 1 || got.NeedsLogin != 1 || got.AlreadyBeingCrawled != 1 {
		t.Errorf("counts: %+v", got)
	}
}

// The button works whatever else runs: a crawl that is running for the link changes nothing.
func TestFindMissingIgnoresRunningCrawls(t *testing.T) {
	setup(t)
	addJob(t, "a")
	exec(t, `INSERT INTO crawl_runs (id, portal_link_id, status, kind) VALUES ('run1', 'l1', 'running', 'listing')`)
	got, err := FindMissing()
	if err != nil || !reflect.DeepEqual(got.JobIDs, []string{"a"}) {
		t.Errorf("%+v %v", got, err)
	}
}

func TestMissingHandler(t *testing.T) {
	setup(t)
	addJob(t, "a")
	rec := httptest.NewRecorder()
	MissingHandler(rec, httptest.NewRequest(http.MethodGet, "/jobs/recrawl/missing", nil))
	var got Missing
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 || !reflect.DeepEqual(got.JobIDs, []string{"a"}) || got.Total != 1 {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if rec := httptest.NewRecorder(); func() int { MissingHandler(rec, httptest.NewRequest(http.MethodPost, "/", nil)); return rec.Code }() != http.StatusMethodNotAllowed {
		t.Error("POST must be refused")
	}
	// nothing missing: an empty list, never null
	exec(t, `UPDATE jobs SET detail_crawl_status = 'success'`)
	rec = httptest.NewRecorder()
	MissingHandler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Body.String() == "" || !json.Valid(rec.Body.Bytes()) || !containsJSON(rec.Body.String(), `"jobIds":[]`) {
		t.Errorf("empty answer: %s", rec.Body)
	}
}

func containsJSON(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
