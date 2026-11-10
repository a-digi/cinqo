// Package profileitems is the profile's own library of skills and experience, and what a persona uses of it (plan
// plan/ai/tools/career/step-173-persona-skills-experience-and-education.md, step 1). The library is global to the profile; a persona
// USES some of its skills and experiences (persona_skills, persona_experience) and may override the title and the description of a
// used experience for its role; the persona's OWN custom items stay in career_skills and career_experience (package persona).
// What a persona finally shows is computed by persona.FetchPersonaDetails from both.
//
// Every change that moves what a persona shows announces "career.persona.changed" for each persona it touches, like the persona's
// own edits, so the job match hears of it. Every check comes before the first write and every multi-row change is one transaction.
package profileitems

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"

	"career-tool-backend/db"
	"career-tool-backend/persona"
)

var (
	ErrUnknownProfile = errors.New("unknown profile id")
	ErrUnknownPersona = errors.New("unknown target role id")
	// ErrUnknownItem is an unknown skill or experience of the library.
	ErrUnknownItem = errors.New("unknown profile item id")
	// ErrOtherProfile: a persona can only use items of its own profile.
	ErrOtherProfile       = errors.New("an item of another profile cannot be used by this target role")
	ErrSkillRequired      = errors.New("a skill needs a name")
	ErrDuplicateSkill     = errors.New("the profile already has this skill")
	ErrExperienceRequired = errors.New("an experience needs a company and a title")
	ErrTitleOverrideEmpty = errors.New("a title override cannot be empty")
)

// publishChanged is a var so a test can watch the announcements.
var publishChanged = persona.PublishChanged

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func requireProfile(id string) error {
	var one int
	switch err := db.CareerDB.QueryRow(`SELECT 1 FROM profiles WHERE id = ?`, id).Scan(&one); {
	case err == sql.ErrNoRows:
		return ErrUnknownProfile
	case err != nil:
		return err
	}
	return nil
}

// personaProfile is the profile of a persona, or ErrUnknownPersona.
func personaProfile(personaID string) (string, error) {
	var profileID string
	switch err := db.CareerDB.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, personaID).Scan(&profileID); {
	case err == sql.ErrNoRows:
		return "", ErrUnknownPersona
	case err != nil:
		return "", err
	}
	return profileID, nil
}

// usersOfSkill / usersOfExperience: the personas that use an item, oldest first.
func usersOfSkill(id string) ([]string, error) {
	return queryIDs(`SELECT persona_id FROM persona_skills WHERE profile_skill_id = ? ORDER BY persona_id`, id)
}

func usersOfExperience(id string) ([]string, error) {
	return queryIDs(`SELECT persona_id FROM persona_experience WHERE profile_experience_id = ? ORDER BY persona_id`, id)
}

func queryIDs(q string, args ...any) ([]string, error) {
	rows, err := db.CareerDB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func announce(personaIDs []string, change string) {
	for _, id := range personaIDs {
		publishChanged(id, change)
	}
}

// --- skills ---

type Skill struct {
	ID        string `json:"id"`
	ProfileID string `json:"profileId"`
	Skill     string `json:"skill"`
	CreatedAt string `json:"createdAt"`
	// UsedBy is how many personas use this skill.
	UsedBy int `json:"usedBy"`
}

// ListSkills is the profile's skills, by name (case-insensitive), with how many personas use each. An unknown profile has none.
func ListSkills(profileID string) ([]Skill, error) {
	rows, err := db.CareerDB.Query(
		`SELECT k.id, k.profile_id, k.skill, k.created_at, (SELECT COUNT(*) FROM persona_skills ps WHERE ps.profile_skill_id = k.id)
		 FROM profile_skills k WHERE k.profile_id = ? ORDER BY k.skill COLLATE NOCASE ASC, k.id ASC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		var s Skill
		if err := rows.Scan(&s.ID, &s.ProfileID, &s.Skill, &s.CreatedAt, &s.UsedBy); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// AddSkill adds a skill to the profile's library. A skill the profile already has (same text, any case) is not added again: the
// existing one is returned with created false.
func AddSkill(profileID, skill string) (Skill, bool, error) {
	skill = strings.TrimSpace(skill)
	if skill == "" {
		return Skill{}, false, ErrSkillRequired
	}
	if err := requireProfile(profileID); err != nil {
		return Skill{}, false, err
	}
	existing, err := ListSkills(profileID)
	if err != nil {
		return Skill{}, false, err
	}
	for _, s := range existing {
		if norm(s.Skill) == norm(skill) {
			return s, false, nil
		}
	}
	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(`INSERT INTO profile_skills (id, profile_id, skill) VALUES (?, ?, ?)`, id, profileID, skill); err != nil {
		return Skill{}, false, err
	}
	all, err := ListSkills(profileID)
	if err != nil {
		return Skill{}, false, err
	}
	for _, s := range all {
		if s.ID == id {
			return s, true, nil
		}
	}
	return Skill{}, false, ErrUnknownItem
}

// RenameSkill changes a skill's text for every persona that uses it. Another skill of the profile with the same text is
// ErrDuplicateSkill.
func RenameSkill(id, skill string) error {
	skill = strings.TrimSpace(skill)
	if skill == "" {
		return ErrSkillRequired
	}
	var profileID, old string
	switch err := db.CareerDB.QueryRow(`SELECT profile_id, skill FROM profile_skills WHERE id = ?`, id).Scan(&profileID, &old); {
	case err == sql.ErrNoRows:
		return ErrUnknownItem
	case err != nil:
		return err
	}
	others, err := ListSkills(profileID)
	if err != nil {
		return err
	}
	for _, s := range others {
		if s.ID != id && norm(s.Skill) == norm(skill) {
			return ErrDuplicateSkill
		}
	}
	if old == skill {
		return nil
	}
	if _, err := db.CareerDB.Exec(`UPDATE profile_skills SET skill = ? WHERE id = ?`, skill, id); err != nil {
		return err
	}
	users, err := usersOfSkill(id)
	if err == nil {
		announce(users, persona.ChangeSkillAdded)
	}
	return err
}

// DeleteSkill removes a skill from the library and so from every persona that used it.
func DeleteSkill(id string) error {
	users, err := usersOfSkill(id)
	if err != nil {
		return err
	}
	res, err := db.CareerDB.Exec(`DELETE FROM profile_skills WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUnknownItem
	}
	announce(users, persona.ChangeSkillRemoved)
	return nil
}

// --- experience ---

type Experience struct {
	ID          string `json:"id"`
	ProfileID   string `json:"profileId"`
	Company     string `json:"company"`
	Title       string `json:"title"`
	StartDate   string `json:"startDate,omitempty"`
	EndDate     string `json:"endDate,omitempty"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt"`
	// UsedBy is how many personas use this experience.
	UsedBy int `json:"usedBy"`
}

// ExperienceInput is what a new library experience is made of.
type ExperienceInput struct {
	Company     string
	Title       string
	StartDate   string
	EndDate     string
	Description string
}

// ListExperience is the profile's experience, newest first (the order the persona lists use), with how many personas use each.
func ListExperience(profileID string) ([]Experience, error) {
	rows, err := db.CareerDB.Query(
		`SELECT x.id, x.profile_id, x.company, x.title, x.start_date, x.end_date, x.description, x.created_at,
		        (SELECT COUNT(*) FROM persona_experience pe WHERE pe.profile_experience_id = x.id)
		 FROM profile_experience x WHERE x.profile_id = ? ORDER BY x.created_at DESC, x.id ASC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Experience{}
	for rows.Next() {
		var e Experience
		var start, end, description sql.NullString
		if err := rows.Scan(&e.ID, &e.ProfileID, &e.Company, &e.Title, &start, &end, &description, &e.CreatedAt, &e.UsedBy); err != nil {
			return nil, err
		}
		e.StartDate, e.EndDate, e.Description = start.String, end.String, description.String
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddExperience adds an experience to the profile's library; company and title are required.
func AddExperience(profileID string, in ExperienceInput) (Experience, error) {
	in.Company, in.Title = strings.TrimSpace(in.Company), strings.TrimSpace(in.Title)
	if in.Company == "" || in.Title == "" {
		return Experience{}, ErrExperienceRequired
	}
	if err := requireProfile(profileID); err != nil {
		return Experience{}, err
	}
	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(
		`INSERT INTO profile_experience (id, profile_id, company, title, start_date, end_date, description) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, profileID, in.Company, in.Title, in.StartDate, in.EndDate, in.Description,
	); err != nil {
		return Experience{}, err
	}
	all, err := ListExperience(profileID)
	if err != nil {
		return Experience{}, err
	}
	for _, e := range all {
		if e.ID == id {
			return e, nil
		}
	}
	return Experience{}, ErrUnknownItem
}

// ExperiencePatch changes only the fields that are set.
type ExperiencePatch struct {
	Company, Title, StartDate, EndDate, Description *string
}

// UpdateExperience changes a library experience for every persona that uses it (except the title and description a persona
// overrides). Company and title may not become empty.
func UpdateExperience(id string, p ExperiencePatch) error {
	var company, title string
	switch err := db.CareerDB.QueryRow(`SELECT company, title FROM profile_experience WHERE id = ?`, id).Scan(&company, &title); {
	case err == sql.ErrNoRows:
		return ErrUnknownItem
	case err != nil:
		return err
	}
	set, args := []string{}, []any{}
	add := func(column string, v *string, required bool) error {
		if v == nil {
			return nil
		}
		text := *v
		if required {
			text = strings.TrimSpace(text)
			if text == "" {
				return ErrExperienceRequired
			}
		}
		set = append(set, column+" = ?")
		args = append(args, text)
		return nil
	}
	for _, f := range []struct {
		column   string
		v        *string
		required bool
	}{{"company", p.Company, true}, {"title", p.Title, true}, {"start_date", p.StartDate, false}, {"end_date", p.EndDate, false}, {"description", p.Description, false}} {
		if err := add(f.column, f.v, f.required); err != nil {
			return err
		}
	}
	if len(set) == 0 {
		return nil
	}
	args = append(args, id)
	if _, err := db.CareerDB.Exec(`UPDATE profile_experience SET `+strings.Join(set, ", ")+` WHERE id = ?`, args...); err != nil {
		return err
	}
	users, err := usersOfExperience(id)
	if err == nil {
		announce(users, persona.ChangeExperienceUpdated)
	}
	return err
}

// DeleteExperience removes an experience from the library and so from every persona that used it.
func DeleteExperience(id string) error {
	users, err := usersOfExperience(id)
	if err != nil {
		return err
	}
	res, err := db.CareerDB.Exec(`DELETE FROM profile_experience WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUnknownItem
	}
	announce(users, persona.ChangeExperienceRemoved)
	return nil
}

// --- what a persona uses ---

// ExperienceUse is one used experience and the persona's overrides of it: nil follows the profile's text; a title override may not be
// empty; a description override may be empty (the persona shows no description).
type ExperienceUse struct {
	ID                  string  `json:"id"`
	TitleOverride       *string `json:"titleOverride,omitempty"`
	DescriptionOverride *string `json:"descriptionOverride,omitempty"`
}

// PersonaUse is what a persona uses of its profile's library.
type PersonaUse struct {
	SkillIDs   []string        `json:"skillIds"`
	Experience []ExperienceUse `json:"experience"`
}

// GetPersonaUse reads what a persona uses. An unknown persona is ErrUnknownPersona.
func GetPersonaUse(personaID string) (PersonaUse, error) {
	use := PersonaUse{SkillIDs: []string{}, Experience: []ExperienceUse{}}
	if _, err := personaProfile(personaID); err != nil {
		return use, err
	}
	ids, err := queryIDs(`SELECT ps.profile_skill_id FROM persona_skills ps JOIN profile_skills k ON k.id = ps.profile_skill_id
		WHERE ps.persona_id = ? ORDER BY k.skill COLLATE NOCASE ASC, k.id ASC`, personaID)
	if err != nil {
		return use, err
	}
	use.SkillIDs = ids
	rows, err := db.CareerDB.Query(
		`SELECT pe.profile_experience_id, pe.title_override, pe.description_override
		 FROM persona_experience pe JOIN profile_experience x ON x.id = pe.profile_experience_id
		 WHERE pe.persona_id = ? ORDER BY x.created_at DESC, x.id ASC`, personaID)
	if err != nil {
		return use, err
	}
	defer rows.Close()
	for rows.Next() {
		var u ExperienceUse
		var title, description sql.NullString
		if err := rows.Scan(&u.ID, &title, &description); err != nil {
			return use, err
		}
		if title.Valid {
			u.TitleOverride = &title.String
		}
		if description.Valid {
			u.DescriptionOverride = &description.String
		}
		use.Experience = append(use.Experience, u)
	}
	return use, rows.Err()
}

// itemsOfProfile checks that every id is a row of the given table that belongs to the profile.
func itemsOfProfile(table, profileID string, ids []string) error {
	for _, id := range ids {
		var owner string
		switch err := db.CareerDB.QueryRow(`SELECT profile_id FROM `+table+` WHERE id = ?`, id).Scan(&owner); {
		case err == sql.ErrNoRows:
			return ErrUnknownItem
		case err != nil:
			return err
		}
		if owner != profileID {
			return ErrOtherProfile
		}
	}
	return nil
}

func dedupe(ids []string) []string {
	out, seen := []string{}, map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func notInClause(column string, n int) string {
	if n == 0 {
		return ""
	}
	return " AND " + column + " NOT IN (" + strings.TrimSuffix(strings.Repeat("?,", n), ",") + ")"
}

// SetPersonaSkills makes the persona use exactly these library skills (none is allowed). Every id must be a skill of the persona's
// own profile (ErrUnknownItem, ErrOtherProfile) and is checked before anything is written; the change is one transaction.
func SetPersonaSkills(personaID string, skillIDs []string) error {
	profileID, err := personaProfile(personaID)
	if err != nil {
		return err
	}
	skillIDs = dedupe(skillIDs)
	if err := itemsOfProfile("profile_skills", profileID, skillIDs); err != nil {
		return err
	}
	before, err := queryIDs(`SELECT profile_skill_id FROM persona_skills WHERE persona_id = ?`, personaID)
	if err != nil {
		return err
	}
	tx, err := db.CareerDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	args := []any{personaID}
	for _, id := range skillIDs {
		args = append(args, id)
	}
	if _, err := tx.Exec(`DELETE FROM persona_skills WHERE persona_id = ?`+notInClause("profile_skill_id", len(skillIDs)), args...); err != nil {
		return err
	}
	for _, id := range skillIDs {
		if _, err := tx.Exec(`INSERT INTO persona_skills (persona_id, profile_skill_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, personaID, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	was, now := toSet(before), toSet(skillIDs)
	if added(was, now) {
		publishChanged(personaID, persona.ChangeSkillAdded)
	}
	if added(now, was) {
		publishChanged(personaID, persona.ChangeSkillRemoved)
	}
	return nil
}

func toSet(ids []string) map[string]bool {
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// added reports whether b has an element that a does not.
func added(a, b map[string]bool) bool {
	for id := range b {
		if !a[id] {
			return true
		}
	}
	return false
}

// SetPersonaExperience makes the persona use exactly these library experiences, with these overrides. Every id must be an experience
// of the persona's own profile, an override of the title may not be empty, all checked before anything is written; one transaction.
func SetPersonaExperience(personaID string, uses []ExperienceUse) error {
	profileID, err := personaProfile(personaID)
	if err != nil {
		return err
	}
	ids, seen, clean := []string{}, map[string]bool{}, []ExperienceUse{}
	for _, u := range uses {
		u.ID = strings.TrimSpace(u.ID)
		if u.ID == "" || seen[u.ID] {
			continue
		}
		seen[u.ID] = true
		if u.TitleOverride != nil {
			t := strings.TrimSpace(*u.TitleOverride)
			if t == "" {
				return ErrTitleOverrideEmpty
			}
			u.TitleOverride = &t
		}
		ids = append(ids, u.ID)
		clean = append(clean, u)
	}
	if err := itemsOfProfile("profile_experience", profileID, ids); err != nil {
		return err
	}
	before, err := GetPersonaUse(personaID)
	if err != nil {
		return err
	}
	tx, err := db.CareerDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	args := []any{personaID}
	for _, id := range ids {
		args = append(args, id)
	}
	if _, err := tx.Exec(`DELETE FROM persona_experience WHERE persona_id = ?`+notInClause("profile_experience_id", len(ids)), args...); err != nil {
		return err
	}
	for _, u := range clean {
		if _, err := tx.Exec(
			`INSERT INTO persona_experience (persona_id, profile_experience_id, title_override, description_override) VALUES (?, ?, ?, ?)
			 ON CONFLICT(persona_id, profile_experience_id) DO UPDATE SET title_override = excluded.title_override, description_override = excluded.description_override`,
			personaID, u.ID, u.TitleOverride, u.DescriptionOverride,
		); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	was := map[string]ExperienceUse{}
	for _, u := range before.Experience {
		was[u.ID] = u
	}
	now := map[string]ExperienceUse{}
	for _, u := range clean {
		now[u.ID] = u
	}
	grew, shrank, edited := false, false, false
	for id, u := range now {
		old, ok := was[id]
		switch {
		case !ok:
			grew = true
		case !sameOverride(old.TitleOverride, u.TitleOverride) || !sameOverride(old.DescriptionOverride, u.DescriptionOverride):
			edited = true
		}
	}
	for id := range was {
		if _, ok := now[id]; !ok {
			shrank = true
		}
	}
	if grew {
		publishChanged(personaID, persona.ChangeExperienceAdded)
	}
	if shrank {
		publishChanged(personaID, persona.ChangeExperienceRemoved)
	}
	if edited {
		publishChanged(personaID, persona.ChangeExperienceUpdated)
	}
	return nil
}

func sameOverride(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// --- single items and the library of a persona's profile ---

// SkillByID is one library skill with its used-by count, or ErrUnknownItem.
func SkillByID(id string) (Skill, error) {
	var profileID string
	switch err := db.CareerDB.QueryRow(`SELECT profile_id FROM profile_skills WHERE id = ?`, id).Scan(&profileID); {
	case err == sql.ErrNoRows:
		return Skill{}, ErrUnknownItem
	case err != nil:
		return Skill{}, err
	}
	all, err := ListSkills(profileID)
	if err != nil {
		return Skill{}, err
	}
	for _, s := range all {
		if s.ID == id {
			return s, nil
		}
	}
	return Skill{}, ErrUnknownItem
}

// ExperienceByID is one library experience with its used-by count, or ErrUnknownItem.
func ExperienceByID(id string) (Experience, error) {
	var profileID string
	switch err := db.CareerDB.QueryRow(`SELECT profile_id FROM profile_experience WHERE id = ?`, id).Scan(&profileID); {
	case err == sql.ErrNoRows:
		return Experience{}, ErrUnknownItem
	case err != nil:
		return Experience{}, err
	}
	all, err := ListExperience(profileID)
	if err != nil {
		return Experience{}, err
	}
	for _, e := range all {
		if e.ID == id {
			return e, nil
		}
	}
	return Experience{}, ErrUnknownItem
}

// SkillsOfPersonasProfile is the library a persona can choose from (its own profile's), or ErrUnknownPersona.
func SkillsOfPersonasProfile(personaID string) ([]Skill, error) {
	profileID, err := personaProfile(personaID)
	if err != nil {
		return nil, err
	}
	return ListSkills(profileID)
}

// ExperienceOfPersonasProfile is the experience library a persona can choose from, or ErrUnknownPersona.
func ExperienceOfPersonasProfile(personaID string) ([]Experience, error) {
	profileID, err := personaProfile(personaID)
	if err != nil {
		return nil, err
	}
	return ListExperience(profileID)
}
