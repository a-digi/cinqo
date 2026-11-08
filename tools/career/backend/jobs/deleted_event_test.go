package jobs

import (
	"testing"

	"career-tool-backend/companies"
)

func TestDeletingALinkedJobPublishesJobDeletedWithItsCompanyId(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	companyID, err := companies.CreateCompany("Acme GmbH", "")
	if err != nil {
		t.Fatal(err)
	}
	id := seedJob(t, "Platform Engineer", "Acme GmbH")
	if err := LinkJobToCompany(id, companyID); err != nil {
		t.Fatal(err)
	}

	if err := DeleteJob(id); err != nil {
		t.Fatal(err)
	}
	got := events.ofTopic("career.job.deleted")
	if len(got) != 1 {
		t.Fatalf("job.deleted events = %d, want 1", len(got))
	}
	p := payloadOf(got[0])
	want := map[string]string{"job_id": id, "title": "Platform Engineer", "company": "Acme GmbH", "company_id": companyID, "source": "manual"}
	if len(p) != len(want) {
		t.Errorf("payload = %v", p)
	}
	for k, v := range want {
		if p[k] != v {
			t.Errorf("payload[%s] = %v, want %s", k, p[k], v)
		}
	}
	if _, err := GetJobByID(id); err == nil {
		t.Error("the job must be gone")
	}
}

func TestDeletingAJobWithoutACompanyLinkSendsAnEmptyCompanyId(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	named := seedJob(t, "Named only", "Free Text Inc") // company text, no link
	bare := seedJob(t, "Bare", "")
	for _, id := range []string{named, bare} {
		if err := DeleteJob(id); err != nil {
			t.Fatal(err)
		}
	}
	got := events.ofTopic("career.job.deleted")
	if len(got) != 2 {
		t.Fatalf("events = %d, want 2", len(got))
	}
	if p := payloadOf(got[0]); p["job_id"] != named || p["company"] != "Free Text Inc" || p["company_id"] != "" {
		t.Errorf("named = %v", p)
	}
	if p := payloadOf(got[1]); p["job_id"] != bare || p["company"] != "" || p["company_id"] != "" {
		t.Errorf("bare = %v", p)
	}
}

func TestDeletingACrawledJobSaysSourceCrawled(t *testing.T) {
	setupJobsDB(t)
	exec(t, `INSERT INTO portals (id, name) VALUES ('p1', 'Alpha')`)
	exec(t, `INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://portal.example/list')`)
	id, _, _, err := SavePortalJob("l1", "https://portal.example/jobs/1", "Job one", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	events := captureEvents(t)
	if err := DeleteJob(id); err != nil {
		t.Fatal(err)
	}
	got := events.ofTopic("career.job.deleted")
	if len(got) != 1 || payloadOf(got[0])["source"] != "crawled" {
		t.Errorf("events = %v", got)
	}
}

func TestDeletingAJobThatDoesNotExistPublishesNothing(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	keep := seedJob(t, "Keeper", "Acme")
	if err := DeleteJob("00000000-0000-4000-8000-000000000000"); err != nil {
		t.Fatal(err)
	}
	if n := len(events.ofTopic("career.job.deleted")); n != 0 {
		t.Errorf("%d job.deleted events for a job that was never there", n)
	}
	if _, err := GetJobByID(keep); err != nil {
		t.Errorf("the other job must stay: %v", err)
	}
}

func TestDeletingTheSameJobTwiceAnnouncesItOnce(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	id := seedJob(t, "Once", "Acme")
	for i := 0; i < 2; i++ {
		if err := DeleteJob(id); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(events.ofTopic("career.job.deleted")); n != 1 {
		t.Errorf("events = %d, want 1", n)
	}
}

func TestSnapshotForDeletedEventKnowsOnlyExistingJobs(t *testing.T) {
	setupJobsDB(t)
	id := seedJob(t, "Here", "Acme")
	if snap, ok := SnapshotForDeletedEvent(id); !ok || snap.ID != id || snap.Title != "Here" {
		t.Errorf("existing job: %v %v", snap, ok)
	}
	if snap, ok := SnapshotForDeletedEvent("nope"); ok || snap != (DeletedJob{}) {
		t.Errorf("unknown job: %v %v", snap, ok)
	}
}
