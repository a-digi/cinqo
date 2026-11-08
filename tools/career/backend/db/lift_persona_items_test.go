package db

import "testing"

func careerInt(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := CareerDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func careerExec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := CareerDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func closeBoth() {
	if JobsDB != nil {
		JobsDB.Close()
	}
	if CareerDB != nil {
		CareerDB.Close()
	}
}

// A career.db from before plan 173: skills and experience belong to the personas. The shared data of the tests below:
//
//	profile P1: persona A (oldest), persona B, persona D (empty); profile P2: persona C.
func seedPersonaItems(t *testing.T) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO profiles (id, first_name) VALUES ('P1', 'One'), ('P2', 'Two')`,
		`INSERT INTO personas (id, profile_id, name, created_at) VALUES
		   ('A', 'P1', 'A', '2026-01-01 10:00:00'), ('B', 'P1', 'B', '2026-01-02 10:00:00'),
		   ('D', 'P1', 'D', '2026-01-03 10:00:00'), ('C', 'P2', 'C', '2026-01-04 10:00:00')`,
		`INSERT INTO career_skills (id, persona_id, skill) VALUES
		   ('s1', 'A', 'Go'), ('s2', 'A', 'Python'), ('s3', 'A', 'go'),
		   ('s4', 'B', 'Go '), ('s5', 'B', 'SQL'), ('s6', 'C', 'Go')`,
		`INSERT INTO career_experience (id, persona_id, company, title, start_date, end_date, description, created_at) VALUES
		   ('e1', 'A', 'Acme', 'Dev', '2020', '2022', 'built X', '2026-01-01 11:00:00'),
		   ('e2', 'A', 'Acme', 'Dev', '2020', '2022', 'second entry', '2026-01-01 12:00:00'),
		   ('e3', 'B', 'acme', 'dev', '2020', '2022', 'different', '2026-01-02 11:00:00'),
		   ('e4', 'B', 'Beta', 'Lead', NULL, NULL, '', '2026-01-02 12:00:00'),
		   ('e5', 'C', 'Acme', 'Dev', '2020', '2022', 'built X', '2026-01-04 11:00:00')`,
	} {
		careerExec(t, q)
	}
}

func startFresh(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
}

func TestLiftMovesPersonaItemsToTheProfileAndKeepsEveryPersonaAsItWas(t *testing.T) {
	startFresh(t)
	defer closeBoth()
	seedPersonaItems(t)
	careerExec(t, `DELETE FROM career_migrations`) // the state of an install from before the migration

	if err := migrateLiftPersonaItems(CareerDB); err != nil {
		t.Fatal(err)
	}
	// the library: P1 has Go, Python, SQL (the repeats are one), P2 has Go
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_skills WHERE profile_id = 'P1'`); n != 3 {
		t.Errorf("P1 skills = %d, want 3", n)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_skills WHERE profile_id = 'P2'`); n != 1 {
		t.Errorf("P2 skills = %d, want 1", n)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_skills WHERE profile_id = 'P1' AND skill = 'Go'`); n != 1 {
		t.Error("the first spelling (Go) must be the library's")
	}
	// the persona links
	for persona, want := range map[string]int{"A": 2, "B": 2, "C": 1, "D": 0} {
		if n := careerInt(t, `SELECT COUNT(*) FROM persona_skills WHERE persona_id = ?`, persona); n != want {
			t.Errorf("persona %s uses %d skills, want %d", persona, n, want)
		}
	}
	// a persona only links items of its own profile
	if n := careerInt(t, `SELECT COUNT(*) FROM persona_skills ps JOIN profile_skills k ON k.id = ps.profile_skill_id JOIN personas p ON p.id = ps.persona_id WHERE k.profile_id <> p.profile_id`); n != 0 {
		t.Errorf("%d links cross profiles", n)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM career_skills`); n != 0 {
		t.Errorf("every skill was lifted, %d are left", n)
	}
	// experience: P1 library has Acme/Dev and Beta/Lead; P2 has its own Acme/Dev
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_experience WHERE profile_id = 'P1'`); n != 2 {
		t.Errorf("P1 experiences = %d, want 2", n)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_experience WHERE profile_id = 'P2'`); n != 1 {
		t.Errorf("P2 experiences = %d, want 1", n)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_experience WHERE profile_id = 'P1' AND company = 'Acme' AND title = 'Dev' AND description = 'built X' AND created_at = '2026-01-01 11:00:00'`); n != 1 {
		t.Error("the oldest persona's entry must be the library item, with its text and time")
	}
	// B shows its own description and title for the same entry: they are its overrides
	if n := careerInt(t, `SELECT COUNT(*) FROM persona_experience WHERE persona_id = 'B' AND description_override = 'different' AND title_override = 'dev'`); n != 1 {
		t.Error("B's differing description and title must be its overrides")
	}
	// A and C follow the library (no override); B's second entry (Beta/Lead, empty description) differs from nothing
	if n := careerInt(t, `SELECT COUNT(*) FROM persona_experience WHERE persona_id IN ('A', 'C') AND (title_override IS NOT NULL OR description_override IS NOT NULL)`); n != 0 {
		t.Error("A and C must follow the library")
	}
	// A's second identical entry is not lifted: it stays A's own, so nothing is lost
	if n := careerInt(t, `SELECT COUNT(*) FROM career_experience`); n != 1 {
		t.Errorf("career_experience rows left = %d, want 1 (A's second entry)", n)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM career_experience WHERE id = 'e2' AND persona_id = 'A'`); n != 1 {
		t.Error("the entry that stays must be e2 of A")
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM career_migrations WHERE name = ?`, liftPersonaItemsName); n != 1 {
		t.Error("the migration must record itself")
	}
}

func TestLiftRunsOnceAndNeverLiftsWhatIsAddedLater(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	seedPersonaItems(t)
	careerExec(t, `DELETE FROM career_migrations`)
	closeBoth()
	if err := InitDatabases(); err != nil { // the start that lifts
		t.Fatal(err)
	}
	libSkills := careerInt(t, `SELECT COUNT(*) FROM profile_skills`)
	// a person adds a persona-own custom skill and experience afterwards
	careerExec(t, `INSERT INTO career_skills (id, persona_id, skill) VALUES ('new', 'D', 'Rust')`)
	careerExec(t, `INSERT INTO career_experience (id, persona_id, company, title) VALUES ('newe', 'D', 'Own', 'Role')`)
	closeBoth()
	if err := InitDatabases(); err != nil { // the second start must change nothing
		t.Fatal(err)
	}
	defer closeBoth()
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_skills`); n != libSkills {
		t.Errorf("a later start lifted a custom skill: library %d, was %d", n, libSkills)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM career_skills WHERE id = 'new'`); n != 1 {
		t.Error("the custom skill must stay the persona's own")
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM career_experience WHERE id IN ('newe', 'e2')`); n != 2 {
		t.Error("custom experiences must stay")
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM career_migrations WHERE name = ?`, liftPersonaItemsName); n != 1 {
		t.Error("exactly one record of the migration")
	}
}

func TestLiftOnAFreshInstallOnlyRecordsItself(t *testing.T) {
	startFresh(t)
	defer closeBoth()
	if n := careerInt(t, `SELECT COUNT(*) FROM career_migrations WHERE name = ?`, liftPersonaItemsName); n != 1 {
		t.Error("a fresh install must record the migration")
	}
	for _, table := range []string{"profile_skills", "profile_experience", "persona_skills", "persona_experience"} {
		if n := careerInt(t, `SELECT COUNT(*) FROM `+table); n != 0 {
			t.Errorf("%s has %d rows on a fresh install", table, n)
		}
	}
}

func TestAFailedLiftLeavesTheOldRowsAndDoesNotRecordItself(t *testing.T) {
	startFresh(t)
	defer closeBoth()
	seedPersonaItems(t)
	careerExec(t, `DELETE FROM career_migrations`)
	careerExec(t, `DROP TABLE persona_skills`) // the copy cannot finish
	if err := migrateLiftPersonaItems(CareerDB); err == nil {
		t.Fatal("expected the lift to fail")
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM career_skills`); n != 6 {
		t.Errorf("a failed lift changed career_skills: %d rows, want 6", n)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM career_experience`); n != 5 {
		t.Errorf("a failed lift changed career_experience: %d rows, want 5", n)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_skills`); n != 0 {
		t.Errorf("a failed lift left %d library skills", n)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM career_migrations WHERE name = ?`, liftPersonaItemsName); n != 0 {
		t.Error("a failed lift must not record itself")
	}
}

// The check before the delete: if what a persona would show differs from what it showed, nothing is changed.
func TestLiftRefusesWhenAPersonaWouldShowSomethingElse(t *testing.T) {
	startFresh(t)
	defer closeBoth()
	seedPersonaItems(t)
	careerExec(t, `DELETE FROM career_migrations`)
	// a trigger that drops one link as it is written: the persona would lose a skill
	careerExec(t, `CREATE TRIGGER lose_link AFTER INSERT ON persona_skills WHEN NEW.persona_id = 'B' BEGIN DELETE FROM persona_skills WHERE persona_id = 'B'; END`)
	err := migrateLiftPersonaItems(CareerDB)
	if err == nil {
		t.Fatal("the verification must refuse a lift that loses a skill")
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM career_skills`); n != 6 {
		t.Errorf("a refused lift changed career_skills: %d rows", n)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_skills`); n != 0 {
		t.Errorf("a refused lift left %d library skills", n)
	}
}

// A database without any rows of the old kind (and the old tables only) still starts.
func TestLiftIgnoresAPersonaWithNothing(t *testing.T) {
	startFresh(t)
	defer closeBoth()
	careerExec(t, `INSERT INTO profiles (id) VALUES ('P')`)
	careerExec(t, `INSERT INTO personas (id, profile_id, name) VALUES ('X', 'P', 'X')`)
	careerExec(t, `DELETE FROM career_migrations`)
	if err := migrateLiftPersonaItems(CareerDB); err != nil {
		t.Fatal(err)
	}
}

// Two stints at the same company and title with different dates are two experiences, not one.
func TestLiftKeepsTwoStintsAtOneCompanyApart(t *testing.T) {
	startFresh(t)
	defer closeBoth()
	careerExec(t, `INSERT INTO profiles (id, first_name) VALUES ('P', 'One')`)
	careerExec(t, `INSERT INTO personas (id, profile_id, name, created_at) VALUES ('A', 'P', 'A', '2026-01-01 10:00:00'), ('B', 'P', 'B', '2026-01-02 10:00:00')`)
	careerExec(t, `INSERT INTO career_experience (id, persona_id, company, title, start_date, end_date, description, created_at) VALUES
	  ('e1', 'A', 'Acme', 'Dev', '2018', '2019', 'first stint', '2026-01-01 11:00:00'),
	  ('e2', 'B', 'Acme', 'Dev', '2020', '2022', 'second stint', '2026-01-02 11:00:00')`)
	careerExec(t, `DELETE FROM career_migrations`)
	if err := migrateLiftPersonaItems(CareerDB); err != nil {
		t.Fatal(err)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM profile_experience`); n != 2 {
		t.Errorf("library experiences = %d, want 2 (different dates are different stints)", n)
	}
	if n := careerInt(t, `SELECT COUNT(*) FROM persona_experience WHERE title_override IS NOT NULL OR description_override IS NOT NULL`); n != 0 {
		t.Errorf("no override expected: %d", n)
	}
}
