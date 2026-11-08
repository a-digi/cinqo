package jobrecruiters

import (
	"errors"
	"fmt"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/recruiters"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
}

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func company(t *testing.T, id, name string) {
	t.Helper()
	exec(t, `INSERT INTO companies (id, name) VALUES (?, ?)`, id, name)
}

func recruiter(t *testing.T, id, companyID, first, last, email string) {
	t.Helper()
	exec(t, `INSERT INTO recruiters (id, first_name, last_name, email) VALUES (?, ?, ?, ?)`, id, first, last, email)
	if companyID != "" {
		exec(t, `INSERT INTO recruiter_companies (recruiter_id, company_id) VALUES (?, ?)`, id, companyID)
	}
}

func job(t *testing.T, id, title string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title) VALUES (?, ?, ?)`, id, "https://e.com/"+id, title)
}

func count(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_recruiters`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMapCreatesOnceAndAMapAgainChangesNothing(t *testing.T) {
	setup(t)
	company(t, "c1", "Acme")
	recruiter(t, "r1", "c1", "Ada", "Lovelace", "ada@acme.test")
	job(t, "j1", "Engineer")

	created, err := Map("j1", "r1")
	if err != nil || !created {
		t.Fatalf("first map: %v %v", created, err)
	}
	var first string
	if err := db.JobsDB.QueryRow(`SELECT created_at FROM job_recruiters`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	exec(t, `UPDATE job_recruiters SET created_at = '2020-01-01 00:00:00'`)
	created, err = Map("j1", "r1")
	if err != nil || created {
		t.Fatalf("second map: created=%v err=%v, want false and no error", created, err)
	}
	var at string
	if err := db.JobsDB.QueryRow(`SELECT created_at FROM job_recruiters`).Scan(&at); err != nil || at != "2020-01-01 00:00:00" || first == "" {
		t.Errorf("a repeated map changed the row: %q %v", at, err)
	}
	if count(t) != 1 {
		t.Errorf("%d rows", count(t))
	}
}

func TestMapRefusesAnUnknownJobOrRecruiterAndWritesNothing(t *testing.T) {
	setup(t)
	company(t, "c1", "Acme")
	recruiter(t, "r1", "c1", "Ada", "L", "a@x")
	job(t, "j1", "Engineer")
	if _, err := Map("nope", "r1"); !errors.Is(err, jobs.ErrUnknownJob) {
		t.Errorf("unknown job: %v", err)
	}
	if _, err := Map("j1", "nope"); !errors.Is(err, recruiters.ErrUnknownRecruiter) {
		t.Errorf("unknown recruiter: %v", err)
	}
	if _, err := Map("nope", "nope"); !errors.Is(err, jobs.ErrUnknownJob) {
		t.Errorf("both unknown must name the job first: %v", err)
	}
	if _, err := Map("", ""); !errors.Is(err, jobs.ErrUnknownJob) {
		t.Errorf("empty ids: %v", err)
	}
	if count(t) != 0 {
		t.Error("a refused map wrote a row")
	}
}

func TestUnmapIsIdempotent(t *testing.T) {
	setup(t)
	company(t, "c1", "Acme")
	recruiter(t, "r1", "c1", "Ada", "L", "a@x")
	recruiter(t, "r2", "c1", "Bob", "M", "b@x")
	job(t, "j1", "Engineer")
	for _, r := range []string{"r1", "r2"} {
		if _, err := Map("j1", r); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := Unmap("j1", "r1"); err != nil {
			t.Fatalf("unmap %d: %v", i, err)
		}
	}
	if err := Unmap("nope", "nope"); err != nil {
		t.Errorf("unmapping unknown ids: %v", err)
	}
	got, _ := ForJob("j1")
	if len(got) != 1 || got[0].ID != "r2" {
		t.Errorf("only r2 should stay: %+v", got)
	}
}

func TestForJobListsTheRecruitersWithTheirCompanyNewestFirst(t *testing.T) {
	setup(t)
	company(t, "c1", "Acme")
	company(t, "c2", "Beta Agency")
	recruiter(t, "r1", "c1", "Ada", "Lovelace", "ada@acme.test")
	recruiter(t, "r2", "c2", "Bob", "Marley", "")
	recruiter(t, "r3", "c2", "Cy", "Young", "cy@beta.test")
	job(t, "j1", "Engineer")
	job(t, "j2", "Other")
	for _, r := range []string{"r1", "r2", "r3"} {
		if _, err := Map("j1", r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Map("j2", "r1"); err != nil {
		t.Fatal(err)
	}
	// one second apart is the resolution of the stored time: give the first its own earlier time
	exec(t, `UPDATE job_recruiters SET created_at = '2026-01-01 10:00:00' WHERE job_id = 'j1' AND recruiter_id = 'r1'`)
	exec(t, `UPDATE job_recruiters SET created_at = '2026-01-02 10:00:00' WHERE job_id = 'j1' AND recruiter_id = 'r2'`)
	exec(t, `UPDATE job_recruiters SET created_at = '2026-01-02 10:00:00' WHERE job_id = 'j1' AND recruiter_id = 'r3'`)

	got, err := ForJob("j1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	// the two with the same time come in the order they were mapped, newest first: r3 then r2, then the older r1
	if got[0].ID != "r3" || got[1].ID != "r2" || got[2].ID != "r1" {
		t.Errorf("order %s %s %s", got[0].ID, got[1].ID, got[2].ID)
	}
	if got[2].CompanyName != "Acme" || got[2].CompanyID != "c1" || got[2].FirstName != "Ada" || got[2].LastName != "Lovelace" || got[2].Email != "ada@acme.test" || got[2].MappedAt != "2026-01-01 10:00:00" {
		t.Errorf("fields %+v", got[2])
	}
	if got[0].CompanyName != "Beta Agency" || got[1].Email != "" {
		t.Errorf("fields %+v %+v", got[0], got[1])
	}
	if other, _ := ForJob("j2"); len(other) != 1 || other[0].ID != "r1" {
		t.Errorf("j2: %+v", other)
	}
	if none, err := ForJob("nope"); err != nil || none == nil || len(none) != 0 {
		t.Errorf("an unknown job must give an empty, non-nil list: %v %v", none, err)
	}
}

func TestForRecruiterListsTheJobsNewestFirst(t *testing.T) {
	setup(t)
	company(t, "c1", "Acme")
	recruiter(t, "r1", "c1", "Ada", "L", "a@x")
	recruiter(t, "r2", "c1", "Bob", "M", "b@x")
	exec(t, `INSERT INTO jobs (id, source_url, title, company, company_id, location) VALUES ('j1', 'https://e.com/1', 'Engineer', 'Acme', 'c1', 'Hamburg')`)
	job(t, "j2", "Plain job")
	if _, err := Map("j1", "r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := Map("j2", "r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := Map("j2", "r2"); err != nil {
		t.Fatal(err)
	}
	exec(t, `UPDATE job_recruiters SET created_at = '2026-01-01 10:00:00' WHERE job_id = 'j1'`)
	exec(t, `UPDATE job_recruiters SET created_at = '2026-01-02 10:00:00' WHERE job_id = 'j2'`)
	got, err := ForRecruiter("r1")
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if got[0].ID != "j2" || got[1].ID != "j1" {
		t.Errorf("order %s %s", got[0].ID, got[1].ID)
	}
	if got[1].Title != "Engineer" || got[1].Company != "Acme" || got[1].CompanyID != "c1" || got[1].Location != "Hamburg" || got[1].MappedAt != "2026-01-01 10:00:00" {
		t.Errorf("fields %+v", got[1])
	}
	if got[0].Company != "" || got[0].CompanyID != "" || got[0].Location != "" {
		t.Errorf("a job without a company or location must have empty fields: %+v", got[0])
	}
	if r2, _ := ForRecruiter("r2"); len(r2) != 1 || r2[0].ID != "j2" {
		t.Errorf("r2: %+v", r2)
	}
	if none, err := ForRecruiter("nope"); err != nil || none == nil || len(none) != 0 {
		t.Errorf("an unknown recruiter must give an empty, non-nil list: %v %v", none, err)
	}
}

// Deleting a job or a recruiter removes the mappings by itself; deleting a company removes none of them (step 169) (foreign keys are on for jobs.db).
func TestDeletingAJobARecruiterOrACompanyRemovesItsMappings(t *testing.T) {
	setup(t)
	company(t, "c1", "Acme")
	company(t, "c2", "Beta")
	recruiter(t, "r1", "c1", "Ada", "L", "a@x")
	recruiter(t, "r2", "c2", "Bob", "M", "b@x")
	for i := 1; i <= 3; i++ {
		job(t, fmt.Sprintf("j%d", i), "Job")
	}
	for _, p := range [][2]string{{"j1", "r1"}, {"j1", "r2"}, {"j2", "r1"}, {"j2", "r2"}, {"j3", "r2"}} {
		if _, err := Map(p[0], p[1]); err != nil {
			t.Fatal(err)
		}
	}
	if count(t) != 5 {
		t.Fatalf("%d", count(t))
	}
	exec(t, `DELETE FROM jobs WHERE id = 'j1'`) // its two rows
	if count(t) != 3 {
		t.Errorf("after deleting a job: %d rows, want 3", count(t))
	}
	exec(t, `DELETE FROM recruiters WHERE id = 'r1'`) // its one remaining row (j2)
	if count(t) != 2 {
		t.Errorf("after deleting a recruiter: %d rows, want 2", count(t))
	}
	exec(t, `DELETE FROM companies WHERE id = 'c2'`) // the recruiter r2 and its rows stay
	if count(t) != 2 {
		t.Errorf("after deleting a company: %d rows, want 2 (a company never removes a recruiter or its mappings)", count(t))
	}
	var recruitersLeft int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM recruiters`).Scan(&recruitersLeft); err != nil || recruitersLeft != 1 {
		t.Errorf("r2 must stay when its company is deleted: %d %v", recruitersLeft, err)
	}
	var jobsLeft int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&jobsLeft); err != nil || jobsLeft != 2 {
		t.Errorf("the jobs themselves must stay: %d %v", jobsLeft, err)
	}
}

func TestTheRecruiterDeleteOfThePackageAlsoRemovesItsMappings(t *testing.T) {
	setup(t)
	company(t, "c1", "Acme")
	recruiter(t, "r1", "c1", "Ada", "L", "a@x")
	job(t, "j1", "Engineer")
	if _, err := Map("j1", "r1"); err != nil {
		t.Fatal(err)
	}
	if err := recruiters.DeleteRecruiter("r1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := ForJob("j1"); len(got) != 0 {
		t.Errorf("%+v", got)
	}
}

func TestIdsAreTrimmed(t *testing.T) {
	setup(t)
	company(t, "c1", "Acme")
	recruiter(t, "r1", "c1", "Ada", "L", "a@x")
	job(t, "j1", "Engineer")
	if created, err := Map(" j1 ", "\tr1\n"); err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	if got, _ := ForJob(" j1"); len(got) != 1 {
		t.Errorf("%+v", got)
	}
	if err := Unmap("j1 ", " r1"); err != nil || count(t) != 0 {
		t.Errorf("%v %d", err, count(t))
	}
}

// A recruiter of several companies lists all of them in link order; the single companyId/companyName are the first; one with
// none lists an empty array and empty strings (step 169).
func TestForJobListsEveryCompanyOfARecruiter(t *testing.T) {
	setup(t)
	company(t, "c1", "Acme")
	company(t, "c2", "Beta")
	recruiter(t, "r1", "c2", "Ada", "L", "")
	exec(t, `INSERT INTO recruiter_companies (recruiter_id, company_id, created_at) VALUES ('r1', 'c1', '2999-01-01 00:00:00')`)
	recruiter(t, "r2", "", "Bob", "M", "")
	job(t, "j1", "Job")
	for _, r := range []string{"r1", "r2"} {
		if _, err := Map("j1", r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ForJob("j1")
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	by := map[string]MappedRecruiter{got[0].ID: got[0], got[1].ID: got[1]}
	r1, r2 := by["r1"], by["r2"]
	if len(r1.Companies) != 2 || r1.Companies[0].ID != "c2" || r1.Companies[1].Name != "Acme" || r1.CompanyID != "c2" || r1.CompanyName != "Beta" {
		t.Errorf("r1: %+v", r1)
	}
	if r2.Companies == nil || len(r2.Companies) != 0 || r2.CompanyID != "" || r2.CompanyName != "" {
		t.Errorf("r2: %+v", r2)
	}
	exec(t, `DELETE FROM companies WHERE id = 'c2'`)
	if got, _ = ForJob("j1"); len(got) != 2 {
		t.Errorf("deleting a company must not unmap a recruiter: %d", len(got))
	}
}

// Plan step-171: a mapped freelancer is marked as one, with no company; the others are not.
func TestForJobMarksAFreelancer(t *testing.T) {
	setup(t)
	company(t, "c1", "Acme")
	recruiter(t, "r1", "c1", "Ada", "L", "")
	recruiter(t, "r2", "", "Free", "Lancer", "")
	exec(t, `UPDATE recruiters SET is_freelancer = 1 WHERE id = 'r2'`)
	recruiter(t, "r3", "", "No", "Company", "")
	job(t, "j1", "Job")
	for _, r := range []string{"r1", "r2", "r3"} {
		if _, err := Map("j1", r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ForJob("j1")
	if err != nil || len(got) != 3 {
		t.Fatalf("%v %v", got, err)
	}
	by := map[string]MappedRecruiter{}
	for _, m := range got {
		by[m.ID] = m
	}
	if by["r1"].Freelancer || by["r3"].Freelancer {
		t.Errorf("only r2 is a freelancer: %+v", got)
	}
	if !by["r2"].Freelancer || len(by["r2"].Companies) != 0 || by["r2"].CompanyID != "" {
		t.Errorf("r2: %+v", by["r2"])
	}
}
