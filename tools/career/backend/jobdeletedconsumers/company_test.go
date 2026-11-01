package jobdeletedconsumers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

func setupDB(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func rows(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// published is a fake core that records the topics of the events it receives (and their payloads).
type published struct {
	mu     sync.Mutex
	events []map[string]any
}

func fakeCoreEvents(t *testing.T) *published {
	t.Helper()
	p := &published{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var ev map[string]any
		_ = json.Unmarshal(raw, &ev)
		p.mu.Lock()
		p.events = append(p.events, ev)
		p.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return p
}

func (p *published) ofTopic(topic string) []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []map[string]any
	for _, ev := range p.events {
		if ev["topic"] == topic {
			out = append(out, ev)
		}
	}
	return out
}

func deliverCompany(method, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	CompanyDeleteHandler(rec, httptest.NewRequest(method, "/events/job-deleted-company", strings.NewReader(body)))
	return rec
}

func eventFor(jobID, companyID string) string {
	b, _ := json.Marshal(map[string]any{"topic": "career.job.deleted", "payload": map[string]string{"job_id": jobID, "title": "T", "company": "Acme", "company_id": companyID, "source": "crawled"}})
	return string(b)
}

func TestTheLastJobOfACompanyWithNoRecruitersDeletesTheCompany(t *testing.T) {
	setupDB(t)
	events := fakeCoreEvents(t)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c1', 'Acme')`)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c2', 'Other')`)

	if rec := deliverCompany(http.MethodPost, eventFor("gone-job", "c1")); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rows(t, `SELECT COUNT(*) FROM companies WHERE id = 'c1'`) != 0 || rows(t, `SELECT COUNT(*) FROM companies WHERE id = 'c2'`) != 1 {
		t.Error("exactly c1 must be gone")
	}
	got := events.ofTopic("career.company.deleted")
	if len(got) != 1 {
		t.Fatalf("company.deleted events = %d, want 1", len(got))
	}
	if p, _ := got[0]["payload"].(map[string]any); p["company_id"] != "c1" || p["name"] != "Acme" {
		t.Errorf("payload = %v", p)
	}
}

func TestACompanyWithARemainingJobIsKept(t *testing.T) {
	setupDB(t)
	events := fakeCoreEvents(t)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c1', 'Acme')`)
	exec(t, `INSERT INTO jobs (id, source_url, title, company_id) VALUES ('other', 'https://x.example/o', 'Other job', 'c1')`)

	if rec := deliverCompany(http.MethodPost, eventFor("gone-job", "c1")); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if rows(t, `SELECT COUNT(*) FROM companies WHERE id = 'c1'`) != 1 || len(events.ofTopic("career.company.deleted")) != 0 {
		t.Error("the company must stay and nothing be announced")
	}
}

func TestACompanyWithARecruiterIsKept(t *testing.T) {
	setupDB(t)
	events := fakeCoreEvents(t)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c1', 'Acme')`)
	exec(t, `INSERT INTO recruiters (id, company_id, first_name) VALUES ('r1', 'c1', 'Rita')`)

	if rec := deliverCompany(http.MethodPost, eventFor("gone-job", "c1")); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if rows(t, `SELECT COUNT(*) FROM companies WHERE id = 'c1'`) != 1 || rows(t, `SELECT COUNT(*) FROM recruiters`) != 1 || len(events.ofTopic("career.company.deleted")) != 0 {
		t.Error("the company and its recruiter must stay and nothing be announced")
	}
}

func TestAJobWithoutACompanyTouchesNoCompany(t *testing.T) {
	setupDB(t)
	events := fakeCoreEvents(t)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c1', 'Acme')`) // unused, but this job does not name it
	if rec := deliverCompany(http.MethodPost, eventFor("gone-job", "")); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if rows(t, `SELECT COUNT(*) FROM companies`) != 1 || len(events.ofTopic("career.company.deleted")) != 0 {
		t.Error("no company may be deleted for a job without one")
	}
}

// A company created by hand with no jobs is never deleted unless a deleted job names it.
func TestOnlyTheNamedCompanyIsEverConsidered(t *testing.T) {
	setupDB(t)
	fakeCoreEvents(t)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c1', 'Acme')`)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c2', 'By hand, no jobs')`)
	deliverCompany(http.MethodPost, eventFor("gone-job", "c1"))
	if rows(t, `SELECT COUNT(*) FROM companies WHERE id = 'c2'`) != 1 {
		t.Error("an unrelated unused company must stay")
	}
}

func TestAnUnknownCompanyAndARedeliveryAreNoOps(t *testing.T) {
	setupDB(t)
	events := fakeCoreEvents(t)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c1', 'Acme')`)
	if rec := deliverCompany(http.MethodPost, eventFor("j", "nope")); rec.Code != http.StatusOK {
		t.Errorf("unknown company: status %d, want 200", rec.Code)
	}
	for i := 0; i < 2; i++ {
		if rec := deliverCompany(http.MethodPost, eventFor("j", "c1")); rec.Code != http.StatusOK {
			t.Errorf("delivery %d: status %d", i, rec.Code)
		}
	}
	if n := len(events.ofTopic("career.company.deleted")); n != 1 {
		t.Errorf("company.deleted events = %d, want 1 (the redelivery finds nothing)", n)
	}
}

func TestCompanyDeleteRejectsBadEventsAndMethods(t *testing.T) {
	setupDB(t)
	fakeCoreEvents(t)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c1', 'Acme')`)
	for name, body := range map[string]string{
		"not json":     `nope`,
		"no payload":   `{"topic":"career.job.deleted"}`,
		"empty job id": `{"payload":{"job_id":"","company_id":"c1"}}`,
	} {
		if rec := deliverCompany(http.MethodPost, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		if rec := deliverCompany(m, eventFor("j", "c1")); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status %d, want 405", m, rec.Code)
		}
	}
	if rows(t, `SELECT COUNT(*) FROM companies`) != 1 {
		t.Error("nothing may be deleted for bad requests")
	}
}

func TestADatabaseFailureIsA500SoCoreRetries(t *testing.T) {
	setupDB(t)
	fakeCoreEvents(t)
	db.JobsDB.Close() // every query now fails
	if rec := deliverCompany(http.MethodPost, eventFor("j", "c1")); rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", rec.Code)
	}
}

// The whole chain through the real code: deleting the last job announces career.job.deleted, the consumer deletes the
// company (it announces career.company.deleted), and a second job of another company is unaffected.
func TestDeletingTheLastJobEndToEnd(t *testing.T) {
	setupDB(t)
	events := fakeCoreEvents(t)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c1', 'Acme')`)
	exec(t, `INSERT INTO companies (id, name) VALUES ('c2', 'Keeps')`)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, company_id) VALUES ('j1', 'https://x.example/1', 'One', 'Acme', 'c1')`)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, company_id) VALUES ('j2', 'https://x.example/2', 'Two', 'Acme', 'c1')`)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, company_id) VALUES ('j3', 'https://x.example/3', 'Three', 'Keeps', 'c2')`)

	relay := func() { // what core does: hand every career.job.deleted event to the consumer
		for _, ev := range events.ofTopic("career.job.deleted") {
			raw, _ := json.Marshal(ev)
			deliverCompany(http.MethodPost, string(raw))
		}
		events.mu.Lock()
		events.events = nil
		events.mu.Unlock()
	}

	if err := jobs.DeleteJob("j1"); err != nil {
		t.Fatal(err)
	}
	relay()
	if rows(t, `SELECT COUNT(*) FROM companies WHERE id = 'c1'`) != 1 {
		t.Fatal("c1 still has j2, it must stay")
	}
	if err := jobs.DeleteJob("j2"); err != nil {
		t.Fatal(err)
	}
	relay()
	if rows(t, `SELECT COUNT(*) FROM companies WHERE id = 'c1'`) != 0 {
		t.Error("c1 lost its last job, it must be deleted")
	}
	if rows(t, `SELECT COUNT(*) FROM companies WHERE id = 'c2'`) != 1 || rows(t, `SELECT COUNT(*) FROM jobs WHERE id = 'j3'`) != 1 {
		t.Error("the other company and its job must be untouched")
	}
}
