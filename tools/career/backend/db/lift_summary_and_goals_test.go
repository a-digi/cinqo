package db

import (
	"database/sql"
	"testing"
)

// The state of a career.db from before plan 174: the summary and goals are the personas' own columns.
//
//	profile P1: persona A (oldest), B, D (no details row), E (empty details); profile P2: persona C; profile P3: no persona.
func seedSummaryAndGoals(t *testing.T) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO profiles (id, first_name) VALUES ('P1', 'One'), ('P2', 'Two'), ('P3', 'Three'), ('P4', 'Four')`,
		`INSERT INTO personas (id, profile_id, name, created_at) VALUES
		   ('A', 'P1', 'A', '2026-01-01 10:00:00'), ('B', 'P1', 'B', '2026-01-02 10:00:00'),
		   ('D', 'P1', 'D', '2026-01-03 10:00:00'), ('E', 'P1', 'E', '2026-01-04 10:00:00'),
		   ('C', 'P2', 'C', '2026-01-05 10:00:00'), ('F', 'P1', 'F', '2026-01-06 10:00:00'), ('G', 'P4', 'G', '2026-01-07 10:00:00')`,
		// A: no goals at all, a summary; B: everything, other summary; E: empty; C: its own profile
		`INSERT INTO persona_details (persona_id, headline, summary, location, desired_titles, desired_locations, min_salary, remote_preference) VALUES
		   ('A', 'h', 'Summary A', 'Berlin', '', 'Berlin', NULL, ''),
		   ('B', 'h', 'Summary B', 'Munich', 'Lead Dev', 'Munich', 90000, 'remote_only'),
		   ('E', '', '', '', '', '', NULL, ''),
		   ('C', '', ' Only C ', '', 'CTO', '', 70000, 'open'),
		   ('F', '', 'Summary A', '', 'Lead Dev', '', 50000, 'remote_only'),
		   ('G', '', 'Only G', '', '', '', NULL, '')`,
	} {
		careerExec(t, q)
	}
	careerExec(t, `DELETE FROM career_migrations WHERE name = ?`, liftSummaryAndGoalsName)
}

type effective struct {
	summary, titles, locations, remote string
	salary                             int64
}

func effectiveOf(t *testing.T, persona string) effective {
	t.Helper()
	var e effective
	err := CareerDB.QueryRow(
		`SELECT COALESCE(d.summary_override, pd.summary), COALESCE(d.desired_titles_override, pd.desired_titles),
		        COALESCE(d.desired_locations_override, pd.desired_locations), COALESCE(d.remote_preference_override, pd.remote_preference),
		        COALESCE(d.min_salary_override, pd.min_salary, 0)
		 FROM persona_details d JOIN personas p ON p.id = d.persona_id JOIN profile_details pd ON pd.profile_id = p.profile_id
		 WHERE d.persona_id = ?`, persona).Scan(&e.summary, &e.titles, &e.locations, &e.remote, &e.salary)
	if err != nil {
		t.Fatalf("effective values of %s: %v", persona, err)
	}
	return e
}

func nullStr(t *testing.T, q string, args ...any) sql.NullString {
	t.Helper()
	var s sql.NullString
	if err := CareerDB.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return s
}

func TestLiftSummaryAndGoalsTakesTheOldestNonEmptyValuePerFieldAsTheProfilesAndKeepsWhatDiffers(t *testing.T) {
	startFresh(t)
	defer closeBoth()
	seedSummaryAndGoals(t)
	if err := migrateLiftSummaryAndGoals(CareerDB); err != nil {
		t.Fatal(err)
	}

	// P1: summary from A (oldest non-empty), titles/salary/remote from B (the first non-empty), locations from A
	var summary, titles, locations, remote string
	var salary int64
	if err := CareerDB.QueryRow(`SELECT summary, desired_titles, desired_locations, COALESCE(min_salary, 0), remote_preference FROM profile_details WHERE profile_id = 'P1'`).
		Scan(&summary, &titles, &locations, &salary, &remote); err != nil {
		t.Fatal(err)
	}
	if summary != "Summary A" || titles != "Lead Dev" || locations != "Berlin" || salary != 90000 || remote != "remote_only" {
		t.Errorf("P1 = %q %q %q %d %q", summary, titles, locations, salary, remote)
	}
	// a profile where nobody has a minimum salary has none (NULL, not 0)
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_details WHERE profile_id = 'P4' AND min_salary IS NULL`); n != 1 {
		t.Error("P4 has no minimum salary: it must be NULL")
	}
	// the trimmed text is the profile's
	if s := nullStr(t, `SELECT summary FROM profile_details WHERE profile_id = 'P2'`); s.String != "Only C" {
		t.Errorf("P2 summary = %q, want the trimmed text", s.String)
	}

	// A follows its summary and locations, and overrides the goals it did not have to empty / "no minimum" / "not set"
	if s := nullStr(t, `SELECT summary_override FROM persona_details WHERE persona_id = 'A'`); s.Valid {
		t.Error("A's summary equals the profile's: it must follow")
	}
	if s := nullStr(t, `SELECT desired_locations_override FROM persona_details WHERE persona_id = 'A'`); s.Valid {
		t.Error("A's locations equal the profile's: it must follow")
	}
	if s := nullStr(t, `SELECT desired_titles_override FROM persona_details WHERE persona_id = 'A'`); !s.Valid || s.String != "" {
		t.Errorf("A had no titles: its own value is '' (got %+v)", s)
	}
	if n := careerInt(t, `SELECT min_salary_override FROM persona_details WHERE persona_id = 'A'`); n != 0 {
		t.Errorf("A's own salary = %d, want 0 (no minimum)", n)
	}
	if s := nullStr(t, `SELECT remote_preference_override FROM persona_details WHERE persona_id = 'A'`); !s.Valid || s.String != "" {
		t.Errorf("A's remote preference must be its own '' (got %+v)", s)
	}
	// B differs only in the summary and the locations
	if s := nullStr(t, `SELECT summary_override FROM persona_details WHERE persona_id = 'B'`); s.String != "Summary B" {
		t.Errorf("B's summary override = %+v", s)
	}
	if s := nullStr(t, `SELECT desired_locations_override FROM persona_details WHERE persona_id = 'B'`); s.String != "Munich" {
		t.Errorf("B's locations override = %+v", s)
	}
	for _, col := range []string{"desired_titles_override", "min_salary_override", "remote_preference_override"} {
		if s := nullStr(t, `SELECT `+col+` FROM persona_details WHERE persona_id = 'B'`); s.Valid {
			t.Errorf("B's %s equals the profile's: it must follow", col)
		}
	}

	// every persona shows exactly what it showed before
	want := map[string]effective{
		"A": {"Summary A", "", "Berlin", "", 0},
		"B": {"Summary B", "Lead Dev", "Munich", "remote_only", 90000},
		"E": {"", "", "", "", 0},
		"C": {"Only C", "CTO", "", "open", 70000},
		"F": {"Summary A", "Lead Dev", "", "remote_only", 50000}, // F's own salary: the OLDEST non-empty one (B's) is the profile's
		"G": {"Only G", "", "", "", 0},
	}
	// E's empty values are overrides to empty, because the profile has values
	for id, w := range want {
		if got := effectiveOf(t, id); got != w {
			t.Errorf("persona %s shows %+v, want %+v", id, got, w)
		}
	}
	// the old columns are not touched
	if s := nullStr(t, `SELECT summary FROM persona_details WHERE persona_id = 'B'`); s.String != "Summary B" {
		t.Error("the old columns must stay as they were")
	}
}

func TestLiftSummaryAndGoalsLeavesAPersonaWithoutDetailsAndAProfileWithoutPersonasAlone(t *testing.T) {
	startFresh(t)
	defer closeBoth()
	seedSummaryAndGoals(t)
	if err := migrateLiftSummaryAndGoals(CareerDB); err != nil {
		t.Fatal(err)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM persona_details WHERE persona_id = 'D'`); n != 0 {
		t.Error("the lift must not create a details row for a persona without one")
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_details WHERE profile_id = 'P3'`); n != 0 {
		t.Error("a profile without personas gets no row")
	}
}

func TestLiftSummaryAndGoalsRunsOnceAndOnAFreshInstallOnlyRecordsItself(t *testing.T) {
	startFresh(t)
	defer closeBoth()
	if n := careerInt(t, `SELECT COUNT(*) FROM career_migrations WHERE name = ?`, liftSummaryAndGoalsName); n != 1 {
		t.Fatalf("a fresh start must record the migration, got %d", n)
	}
	seedSummaryAndGoals(t)
	if err := migrateLiftSummaryAndGoals(CareerDB); err != nil {
		t.Fatal(err)
	}
	// a persona edited after the lift: a second start must not lift again (it would overwrite the profile's text)
	careerExec(t, `UPDATE profile_details SET summary = 'Edited' WHERE profile_id = 'P1'`)
	careerExec(t, `UPDATE persona_details SET summary_override = NULL WHERE persona_id = 'B'`)
	if err := migrateLiftSummaryAndGoals(CareerDB); err != nil {
		t.Fatal(err)
	}
	if s := nullStr(t, `SELECT summary FROM profile_details WHERE profile_id = 'P1'`); s.String != "Edited" {
		t.Errorf("a second run changed the profile: %q", s.String)
	}
	if s := nullStr(t, `SELECT summary_override FROM persona_details WHERE persona_id = 'B'`); s.Valid {
		t.Error("a second run changed a persona")
	}
}

func TestOverrideColumnsAreAddedToAnInstalledPersonaDetails(t *testing.T) {
	startFresh(t)
	defer closeBoth()
	// an install from before: a persona_details without the columns
	careerExec(t, `PRAGMA foreign_keys = OFF`)
	careerExec(t, `DROP TABLE persona_details`)
	careerExec(t, `CREATE TABLE persona_details (persona_id TEXT PRIMARY KEY, headline TEXT NOT NULL DEFAULT '', summary TEXT NOT NULL DEFAULT '',
	  location TEXT NOT NULL DEFAULT '', desired_titles TEXT NOT NULL DEFAULT '', desired_locations TEXT NOT NULL DEFAULT '', min_salary INTEGER,
	  remote_preference TEXT NOT NULL DEFAULT '', updated_at TEXT)`)
	for i := 0; i < 2; i++ { // twice: idempotent
		if err := migratePersonaDetailsOverrides(CareerDB); err != nil {
			t.Fatal(err)
		}
	}
	for _, col := range []string{"summary_override", "desired_titles_override", "desired_locations_override", "min_salary_override", "remote_preference_override"} {
		_, has, err := tableHasColumn(CareerDB, "persona_details", col)
		if err != nil || !has {
			t.Errorf("column %s missing (err %v)", col, err)
		}
	}
}

func TestLiftSummaryAndGoalsRefusesAnInvalidRemotePreferenceOverrideAtTheDatabase(t *testing.T) {
	startFresh(t)
	defer closeBoth()
	careerExec(t, `INSERT INTO profiles (id, first_name) VALUES ('P', 'x')`)
	careerExec(t, `INSERT INTO personas (id, profile_id, name) VALUES ('X', 'P', 'x')`)
	if _, err := CareerDB.Exec(`INSERT INTO persona_details (persona_id, remote_preference_override) VALUES ('X', 'sometimes')`); err == nil {
		t.Error("an unknown remote preference override must be refused by the table")
	}
}
