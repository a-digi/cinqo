package profileitems

import (
	"errors"
	"reflect"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

type world struct {
	p1, p2         string // profiles
	pa, pb, pc, pd string // personas: pa and pb and pd of p1, pc of p2
	events         []string
}

func setup(t *testing.T) *world {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	w := &world{}
	var err error
	if w.p1, err = profile.CreateProfile("Ada", "L"); err != nil {
		t.Fatal(err)
	}
	if w.p2, err = profile.CreateProfile("Bob", "M"); err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct {
		dst     *string
		profile string
		name    string
	}{{&w.pa, w.p1, "A"}, {&w.pb, w.p1, "B"}, {&w.pd, w.p1, "D"}, {&w.pc, w.p2, "C"}} {
		if *x.dst, err = persona.CreatePersona(x.profile, x.name, ""); err != nil {
			t.Fatal(err)
		}
	}
	old := publishChanged
	publishChanged = func(personaID, change string) { w.events = append(w.events, personaID+":"+change) }
	t.Cleanup(func() { publishChanged = old })
	return w
}

func skillOf(t *testing.T, profileID, text string) Skill {
	t.Helper()
	s, _, err := AddSkill(profileID, text)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func expOf(t *testing.T, profileID, company, title string) Experience {
	t.Helper()
	e, err := AddExperience(profileID, ExperienceInput{Company: company, Title: title, Description: "base"})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.CareerDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func sp(s string) *string { return &s }

func TestAddSkillTrimsDedupesAnyCaseAndNeedsAName(t *testing.T) {
	w := setup(t)
	a, created, err := AddSkill(w.p1, "  Go ")
	if err != nil || !created || a.Skill != "Go" || a.UsedBy != 0 {
		t.Fatalf("%+v %v %v", a, created, err)
	}
	b, created, err := AddSkill(w.p1, "GO")
	if err != nil || created || b.ID != a.ID {
		t.Fatalf("the same skill in another case must return the existing one: %+v %v %v", b, created, err)
	}
	if _, _, err := AddSkill(w.p1, "   "); !errors.Is(err, ErrSkillRequired) {
		t.Errorf("blank: %v", err)
	}
	if _, _, err := AddSkill("nope", "Go"); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
	// the other profile may have its own Go
	c, created, err := AddSkill(w.p2, "Go")
	if err != nil || !created || c.ID == a.ID {
		t.Errorf("another profile has its own: %+v %v %v", c, created, err)
	}
	for _, s := range []string{"rust", "Python", "ada"} {
		skillOf(t, w.p1, s)
	}
	list, _ := ListSkills(w.p1)
	var names []string
	for _, s := range list {
		names = append(names, s.Skill)
	}
	if !reflect.DeepEqual(names, []string{"ada", "Go", "Python", "rust"}) {
		t.Errorf("sorted by name, any case: %v", names)
	}
}

func TestRenameSkillRefusesADuplicateAndAnnouncesTheUsers(t *testing.T) {
	w := setup(t)
	g, rs := skillOf(t, w.p1, "Go"), skillOf(t, w.p1, "Rust")
	if err := SetPersonaSkills(w.pa, []string{g.ID}); err != nil {
		t.Fatal(err)
	}
	w.events = nil
	if err := RenameSkill(g.ID, "rust"); !errors.Is(err, ErrDuplicateSkill) {
		t.Errorf("duplicate: %v", err)
	}
	if err := RenameSkill(g.ID, " "); !errors.Is(err, ErrSkillRequired) {
		t.Errorf("blank: %v", err)
	}
	if err := RenameSkill("nope", "X"); !errors.Is(err, ErrUnknownItem) {
		t.Errorf("unknown: %v", err)
	}
	if len(w.events) != 0 {
		t.Errorf("refusals announce nothing: %v", w.events)
	}
	if err := RenameSkill(g.ID, "Golang"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w.events, []string{w.pa + ":skill_added"}) {
		t.Errorf("only the user is told: %v", w.events)
	}
	w.events = nil
	if err := RenameSkill(g.ID, "Golang"); err != nil || len(w.events) != 0 {
		t.Errorf("the same name changes nothing: %v %v", err, w.events)
	}
	if err := RenameSkill(rs.ID, "RUST"); err != nil { // its own name in another case is allowed
		t.Errorf("own name in another case: %v", err)
	}
}

func TestDeleteSkillRemovesItFromEveryPersonaAndAnnouncesThem(t *testing.T) {
	w := setup(t)
	g := skillOf(t, w.p1, "Go")
	for _, p := range []string{w.pa, w.pb} {
		if err := SetPersonaSkills(p, []string{g.ID}); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := ListSkills(w.p1)
	if list[0].UsedBy != 2 {
		t.Errorf("used by %d, want 2", list[0].UsedBy)
	}
	w.events = nil
	if err := DeleteSkill(g.ID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT COUNT(*) FROM persona_skills`); n != 0 {
		t.Errorf("links left: %d", n)
	}
	want := map[string]bool{w.pa + ":skill_removed": true, w.pb + ":skill_removed": true}
	if len(w.events) != 2 || !want[w.events[0]] || !want[w.events[1]] {
		t.Errorf("events %v", w.events)
	}
	if err := DeleteSkill(g.ID); !errors.Is(err, ErrUnknownItem) {
		t.Errorf("twice: %v", err)
	}
}

func TestSetPersonaSkillsReplacesTheSetAndChecksEverythingFirst(t *testing.T) {
	w := setup(t)
	a, b, c := skillOf(t, w.p1, "A"), skillOf(t, w.p1, "B"), skillOf(t, w.p1, "C")
	other := skillOf(t, w.p2, "Other")
	if err := SetPersonaSkills(w.pa, []string{a.ID, b.ID, a.ID, " "}); err != nil {
		t.Fatal(err)
	}
	use, _ := GetPersonaUse(w.pa)
	if !reflect.DeepEqual(use.SkillIDs, []string{a.ID, b.ID}) {
		t.Fatalf("%v", use.SkillIDs)
	}
	w.events = nil
	if err := SetPersonaSkills(w.pa, []string{a.ID, b.ID}); err != nil || len(w.events) != 0 {
		t.Errorf("the same set changes nothing and announces nothing: %v %v", err, w.events)
	}
	// replace: B goes, C comes
	if err := SetPersonaSkills(w.pa, []string{a.ID, c.ID}); err != nil {
		t.Fatal(err)
	}
	use, _ = GetPersonaUse(w.pa)
	if !reflect.DeepEqual(use.SkillIDs, []string{a.ID, c.ID}) {
		t.Errorf("%v", use.SkillIDs)
	}
	if !reflect.DeepEqual(w.events, []string{w.pa + ":skill_added", w.pa + ":skill_removed"}) {
		t.Errorf("events %v", w.events)
	}
	// refusals write nothing
	w.events = nil
	if err := SetPersonaSkills(w.pa, []string{a.ID, other.ID}); !errors.Is(err, ErrOtherProfile) {
		t.Errorf("other profile: %v", err)
	}
	if err := SetPersonaSkills(w.pa, []string{a.ID, "nope"}); !errors.Is(err, ErrUnknownItem) {
		t.Errorf("unknown: %v", err)
	}
	if err := SetPersonaSkills("ghost", nil); !errors.Is(err, ErrUnknownPersona) {
		t.Errorf("unknown persona: %v", err)
	}
	use, _ = GetPersonaUse(w.pa)
	if !reflect.DeepEqual(use.SkillIDs, []string{a.ID, c.ID}) || len(w.events) != 0 {
		t.Errorf("a refusal must change nothing: %v %v", use.SkillIDs, w.events)
	}
	// none is allowed
	if err := SetPersonaSkills(w.pa, nil); err != nil {
		t.Fatal(err)
	}
	if use, _ = GetPersonaUse(w.pa); len(use.SkillIDs) != 0 {
		t.Errorf("%v", use.SkillIDs)
	}
	// another persona is untouched
	if err := SetPersonaSkills(w.pb, []string{a.ID}); err != nil {
		t.Fatal(err)
	}
	if err := SetPersonaSkills(w.pa, []string{c.ID}); err != nil {
		t.Fatal(err)
	}
	if use, _ = GetPersonaUse(w.pb); !reflect.DeepEqual(use.SkillIDs, []string{a.ID}) {
		t.Errorf("B: %v", use.SkillIDs)
	}
}

func TestExperienceCrudNeedsCompanyAndTitle(t *testing.T) {
	w := setup(t)
	if _, err := AddExperience(w.p1, ExperienceInput{Company: " ", Title: "Dev"}); !errors.Is(err, ErrExperienceRequired) {
		t.Errorf("no company: %v", err)
	}
	if _, err := AddExperience(w.p1, ExperienceInput{Company: "Acme"}); !errors.Is(err, ErrExperienceRequired) {
		t.Errorf("no title: %v", err)
	}
	if _, err := AddExperience("nope", ExperienceInput{Company: "A", Title: "T"}); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
	e, err := AddExperience(w.p1, ExperienceInput{Company: " Acme ", Title: "Dev", StartDate: "2020", EndDate: "2022", Description: "built X"})
	if err != nil || e.Company != "Acme" || e.StartDate != "2020" || e.Description != "built X" || e.UsedBy != 0 {
		t.Fatalf("%+v %v", e, err)
	}
	if err := UpdateExperience(e.ID, ExperiencePatch{Company: sp(" ")}); !errors.Is(err, ErrExperienceRequired) {
		t.Errorf("blank company: %v", err)
	}
	if err := UpdateExperience(e.ID, ExperiencePatch{Title: sp("")}); !errors.Is(err, ErrExperienceRequired) {
		t.Errorf("blank title: %v", err)
	}
	if err := UpdateExperience("nope", ExperiencePatch{Title: sp("X")}); !errors.Is(err, ErrUnknownItem) {
		t.Errorf("unknown: %v", err)
	}
	if err := UpdateExperience(e.ID, ExperiencePatch{Title: sp("Lead"), Description: sp("")}); err != nil {
		t.Fatal(err)
	}
	list, _ := ListExperience(w.p1)
	if len(list) != 1 || list[0].Title != "Lead" || list[0].Description != "" || list[0].Company != "Acme" || list[0].StartDate != "2020" {
		t.Errorf("only the given fields change: %+v", list)
	}
	if err := UpdateExperience(e.ID, ExperiencePatch{}); err != nil {
		t.Errorf("an empty patch is a no-op: %v", err)
	}
	if err := DeleteExperience(e.ID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteExperience(e.ID); !errors.Is(err, ErrUnknownItem) {
		t.Errorf("twice: %v", err)
	}
}

func TestSetPersonaExperienceOverridesAndRefusesBadInput(t *testing.T) {
	w := setup(t)
	e1, e2 := expOf(t, w.p1, "Acme", "Dev"), expOf(t, w.p1, "Beta", "Lead")
	foreign := expOf(t, w.p2, "Zed", "X")
	if err := SetPersonaExperience(w.pa, []ExperienceUse{{ID: e1.ID, TitleOverride: sp(" Backend Dev "), DescriptionOverride: sp("")}, {ID: e2.ID}}); err != nil {
		t.Fatal(err)
	}
	use, _ := GetPersonaUse(w.pa)
	by := map[string]ExperienceUse{}
	for _, u := range use.Experience {
		by[u.ID] = u
	}
	if u := by[e1.ID]; u.TitleOverride == nil || *u.TitleOverride != "Backend Dev" || u.DescriptionOverride == nil || *u.DescriptionOverride != "" {
		t.Errorf("e1 overrides (a blank description is an override to empty): %+v", u)
	}
	if u := by[e2.ID]; u.TitleOverride != nil || u.DescriptionOverride != nil {
		t.Errorf("e2 follows the profile: %+v", u)
	}
	// overrides are changed and reset by a new call
	w.events = nil
	if err := SetPersonaExperience(w.pa, []ExperienceUse{{ID: e1.ID}, {ID: e2.ID}}); err != nil {
		t.Fatal(err)
	}
	use, _ = GetPersonaUse(w.pa)
	for _, u := range use.Experience {
		if u.TitleOverride != nil || u.DescriptionOverride != nil {
			t.Errorf("the overrides must be reset: %+v", u)
		}
	}
	if !reflect.DeepEqual(w.events, []string{w.pa + ":experience_updated"}) {
		t.Errorf("events %v", w.events)
	}
	// refusals
	w.events = nil
	for name, uses := range map[string][]ExperienceUse{
		"blank title override": {{ID: e1.ID, TitleOverride: sp("  ")}},
		"other profile":        {{ID: e1.ID}, {ID: foreign.ID}},
		"unknown":              {{ID: "nope"}},
	} {
		err := SetPersonaExperience(w.pa, uses)
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if err := SetPersonaExperience(w.pa, []ExperienceUse{{ID: foreign.ID}}); !errors.Is(err, ErrOtherProfile) {
		t.Errorf("other profile: %v", err)
	}
	if err := SetPersonaExperience(w.pa, []ExperienceUse{{ID: e1.ID, TitleOverride: sp(" ")}}); !errors.Is(err, ErrTitleOverrideEmpty) {
		t.Errorf("blank title: %v", err)
	}
	use, _ = GetPersonaUse(w.pa)
	if len(use.Experience) != 2 || len(w.events) != 0 {
		t.Errorf("a refusal must change nothing: %+v %v", use, w.events)
	}
	// dropping one announces a removal; adding announces an addition
	if err := SetPersonaExperience(w.pa, []ExperienceUse{{ID: e1.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := SetPersonaExperience(w.pa, []ExperienceUse{{ID: e1.ID}, {ID: e2.ID}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w.events, []string{w.pa + ":experience_removed", w.pa + ":experience_added"}) {
		t.Errorf("events %v", w.events)
	}
	if err := SetPersonaExperience("ghost", nil); !errors.Is(err, ErrUnknownPersona) {
		t.Errorf("unknown persona: %v", err)
	}
}

func TestUpdateAndDeleteExperienceAnnounceTheUsersAndUsedByCounts(t *testing.T) {
	w := setup(t)
	e := expOf(t, w.p1, "Acme", "Dev")
	for _, p := range []string{w.pa, w.pb} {
		if err := SetPersonaExperience(p, []ExperienceUse{{ID: e.ID}}); err != nil {
			t.Fatal(err)
		}
	}
	if list, _ := ListExperience(w.p1); list[0].UsedBy != 2 {
		t.Errorf("used by %d", list[0].UsedBy)
	}
	w.events = nil
	if err := UpdateExperience(e.ID, ExperiencePatch{Description: sp("new")}); err != nil {
		t.Fatal(err)
	}
	if len(w.events) != 2 || w.events[0][len(w.events[0])-len("experience_updated"):] != "experience_updated" {
		t.Errorf("events %v", w.events)
	}
	w.events = nil
	if err := DeleteExperience(e.ID); err != nil {
		t.Fatal(err)
	}
	if len(w.events) != 2 || n(t) != 0 {
		t.Errorf("events %v links %d", w.events, n(t))
	}
}

func n(t *testing.T) int { return count(t, `SELECT COUNT(*) FROM persona_experience`) }

func TestAnItemWithNoUserAnnouncesNothing(t *testing.T) {
	w := setup(t)
	s, e := skillOf(t, w.p1, "Go"), expOf(t, w.p1, "Acme", "Dev")
	_ = RenameSkill(s.ID, "Golang")
	_ = UpdateExperience(e.ID, ExperiencePatch{Title: sp("Lead")})
	_ = DeleteSkill(s.ID)
	_ = DeleteExperience(e.ID)
	if len(w.events) != 0 {
		t.Errorf("nobody uses them: %v", w.events)
	}
}

func TestCascades(t *testing.T) {
	w := setup(t)
	s, e := skillOf(t, w.p1, "Go"), expOf(t, w.p1, "Acme", "Dev")
	for _, p := range []string{w.pa, w.pb} {
		if err := SetPersonaSkills(p, []string{s.ID}); err != nil {
			t.Fatal(err)
		}
		if err := SetPersonaExperience(p, []ExperienceUse{{ID: e.ID, TitleOverride: sp("T")}}); err != nil {
			t.Fatal(err)
		}
	}
	// a persona's own custom items
	if err := persona.AddCareerSkill(w.pa, "Own"); err != nil {
		t.Fatal(err)
	}
	// deleting a persona removes its links and its own items; the library stays
	if err := persona.DeletePersona(w.pa); err != nil {
		t.Fatal(err)
	}
	if count(t, `SELECT COUNT(*) FROM persona_skills WHERE persona_id = ?`, w.pa)+count(t, `SELECT COUNT(*) FROM persona_experience WHERE persona_id = ?`, w.pa)+count(t, `SELECT COUNT(*) FROM career_skills WHERE persona_id = ?`, w.pa) != 0 {
		t.Error("a deleted persona left rows")
	}
	if count(t, `SELECT COUNT(*) FROM profile_skills`) != 1 || count(t, `SELECT COUNT(*) FROM profile_experience`) != 1 {
		t.Error("the library must stay")
	}
	if count(t, `SELECT COUNT(*) FROM persona_skills WHERE persona_id = ?`, w.pb) != 1 {
		t.Error("another persona keeps its link")
	}
	// deleting the profile removes everything of it
	if err := profile.DeleteProfile(w.p1); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"profile_skills", "profile_experience", "persona_skills", "persona_experience"} {
		if count(t, `SELECT COUNT(*) FROM `+table) != 0 {
			t.Errorf("%s has rows after the profile was deleted", table)
		}
	}
}

func TestGetPersonaUseOfAnUnknownPersona(t *testing.T) {
	setup(t)
	if _, err := GetPersonaUse("ghost"); !errors.Is(err, ErrUnknownPersona) {
		t.Errorf("%v", err)
	}
}
