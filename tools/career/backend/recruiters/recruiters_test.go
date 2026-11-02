package recruiters

import (
	"errors"
	"reflect"
	"testing"

	"career-tool-backend/companies"
	"career-tool-backend/db"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	for _, q := range []string{
		`INSERT INTO companies (id, name) VALUES ('c1', 'Acme'), ('c2', 'Beta'), ('c3', 'Gamma')`,
	} {
		if _, err := db.JobsDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func count(t *testing.T, q string) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func find(t *testing.T, id string) recruiter {
	t.Helper()
	list, err := ListRecruiters("")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range list {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("recruiter %s not listed", id)
	return recruiter{}
}

func TestCreateWithSeveralOneOrNoCompanies(t *testing.T) {
	setup(t)
	two, err := CreateRecruiter([]string{"c2", " c1 ", "c2", ""}, "Ada", "L", "a@x")
	if err != nil {
		t.Fatal(err)
	}
	r := find(t, two)
	if !reflect.DeepEqual(r.CompanyIDs, []string{"c2", "c1"}) || r.CompanyID != "c2" || len(r.Companies) != 2 || r.Companies[1].Name != "Acme" {
		t.Errorf("two companies (order kept, blanks and repeats dropped): %+v", r)
	}
	none, err := CreateRecruiter(nil, "Bob", "M", "")
	if err != nil {
		t.Fatal(err)
	}
	r = find(t, none)
	if r.CompanyID != "" || r.CompanyIDs == nil || len(r.CompanyIDs) != 0 || r.Companies == nil || len(r.Companies) != 0 {
		t.Errorf("no company must be empty lists (not null) and an empty companyId: %+v", r)
	}
}

func TestCreateWithAnUnknownCompanyWritesNothing(t *testing.T) {
	setup(t)
	_, err := CreateRecruiter([]string{"c1", "nope"}, "Ada", "L", "")
	if !errors.Is(err, companies.ErrUnknownCompany) {
		t.Fatalf("err = %v", err)
	}
	if count(t, `SELECT COUNT(*) FROM recruiters`) != 0 || count(t, `SELECT COUNT(*) FROM recruiter_companies`) != 0 {
		t.Error("a failed create must leave nothing behind")
	}
}

func TestListFiltersByCompanyThroughTheLinks(t *testing.T) {
	setup(t)
	a, _ := CreateRecruiter([]string{"c1", "c2"}, "A", "", "")
	b, _ := CreateRecruiter([]string{"c2"}, "B", "", "")
	CreateRecruiter(nil, "C", "", "")
	ids := func(company string) []string {
		list, err := ListRecruiters(company)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, r := range list {
			out = append(out, r.ID)
		}
		return out
	}
	if got := ids("c1"); !reflect.DeepEqual(got, []string{a}) {
		t.Errorf("c1: %v", got)
	}
	if got := ids("c2"); !reflect.DeepEqual(got, []string{a, b}) {
		t.Errorf("c2: %v", got)
	}
	if got := ids("c3"); len(got) != 0 {
		t.Errorf("c3: %v", got)
	}
	if got := ids(""); len(got) != 3 {
		t.Errorf("all: %v", got)
	}
	// the filter does not hide the recruiter's other companies
	list, _ := ListRecruiters("c1")
	if len(list[0].Companies) != 2 {
		t.Errorf("a filtered recruiter must still list all its companies: %+v", list[0])
	}
}

func TestSetCompaniesReplacesTheSetAndKeepsTheLinksThatStay(t *testing.T) {
	setup(t)
	id, _ := CreateRecruiter([]string{"c1", "c2"}, "A", "", "")
	if _, err := db.JobsDB.Exec(`UPDATE recruiter_companies SET created_at = '2020-01-01 00:00:00' WHERE company_id = 'c2'`); err != nil {
		t.Fatal(err)
	}
	if err := SetCompanies(id, []string{"c2", "c3"}); err != nil {
		t.Fatal(err)
	}
	r := find(t, id)
	if !reflect.DeepEqual(r.CompanyIDs, []string{"c2", "c3"}) {
		t.Errorf("companies = %v", r.CompanyIDs)
	}
	if count(t, `SELECT COUNT(*) FROM recruiter_companies WHERE company_id = 'c2' AND created_at = '2020-01-01 00:00:00'`) != 1 {
		t.Error("a company that stays must keep its own link")
	}
	if r.UpdatedAt == "" {
		t.Error("updated_at must be set")
	}
	if err := SetCompanies(id, nil); err != nil {
		t.Fatal(err)
	}
	if r = find(t, id); len(r.CompanyIDs) != 0 {
		t.Errorf("none is allowed: %v", r.CompanyIDs)
	}
}

func TestSetCompaniesFailsWholeOnUnknownIds(t *testing.T) {
	setup(t)
	id, _ := CreateRecruiter([]string{"c1"}, "A", "", "")
	if err := SetCompanies(id, []string{"c2", "nope"}); !errors.Is(err, companies.ErrUnknownCompany) {
		t.Fatalf("err = %v", err)
	}
	if got := find(t, id).CompanyIDs; !reflect.DeepEqual(got, []string{"c1"}) {
		t.Errorf("nothing may change: %v", got)
	}
	if err := SetCompanies("ghost", []string{"c1"}); !errors.Is(err, ErrUnknownRecruiter) {
		t.Errorf("unknown recruiter: %v", err)
	}
}

// The point of step 169: a company going away never takes a recruiter with it.
func TestDeletingACompanyKeepsTheRecruiterAndOnlyDropsItsLink(t *testing.T) {
	setup(t)
	id, _ := CreateRecruiter([]string{"c1", "c2"}, "A", "", "")
	if err := companies.DeleteCompany("c1"); err != nil {
		t.Fatal(err)
	}
	if got := find(t, id).CompanyIDs; !reflect.DeepEqual(got, []string{"c2"}) {
		t.Errorf("after deleting c1: %v", got)
	}
	if err := companies.DeleteCompany("c2"); err != nil {
		t.Fatal(err)
	}
	if r := find(t, id); len(r.CompanyIDs) != 0 || r.FirstName != "A" {
		t.Errorf("after deleting the last company the recruiter stays: %+v", r)
	}
}

func TestDeletingARecruiterRemovesItsLinksButNoCompany(t *testing.T) {
	setup(t)
	id, _ := CreateRecruiter([]string{"c1", "c2"}, "A", "", "")
	if err := DeleteRecruiter(id); err != nil {
		t.Fatal(err)
	}
	if count(t, `SELECT COUNT(*) FROM recruiter_companies`) != 0 || count(t, `SELECT COUNT(*) FROM companies`) != 3 {
		t.Error("links go, companies stay")
	}
}

func TestACompanyWithALinkedRecruiterIsNotCleanedUpAndIsCounted(t *testing.T) {
	setup(t)
	CreateRecruiter([]string{"c1"}, "A", "", "")
	CreateRecruiter([]string{"c1", "c2"}, "B", "", "")
	if deleted, err := companies.DeleteCompanyIfUnused("c1"); err != nil || deleted {
		t.Errorf("c1 has recruiters: deleted=%v err=%v", deleted, err)
	}
	if deleted, err := companies.DeleteCompanyIfUnused("c3"); err != nil || !deleted {
		t.Errorf("c3 has none: deleted=%v err=%v", deleted, err)
	}
	list, err := companies.ListCompanies()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, c := range list {
		got[c.ID] = c.RecruiterCount
	}
	if got["c1"] != 2 || got["c2"] != 1 {
		t.Errorf("recruiterCount = %v", got)
	}
}

// --- plan step-171: freelancers ---

func TestAFreelancerIsCreatedWithNoCompanyAndListedAsOne(t *testing.T) {
	setup(t)
	id, err := CreateRecruiterOfKind(nil, true, "Free", "Lancer", "f@x")
	if err != nil {
		t.Fatal(err)
	}
	r := find(t, id)
	if !r.Freelancer || len(r.CompanyIDs) != 0 || r.CompanyID != "" {
		t.Errorf("freelancer: %+v", r)
	}
	plain, err := CreateRecruiter([]string{"c1"}, "Ada", "L", "")
	if err != nil {
		t.Fatal(err)
	}
	if find(t, plain).Freelancer {
		t.Error("CreateRecruiter must not create a freelancer")
	}
	none, err := CreateRecruiter(nil, "Bob", "M", "")
	if err != nil {
		t.Fatal(err)
	}
	if find(t, none).Freelancer {
		t.Error("a recruiter with no company is not a freelancer unless asked")
	}
}

func TestAFreelancerWithCompaniesIsRefusedAndNothingIsWritten(t *testing.T) {
	setup(t)
	if _, err := CreateRecruiterOfKind([]string{"c1"}, true, "Free", "L", ""); !errors.Is(err, ErrFreelancerWithCompanies) {
		t.Fatalf("create: %v", err)
	}
	if n := count(t, `SELECT COUNT(*) FROM recruiters`); n != 0 {
		t.Errorf("a refused create wrote %d recruiter(s)", n)
	}
	// the refusal comes before the company check: a freelancer with an unknown company is still the freelancer error
	if _, err := CreateRecruiterOfKind([]string{"nope"}, true, "Free", "L", ""); !errors.Is(err, ErrFreelancerWithCompanies) {
		t.Fatalf("unknown company: %v", err)
	}
	id, _ := CreateRecruiterOfKind(nil, true, "Free", "L", "")
	if err := SetCompanies(id, []string{"c1"}); !errors.Is(err, ErrFreelancerWithCompanies) {
		t.Fatalf("SetCompanies on a freelancer: %v", err)
	}
	if err := SetKind(id, true, []string{"c1"}); !errors.Is(err, ErrFreelancerWithCompanies) {
		t.Fatalf("SetKind: %v", err)
	}
	if n := count(t, `SELECT COUNT(*) FROM recruiter_companies`); n != 0 {
		t.Errorf("a refusal linked %d companies", n)
	}
	if !find(t, id).Freelancer {
		t.Error("a refusal must not change the flag")
	}
}

func TestMakingSomeoneAFreelancerClearsTheirCompaniesAndBackAgain(t *testing.T) {
	setup(t)
	id, _ := CreateRecruiter([]string{"c1", "c2"}, "Ada", "L", "")
	if err := SetFreelancer(id, true); err != nil {
		t.Fatal(err)
	}
	r := find(t, id)
	if !r.Freelancer || len(r.CompanyIDs) != 0 {
		t.Fatalf("after SetFreelancer(true): %+v", r)
	}
	if n := count(t, `SELECT COUNT(*) FROM companies`); n != 3 {
		t.Errorf("companies must stay: %d", n)
	}
	// back to a company recruiter in one call
	if err := SetKind(id, false, []string{"c3"}); err != nil {
		t.Fatal(err)
	}
	r = find(t, id)
	if r.Freelancer || !reflect.DeepEqual(r.CompanyIDs, []string{"c3"}) {
		t.Fatalf("after SetKind(false, c3): %+v", r)
	}
	// SetFreelancer(false) on a freelancer keeps it company-less
	if err := SetFreelancer(id, true); err != nil {
		t.Fatal(err)
	}
	if err := SetFreelancer(id, false); err != nil {
		t.Fatal(err)
	}
	r = find(t, id)
	if r.Freelancer || len(r.CompanyIDs) != 0 {
		t.Errorf("after SetFreelancer(false): %+v", r)
	}
	// an ordinary SetCompanies works again once the flag is off
	if err := SetCompanies(id, []string{"c1"}); err != nil {
		t.Fatal(err)
	}
}

func TestSetKindFailsWholeOnAnUnknownRecruiterOrCompany(t *testing.T) {
	setup(t)
	id, _ := CreateRecruiter([]string{"c1"}, "Ada", "L", "")
	if err := SetKind("nope", false, []string{"c1"}); !errors.Is(err, ErrUnknownRecruiter) {
		t.Errorf("unknown recruiter: %v", err)
	}
	if err := SetFreelancer("nope", true); !errors.Is(err, ErrUnknownRecruiter) {
		t.Errorf("unknown recruiter (SetFreelancer true): %v", err)
	}
	if err := SetFreelancer("nope", false); !errors.Is(err, ErrUnknownRecruiter) {
		t.Errorf("unknown recruiter (SetFreelancer false): %v", err)
	}
	if err := SetKind(id, false, []string{"c2", "nope"}); !errors.Is(err, companies.ErrUnknownCompany) {
		t.Errorf("unknown company: %v", err)
	}
	r := find(t, id)
	if r.Freelancer || !reflect.DeepEqual(r.CompanyIDs, []string{"c1"}) {
		t.Errorf("a refusal must change nothing: %+v", r)
	}
}

func TestListFiltersByFreelancerAndCombinesWithTheCompanyFilter(t *testing.T) {
	setup(t)
	f1, _ := CreateRecruiterOfKind(nil, true, "F", "One", "")
	f2, _ := CreateRecruiterOfKind(nil, true, "F", "Two", "")
	a, _ := CreateRecruiter([]string{"c1"}, "Ada", "L", "")
	none, _ := CreateRecruiter(nil, "No", "Company", "")
	list, err := ListRecruitersOfKind("", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != f1 || list[1].ID != f2 {
		t.Errorf("freelancers only: %+v", list)
	}
	all, _ := ListRecruitersOfKind("", false)
	if len(all) != 4 {
		t.Errorf("no filter: %d", len(all))
	}
	// a freelancer has no company, so freelancers of a company are none
	both, _ := ListRecruitersOfKind("c1", true)
	if len(both) != 0 {
		t.Errorf("freelancer of c1: %+v", both)
	}
	byCompany, _ := ListRecruitersOfKind("c1", false)
	if len(byCompany) != 1 || byCompany[0].ID != a {
		t.Errorf("by company: %+v", byCompany)
	}
	if find(t, none).Freelancer {
		t.Error("the recruiter with no company is not a freelancer")
	}
}

func TestDeletingACompanyNeverTouchesAFreelancer(t *testing.T) {
	setup(t)
	f, _ := CreateRecruiterOfKind(nil, true, "F", "One", "")
	a, _ := CreateRecruiter([]string{"c1"}, "Ada", "L", "")
	if _, err := db.JobsDB.Exec(`DELETE FROM companies WHERE id = 'c1'`); err != nil {
		t.Fatal(err)
	}
	if !find(t, f).Freelancer {
		t.Error("the freelancer changed")
	}
	r := find(t, a)
	if r.Freelancer || len(r.CompanyIDs) != 0 {
		t.Errorf("the company recruiter stays, no company, still not a freelancer: %+v", r)
	}
	if err := DeleteRecruiter(f); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT COUNT(*) FROM recruiters`); n != 1 {
		t.Errorf("recruiters = %d", n)
	}
}

func TestUpdatingTheNameKeepsTheFlag(t *testing.T) {
	setup(t)
	id, _ := CreateRecruiterOfKind(nil, true, "F", "One", "")
	n := "Renamed"
	if err := UpdateRecruiter(id, &n, nil, nil); err != nil {
		t.Fatal(err)
	}
	r := find(t, id)
	if !r.Freelancer || r.FirstName != "Renamed" {
		t.Errorf("%+v", r)
	}
}
