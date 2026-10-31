package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// A career.db from before step 130 has persona_details without remote_preference: the migration adds it, every
// existing row becomes "not set" (empty), a second run changes nothing, and the CHECK refuses a value outside the four.
func TestMigratePersonaDetailsRemotePreferenceKeepsExistingRowsUnset(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, q := range []string{
		`CREATE TABLE persona_details (persona_id TEXT PRIMARY KEY, headline TEXT NOT NULL DEFAULT '', summary TEXT NOT NULL DEFAULT '',
			location TEXT NOT NULL DEFAULT '', desired_titles TEXT NOT NULL DEFAULT '', desired_locations TEXT NOT NULL DEFAULT '',
			min_salary INTEGER, updated_at TEXT)`,
		`INSERT INTO persona_details (persona_id, headline, desired_locations) VALUES ('p1', 'Engineer', 'Hamburg, Remote')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ { // twice: idempotent
		if err := migratePersonaDetailsRemotePreference(database); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}
	var pref, headline, desired string
	if err := database.QueryRow(`SELECT remote_preference, headline, desired_locations FROM persona_details WHERE persona_id = 'p1'`).Scan(&pref, &headline, &desired); err != nil {
		t.Fatal(err)
	}
	if pref != "" || headline != "Engineer" || desired != "Hamburg, Remote" {
		t.Errorf("preference %q, headline %q, desired %q", pref, headline, desired)
	}
	for _, v := range []string{"onsite", "open", "remote_only", ""} {
		if _, err := database.Exec(`UPDATE persona_details SET remote_preference = ? WHERE persona_id = 'p1'`, v); err != nil {
			t.Errorf("%q must be allowed: %v", v, err)
		}
	}
	if _, err := database.Exec(`UPDATE persona_details SET remote_preference = 'remote' WHERE persona_id = 'p1'`); err == nil {
		t.Error("the CHECK must refuse an unknown value")
	}
}

func TestMigratePersonaDetailsRemotePreferenceWithoutTheTableIsANoOp(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migratePersonaDetailsRemotePreference(database); err != nil {
		t.Fatal(err)
	}
}

// A fresh install has the column straight from the schema, and an install rebuilt by the older migrations (whose
// persona_details the base schema leaves alone) gets it from the migration that runs after the base schema.
func TestFreshInstallHasTheRemotePreferenceColumn(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()
	if _, has, err := tableHasColumn(CareerDB, "persona_details", "remote_preference"); err != nil || !has {
		t.Errorf("remote_preference: %v %v", has, err)
	}
}

// An existing career.db whose persona_details has the old shape: the base schema's CREATE TABLE IF NOT EXISTS leaves it
// alone, so the column must come from the migration that follows it, through the real start-up path.
func TestStartUpAddsTheColumnToAnExistingPersonaDetails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	old, err := sql.Open("sqlite", filepath.Join(dir, "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE persona_details (persona_id TEXT PRIMARY KEY, headline TEXT NOT NULL DEFAULT '', summary TEXT NOT NULL DEFAULT '',
			location TEXT NOT NULL DEFAULT '', desired_titles TEXT NOT NULL DEFAULT '', desired_locations TEXT NOT NULL DEFAULT '',
			min_salary INTEGER, updated_at TEXT)`,
		`INSERT INTO persona_details (persona_id, headline) VALUES ('p1', 'Kept')`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()
	for i := 0; i < 2; i++ { // a restart too
		if err := InitDatabases(); err != nil {
			t.Fatalf("start %d: %v", i+1, err)
		}
		var headline, pref string
		if err := CareerDB.QueryRow(`SELECT headline, remote_preference FROM persona_details WHERE persona_id = 'p1'`).Scan(&headline, &pref); err != nil {
			t.Fatalf("start %d: %v", i+1, err)
		}
		if headline != "Kept" || pref != "" {
			t.Errorf("start %d: headline %q preference %q", i+1, headline, pref)
		}
		JobsDB.Close()
		CareerDB.Close()
	}
}
