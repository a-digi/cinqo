package jobs

import (
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs/deletedjobs"
)

type remembered struct {
	hash, url, title, company, location, portal, posted, reason string
	score                                                       *int
}

func memory(t *testing.T) []remembered {
	t.Helper()
	r, err := db.DeletedJobsDB.Query(`SELECT url_hash, source_url, title, company, location, portal_name, posted_at, match_score, reason FROM deleted_jobs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var out []remembered
	for r.Next() {
		var x remembered
		if err := r.Scan(&x.hash, &x.url, &x.title, &x.company, &x.location, &x.portal, &x.posted, &x.score, &x.reason); err != nil {
			t.Fatal(err)
		}
		out = append(out, x)
	}
	return out
}

func addCrawledJob(t *testing.T, id, url string) {
	t.Helper()
	exec(t, `INSERT INTO portals (id, name) VALUES ('p-'||?, 'StepStone')`, id)
	exec(t, `INSERT INTO portal_links (id, portal_id, url) VALUES ('l-'||?, 'p-'||?, 'https://www.stepstone.de/list')`, id, id)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description, posted_at, portal_link_id, source) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', 'd', 'today', 'l-'||?, 'crawled')`, id, url, id)
}

const stepstoneURL = "https://www.stepstone.de/stellenangebote--Head-of-Software-Engineering-Hamburg-freenet--14612706-inline.html"

// Deleting a crawled job remembers it first: the hash of its URL and a readable snapshot with its portal and last score.
func TestDeletingACrawledJobRemembersIt(t *testing.T) {
	setupJobsDB(t)
	addCrawledJob(t, "j1", stepstoneURL)
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('j1', 'p', 61, 'completed', 'manual')`)
	if err := DeleteJob("j1"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE id = 'j1'`).Scan(&n)
	if n != 0 {
		t.Fatal("the job was not deleted")
	}
	got := memory(t)
	if len(got) != 1 {
		t.Fatalf("%d entries", len(got))
	}
	r := got[0]
	if r.hash != deletedjobs.URLHash(stepstoneURL) || r.url != stepstoneURL || r.title != "Backend Engineer (m/w/d)" || r.company != "Acme" || r.location != "Hamburg" ||
		r.portal != "StepStone" || r.posted != "today" || r.reason != "deleted" || r.score == nil || *r.score != 61 {
		t.Errorf("%+v", r)
	}
}

// A job added by hand is never remembered, and neither is an id that does not exist; the delete itself is unaffected.
func TestAManualJobAndAnUnknownIDAreNotRemembered(t *testing.T) {
	setupJobsDB(t)
	j := mustAdd(t, ManualJobInput{Title: "Typed in", Company: "Me", SourceURL: "https://example.com/my-job"})
	if err := DeleteJob(j.ID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteJob("no-such-job"); err != nil {
		t.Fatal(err)
	}
	if got := memory(t); len(got) != 0 {
		t.Errorf("remembered: %+v", got)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE id = ?`, j.ID).Scan(&n)
	if n != 0 {
		t.Error("the manual job was not deleted")
	}
}

// A job with no company, location or portal is still remembered by its URL.
func TestAJobWithoutCompanyOrPortalIsRememberedByItsURL(t *testing.T) {
	setupJobsDB(t)
	exec(t, `INSERT INTO jobs (id, source_url, title, source) VALUES ('bare', 'https://example.com/jobs/9', 'Bare job', 'crawled')`)
	if err := DeleteJob("bare"); err != nil {
		t.Fatal(err)
	}
	got := memory(t)
	if len(got) != 1 || got[0].company != "" || got[0].location != "" || got[0].portal != "" || got[0].score != nil || got[0].hash != deletedjobs.URLHash("https://example.com/jobs/9") {
		t.Errorf("%+v", got)
	}
}

// The same URL deleted again (the job came back) is one entry, not two.
func TestDeletingTheSameURLAgainKeepsOneEntry(t *testing.T) {
	setupJobsDB(t)
	addCrawledJob(t, "j1", stepstoneURL)
	if err := DeleteJob("j1"); err != nil {
		t.Fatal(err)
	}
	exec(t, `INSERT INTO jobs (id, source_url, title, source) VALUES ('j2', ?, 'Back again', 'crawled')`, "http://stepstone.de"+stepstoneURL[len("https://www.stepstone.de"):]+"?utm_source=x")
	if err := DeleteJob("j2"); err != nil {
		t.Fatal(err)
	}
	got := memory(t)
	if len(got) != 1 || got[0].title != "Back again" {
		t.Errorf("%+v", got)
	}
}

// The memory is a safety net: when it is unavailable or broken the delete still happens.
func TestADeleteStillHappensWhenTheMemoryIsBroken(t *testing.T) {
	setupJobsDB(t)
	addCrawledJob(t, "j1", stepstoneURL)
	addCrawledJob(t, "j2", stepstoneURL+"2")
	broken := db.DeletedJobsDB
	db.DeletedJobsDB = nil // never opened
	if err := DeleteJob("j1"); err != nil {
		t.Fatalf("with no memory: %v", err)
	}
	db.DeletedJobsDB = broken
	_ = broken.Close() // opened, then closed: every call fails
	if err := DeleteJob("j2"); err != nil {
		t.Fatalf("with a closed memory: %v", err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE id IN ('j1','j2')`).Scan(&n)
	if n != 0 {
		t.Errorf("%d jobs survived", n)
	}
}

// The undo takes back only what the call itself added: for a delete that removes nothing.
func TestTheUndoRemovesOnlyANewEntry(t *testing.T) {
	setupJobsDB(t)
	addCrawledJob(t, "j1", stepstoneURL)
	forget := RememberBeforeDelete("j1", deletedjobs.ReasonLowMatch)
	if len(memory(t)) != 1 {
		t.Fatal("not remembered")
	}
	forget()
	if len(memory(t)) != 0 {
		t.Error("the new entry was not taken back")
	}
	// an entry that was already there stays when a later call undoes
	if _, err := deletedjobs.Remember(deletedjobs.Entry{SourceURL: stepstoneURL, Title: "old", Reason: deletedjobs.ReasonDeleted}); err != nil {
		t.Fatal(err)
	}
	forget = RememberBeforeDelete("j1", deletedjobs.ReasonLowMatch)
	forget()
	got := memory(t)
	if len(got) != 1 || got[0].reason != "low_match" {
		t.Errorf("a pre-existing entry should stay (refreshed): %+v", got)
	}
	// nothing to undo for a job that is not remembered
	RememberBeforeDelete("nope", deletedjobs.ReasonDeleted)()
	RememberBeforeDelete("j1", "bad-reason")()
}

// A delete that removes nothing (here a trigger swallows it) or fails must not leave an entry for a job that still
// exists: the entry written first is taken back.
func TestAnEntryIsTakenBackWhenTheDeleteRemovesNothingOrFails(t *testing.T) {
	setupJobsDB(t)
	addCrawledJob(t, "swallowed", stepstoneURL)
	exec(t, `CREATE TRIGGER swallow BEFORE DELETE ON jobs BEGIN SELECT RAISE(IGNORE); END`)
	if err := DeleteJob("swallowed"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE id = 'swallowed'`).Scan(&n)
	if n != 1 {
		t.Fatal("the trigger should have kept the job")
	}
	if got := memory(t); len(got) != 0 {
		t.Errorf("a job that still exists was remembered: %+v", got)
	}

	exec(t, `DROP TRIGGER swallow`)
	exec(t, `CREATE TRIGGER refuse BEFORE DELETE ON jobs BEGIN SELECT RAISE(ABORT, 'no'); END`)
	if err := DeleteJob("swallowed"); err == nil {
		t.Fatal("the failing delete should return its error")
	}
	if got := memory(t); len(got) != 0 {
		t.Errorf("a failed delete left an entry: %+v", got)
	}
}
