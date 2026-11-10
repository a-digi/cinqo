package hobbies

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
	h, err := Add(p1, Input{Name: "  Chess ", Description: "club player"})
	if err != nil || h.ID == "" || h.ProfileID != p1 || h.Name != "Chess" || h.Description != "club player" || h.CreatedAt == "" {
		t.Errorf("%+v %v", h, err)
	}
	only, err := Add(p1, Input{Name: "Hiking"})
	if err != nil || only.Description != "" {
		t.Errorf("only a name is needed: %+v %v", only, err)
	}
}

func TestAddRefusesBadInputAndWritesNothing(t *testing.T) {
	p1, _ := setup(t)
	for name, c := range map[string]struct {
		in   Input
		want error
	}{
		"no name":          {Input{Name: " "}, ErrNameRequired},
		"name too long":    {Input{Name: strings.Repeat("x", 201)}, ErrNameTooLong},
		"text too long":    {Input{Name: "X", Description: strings.Repeat("x", 2001)}, ErrTextTooLong},
		"exactly the text": {Input{Name: "ok", Description: strings.Repeat("é", 2000)}, nil},
		"exactly 200":      {Input{Name: strings.Repeat("é", 200)}, nil},
	} {
		if _, err := Add(p1, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if _, err := Add("nope", Input{Name: "Chess"}); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("%v", err)
	}
	if list, _ := List(p1); len(list) != 2 {
		t.Errorf("only the two allowed ones are written: %+v", list)
	}
}

func TestAHobbyIsListedOncePerProfileIgnoringCaseAndListIsByName(t *testing.T) {
	p1, p2 := setup(t)
	for _, n := range []string{"hiking", "Chess", "Archery"} {
		if _, err := Add(p1, Input{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Add(p1, Input{Name: " CHESS "}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("%v", err)
	}
	if _, err := Add(p2, Input{Name: "Chess"}); err != nil {
		t.Errorf("another profile may list it: %v", err)
	}
	list, _ := List(p1)
	var got []string
	for _, h := range list {
		got = append(got, h.Name)
	}
	if strings.Join(got, ",") != "Archery,Chess,hiking" {
		t.Errorf("%v", got)
	}
	if l, err := List("nope"); err != nil || l == nil || len(l) != 0 {
		t.Errorf("%v %v", l, err)
	}
}

func TestUpdateChangesOnlyWhatIsSetAndChecksTheWholeEntry(t *testing.T) {
	p1, _ := setup(t)
	h, _ := Add(p1, Input{Name: "Chess", Description: "club"})
	if u, err := Update(h.ID, Patch{Description: sp("")}); err != nil || u.Name != "Chess" || u.Description != "" {
		t.Errorf("%+v %v", u, err)
	}
	if u, err := Update(h.ID, Patch{Name: sp(" Go ")}); err != nil || u.Name != "Go" {
		t.Errorf("%+v %v", u, err)
	}
	if _, err := Update(h.ID, Patch{Name: sp("")}); !errors.Is(err, ErrNameRequired) {
		t.Errorf("%v", err)
	}
	if again, _ := ByID(h.ID); again.Name != "Go" {
		t.Errorf("a refusal must change nothing: %+v", again)
	}
	other, _ := Add(p1, Input{Name: "Hiking"})
	if _, err := Update(other.ID, Patch{Name: sp("go")}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("%v", err)
	}
	if u, err := Update(h.ID, Patch{Name: sp("GO")}); err != nil || u.Name != "GO" {
		t.Errorf("its own name is not a duplicate: %+v %v", u, err)
	}
	if _, err := Update("nope", Patch{}); !errors.Is(err, ErrUnknownHobby) {
		t.Errorf("%v", err)
	}
}

func TestDeleteAndTheCascade(t *testing.T) {
	p1, p2 := setup(t)
	h, _ := Add(p1, Input{Name: "Chess"})
	Add(p2, Input{Name: "Chess"})
	if err := Delete(h.ID); err != nil {
		t.Fatal(err)
	}
	if err := Delete(h.ID); !errors.Is(err, ErrUnknownHobby) {
		t.Errorf("%v", err)
	}
	Add(p1, Input{Name: "Go"})
	if err := profile.DeleteProfile(p1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM profile_hobbies`).Scan(&n); err != nil || n != 1 {
		t.Errorf("%d rows left, want the other profile's only: %v", n, err)
	}
}

func TestListIgnoresTheCaseOfLettersWhenOrdering(t *testing.T) {
	p1, _ := setup(t)
	for _, n := range []string{"banana", "Apple", "Cherry", "apple pie"} {
		if _, err := Add(p1, Input{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := List(p1)
	var got []string
	for _, h := range list {
		got = append(got, h.Name)
	}
	if strings.Join(got, ",") != "Apple,apple pie,banana,Cherry" {
		t.Errorf("%v", got)
	}
}
