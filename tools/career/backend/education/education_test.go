package education

import (
	"errors"
	"reflect"
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
	e, err := Add(p1, Input{Institution: "  MIT ", Degree: " B.Sc. ", FieldOfStudy: "Physics", StartDate: "2010-09", EndDate: "2014", Description: "thesis on X"})
	if err != nil {
		t.Fatal(err)
	}
	if e.ID == "" || e.ProfileID != p1 || e.Institution != "MIT" || e.Degree != "B.Sc." || e.FieldOfStudy != "Physics" || e.StartDate != "2010-09" || e.EndDate != "2014" || e.Description != "thesis on X" || e.CreatedAt == "" {
		t.Errorf("%+v", e)
	}
	only, err := Add(p1, Input{Institution: "Evening school"})
	if err != nil || only.Degree != "" || only.StartDate != "" || only.EndDate != "" {
		t.Errorf("only an institution is needed: %+v %v", only, err)
	}
}

func TestAddRefusesBadInputAndWritesNothing(t *testing.T) {
	p1, _ := setup(t)
	cases := map[string]struct {
		in   Input
		want error
	}{
		"no institution":       {Input{Institution: "  "}, ErrInstitutionRequired},
		"bad start":            {Input{Institution: "X", StartDate: "last year"}, ErrInvalidDate},
		"bad end":              {Input{Institution: "X", EndDate: "2020-13"}, ErrInvalidDate},
		"day out of range":     {Input{Institution: "X", StartDate: "2020-01-32"}, ErrInvalidDate},
		"year out of range":    {Input{Institution: "X", StartDate: "1800"}, ErrInvalidDate},
		"month zero":           {Input{Institution: "X", EndDate: "2020-00"}, ErrInvalidDate},
		"two digit year":       {Input{Institution: "X", StartDate: "99"}, ErrInvalidDate},
		"slashes":              {Input{Institution: "X", StartDate: "2020/01/01"}, ErrInvalidDate},
		"end before start":     {Input{Institution: "X", StartDate: "2020-06", EndDate: "2020-05"}, ErrEndBeforeStart},
		"end year before":      {Input{Institution: "X", StartDate: "2020", EndDate: "2019-12"}, ErrEndBeforeStart},
		"end before start day": {Input{Institution: "X", StartDate: "2020-06-15", EndDate: "2020-06-14"}, ErrEndBeforeStart},
	}
	for name, c := range cases {
		if _, err := Add(p1, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if _, err := Add("nope", Input{Institution: "X"}); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
	if list, _ := List(p1); len(list) != 0 {
		t.Errorf("a refusal wrote %d entries", len(list))
	}
}

func TestSameDayAndSameMonthAreAllowedAndShapesMix(t *testing.T) {
	p1, _ := setup(t)
	for name, in := range map[string]Input{
		"same month":  {Institution: "A", StartDate: "2020-06", EndDate: "2020-06"},
		"same day":    {Institution: "B", StartDate: "2020-06-15", EndDate: "2020-06-15"},
		"year to day": {Institution: "C", StartDate: "2019", EndDate: "2020-06-15"},
		"only end":    {Institution: "D", EndDate: "2020"},
		"only start":  {Institution: "E", StartDate: "2020"},
	} {
		if _, err := Add(p1, in); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestListOrdersTheOngoingFirstThenTheLatestEnd(t *testing.T) {
	p1, p2 := setup(t)
	add := func(profile, inst, start, end string) {
		if _, err := Add(profile, Input{Institution: inst, StartDate: start, EndDate: end}); err != nil {
			t.Fatal(err)
		}
	}
	add(p1, "Old", "2000", "2004")
	add(p1, "Ongoing", "2023", "")
	add(p1, "Recent", "2015", "2019")
	add(p1, "Middle", "2008", "2012")
	add(p2, "Other profile", "2020", "")
	list, err := List(p1)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Institution)
	}
	if !reflect.DeepEqual(names, []string{"Ongoing", "Recent", "Middle", "Old"}) {
		t.Errorf("order %v", names)
	}
	if other, _ := List(p2); len(other) != 1 {
		t.Errorf("another profile has its own: %v", other)
	}
	if none, err := List("nope"); err != nil || len(none) != 0 || none == nil {
		t.Errorf("an unknown profile has an empty list, not null: %v %v", none, err)
	}
}

func TestUpdateChangesOnlyWhatIsGivenAndChecksTheWholeEntry(t *testing.T) {
	p1, _ := setup(t)
	e, _ := Add(p1, Input{Institution: "MIT", Degree: "B.Sc.", StartDate: "2010", EndDate: "2014", Description: "d"})
	u, err := Update(e.ID, Patch{Degree: sp("M.Sc.")})
	if err != nil || u.Degree != "M.Sc." || u.Institution != "MIT" || u.StartDate != "2010" || u.EndDate != "2014" || u.Description != "d" {
		t.Fatalf("%+v %v", u, err)
	}
	// a date alone is checked against the other one
	if _, err := Update(e.ID, Patch{StartDate: sp("2015")}); !errors.Is(err, ErrEndBeforeStart) {
		t.Errorf("start after the end: %v", err)
	}
	if _, err := Update(e.ID, Patch{EndDate: sp("2009")}); !errors.Is(err, ErrEndBeforeStart) {
		t.Errorf("end before the start: %v", err)
	}
	if _, err := Update(e.ID, Patch{EndDate: sp("soon")}); !errors.Is(err, ErrInvalidDate) {
		t.Errorf("bad date: %v", err)
	}
	if _, err := Update(e.ID, Patch{Institution: sp(" ")}); !errors.Is(err, ErrInstitutionRequired) {
		t.Errorf("blank institution: %v", err)
	}
	after, _ := ByID(e.ID)
	if after.StartDate != "2010" || after.EndDate != "2014" || after.Institution != "MIT" {
		t.Errorf("a refusal must change nothing: %+v", after)
	}
	// an empty end date means still studying; a cleared start is allowed
	c, err := Update(e.ID, Patch{EndDate: sp(""), StartDate: sp("")})
	if err != nil || c.EndDate != "" || c.StartDate != "" {
		t.Errorf("%+v %v", c, err)
	}
	if _, err := Update("nope", Patch{Degree: sp("x")}); !errors.Is(err, ErrUnknownEducation) {
		t.Errorf("unknown: %v", err)
	}
	if same, err := Update(e.ID, Patch{}); err != nil || same.ID != e.ID {
		t.Errorf("an empty patch is a no-op: %+v %v", same, err)
	}
}

func TestDeleteAndTheCascade(t *testing.T) {
	p1, _ := setup(t)
	a, _ := Add(p1, Input{Institution: "A"})
	b, _ := Add(p1, Input{Institution: "B"})
	if err := Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := Delete(a.ID); !errors.Is(err, ErrUnknownEducation) {
		t.Errorf("twice: %v", err)
	}
	if list, _ := List(p1); len(list) != 1 || list[0].ID != b.ID {
		t.Errorf("%v", list)
	}
	// deleting the profile deletes its education
	if err := profile.DeleteProfile(p1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM profile_education`).Scan(&n); err != nil || n != 0 {
		t.Errorf("education left after the profile was deleted: %d %v", n, err)
	}
}

func TestByIDOfAnUnknownEntry(t *testing.T) {
	setup(t)
	if _, err := ByID("nope"); !errors.Is(err, ErrUnknownEducation) {
		t.Errorf("%v", err)
	}
}
