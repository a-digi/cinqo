package companies

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"career-tool-backend/db"
)

// events is a fake core that records every published event.
type events struct {
	mu  sync.Mutex
	got []map[string]any
}

func captureEvents(t *testing.T) *events {
	t.Helper()
	e := &events{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var ev map[string]any
		_ = json.Unmarshal(raw, &ev)
		e.mu.Lock()
		e.got = append(e.got, ev)
		e.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return e
}

func (e *events) ofTopic(topic string) []map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []map[string]any
	for _, ev := range e.got {
		if ev["topic"] == topic {
			out = append(out, ev)
		}
	}
	return out
}

func (e *events) total() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.got)
}

func payload(ev map[string]any) map[string]any {
	p, _ := ev["payload"].(map[string]any)
	return p
}

func TestDeletingACompanyPublishesCompanyDeletedWithItsIdAndName(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	add(t, "c1", "Acme GmbH", "", "2026-06-01 10:00:00")
	add(t, "c2", "Other", "", "2026-06-02 10:00:00")

	if err := DeleteCompany("c1"); err != nil {
		t.Fatal(err)
	}
	got := ev.ofTopic("career.company.deleted")
	if len(got) != 1 {
		t.Fatalf("events = %d, want 1", len(got))
	}
	p := payload(got[0])
	if len(p) != 2 || p["company_id"] != "c1" || p["name"] != "Acme GmbH" {
		t.Errorf("payload = %v", p)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM companies`).Scan(&n)
	if n != 1 {
		t.Errorf("companies left = %d, want only the other one", n)
	}
}

func TestDeletingAnUnknownCompanyPublishesNothing(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	add(t, "c1", "Acme", "", "2026-06-01 10:00:00")
	if err := DeleteCompany("nope"); err != nil {
		t.Fatal(err)
	}
	if n := ev.total(); n != 0 {
		t.Errorf("%d events for a company that was never there", n)
	}
}

func TestDeletingTheSameCompanyTwiceAnnouncesItOnce(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	add(t, "c1", "Acme", "", "2026-06-01 10:00:00")
	for i := 0; i < 2; i++ {
		if err := DeleteCompany("c1"); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(ev.ofTopic("career.company.deleted")); n != 1 {
		t.Errorf("events = %d, want 1", n)
	}
}

// Deleting a company keeps its recruiters (only their link rows go, step 169) and un-links its jobs, which stay; neither is a separate announcement.
func TestARecruiterSurvivesItsCompanyAndUnlinkedJobsSendNoEventsOfTheirOwn(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	add(t, "c1", "Acme", "", "2026-06-01 10:00:00")
	exec(t, `INSERT INTO recruiters (id, first_name) VALUES ('r1', 'Rita'); INSERT INTO recruiter_companies (recruiter_id, company_id) VALUES ('r1', 'c1')`)
	exec(t, `INSERT INTO jobs (id, source_url, title, company_id) VALUES ('j1', 'https://x.example/1', 'Job', 'c1')`)

	if err := DeleteCompany("c1"); err != nil {
		t.Fatal(err)
	}
	if n := ev.total(); n != 1 {
		t.Errorf("events = %d, want exactly the company's own", n)
	}
	var recruiters, links, linked, jobs int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM recruiter_companies`).Scan(&links)
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM recruiters`).Scan(&recruiters)
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE company_id IS NOT NULL`).Scan(&linked)
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&jobs)
	if recruiters != 1 || links != 0 || linked != 0 || jobs != 1 {
		t.Errorf("recruiters=%d links=%d linkedJobs=%d jobs=%d, want 1 0 0 1", recruiters, links, linked, jobs)
	}
}

func TestDeletingACompanyStillWorksWhenCoreIsUnreachable(t *testing.T) {
	setup(t)
	t.Setenv("CORE_API_URL", "http://127.0.0.1:1")
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	add(t, "c1", "Acme", "", "2026-06-01 10:00:00")
	if err := DeleteCompany("c1"); err != nil {
		t.Fatalf("a failed announcement must not fail the delete: %v", err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM companies`).Scan(&n)
	if n != 0 {
		t.Error("the company was not deleted")
	}
}

func count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func addJobTo(t *testing.T, id, companyID string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title, company_id) VALUES (?, ?, 'Job', ?)`, id, "https://x.example/"+id, companyID)
}

func TestAnUnusedCompanyIsDeletedAndAnnounced(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	add(t, "c1", "Acme GmbH", "", "2026-06-01 10:00:00")
	add(t, "c2", "Other", "", "2026-06-02 10:00:00")

	deleted, err := DeleteCompanyIfUnused("c1")
	if err != nil || !deleted {
		t.Fatalf("deleted=%v err=%v", deleted, err)
	}
	if count(t, `SELECT COUNT(*) FROM companies WHERE id = 'c1'`) != 0 || count(t, `SELECT COUNT(*) FROM companies WHERE id = 'c2'`) != 1 {
		t.Error("exactly c1 must be gone")
	}
	got := ev.ofTopic("career.company.deleted")
	if len(got) != 1 || payload(got[0])["company_id"] != "c1" || payload(got[0])["name"] != "Acme GmbH" {
		t.Errorf("events = %v", got)
	}
}

func TestACompanyWithAnotherJobIsKept(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	add(t, "c1", "Acme", "", "2026-06-01 10:00:00")
	addJobTo(t, "j-other", "c1")

	deleted, err := DeleteCompanyIfUnused("c1")
	if err != nil || deleted {
		t.Fatalf("deleted=%v err=%v", deleted, err)
	}
	if count(t, `SELECT COUNT(*) FROM companies WHERE id = 'c1'`) != 1 || count(t, `SELECT COUNT(*) FROM jobs WHERE company_id = 'c1'`) != 1 {
		t.Error("the company and its job must stay")
	}
	if n := ev.total(); n != 0 {
		t.Errorf("%d events for a company that stayed", n)
	}
}

func TestACompanyWithARecruiterIsKept(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	add(t, "c1", "Acme", "", "2026-06-01 10:00:00")
	exec(t, `INSERT INTO recruiters (id, first_name) VALUES ('r1', 'Rita'); INSERT INTO recruiter_companies (recruiter_id, company_id) VALUES ('r1', 'c1')`)

	deleted, err := DeleteCompanyIfUnused("c1")
	if err != nil || deleted {
		t.Fatalf("deleted=%v err=%v", deleted, err)
	}
	if count(t, `SELECT COUNT(*) FROM companies WHERE id = 'c1'`) != 1 || count(t, `SELECT COUNT(*) FROM recruiters`) != 1 {
		t.Error("the company and its recruiter must stay")
	}
	if n := ev.total(); n != 0 {
		t.Errorf("%d events for a company that stayed", n)
	}
}

func TestACompanyWithBothAJobAndARecruiterIsKept(t *testing.T) {
	setup(t)
	add(t, "c1", "Acme", "", "2026-06-01 10:00:00")
	addJobTo(t, "j1", "c1")
	exec(t, `INSERT INTO recruiters (id, first_name) VALUES ('r1', 'Rita'); INSERT INTO recruiter_companies (recruiter_id, company_id) VALUES ('r1', 'c1')`)
	if deleted, err := DeleteCompanyIfUnused("c1"); err != nil || deleted {
		t.Fatalf("deleted=%v err=%v", deleted, err)
	}
}

// Only jobs linked to THIS company count: another company's jobs and unlinked jobs do not keep it alive.
func TestOtherCompaniesJobsAndUnlinkedJobsDoNotKeepItAlive(t *testing.T) {
	setup(t)
	add(t, "c1", "Acme", "", "2026-06-01 10:00:00")
	add(t, "c2", "Other", "", "2026-06-02 10:00:00")
	addJobTo(t, "j2", "c2")
	exec(t, `INSERT INTO jobs (id, source_url, title, company) VALUES ('j3', 'https://x.example/3', 'Free text', 'Acme')`)
	exec(t, `INSERT INTO recruiters (id, first_name) VALUES ('r2', 'Rita'); INSERT INTO recruiter_companies (recruiter_id, company_id) VALUES ('r2', 'c2')`)

	deleted, err := DeleteCompanyIfUnused("c1")
	if err != nil || !deleted {
		t.Fatalf("deleted=%v err=%v", deleted, err)
	}
	if count(t, `SELECT COUNT(*) FROM companies WHERE id = 'c2'`) != 1 || count(t, `SELECT COUNT(*) FROM jobs`) != 2 || count(t, `SELECT COUNT(*) FROM recruiters`) != 1 {
		t.Error("the other company, the jobs and the recruiter must be untouched")
	}
}

func TestAnUnknownCompanyIsNoErrorAndNoEvent(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	deleted, err := DeleteCompanyIfUnused("nope")
	if err != nil || deleted || ev.total() != 0 {
		t.Errorf("deleted=%v err=%v events=%d", deleted, err, ev.total())
	}
}

func TestDeletingAnUnusedCompanyTwiceAnnouncesItOnce(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	add(t, "c1", "Acme", "", "2026-06-01 10:00:00")
	for i := 0; i < 2; i++ {
		if _, err := DeleteCompanyIfUnused("c1"); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(ev.ofTopic("career.company.deleted")); n != 1 {
		t.Errorf("events = %d, want 1", n)
	}
}
