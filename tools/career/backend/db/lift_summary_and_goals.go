package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// liftSummaryAndGoalsName is the name of the one-time migration in career_migrations.
const liftSummaryAndGoalsName = "lift_summary_and_goals_v1"

// migratePersonaDetailsOverrides adds the nullable override columns (plan 174, step 1) to an already-installed persona_details table:
// NULL follows the profile's value, any value (also ” or 0) is the persona's own. A no-op on a fresh install (the base schema has them).
func migratePersonaDetailsOverrides(db *sql.DB) error {
	cols := []struct{ name, def string }{
		{"summary_override", "TEXT"},
		{"desired_titles_override", "TEXT"},
		{"desired_locations_override", "TEXT"},
		{"min_salary_override", "INTEGER"},
		{"remote_preference_override", "TEXT CHECK (remote_preference_override IN ('', 'onsite', 'open', 'remote_only'))"},
	}
	for _, c := range cols {
		exists, hasColumn, err := tableHasColumn(db, "persona_details", c.name)
		if err != nil {
			return err
		}
		if !exists || hasColumn {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE persona_details ADD COLUMN ` + c.name + ` ` + c.def); err != nil {
			return err
		}
	}
	return nil
}

// migrateLiftSummaryAndGoals lifts the summary and the goals of the personas to their profile (plan 174, step 1), once:
//
//   - per profile, each of summary, desired titles, desired locations, minimum salary and remote preference becomes the first
//     non-empty value (a minimum salary above 0, a remote preference other than ”) found by persona age, oldest first;
//   - per persona and field, the override stays NULL when the persona's old value equals the profile's (trimmed), and is set to the
//     old value otherwise (also an empty one, which is an override to empty), so every persona shows exactly what it showed before;
//   - the old columns are not touched (nothing is deleted); a persona without a details row has nothing to keep and follows the profile.
//
// A check that every persona's effective values equal its old values runs before the commit; a difference rolls everything back and
// stops the start. It runs once (the row in career_migrations); on a fresh install it only records itself.
func migrateLiftSummaryAndGoals(db *sql.DB) error {
	var done int
	if err := db.QueryRow(`SELECT COUNT(*) FROM career_migrations WHERE name = ?`, liftSummaryAndGoalsName).Scan(&done); err != nil {
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
	if err := liftSummaryAndGoals(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO career_migrations (name) VALUES (?)`, liftSummaryAndGoalsName); err != nil {
		return err
	}
	return tx.Commit()
}

type liftDetails struct {
	personaID, profileID               string
	summary, titles, locations, remote string
	salary                             int64
}

type profileValues struct {
	summary, titles, locations, remote string
	salary                             int64
}

func liftSummaryAndGoals(tx *sql.Tx) error {
	// 1. read everything first (one connection): the personas that have a details row, oldest first
	rows, err := tx.Query(
		`SELECT p.id, p.profile_id, d.summary, d.desired_titles, d.desired_locations, COALESCE(d.min_salary, 0), d.remote_preference
		 FROM personas p JOIN persona_details d ON d.persona_id = p.id
		 ORDER BY p.created_at ASC, p.id ASC`)
	if err != nil {
		return err
	}
	var all []liftDetails
	for rows.Next() {
		var d liftDetails
		if err := rows.Scan(&d.personaID, &d.profileID, &d.summary, &d.titles, &d.locations, &d.salary, &d.remote); err != nil {
			rows.Close()
			return err
		}
		all = append(all, d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// 2. the profile's values: the first non-empty one, field by field, oldest persona first
	profiles := map[string]*profileValues{}
	var order []string
	for _, d := range all {
		pv := profiles[d.profileID]
		if pv == nil {
			pv = &profileValues{}
			profiles[d.profileID] = pv
			order = append(order, d.profileID)
		}
		if pv.summary == "" {
			pv.summary = strings.TrimSpace(d.summary)
		}
		if pv.titles == "" {
			pv.titles = strings.TrimSpace(d.titles)
		}
		if pv.locations == "" {
			pv.locations = strings.TrimSpace(d.locations)
		}
		if pv.salary == 0 && d.salary > 0 {
			pv.salary = d.salary
		}
		if pv.remote == "" {
			pv.remote = d.remote
		}
	}
	for _, profileID := range order {
		pv := profiles[profileID]
		var salary any
		if pv.salary > 0 {
			salary = pv.salary
		}
		if _, err := tx.Exec(
			`INSERT INTO profile_details (profile_id, summary, desired_titles, desired_locations, min_salary, remote_preference, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, datetime('now'))`,
			profileID, pv.summary, pv.titles, pv.locations, salary, pv.remote,
		); err != nil {
			return err
		}
	}

	// 3. the persona's own values: only where they differ from the profile's
	for _, d := range all {
		pv := profiles[d.profileID]
		var summary, titles, locations, salary, remote any
		if strings.TrimSpace(d.summary) != pv.summary {
			summary = d.summary
		}
		if strings.TrimSpace(d.titles) != pv.titles {
			titles = d.titles
		}
		if strings.TrimSpace(d.locations) != pv.locations {
			locations = d.locations
		}
		if d.salary != pv.salary {
			salary = d.salary
		}
		if d.remote != pv.remote {
			remote = d.remote
		}
		if _, err := tx.Exec(
			`UPDATE persona_details SET summary_override = ?, desired_titles_override = ?, desired_locations_override = ?,
			   min_salary_override = ?, remote_preference_override = ? WHERE persona_id = ?`,
			summary, titles, locations, salary, remote, d.personaID,
		); err != nil {
			return err
		}
	}

	// 4. verify, before the commit: every persona's effective values equal its old values
	vr, err := tx.Query(
		`SELECT d.persona_id, d.summary, d.desired_titles, d.desired_locations, COALESCE(d.min_salary, 0), d.remote_preference,
		        COALESCE(d.summary_override, pd.summary), COALESCE(d.desired_titles_override, pd.desired_titles),
		        COALESCE(d.desired_locations_override, pd.desired_locations), COALESCE(d.min_salary_override, pd.min_salary, 0),
		        COALESCE(d.remote_preference_override, pd.remote_preference)
		 FROM persona_details d JOIN personas p ON p.id = d.persona_id JOIN profile_details pd ON pd.profile_id = p.profile_id`)
	if err != nil {
		return err
	}
	defer vr.Close()
	checked := 0
	for vr.Next() {
		var id, s0, t0, l0, r0, s1, t1, l1, r1 string
		var m0, m1 int64
		if err := vr.Scan(&id, &s0, &t0, &l0, &m0, &r0, &s1, &t1, &l1, &m1, &r1); err != nil {
			return err
		}
		checked++
		if strings.TrimSpace(s0) != strings.TrimSpace(s1) || strings.TrimSpace(t0) != strings.TrimSpace(t1) ||
			strings.TrimSpace(l0) != strings.TrimSpace(l1) || m0 != m1 || r0 != r1 {
			return fmt.Errorf("lifting the summary and goals: persona %s would show other values than before", id)
		}
	}
	if err := vr.Err(); err != nil {
		return err
	}
	if checked != len(all) {
		return fmt.Errorf("lifting the summary and goals: checked %d personas, expected %d", checked, len(all))
	}
	return nil
}
