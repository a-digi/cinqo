package courses

import (
	"errors"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/profile"
)

func setup(t *testing.T) (p1, p2 string) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	var err error
	if p1, err = profile.CreateProfile("Ada", "L"); err != nil {
		t.Fatal(err)
	}
	if p2, err = profile.CreateProfile("Bob", "M"); err != nil {
		t.Fatal(err)
	}
	return
}

func sp(s string) *string { return &s }

func TestAddTrimsAndReadsBack(t *testing.T) {
	p1, _ := setup(t)
	c, err := Add(p1, Input{Title: "  Go course ", Provider: " Udemy ", CompletedDate: " 2024-05 ", CredentialURL: " https://example.com/cert/1 ", Description: "advanced"})
	if err != nil {
		t.Fatal(err)
	}
	if c.ID == "" || c.ProfileID != p1 || c.Title != "Go course" || c.Provider != "Udemy" || c.CompletedDate != "2024-05" || c.CredentialURL != "https://example.com/cert/1" || c.Description != "advanced" || c.CreatedAt == "" {
		t.Errorf("%+v", c)
	}
	only, err := Add(p1, Input{Title: "Evening course"})
	if err != nil || only.Provider != "" || only.CompletedDate != "" || only.CredentialURL != "" {
		t.Errorf("only a title is needed: %+v %v", only, err)
	}
}

func TestAddRefusesBadInputAndWritesNothing(t *testing.T) {
	p1, _ := setup(t)
	cases := map[string]struct {
		in   Input
		want error
	}{
		"no title":          {Input{Title: " "}, ErrTitleRequired},
		"bad date":          {Input{Title: "X", CompletedDate: "last year"}, ErrInvalidDate},
		"month 13":          {Input{Title: "X", CompletedDate: "2020-13"}, ErrInvalidDate},
		"year 1800":         {Input{Title: "X", CompletedDate: "1800"}, ErrInvalidDate},
		"slashes":           {Input{Title: "X", CompletedDate: "2020/01/01"}, ErrInvalidDate},
		"day 32":            {Input{Title: "X", CompletedDate: "2020-01-32"}, ErrInvalidDate},
		"day 0":             {Input{Title: "X", CompletedDate: "2020-01-00"}, ErrInvalidDate},
		"month 0":           {Input{Title: "X", CompletedDate: "2020-00"}, ErrInvalidDate},
		"year 2101":         {Input{Title: "X", CompletedDate: "2101"}, ErrInvalidDate},
		"javascript link":   {Input{Title: "X", CredentialURL: "javascript:alert(1)"}, ErrInvalidURL},
		"data link":         {Input{Title: "X", CredentialURL: "data:text/html,hi"}, ErrInvalidURL},
		"ftp link":          {Input{Title: "X", CredentialURL: "ftp://example.com/a"}, ErrInvalidURL},
		"relative link":     {Input{Title: "X", CredentialURL: "/cert/1"}, ErrInvalidURL},
		"no host":           {Input{Title: "X", CredentialURL: "https://"}, ErrInvalidURL},
		"space in link":     {Input{Title: "X", CredentialURL: "https://example.com/a b"}, ErrInvalidURL},
		"scheme only text":  {Input{Title: "X", CredentialURL: "example.com"}, ErrInvalidURL},
		"title too long":    {Input{Title: strings.Repeat("x", 201)}, ErrTooLong},
		"provider too long": {Input{Title: "X", Provider: strings.Repeat("x", 201)}, ErrTooLong},
		"text too long":     {Input{Title: "X", Description: strings.Repeat("x", 2001)}, ErrTooLong},
		"link too long":     {Input{Title: "X", CredentialURL: "https://example.com/" + strings.Repeat("x", 2000)}, ErrTooLong},
	}
	for name, c := range cases {
		if _, err := Add(p1, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if _, err := Add("nope", Input{Title: "X"}); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("%v", err)
	}
	if list, _ := List(p1); len(list) != 0 {
		t.Errorf("a refusal must write nothing: %+v", list)
	}
}

func TestTheAcceptedShapesAndLimits(t *testing.T) {
	p1, _ := setup(t)
	for _, in := range []Input{
		{Title: "lo", CompletedDate: "1900"}, {Title: "hi", CompletedDate: "2100-12-31"}, {Title: "jan", CompletedDate: "2020-01"}, {Title: "a", CompletedDate: "2024"}, {Title: "b", CompletedDate: "2024-05"}, {Title: "c", CompletedDate: "2024-05-31"},
		{Title: "d", CredentialURL: "http://example.com"}, {Title: "e", CredentialURL: "HTTPS://Example.com/x?y=1#z"},
		{Title: strings.Repeat("é", 200), Provider: strings.Repeat("é", 200), Description: strings.Repeat("é", 2000)},
	} {
		if _, err := Add(p1, in); err != nil {
			t.Errorf("%+v: %v", in, err)
		}
	}
}

func TestListIsNewestCompletedFirstUndatedLast(t *testing.T) {
	p1, p2 := setup(t)
	for _, in := range []Input{{Title: "none"}, {Title: "old", CompletedDate: "2019"}, {Title: "new", CompletedDate: "2024-03"}, {Title: "mid", CompletedDate: "2021-06-01"}} {
		if _, err := Add(p1, in); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := List(p1)
	var got []string
	for _, c := range list {
		got = append(got, c.Title)
	}
	if strings.Join(got, ",") != "new,mid,old,none" {
		t.Errorf("%v", got)
	}
	if l, err := List(p2); err != nil || len(l) != 0 {
		t.Errorf("another profile's courses leaked: %+v %v", l, err)
	}
	if l, err := List("nope"); err != nil || l == nil || len(l) != 0 {
		t.Errorf("%v %v", l, err)
	}
}

func TestUpdateChangesOnlyWhatIsSetAndChecksTheWholeEntry(t *testing.T) {
	p1, _ := setup(t)
	c, _ := Add(p1, Input{Title: "Go", Provider: "Udemy", CompletedDate: "2024", CredentialURL: "https://example.com/c"})
	u, err := Update(c.ID, Patch{Provider: sp(" Coursera ")})
	if err != nil || u.Provider != "Coursera" || u.Title != "Go" || u.CompletedDate != "2024" || u.CredentialURL != "https://example.com/c" {
		t.Errorf("%+v %v", u, err)
	}
	if u, err := Update(c.ID, Patch{CompletedDate: sp(""), CredentialURL: sp(""), Description: sp("d")}); err != nil || u.CompletedDate != "" || u.CredentialURL != "" || u.Description != "d" {
		t.Errorf("the date and link can be cleared: %+v %v", u, err)
	}
	for name, p := range map[string]struct {
		patch Patch
		want  error
	}{
		"empty title": {Patch{Title: sp("")}, ErrTitleRequired},
		"bad date":    {Patch{CompletedDate: sp("2020-99")}, ErrInvalidDate},
		"bad link":    {Patch{CredentialURL: sp("javascript:1")}, ErrInvalidURL},
	} {
		if _, err := Update(c.ID, p.patch); !errors.Is(err, p.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if again, _ := ByID(c.ID); again.Title != "Go" || again.Provider != "Coursera" || again.Description != "d" {
		t.Errorf("a refusal must change nothing: %+v", again)
	}
	if _, err := Update("nope", Patch{}); !errors.Is(err, ErrUnknownCourse) {
		t.Errorf("%v", err)
	}
}

func TestDeleteAndTheCascade(t *testing.T) {
	p1, p2 := setup(t)
	c, _ := Add(p1, Input{Title: "Go"})
	Add(p2, Input{Title: "Go"})
	if err := Delete(c.ID); err != nil {
		t.Fatal(err)
	}
	if err := Delete(c.ID); !errors.Is(err, ErrUnknownCourse) {
		t.Errorf("%v", err)
	}
	Add(p1, Input{Title: "Rust"})
	if err := profile.DeleteProfile(p1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM profile_courses`).Scan(&n); err != nil || n != 1 {
		t.Errorf("%d rows left, want the other profile's only: %v", n, err)
	}
}

func TestSQLInATitleIsOnlyAValue(t *testing.T) {
	p1, _ := setup(t)
	if _, err := Add(p1, Input{Title: "x'); DROP TABLE profile_courses;--"}); err != nil {
		t.Fatal(err)
	}
	if l, err := List(p1); err != nil || len(l) != 1 {
		t.Errorf("%v %v", l, err)
	}
}
