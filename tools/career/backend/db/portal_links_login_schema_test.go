package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func loginColumns(t *testing.T, d *sql.DB) (requires, loginURL bool) {
	t.Helper()
	_, requires, err := tableHasColumn(d, "portal_links", "requires_login")
	if err != nil {
		t.Fatal(err)
	}
	_, loginURL, err = tableHasColumn(d, "portal_links", "login_url")
	if err != nil {
		t.Fatal(err)
	}
	return requires, loginURL
}

// A fresh install gets the two login settings from the base schema, "no login needed" and no login page by default, and a restart
// changes nothing.
func TestPortalLinksLoginSchema_FreshInstall(t *testing.T) {
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

	if r, u := loginColumns(t, JobsDB); !r || !u {
		t.Fatalf("portal_links lacks the login columns (requires_login=%v login_url=%v)", r, u)
	}
	if _, err := JobsDB.Exec(`INSERT INTO portals (id, name) VALUES ('p1', 'P')`); err != nil {
		t.Fatal(err)
	}
	if _, err := JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://x')`); err != nil {
		t.Fatal(err)
	}
	var requires int
	var loginURL string
	if err := JobsDB.QueryRow(`SELECT requires_login, login_url FROM portal_links WHERE id = 'l1'`).Scan(&requires, &loginURL); err != nil {
		t.Fatal(err)
	}
	if requires != 0 || loginURL != "" {
		t.Fatalf("defaults: requires_login=%d login_url=%q, want 0 and empty", requires, loginURL)
	}
}

// An install from before plan 182 keeps its links, which get "no login needed", and the migration can run again without harm.
func TestMigratePortalLinksLogin_ExistingInstall(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, q := range []string{
		`CREATE TABLE portals (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE portal_links (id TEXT PRIMARY KEY, portal_id TEXT NOT NULL, url TEXT NOT NULL, title TEXT)`,
		`INSERT INTO portals (id, name) VALUES ('p1', 'Old')`,
		`INSERT INTO portal_links (id, portal_id, url, title) VALUES ('l1', 'p1', 'https://old.example', 'Old link')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if r, u := loginColumns(t, d); r || u {
		t.Fatal("the old table already has the columns")
	}
	for i := 0; i < 2; i++ { // twice: idempotent
		if err := migratePortalLinksLogin(d); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	if r, u := loginColumns(t, d); !r || !u {
		t.Fatalf("columns missing after the migration (%v %v)", r, u)
	}
	var url, title, loginURL string
	var requires int
	if err := d.QueryRow(`SELECT url, title, requires_login, login_url FROM portal_links WHERE id = 'l1'`).Scan(&url, &title, &requires, &loginURL); err != nil {
		t.Fatal(err)
	}
	if url != "https://old.example" || title != "Old link" || requires != 0 || loginURL != "" {
		t.Fatalf("the existing link changed: %q %q %d %q", url, title, requires, loginURL)
	}
}

// A database that has no portal_links table at all is left alone (a fresh install creates it from the base schema instead).
func TestMigratePortalLinksLogin_WithoutTheTableIsANoOp(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := migratePortalLinksLogin(d); err != nil {
		t.Fatal(err)
	}
	if exists, _, err := tableHasColumn(d, "portal_links", "requires_login"); err != nil || exists {
		t.Fatalf("the migration created the table: exists=%v err=%v", exists, err)
	}
}

// A database that already has one of the two columns gets the other one.
func TestMigratePortalLinksLogin_AddsAMissingColumn(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, q := range []string{
		`CREATE TABLE portal_links (id TEXT PRIMARY KEY, portal_id TEXT NOT NULL, url TEXT NOT NULL, requires_login INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO portal_links (id, portal_id, url, requires_login) VALUES ('l1', 'p1', 'https://x', 1)`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := migratePortalLinksLogin(d); err != nil {
		t.Fatal(err)
	}
	var requires int
	var loginURL string
	if err := d.QueryRow(`SELECT requires_login, login_url FROM portal_links WHERE id = 'l1'`).Scan(&requires, &loginURL); err != nil {
		t.Fatal(err)
	}
	if requires != 1 || loginURL != "" {
		t.Fatalf("got %d %q, want the existing 1 kept and an empty login_url", requires, loginURL)
	}
}

// The start-up path itself: an install that has a jobs.db from before plan 182 gets the columns when the tool starts, and keeps its link.
func TestStartUpGivesAnOldInstallTheLoginColumns(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	old, err := sql.Open("sqlite", filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE portals (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT)`,
		`CREATE TABLE portal_links (id TEXT PRIMARY KEY, portal_id TEXT NOT NULL REFERENCES portals(id) ON DELETE CASCADE, url TEXT NOT NULL, title TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT, UNIQUE(portal_id, url))`,
		`INSERT INTO portals (id, name) VALUES ('p1', 'Old')`,
		`INSERT INTO portal_links (id, portal_id, url, title) VALUES ('l1', 'p1', 'https://old.example', 'Old link')`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	old.Close()

	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()
	if r, u := loginColumns(t, JobsDB); !r || !u {
		t.Fatalf("start-up did not add the login columns (%v %v)", r, u)
	}
	var title, loginURL string
	var requires int
	if err := JobsDB.QueryRow(`SELECT title, requires_login, login_url FROM portal_links WHERE id = 'l1'`).Scan(&title, &requires, &loginURL); err != nil {
		t.Fatal(err)
	}
	if title != "Old link" || requires != 0 || loginURL != "" {
		t.Errorf("the old link changed: %q %d %q", title, requires, loginURL)
	}
}
