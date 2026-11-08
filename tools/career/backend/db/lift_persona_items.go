package db

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// liftPersonaItemsName is the name of the one-time migration in career_migrations.
const liftPersonaItemsName = "lift_persona_items_v1"

// migrateLiftPersonaItems lifts the skills and experience that belong to personas to their profile (plan 173, step 1), once:
//
//   - every skill is copied to the profile's library, de-duplicated by trimmed, lower-case text (the first spelling wins), and the
//     persona is linked to it (persona_skills);
//   - every experience is copied to the library, de-duplicated by company + title + start + end (trimmed, lower-case), the first
//     persona's (by persona age) text becomes the library item, and every persona that had the same entry is linked to it; a persona
//     whose description or title differs from the library item gets it as its own override, so it shows exactly what it showed before;
//   - a second entry of one persona with the same key is NOT lifted (it stays that persona's own, so nothing is lost);
//   - only the rows that were lifted are deleted from career_skills/career_experience, and only after a check that every persona
//     shows exactly the same set of skills and experiences as before. Anything the check does not accept rolls the whole
//     migration back and stops the start.
//
// It runs once (the row in career_migrations): custom items a person adds later are never lifted by a later start. On a fresh
// install it only records itself.
func migrateLiftPersonaItems(db *sql.DB) error {
	var done int
	if err := db.QueryRow(`SELECT COUNT(*) FROM career_migrations WHERE name = ?`, liftPersonaItemsName).Scan(&done); err != nil {
		return err
	}
	if done > 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := liftPersonaItems(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO career_migrations (name) VALUES (?)`, liftPersonaItemsName); err != nil {
		return err
	}
	return tx.Commit()
}

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

type liftPersona struct{ id, profileID string }

type liftSkill struct{ id, personaID, skill string }

type liftExperience struct {
	id, personaID, company, title string
	start, end, description       sql.NullString
	createdAt                     string
}

func liftPersonaItems(tx *sql.Tx) error {
	// 1. read everything (one pass each, the connection is the only one)
	var personas []liftPersona
	rows, err := tx.Query(`SELECT id, profile_id FROM personas ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p liftPersona
		if err := rows.Scan(&p.id, &p.profileID); err != nil {
			rows.Close()
			return err
		}
		personas = append(personas, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	order := map[string]int{}
	profileOf := map[string]string{}
	for i, p := range personas {
		order[p.id] = i
		profileOf[p.id] = p.profileID
	}

	var skills []liftSkill
	rows, err = tx.Query(`SELECT id, persona_id, skill FROM career_skills`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var s liftSkill
		if err := rows.Scan(&s.id, &s.personaID, &s.skill); err != nil {
			rows.Close()
			return err
		}
		skills = append(skills, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	var exps []liftExperience
	rows, err = tx.Query(`SELECT id, persona_id, company, title, start_date, end_date, description, created_at FROM career_experience`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var e liftExperience
		if err := rows.Scan(&e.id, &e.personaID, &e.company, &e.title, &e.start, &e.end, &e.description, &e.createdAt); err != nil {
			rows.Close()
			return err
		}
		exps = append(exps, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// A row of a persona that no longer exists (never happens with the cascade) is left alone.
	skills = filterKnown(skills, order, func(s liftSkill) string { return s.personaID })
	exps = filterKnown(exps, order, func(e liftExperience) string { return e.personaID })

	// Deterministic order: persona age, then the row's own order.
	sort.SliceStable(skills, func(i, j int) bool {
		a, b := skills[i], skills[j]
		if order[a.personaID] != order[b.personaID] {
			return order[a.personaID] < order[b.personaID]
		}
		if a.skill != b.skill {
			return a.skill < b.skill
		}
		return a.id < b.id
	})
	sort.SliceStable(exps, func(i, j int) bool {
		a, b := exps[i], exps[j]
		if order[a.personaID] != order[b.personaID] {
			return order[a.personaID] < order[b.personaID]
		}
		if a.createdAt != b.createdAt {
			return a.createdAt < b.createdAt
		}
		return a.id < b.id
	})

	// 2. skills
	librarySkill := map[string]string{} // profile|key -> profile_skills.id
	linkedSkill := map[string]bool{}    // persona|profile skill id
	liftedSkillRows := []string{}
	for _, s := range skills {
		profileID := profileOf[s.personaID]
		key := profileID + "|" + norm(s.skill)
		id, ok := librarySkill[key]
		if !ok {
			id = uuid.NewString()
			if _, err := tx.Exec(`INSERT INTO profile_skills (id, profile_id, skill) VALUES (?, ?, ?)`, id, profileID, strings.TrimSpace(s.skill)); err != nil {
				return err
			}
			librarySkill[key] = id
		}
		link := s.personaID + "|" + id
		if !linkedSkill[link] {
			if _, err := tx.Exec(`INSERT INTO persona_skills (persona_id, profile_skill_id) VALUES (?, ?)`, s.personaID, id); err != nil {
				return err
			}
			linkedSkill[link] = true
		}
		// a repeat of the same skill in one persona (another spelling) is the same skill: nothing is lost by dropping the row
		liftedSkillRows = append(liftedSkillRows, s.id)
	}

	// 3. experience
	type libraryExp struct {
		id, title, description string
	}
	libraryByKey := map[string]libraryExp{}
	linkedExp := map[string]bool{}
	liftedExpRows := []string{}
	for _, e := range exps {
		profileID := profileOf[e.personaID]
		key := strings.Join([]string{profileID, norm(e.company), norm(e.title), strings.TrimSpace(e.start.String), strings.TrimSpace(e.end.String)}, "|")
		lib, ok := libraryByKey[key]
		if !ok {
			lib = libraryExp{id: uuid.NewString(), title: e.title, description: e.description.String}
			if _, err := tx.Exec(
				`INSERT INTO profile_experience (id, profile_id, company, title, start_date, end_date, description, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				lib.id, profileID, e.company, e.title, e.start, e.end, e.description, e.createdAt,
			); err != nil {
				return err
			}
			libraryByKey[key] = lib
		}
		link := e.personaID + "|" + lib.id
		if linkedExp[link] {
			continue // a second identical entry of one persona stays that persona's own
		}
		var titleOverride, descriptionOverride sql.NullString
		if e.title != lib.title {
			titleOverride = sql.NullString{String: e.title, Valid: true}
		}
		if strings.TrimSpace(e.description.String) != strings.TrimSpace(lib.description) {
			descriptionOverride = sql.NullString{String: e.description.String, Valid: true}
		}
		if _, err := tx.Exec(
			`INSERT INTO persona_experience (persona_id, profile_experience_id, title_override, description_override) VALUES (?, ?, ?, ?)`,
			e.personaID, lib.id, titleOverride, descriptionOverride,
		); err != nil {
			return err
		}
		linkedExp[link] = true
		liftedExpRows = append(liftedExpRows, e.id)
	}

	// 4. verify, before anything is deleted: every persona shows the same skills and experiences as before
	if err := verifyLift(tx, order, skills, exps, liftedSkillRows, liftedExpRows); err != nil {
		return err
	}

	// 5. clear the lifted rows
	for _, id := range liftedSkillRows {
		if _, err := tx.Exec(`DELETE FROM career_skills WHERE id = ?`, id); err != nil {
			return err
		}
	}
	for _, id := range liftedExpRows {
		if _, err := tx.Exec(`DELETE FROM career_experience WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func filterKnown[T any](in []T, known map[string]int, personaOf func(T) string) []T {
	out := in[:0]
	for _, v := range in {
		if _, ok := known[personaOf(v)]; ok {
			out = append(out, v)
		}
	}
	return out
}

// verifyLift compares, per persona, what it showed before (its own rows) with what it would show now (the library items it is linked
// to with the overrides applied, plus the rows that were not lifted): the set of normalized skills, and the multiset of experiences
// (company, title, dates, description, normalized). A difference is an error and rolls the migration back.
func verifyLift(tx *sql.Tx, order map[string]int, skills []liftSkill, exps []liftExperience, liftedSkillRows, liftedExpRows []string) error {
	liftedS := map[string]bool{}
	for _, id := range liftedSkillRows {
		liftedS[id] = true
	}
	liftedE := map[string]bool{}
	for _, id := range liftedExpRows {
		liftedE[id] = true
	}

	beforeSkills := map[string]map[string]bool{}
	for _, s := range skills {
		if beforeSkills[s.personaID] == nil {
			beforeSkills[s.personaID] = map[string]bool{}
		}
		beforeSkills[s.personaID][norm(s.skill)] = true
	}
	afterSkills := map[string]map[string]bool{}
	rows, err := tx.Query(`SELECT ps.persona_id, k.skill FROM persona_skills ps JOIN profile_skills k ON k.id = ps.profile_skill_id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p, skill string
		if err := rows.Scan(&p, &skill); err != nil {
			rows.Close()
			return err
		}
		if afterSkills[p] == nil {
			afterSkills[p] = map[string]bool{}
		}
		afterSkills[p][norm(skill)] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, s := range skills {
		if !liftedS[s.id] {
			if afterSkills[s.personaID] == nil {
				afterSkills[s.personaID] = map[string]bool{}
			}
			afterSkills[s.personaID][norm(s.skill)] = true
		}
	}
	for p := range order {
		if len(beforeSkills[p]) != len(afterSkills[p]) {
			return fmt.Errorf("lifting skills: persona %s shows %d skills before and %d after", p, len(beforeSkills[p]), len(afterSkills[p]))
		}
		for k := range beforeSkills[p] {
			if !afterSkills[p][k] {
				return fmt.Errorf("lifting skills: persona %s lost the skill %q", p, k)
			}
		}
	}

	expKey := func(company, title, start, end, description string) string {
		return strings.Join([]string{norm(company), norm(title), strings.TrimSpace(start), strings.TrimSpace(end), strings.TrimSpace(description)}, "|")
	}
	before := map[string]map[string]int{}
	for _, e := range exps {
		if before[e.personaID] == nil {
			before[e.personaID] = map[string]int{}
		}
		before[e.personaID][expKey(e.company, e.title, e.start.String, e.end.String, e.description.String)]++
	}
	after := map[string]map[string]int{}
	rows, err = tx.Query(
		`SELECT pe.persona_id, x.company, COALESCE(pe.title_override, x.title), x.start_date, x.end_date,
		        COALESCE(pe.description_override, x.description)
		 FROM persona_experience pe JOIN profile_experience x ON x.id = pe.profile_experience_id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p, company, title string
		var start, end, description sql.NullString
		if err := rows.Scan(&p, &company, &title, &start, &end, &description); err != nil {
			rows.Close()
			return err
		}
		if after[p] == nil {
			after[p] = map[string]int{}
		}
		after[p][expKey(company, title, start.String, end.String, description.String)]++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, e := range exps {
		if !liftedE[e.id] {
			if after[e.personaID] == nil {
				after[e.personaID] = map[string]int{}
			}
			after[e.personaID][expKey(e.company, e.title, e.start.String, e.end.String, e.description.String)]++
		}
	}
	for p := range order {
		for k, n := range before[p] {
			if after[p][k] != n {
				return fmt.Errorf("lifting experience: persona %s shows %d of %q before and %d after", p, n, k, after[p][k])
			}
		}
		for k, n := range after[p] {
			if before[p][k] != n {
				return fmt.Errorf("lifting experience: persona %s shows %d of %q after and %d before", p, n, k, before[p][k])
			}
		}
	}
	return nil
}
