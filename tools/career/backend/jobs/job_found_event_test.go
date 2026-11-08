package jobs

import "testing"

func payloadOf(ev map[string]any) map[string]any {
	p, _ := ev["payload"].(map[string]any)
	return p
}

func TestAManualJobWithoutACompanyPublishesJobFoundButNotJobCreated(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	j := mustAdd(t, ManualJobInput{Title: "Backend Engineer"})

	if n := len(events.ofTopic("career.job.created")); n != 0 {
		t.Errorf("job.created must stay company-only, got %d", n)
	}
	found := events.ofTopic("career.job.found")
	if len(found) != 1 {
		t.Fatalf("job.found events = %d, want 1", len(found))
	}
	p := payloadOf(found[0])
	if p["job_id"] != j.ID || p["title"] != "Backend Engineer" || p["company"] != "" || p["source"] != "manual" {
		t.Errorf("payload = %v", p)
	}
}

func TestAManualJobWithACompanyPublishesBothEvents(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)
	j := mustAdd(t, ManualJobInput{Title: "Platform Engineer", Company: "Acme GmbH"})

	created := events.ofTopic("career.job.created")
	if len(created) != 1 || payloadOf(created[0])["job_id"] != j.ID || payloadOf(created[0])["company"] != "Acme GmbH" || len(payloadOf(created[0])) != 2 {
		t.Errorf("job.created must be unchanged: %v", created)
	}
	found := events.ofTopic("career.job.found")
	if len(found) != 1 || payloadOf(found[0])["company"] != "Acme GmbH" || payloadOf(found[0])["title"] != "Platform Engineer" {
		t.Errorf("job.found = %v", found)
	}
}

func TestACrawledJobPublishesJobFoundOncePerNewRowWithOrWithoutACompany(t *testing.T) {
	setupJobsDB(t)
	exec(t, `INSERT INTO portals (id, name) VALUES ('p1', 'Alpha')`)
	exec(t, `INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://portal.example/list')`)
	events := captureEvents(t)

	withCompany, created, _, err := SavePortalJob("l1", "https://portal.example/jobs/1", "Job one", "Acme", "Berlin", "d", "today")
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	noCompany, created, _, err := SavePortalJob("l1", "https://portal.example/jobs/2", "Job two", "", "", "", "")
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	found := events.ofTopic("career.job.found")
	if len(found) != 2 {
		t.Fatalf("job.found events = %d, want 2", len(found))
	}
	if p := payloadOf(found[0]); p["job_id"] != withCompany || p["source"] != "crawled" || p["company"] != "Acme" {
		t.Errorf("first = %v", p)
	}
	if p := payloadOf(found[1]); p["job_id"] != noCompany || p["company"] != "" || p["title"] != "Job two" {
		t.Errorf("second = %v", p)
	}
	if n := len(events.ofTopic("career.job.created")); n != 1 {
		t.Errorf("job.created is for the job with a company only, got %d", n)
	}

	// Saving the same posting again updates it: no new event of either kind.
	if _, created, _, err := SavePortalJob("l1", "https://portal.example/jobs/1", "Job one", "Acme", "Berlin", "d2", "today"); err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if n := len(events.ofTopic("career.job.found")); n != 2 {
		t.Errorf("an update must not publish job.found, got %d in total", n)
	}
	// The same title and company at another URL is a cross-portal duplicate: no event either.
	if _, created, duplicate, err := SavePortalJob("l1", "https://other.example/jobs/9", "Job one", "Acme", "Berlin", "d", "today"); err != nil || created || !duplicate {
		t.Fatalf("created=%v duplicate=%v err=%v", created, duplicate, err)
	}
	if n := len(events.ofTopic("career.job.found")); n != 2 {
		t.Errorf("a duplicate must not publish job.found, got %d in total", n)
	}
}

func TestSaveJobPublishesJobFoundOnlyForANewURL(t *testing.T) {
	setupJobsDB(t)
	events := captureEvents(t)

	id, created, err := saveJob("https://x.example/a", "Alpha", "", "", "", "")
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	found := events.ofTopic("career.job.found")
	if len(found) != 1 || payloadOf(found[0])["job_id"] != id || payloadOf(found[0])["source"] != "crawled" || payloadOf(found[0])["company"] != "" {
		t.Fatalf("job.found = %v", found)
	}
	if n := len(events.ofTopic("career.job.created")); n != 0 {
		t.Errorf("no company, no job.created: %d", n)
	}
	if _, created, err := saveJob("https://x.example/a", "Alpha 2", "", "", "", ""); err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if n := len(events.ofTopic("career.job.found")); n != 1 {
		t.Errorf("re-saving must not publish again, got %d", n)
	}
}
