package languages

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
func bp(b bool) *bool     { return &b }

func TestAddTrimsNormalisesAndReadsBack(t *testing.T) {
	p1, _ := setup(t)
	l, err := Add(p1, Input{Language: "  German ", Level: " C1 ", CanSpeak: true, CanWrite: false})
	if err != nil {
		t.Fatal(err)
	}
	if l.ID == "" || l.ProfileID != p1 || l.Language != "German" || l.Level != "c1" || !l.CanSpeak || l.CanWrite || l.CreatedAt == "" {
		t.Errorf("%+v", l)
	}
	w, err := Add(p1, Input{Language: "Albanian", CanWrite: true})
	if err != nil || w.Level != "" || w.CanSpeak || !w.CanWrite {
		t.Errorf("an empty level and only written are fine: %+v %v", w, err)
	}
}

func TestAddRefusesBadInputAndWritesNothing(t *testing.T) {
	p1, _ := setup(t)
	cases := map[string]struct {
		in   Input
		want error
	}{
		"no name":            {Input{Language: "  ", CanSpeak: true}, ErrNameRequired},
		"name too long":      {Input{Language: strings.Repeat("x", 201), CanSpeak: true}, ErrTooLong},
		"bad level":          {Input{Language: "German", Level: "fluent", CanSpeak: true}, ErrInvalidLevel},
		"level d1":           {Input{Language: "German", Level: "d1", CanSpeak: true}, ErrInvalidLevel},
		"neither spoken nor": {Input{Language: "German", Level: "b2"}, ErrSpokenOrWritten},
	}
	for name, c := range cases {
		if _, err := Add(p1, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if _, err := Add("nope", Input{Language: "German", CanSpeak: true}); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
	if list, _ := List(p1); len(list) != 0 {
		t.Errorf("a refusal must write nothing: %+v", list)
	}
}

func TestTheNameLimitIsExactlyTwoHundredCharacters(t *testing.T) {
	p1, _ := setup(t)
	if _, err := Add(p1, Input{Language: strings.Repeat("é", 200), CanSpeak: true}); err != nil {
		t.Errorf("200 characters (counted as characters, not bytes) are allowed: %v", err)
	}
}

func TestEveryLevelOnTheListIsAccepted(t *testing.T) {
	p1, _ := setup(t)
	for i, level := range Levels {
		if _, err := Add(p1, Input{Language: "L" + string(rune('a'+i)), Level: level, CanSpeak: true}); err != nil {
			t.Errorf("%s: %v", level, err)
		}
	}
	if len(Levels) != 7 {
		t.Errorf("%v", Levels)
	}
}

func TestALanguageIsListedOncePerProfileIgnoringCase(t *testing.T) {
	p1, p2 := setup(t)
	if _, err := Add(p1, Input{Language: "German", CanSpeak: true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"German", "german", " GERMAN "} {
		if _, err := Add(p1, Input{Language: name, CanWrite: true}); !errors.Is(err, ErrDuplicate) {
			t.Errorf("%q: %v", name, err)
		}
	}
	if _, err := Add(p2, Input{Language: "German", CanSpeak: true}); err != nil {
		t.Errorf("another profile may list it: %v", err)
	}
}

func TestListOrderIsNativeThenCEFRThenNoLevelEachByName(t *testing.T) {
	p1, _ := setup(t)
	for _, in := range []Input{
		{Language: "Zulu"}, {Language: "Basque", Level: "a1"}, {Language: "french", Level: "b2"}, {Language: "Arabic", Level: "b2"},
		{Language: "Greek", Level: "native"}, {Language: "Dutch", Level: "c2"}, {Language: "Czech", Level: "c1"}, {Language: "Danish", Level: "b1"}, {Language: "Estonian", Level: "a2"},
		{Language: "Albanian", Level: ""},
	} {
		in.CanSpeak = true
		if _, err := Add(p1, in); err != nil {
			t.Fatal(err)
		}
	}
	list, err := List(p1)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range list {
		got = append(got, l.Language)
	}
	want := "Greek,Dutch,Czech,Arabic,french,Danish,Estonian,Basque,Albanian,Zulu"
	if strings.Join(got, ",") != want {
		t.Errorf("%s, want %s", strings.Join(got, ","), want)
	}
}

func TestListIsEmptyForAnUnknownProfileAndOnlyTheProfilesOwn(t *testing.T) {
	p1, p2 := setup(t)
	if _, err := Add(p1, Input{Language: "German", CanSpeak: true}); err != nil {
		t.Fatal(err)
	}
	if l, err := List("nope"); err != nil || l == nil || len(l) != 0 {
		t.Errorf("%v %v", l, err)
	}
	if l, _ := List(p2); len(l) != 0 {
		t.Errorf("another profile's languages leaked: %+v", l)
	}
}

func TestUpdateChangesOnlyWhatIsSetAndChecksTheWholeEntry(t *testing.T) {
	p1, _ := setup(t)
	l, _ := Add(p1, Input{Language: "German", Level: "b1", CanSpeak: true})
	u, err := Update(l.ID, Patch{Level: sp("C2")})
	if err != nil || u.Level != "c2" || u.Language != "German" || !u.CanSpeak || u.CanWrite {
		t.Errorf("%+v %v", u, err)
	}
	u, err = Update(l.ID, Patch{CanWrite: bp(true), CanSpeak: bp(false)})
	if err != nil || u.CanSpeak || !u.CanWrite {
		t.Errorf("%+v %v", u, err)
	}
	// switching both off leaves neither: refused, nothing changes
	if _, err := Update(l.ID, Patch{CanWrite: bp(false)}); !errors.Is(err, ErrSpokenOrWritten) {
		t.Errorf("%v", err)
	}
	if _, err := Update(l.ID, Patch{Level: sp("zz")}); !errors.Is(err, ErrInvalidLevel) {
		t.Errorf("%v", err)
	}
	if _, err := Update(l.ID, Patch{Language: sp(" ")}); !errors.Is(err, ErrNameRequired) {
		t.Errorf("%v", err)
	}
	if again, _ := ByID(l.ID); again.Level != "c2" || again.CanSpeak || !again.CanWrite {
		t.Errorf("a refusal must change nothing: %+v", again)
	}
	if u, err := Update(l.ID, Patch{Level: sp("")}); err != nil || u.Level != "" {
		t.Errorf("the level can be cleared: %+v %v", u, err)
	}
	if _, err := Update("nope", Patch{Level: sp("a1")}); !errors.Is(err, ErrUnknownLanguage) {
		t.Errorf("%v", err)
	}
}

func TestRenamingToAnotherListedLanguageIsADuplicateButKeepingItsOwnNameIsNot(t *testing.T) {
	p1, _ := setup(t)
	a, _ := Add(p1, Input{Language: "German", CanSpeak: true})
	if _, err := Add(p1, Input{Language: "French", CanSpeak: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(a.ID, Patch{Language: sp("french")}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("%v", err)
	}
	if u, err := Update(a.ID, Patch{Language: sp("GERMAN")}); err != nil || u.Language != "GERMAN" {
		t.Errorf("only its own name (changing the case) is fine: %+v %v", u, err)
	}
}

func TestDeleteRemovesTheEntryAndAnUnknownOneIsAnError(t *testing.T) {
	p1, _ := setup(t)
	l, _ := Add(p1, Input{Language: "German", CanSpeak: true})
	if err := Delete(l.ID); err != nil {
		t.Fatal(err)
	}
	if err := Delete(l.ID); !errors.Is(err, ErrUnknownLanguage) {
		t.Errorf("%v", err)
	}
	if _, err := ByID(l.ID); !errors.Is(err, ErrUnknownLanguage) {
		t.Errorf("%v", err)
	}
}

func TestDeletingTheProfileDeletesItsLanguages(t *testing.T) {
	p1, p2 := setup(t)
	if _, err := Add(p1, Input{Language: "German", CanSpeak: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(p2, Input{Language: "German", CanSpeak: true}); err != nil {
		t.Fatal(err)
	}
	if err := profile.DeleteProfile(p1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM profile_languages`).Scan(&n); err != nil || n != 1 {
		t.Errorf("%d rows left, want only the other profile's: %v", n, err)
	}
}

func TestAnEntryIsStoredAsGivenInTheTable(t *testing.T) {
	p1, _ := setup(t)
	l, _ := Add(p1, Input{Language: "German", Level: "b2", CanSpeak: true, CanWrite: true})
	var speak, write int
	var level string
	if err := db.CareerDB.QueryRow(`SELECT level, can_speak, can_write FROM profile_languages WHERE id = ?`, l.ID).Scan(&level, &speak, &write); err != nil {
		t.Fatal(err)
	}
	if level != "b2" || speak != 1 || write != 1 {
		t.Errorf("%s %d %d", level, speak, write)
	}
}

func TestANameWithSQLInItIsOnlyAValue(t *testing.T) {
	p1, _ := setup(t)
	if _, err := Add(p1, Input{Language: "x'); DROP TABLE profile_languages;--", CanSpeak: true}); err != nil {
		t.Fatal(err)
	}
	if l, err := List(p1); err != nil || len(l) != 1 {
		t.Errorf("%v %v", l, err)
	}
}
