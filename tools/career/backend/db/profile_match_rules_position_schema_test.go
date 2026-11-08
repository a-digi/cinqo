package db

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
)

// legacyRules is a jobs.db from before step 143: profile_match_rules without position, with rules in no special order.
func legacyRules(t *testing.T, path string) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE profile_match_rules (
			id TEXT PRIMARY KEY, profile_id TEXT NOT NULL, min_percent INTEGER NOT NULL CHECK (min_percent BETWEEN 1 AND 100),
			status_id TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT, UNIQUE (profile_id, min_percent))`,
		`INSERT INTO profile_match_rules (id, profile_id, min_percent, status_id) VALUES
			('a60', 'A', 60, 'fixed-applied'), ('a85', 'A', 85, 'fixed-applied'), ('a70', 'A', 70, 'fixed-applied'),
			('b50', 'B', 50, 'fixed-applied'), ('b99', 'B', 99, 'fixed-applied')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func positions(t *testing.T, database *sql.DB) map[string]int {
	t.Helper()
	rows, err := database.Query(`SELECT id, position FROM profile_match_rules`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var p int
		if err := rows.Scan(&id, &p); err != nil {
			t.Fatal(err)
		}
		out[id] = p
	}
	return out
}

// Existing rules keep acting as before: the highest percentage first, so position 0 is the highest, per profile.
func TestMigrateRulePositionNumbersExistingRulesByDescendingPercentagePerProfile(t *testing.T) {
	database := legacyRules(t, filepath.Join(t.TempDir(), "jobs.db"))
	defer database.Close()
	if err := migrateProfileMatchRulesPosition(database); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"a85": 0, "a70": 1, "a60": 2, "b99": 0, "b50": 1}
	if got := positions(t, database); !reflect.DeepEqual(got, want) {
		t.Errorf("positions = %v, want %v", got, want)
	}
}

// A restart must not renumber what a person has put in order.
func TestMigrateRulePositionRunsOnlyOnceAndNeverRenumbers(t *testing.T) {
	database := legacyRules(t, filepath.Join(t.TempDir(), "jobs.db"))
	defer database.Close()
	if err := migrateProfileMatchRulesPosition(database); err != nil {
		t.Fatal(err)
	}
	// the person drags the lowest percentage to the top
	for id, p := range map[string]int{"a60": 0, "a85": 1, "a70": 2} {
		if _, err := database.Exec(`UPDATE profile_match_rules SET position = ? WHERE id = ?`, p, id); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ { // restarts
		if err := migrateProfileMatchRulesPosition(database); err != nil {
			t.Fatalf("restart %d: %v", i+1, err)
		}
	}
	got := positions(t, database)
	if got["a60"] != 0 || got["a85"] != 1 || got["a70"] != 2 {
		t.Errorf("a restart renumbered the person's order: %v", got)
	}
}

func TestMigrateRulePositionWithoutTheTableIsANoOp(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateProfileMatchRulesPosition(database); err != nil {
		t.Fatal(err)
	}
	if exists, _, _ := tableHasColumn(database, "profile_match_rules", "position"); exists {
		t.Error("the migration must not create the table")
	}
}

func TestMigrateRulePositionOfATableWithoutRulesAddsTheColumn(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TABLE profile_match_rules (id TEXT PRIMARY KEY, profile_id TEXT NOT NULL, min_percent INTEGER NOT NULL, status_id TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := migrateProfileMatchRulesPosition(database); err != nil {
		t.Fatal(err)
	}
	if _, has, _ := tableHasColumn(database, "profile_match_rules", "position"); !has {
		t.Error("the column was not added")
	}
}

// Ten rules in one profile (the limit): all numbered 0..9, highest percentage first.
func TestMigrateRulePositionNumbersTenRules(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TABLE profile_match_rules (id TEXT PRIMARY KEY, profile_id TEXT NOT NULL, min_percent INTEGER NOT NULL, status_id TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 10; i++ {
		if _, err := database.Exec(`INSERT INTO profile_match_rules VALUES (?, 'A', ?, 's')`, "r"+string(rune('a'+i)), i*10); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateProfileMatchRulesPosition(database); err != nil {
		t.Fatal(err)
	}
	rows, _ := database.Query(`SELECT min_percent, position FROM profile_match_rules ORDER BY position`)
	defer rows.Close()
	i := 0
	for rows.Next() {
		var pct, pos int
		_ = rows.Scan(&pct, &pos)
		if pos != i || pct != (10-i)*10 {
			t.Errorf("position %d holds %d%%, want %d%%", pos, pct, (10-i)*10)
		}
		i++
	}
	if i != 10 {
		t.Errorf("rules = %d", i)
	}
}

// A brand-new database has the column from the schema; the migration then does nothing.
func TestAFreshDatabaseHasTheColumnAndTheMigrationLeavesItAlone(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()
	if _, has, err := tableHasColumn(JobsDB, "profile_match_rules", "position"); err != nil || !has {
		t.Fatalf("has=%v err=%v", has, err)
	}
	var dflt sql.NullString
	var notNull int
	rows, _ := JobsDB.Query(`PRAGMA table_info(profile_match_rules)`)
	defer rows.Close()
	for rows.Next() {
		var cid, pk int
		var name, ctype string
		_ = rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk)
		if name == "position" && (notNull != 1 || dflt.String != "0") {
			t.Errorf("position must be NOT NULL DEFAULT 0, got notnull=%d default=%q", notNull, dflt.String)
		}
	}
}

// Through the real start: a jobs.db made by the previous version is upgraded by InitDatabases and its rules keep their meaning.
func TestStartingOnAnOldDatabaseUpgradesItsRules(t *testing.T) {
	dir := t.TempDir()
	legacy := legacyRules(t, filepath.Join(dir, "jobs.db"))
	legacy.Close()
	t.Setenv("TOOL_DB_DIR", dir)
	if err := InitDatabases(); err != nil {
		t.Fatalf("InitDatabases on an old database: %v", err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()
	want := map[string]int{"a85": 0, "a70": 1, "a60": 2, "b99": 0, "b50": 1}
	if got := positions(t, JobsDB); !reflect.DeepEqual(got, want) {
		t.Errorf("positions after the start = %v, want %v", got, want)
	}
	JobsDB.Close()
	CareerDB.Close()
	// a second start changes nothing
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	if got := positions(t, JobsDB); !reflect.DeepEqual(got, want) {
		t.Errorf("positions after a second start = %v, want %v", got, want)
	}
}
