package companies

import (
	"fmt"
	"slices"
	"testing"

	"career-tool-backend/db"
)

func setup(t *testing.T) {
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

// add creates a company with a known creation time (seconds apart, so the order is the order of the calls).
func add(t *testing.T, id, name, description, createdAt string) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`INSERT INTO companies (id, name, description, created_at) VALUES (?, ?, ?, ?)`, id, name, description, createdAt); err != nil {
		t.Fatal(err)
	}
}

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func names(p CompaniesPage) []string {
	var out []string
	for _, c := range p.Companies {
		out = append(out, c.Name)
	}
	return out
}

// seed makes ten companies "Company 00" to "Company 09", oldest first.
func seed(t *testing.T) {
	t.Helper()
	for i := 0; i < 10; i++ {
		add(t, fmt.Sprintf("id%02d", i), fmt.Sprintf("Company %02d", i), "", fmt.Sprintf("2026-06-%02d 10:00:00", i+1))
	}
}

func TestNoParametersListsEveryCompanyOldestFirst(t *testing.T) {
	setup(t)
	seed(t)
	p, err := ListCompaniesPage("", 0, 0)
	if err != nil || p.Total != 10 || len(p.Companies) != 10 || p.Companies[0].Name != "Company 00" || p.Companies[9].Name != "Company 09" {
		t.Fatalf("%v, %+v", err, names(p))
	}
	all, err := ListCompanies()
	if err != nil || len(all) != 10 || all[0].Name != "Company 00" {
		t.Errorf("ListCompanies: %d, %v", len(all), err)
	}
	// a limit of zero or less is "no limit" for the function (the handler clamps a limit that was given)
	for _, limit := range []int{0, -3} {
		if p, _ := ListCompaniesPage("", limit, 4); len(p.Companies) != 10 {
			t.Errorf("limit %d: %d companies", limit, len(p.Companies))
		}
	}
}

// Companies created in the same second keep a stable order (the id breaks the tie), so pages never repeat or skip one.
func TestTheSameSecondKeepsAStableOrder(t *testing.T) {
	setup(t)
	for _, id := range []string{"c", "a", "e", "b", "d"} {
		add(t, id, "Same "+id, "", "2026-06-01 10:00:00")
	}
	p, _ := ListCompaniesPage("", 0, 0)
	got := []string{}
	for _, c := range p.Companies {
		got = append(got, c.ID)
	}
	if !slices.Equal(got, []string{"a", "b", "c", "d", "e"}) {
		t.Errorf("%v", got)
	}
	var seen []string
	for off := 0; off < 5; off += 2 {
		page, _ := ListCompaniesPage("", 2, off)
		for _, c := range page.Companies {
			seen = append(seen, c.ID)
		}
	}
	if !slices.Equal(seen, []string{"a", "b", "c", "d", "e"}) {
		t.Errorf("pages of 2 saw %v", seen)
	}
}

func TestPagesAreDisjointAndCoverEverything(t *testing.T) {
	setup(t)
	seed(t)
	var all []string
	for _, off := range []int{0, 3, 6, 9} {
		p, err := ListCompaniesPage("", 3, off)
		if err != nil || p.Total != 10 {
			t.Fatalf("offset %d: total %d, %v", off, p.Total, err)
		}
		all = append(all, names(p)...)
	}
	if len(all) != 10 || all[0] != "Company 00" || all[9] != "Company 09" {
		t.Errorf("%v", all)
	}
	if p, _ := ListCompaniesPage("", 3, 9); len(p.Companies) != 1 {
		t.Errorf("the last page has %d", len(p.Companies))
	}
	// past the end: an empty list (not null), the total unchanged
	p, _ := ListCompaniesPage("", 3, 30)
	if p.Companies == nil || len(p.Companies) != 0 || p.Total != 10 {
		t.Errorf("past the end: %+v", p)
	}
	// a negative offset counts as 0
	if p, _ := ListCompaniesPage("", 3, -5); len(p.Companies) != 3 || p.Companies[0].Name != "Company 00" {
		t.Errorf("negative offset: %v", names(p))
	}
}

func TestTheLimitIsClamped(t *testing.T) {
	cases := map[int]int{-5: 1, 0: 1, 1: 1, 2: 2, 199: 199, 200: 200, 201: 200, 99999: 200}
	for in, want := range cases {
		if got := ClampCompaniesLimit(in); got != want {
			t.Errorf("ClampCompaniesLimit(%d) = %d, want %d", in, got, want)
		}
	}
	if MinCompaniesLimit != 1 || MaxCompaniesLimit != 200 {
		t.Errorf("limits %d, %d", MinCompaniesLimit, MaxCompaniesLimit)
	}
	setup(t)
	tx, _ := db.JobsDB.Begin()
	for i := 0; i < 230; i++ {
		if _, err := tx.Exec(`INSERT INTO companies (id, name, description, created_at) VALUES (?, ?, '', '2026-06-01 10:00:00')`, fmt.Sprintf("id%03d", i), fmt.Sprintf("C %03d", i)); err != nil {
			t.Fatal(err)
		}
	}
	_ = tx.Commit()
	if p, _ := ListCompaniesPage("", 500, 0); len(p.Companies) != 200 || p.Total != 230 {
		t.Errorf("limit 500: %d companies of %d", len(p.Companies), p.Total)
	}
	if p, _ := ListCompaniesPage("", 1, 0); len(p.Companies) != 1 {
		t.Errorf("limit 1: %d", len(p.Companies))
	}
	if p, _ := ListCompaniesPage("", 0, 0); len(p.Companies) != 230 {
		t.Errorf("no limit: %d", len(p.Companies))
	}
}

func TestSearchMatchesNameAndDescriptionCaseInsensitively(t *testing.T) {
	setup(t)
	add(t, "1", "Adesso SE", "software house", "2026-06-01 10:00:00")
	add(t, "2", "Globex", "Works with ADESSO on projects", "2026-06-02 10:00:00")
	add(t, "3", "Initech", "tax software", "2026-06-03 10:00:00")
	add(t, "4", "Umbrella", "", "2026-06-04 10:00:00")
	check := func(query string, want ...string) {
		t.Helper()
		p, err := ListCompaniesPage(query, 0, 0)
		if err != nil || !slices.Equal(names(p), want) || p.Total != len(want) {
			t.Errorf("%q: %v (total %d), %v; want %v", query, names(p), p.Total, err, want)
		}
	}
	check("adesso", "Adesso SE", "Globex")
	check("ADESSO", "Adesso SE", "Globex")
	check("  adesso  ", "Adesso SE", "Globex")
	check("soft", "Adesso SE", "Initech")
	check("umbrella", "Umbrella")
	check("projects", "Globex")
	check("nothing like this")
	check("", "Adesso SE", "Globex", "Initech", "Umbrella")
	check("   ", "Adesso SE", "Globex", "Initech", "Umbrella")
	check("se", "Adesso SE") // in its name and in its description ("software house"); in no other company
}

// The total is the number of matches while the page is limited; the page holds only some of them.
func TestTotalCountsEveryMatchNotJustThePage(t *testing.T) {
	setup(t)
	seed(t)
	add(t, "x", "Other", "", "2026-07-01 10:00:00")
	p, err := ListCompaniesPage("company", 4, 4)
	if err != nil || p.Total != 10 || len(p.Companies) != 4 || p.Companies[0].Name != "Company 04" {
		t.Errorf("%v, total %d, %v", err, p.Total, names(p))
	}
	if p, _ := ListCompaniesPage("company 0", 100, 0); p.Total != 10 {
		t.Errorf("total %d", p.Total)
	}
	if p, _ := ListCompaniesPage("other", 4, 0); p.Total != 1 || len(p.Companies) != 1 {
		t.Errorf("other: %d %d", p.Total, len(p.Companies))
	}
}

// % _ and \ typed by the person are characters, not wildcards.
func TestWildcardsAreLiteral(t *testing.T) {
	setup(t)
	add(t, "1", "100% Remote GmbH", "", "2026-06-01 10:00:00")
	add(t, "2", "Acme", "", "2026-06-02 10:00:00")
	add(t, "3", "a_b Holding", "", "2026-06-03 10:00:00")
	add(t, "4", "axb Holding", "", "2026-06-04 10:00:00")
	add(t, "5", `C:\dev Ltd`, "", "2026-06-05 10:00:00")
	check := func(query string, want ...string) {
		t.Helper()
		p, err := ListCompaniesPage(query, 0, 0)
		if err != nil || !slices.Equal(names(p), want) {
			t.Errorf("%q: %v, %v; want %v", query, names(p), err, want)
		}
	}
	check("%", "100% Remote GmbH")
	check("100%", "100% Remote GmbH")
	check("_", "a_b Holding")
	check("a_b", "a_b Holding")
	check(`\`, `C:\dev Ltd`)
	check(`c:\dev`, `C:\dev Ltd`)
	check("%%")
	check("a%b")
}

// The overview counts all companies, whatever the search or the page.
func TestTheSummaryIsIndependentOfSearchAndPage(t *testing.T) {
	setup(t)
	add(t, "a", "Alpha", "", "2026-06-01 10:00:00")
	add(t, "b", "Beta", "", "2026-06-02 10:00:00")
	add(t, "c", "Gamma", "", "2026-06-03 10:00:00")
	exec(t, `INSERT INTO jobs (id, source_url, title, company_id) VALUES ('j1', 'u1', 't', 'a'), ('j2', 'u2', 't', 'a'), ('j3', 'u3', 't', 'b')`)
	exec(t, `INSERT INTO recruiters (id, first_name) VALUES ('r1', 'x'); INSERT INTO recruiter_companies (recruiter_id, company_id) VALUES ('r1', 'a')`)
	want := CompanySummary{Companies: 3, WithJobs: 2, WithRecruiters: 1, Jobs: 3, Recruiters: 1}
	for _, c := range []struct {
		q          string
		limit, off int
	}{{"", 0, 0}, {"alpha", 0, 0}, {"nothing", 0, 0}, {"", 1, 2}, {"beta", 1, 0}, {"", 2, 99}} {
		p, err := ListCompaniesPage(c.q, c.limit, c.off)
		if err != nil || p.Summary != want {
			t.Errorf("%+v: %+v, %v", c, p.Summary, err)
		}
	}
	// the per-company counts are still on each row
	p, _ := ListCompaniesPage("alpha", 0, 0)
	if len(p.Companies) != 1 || p.Companies[0].JobCount != 2 || p.Companies[0].RecruiterCount != 1 {
		t.Errorf("%+v", p.Companies)
	}
}

func TestAnEmptyDatabase(t *testing.T) {
	setup(t)
	p, err := ListCompaniesPage("", 10, 0)
	if err != nil || p.Total != 0 || p.Companies == nil || len(p.Companies) != 0 || p.Summary != (CompanySummary{}) {
		t.Errorf("%+v, %v", p, err)
	}
}
