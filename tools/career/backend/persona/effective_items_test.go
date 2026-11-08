package persona

import (
	"errors"
	"reflect"
	"testing"

	"career-tool-backend/db"
)

// Plan 173, step 1: what a persona shows is the library items it uses (overrides applied) plus its own custom items.
func execCareer(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.CareerDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func profileOf(t *testing.T, personaID string) string {
	t.Helper()
	var p string
	if err := db.CareerDB.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, personaID).Scan(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSkillsAreTheUsedLibrarySkillsPlusTheOwnOnesWithoutRepeats(t *testing.T) {
	id := newPersona(t)
	pr := profileOf(t, id)
	execCareer(t, `INSERT INTO profile_skills (id, profile_id, skill) VALUES ('k1', ?, 'Go'), ('k2', ?, 'Python'), ('k3', ?, 'SQL')`, pr, pr, pr)
	execCareer(t, `INSERT INTO persona_skills (persona_id, profile_skill_id) VALUES (?, 'k1'), (?, 'k2')`, id, id) // SQL is not used
	if err := AddCareerSkill(id, "rust"); err != nil {
		t.Fatal(err)
	}
	if err := AddCareerSkill(id, "go"); err != nil { // the same skill as the library's Go, in another case
		t.Fatal(err)
	}
	d, err := FetchPersonaDetails(id)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Go", "Python", "rust"}; !reflect.DeepEqual(d.Skills, want) {
		t.Errorf("skills %v, want %v (sorted, the library's Go wins over the own go, SQL is not used)", d.Skills, want)
	}
	got := map[string]string{}
	for _, it := range d.SkillItems {
		got[it.Skill] = it.Source
	}
	if got["Go"] != SourceProfile || got["Python"] != SourceProfile || got["rust"] != SourcePersona || len(got) != 3 {
		t.Errorf("sources %v", got)
	}
	// another persona of the same profile that uses nothing shows only its own
	other, err := CreatePersona(pr, "Other", "")
	if err != nil {
		t.Fatal(err)
	}
	d2, _ := FetchPersonaDetails(other)
	if len(d2.Skills) != 0 || len(d2.SkillItems) != 0 || d2.Skills == nil || d2.SkillItems == nil {
		t.Errorf("an empty persona must show empty lists, not null: %+v", d2)
	}
}

func TestExperienceAppliesTheOverridesAndMixesTheOwnOnesNewestFirst(t *testing.T) {
	id := newPersona(t)
	pr := profileOf(t, id)
	execCareer(t, `INSERT INTO profile_experience (id, profile_id, company, title, start_date, end_date, description, created_at) VALUES
	  ('x1', ?, 'Acme', 'Dev', '2020', '2022', 'base one', '2026-01-01 10:00:00'),
	  ('x2', ?, 'Beta', 'Lead', '2022', '', 'base two', '2026-01-03 10:00:00'),
	  ('x3', ?, 'Gamma', 'Intern', '2018', '2019', 'not used', '2026-01-02 10:00:00')`, pr, pr, pr)
	execCareer(t, `INSERT INTO persona_experience (persona_id, profile_experience_id, title_override, description_override) VALUES
	  (?, 'x1', 'Backend Dev', ''), (?, 'x2', NULL, NULL)`, id, id)
	execCareer(t, `INSERT INTO career_experience (id, persona_id, company, title, description, created_at) VALUES ('own', ?, 'Own', 'Freelance', 'mine', '2026-01-02 12:00:00')`, id)
	d, err := FetchPersonaDetails(id)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range d.Experience {
		ids = append(ids, e.ID)
	}
	if !reflect.DeepEqual(ids, []string{"x2", "own", "x1"}) {
		t.Fatalf("newest first, the unused x3 absent: %v", ids)
	}
	by := map[string]careerExperience{}
	for _, e := range d.Experience {
		by[e.ID] = e
	}
	if e := by["x1"]; e.Title != "Backend Dev" || !e.TitleOverridden || e.Description != "" || !e.DescriptionOverridden || e.Company != "Acme" || e.StartDate != "2020" || e.Source != SourceProfile {
		t.Errorf("x1 (title and description overridden, the company and the dates are the profile's): %+v", e)
	}
	if e := by["x2"]; e.Title != "Lead" || e.TitleOverridden || e.Description != "base two" || e.DescriptionOverridden || e.Source != SourceProfile {
		t.Errorf("x2 follows the profile: %+v", e)
	}
	if e := by["own"]; e.Source != SourcePersona || e.Title != "Freelance" || e.TitleOverridden {
		t.Errorf("own: %+v", e)
	}
	// editing the profile item changes what the persona shows, but not what it overrides
	execCareer(t, `UPDATE profile_experience SET title = 'Senior Dev', description = 'new base', company = 'Acme Inc' WHERE id = 'x1'`)
	execCareer(t, `UPDATE profile_experience SET description = 'new base two' WHERE id = 'x2'`)
	d, _ = FetchPersonaDetails(id)
	for _, e := range d.Experience {
		switch e.ID {
		case "x1":
			if e.Title != "Backend Dev" || e.Description != "" || e.Company != "Acme Inc" {
				t.Errorf("x1 after the profile edit: %+v", e)
			}
		case "x2":
			if e.Description != "new base two" {
				t.Errorf("x2 must follow the edit: %+v", e)
			}
		}
	}
}

func TestOldPersonaRoutesStillWriteOwnItemsOnly(t *testing.T) {
	id := newPersona(t)
	pr := profileOf(t, id)
	execCareer(t, `INSERT INTO profile_experience (id, profile_id, company, title) VALUES ('x1', ?, 'Acme', 'Dev')`, pr)
	execCareer(t, `INSERT INTO persona_experience (persona_id, profile_experience_id) VALUES (?, 'x1')`, id)
	own, err := AddCareerExperience(id, "Own", "Role", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// the persona's own routes refuse an experience that comes from the library, and change nothing
	if err := RemoveCareerExperience(id, "x1"); !errors.Is(err, ErrProfileItem) {
		t.Fatalf("remove: %v", err)
	}
	title := "Changed"
	if err := UpdateCareerExperience(UpdateCareerExperienceArgs{PersonaID: id, ID: "x1", Title: &title}); !errors.Is(err, ErrProfileItem) {
		t.Fatalf("update: %v", err)
	}
	d, _ := FetchPersonaDetails(id)
	if len(d.Experience) != 2 {
		t.Fatalf("the library experience must stay: %+v", d.Experience)
	}
	var libTitle string
	if err := db.CareerDB.QueryRow(`SELECT title FROM profile_experience WHERE id = 'x1'`).Scan(&libTitle); err != nil || libTitle != "Dev" {
		t.Fatalf("the library item must be untouched: %q %v", libTitle, err)
	}
	// an unknown id stays the silent no-op it always was
	if err := RemoveCareerExperience(id, "nope"); err != nil {
		t.Errorf("unknown id: %v", err)
	}
	if err := RemoveCareerExperience(id, own); err != nil {
		t.Fatal(err)
	}
	d, _ = FetchPersonaDetails(id)
	if len(d.Experience) != 1 || d.Experience[0].Source != SourceProfile {
		t.Errorf("%+v", d.Experience)
	}
}

// Plan 173 decision 7: a persona proposed by the AI links the library skills and experiences it names and keeps the rest as its own.
func TestAppliedPersonaLinksTheLibraryAndKeepsTheRestOwn(t *testing.T) {
	profileID := setupRunsDB(t)
	execCareer(t, `INSERT INTO profile_skills (id, profile_id, skill) VALUES ('k1', ?, 'Go'), ('k2', ?, 'Python')`, profileID, profileID)
	execCareer(t, `INSERT INTO profile_experience (id, profile_id, company, title, start_date, end_date, description) VALUES
	  ('x1', ?, 'Acme', 'Dev', '2020', '2022', 'base'), ('x2', ?, 'Beta', 'Lead', '2022', '', 'lead base')`, profileID, profileID)
	runID := mustRun(t, profileID) // before the transaction: the connection is the only one
	tx, err := db.CareerDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	id, err := insertAppliedPersona(tx, profileID, runID, ProposedPersona{
		Name:   "Backend",
		Skills: []string{" go ", "Rust"}, // Go is in the library (any case), Rust is not
		Experience: []ProposedRole{
			{Company: "acme", Title: "Dev", StartDate: "2020", EndDate: "2022", Description: "base"},                   // same: linked, no override
			{Company: "Beta", Title: "Lead", StartDate: "2022", EndDate: "", Description: "my own wording"},            // linked, description is the persona's
			{Company: "Gamma", Title: "Intern", StartDate: "2018", EndDate: "2019", Description: "not in the library"}, // own
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	d, err := FetchPersonaDetails(id)
	if err != nil {
		t.Fatal(err)
	}
	src := map[string]string{}
	for _, it := range d.SkillItems {
		src[it.Skill] = it.Source
	}
	if src["Go"] != SourceProfile || src["Rust"] != SourcePersona || len(src) != 2 {
		t.Errorf("skills %v (Go linked, Rust own, Python not used)", src)
	}
	by := map[string]careerExperience{}
	for _, e := range d.Experience {
		by[e.Company] = e
	}
	if e := by["Acme"]; e.Source != SourceProfile || e.DescriptionOverridden || e.TitleOverridden || e.Description != "base" {
		t.Errorf("Acme: %+v", e)
	}
	if e := by["Beta"]; e.Source != SourceProfile || !e.DescriptionOverridden || e.Description != "my own wording" {
		t.Errorf("Beta: %+v", e)
	}
	if e := by["Gamma"]; e.Source != SourcePersona {
		t.Errorf("Gamma: %+v", e)
	}
	if len(d.Experience) != 3 {
		t.Errorf("experience %+v", d.Experience)
	}
	// the library is not changed by applying a persona
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM profile_skills`).Scan(&n); err != nil || n != 2 {
		t.Errorf("the library gained skills: %d %v", n, err)
	}
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM profile_experience`).Scan(&n); err != nil || n != 2 {
		t.Errorf("the library gained experiences: %d %v", n, err)
	}
}
