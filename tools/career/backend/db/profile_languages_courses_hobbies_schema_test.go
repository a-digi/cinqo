package db

import (
	"strings"
	"testing"
)

// The three tables of plan 231 exist, belong to a profile, go with it, back the one-entry-per-name rule, and creating the schema again
// changes nothing.
func TestProfileLanguagesCoursesHobbiesSchema(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()

	if _, err := CareerDB.Exec(`INSERT INTO profiles (id) VALUES ('p1'), ('p2')`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO profile_languages (id, profile_id, language) VALUES ('l1', 'p1', 'German')`,
		`INSERT INTO profile_courses (id, profile_id, title) VALUES ('c1', 'p1', 'Go')`,
		`INSERT INTO profile_hobbies (id, profile_id, name) VALUES ('h1', 'p1', 'Chess')`,
	} {
		if _, err := CareerDB.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	// the defaults
	var level string
	var speak, write int
	if err := CareerDB.QueryRow(`SELECT level, can_speak, can_write FROM profile_languages WHERE id = 'l1'`).Scan(&level, &speak, &write); err != nil || level != "" || speak != 0 || write != 0 {
		t.Errorf("%q %d %d %v", level, speak, write, err)
	}
	// an entry of an unknown profile is refused by the foreign key
	for _, q := range []string{
		`INSERT INTO profile_languages (id, profile_id, language) VALUES ('l9', 'nope', 'X')`,
		`INSERT INTO profile_courses (id, profile_id, title) VALUES ('c9', 'nope', 'X')`,
		`INSERT INTO profile_hobbies (id, profile_id, name) VALUES ('h9', 'nope', 'X')`,
	} {
		if _, err := CareerDB.Exec(q); err == nil {
			t.Errorf("%s: an unknown profile must be refused", q)
		}
	}
	// one language and one hobby per name and profile, ignoring the case of plain letters; the same name for another profile is fine
	for _, q := range []string{
		`INSERT INTO profile_languages (id, profile_id, language) VALUES ('l2', 'p1', 'GERMAN')`,
		`INSERT INTO profile_hobbies (id, profile_id, name) VALUES ('h2', 'p1', 'chess')`,
	} {
		if _, err := CareerDB.Exec(q); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
			t.Errorf("%s: %v, want a unique violation", q, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO profile_languages (id, profile_id, language) VALUES ('l3', 'p2', 'German')`,
		`INSERT INTO profile_hobbies (id, profile_id, name) VALUES ('h3', 'p2', 'Chess')`,
		`INSERT INTO profile_courses (id, profile_id, title) VALUES ('c2', 'p1', 'Go')`, // courses may repeat a title
	} {
		if _, err := CareerDB.Exec(q); err != nil {
			t.Errorf("%s: %v", q, err)
		}
	}
	// the schema can be created again over existing data
	if _, err := CareerDB.Exec(careerSchema); err != nil {
		t.Fatalf("a second run: %v", err)
	}
	// deleting the profile deletes its entries and only those
	if _, err := CareerDB.Exec(`DELETE FROM profiles WHERE id = 'p1'`); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int{"profile_languages": 1, "profile_courses": 0, "profile_hobbies": 1} {
		var n int
		if err := CareerDB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != want {
			t.Errorf("%s: %d rows, want %d (%v)", table, n, want, err)
		}
	}
}
