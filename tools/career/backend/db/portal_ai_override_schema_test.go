package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func aiColumns(t *testing.T, d *sql.DB, table string) (platform, model bool) {
	t.Helper()
	_, platform, err := tableHasColumn(d, table, "ai_platform_id")
	if err != nil {
		t.Fatal(err)
	}
	_, model, err = tableHasColumn(d, table, "ai_model")
	if err != nil {
		t.Fatal(err)
	}
	return platform, model
}

// A fresh install gets the override columns from the base schema, empty by default,
// and a restart changes nothing.
func TestPortalsAiOverrideSchema_FreshInstall(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	JobsDB.Close()
	CareerDB.Close()
	if err := InitDatabases(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()

	for _, table := range []string{"portals", "portal_links"} {
		if p, m := aiColumns(t, JobsDB, table); !p || !m {
			t.Fatalf("%s lacks the override columns (platform=%v model=%v)", table, p, m)
		}
	}
	if _, err := JobsDB.Exec(`INSERT INTO portals (id, name) VALUES ('p1', 'P')`); err != nil {
		t.Fatal(err)
	}
	if _, err := JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://x')`); err != nil {
		t.Fatal(err)
	}
	var pp, pm, lp, lm string
	if err := JobsDB.QueryRow(`SELECT ai_platform_id, ai_model FROM portals WHERE id = 'p1'`).Scan(&pp, &pm); err != nil {
		t.Fatal(err)
	}
	if err := JobsDB.QueryRow(`SELECT ai_platform_id, ai_model FROM portal_links WHERE id = 'l1'`).Scan(&lp, &lm); err != nil {
		t.Fatal(err)
	}
	if pp != "" || pm != "" || lp != "" || lm != "" {
		t.Fatalf("defaults not empty: %q %q %q %q", pp, pm, lp, lm)
	}
}

// An install from before step 102 keeps its rows, which get "no override", and the
// migration can run again without harm.
func TestMigratePortalsAiOverride_ExistingInstall(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, q := range []string{
		`CREATE TABLE portals (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT)`,
		`CREATE TABLE portal_links (id TEXT PRIMARY KEY, portal_id TEXT NOT NULL, url TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT)`,
		`INSERT INTO portals (id, name) VALUES ('p1', 'Old')`,
		`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://old.example')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if p, m := aiColumns(t, d, "portals"); p || m {
		t.Fatal("the old portals table already has the columns")
	}

	for i := 0; i < 2; i++ { // twice: idempotent
		if err := migratePortalsAiOverride(d); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	for _, table := range []string{"portals", "portal_links"} {
		if p, m := aiColumns(t, d, table); !p || !m {
			t.Fatalf("%s lacks the columns after migration", table)
		}
	}
	var name, pp, pm, lp, lm string
	if err := d.QueryRow(`SELECT name, ai_platform_id, ai_model FROM portals WHERE id = 'p1'`).Scan(&name, &pp, &pm); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT ai_platform_id, ai_model FROM portal_links WHERE id = 'l1'`).Scan(&lp, &lm); err != nil {
		t.Fatal(err)
	}
	if name != "Old" || pp != "" || pm != "" || lp != "" || lm != "" {
		t.Fatalf("existing rows changed: %q %q %q %q %q", name, pp, pm, lp, lm)
	}
}

// With no table at all (a brand-new database that has not created them yet) it is a no-op.
func TestMigratePortalsAiOverride_NoTables(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := migratePortalsAiOverride(d); err != nil {
		t.Fatal(err)
	}
}
