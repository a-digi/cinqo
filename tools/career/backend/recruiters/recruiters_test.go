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
