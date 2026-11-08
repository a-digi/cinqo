package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// An install from before plan 197 keeps its links, which follow the Browser's engine and have no block; the migration can run again.
func TestMigratePortalLinksCrawlEngine_ExistingInstall(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, q := range []string{
		`CREATE TABLE portal_links (id TEXT PRIMARY KEY, portal_id TEXT NOT NULL, url TEXT NOT NULL, title TEXT)`,
		`INSERT INTO portal_links (id, portal_id, url, title) VALUES ('l1', 'p1', 'https://old.example', 'Old link')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := migratePortalLinksCrawlEngine(d); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	for _, col := range []string{"crawl_engine", "engine_block_code", "engine_block_at", "engine_hint_dismissed_at"} {
		if _, has, err := tableHasColumn(d, "portal_links", col); err != nil || !has {
			t.Fatalf("column %s missing (%v)", col, err)
		}
	}
	var engine, block string
	var at, dismissed sql.NullString
	if err := d.QueryRow(`SELECT crawl_engine, engine_block_code, engine_block_at, engine_hint_dismissed_at FROM portal_links WHERE id = 'l1'`).Scan(&engine, &block, &at, &dismissed); err != nil {
		t.Fatal(err)
	}
	if engine != "" || block != "" || at.Valid || dismissed.Valid {
		t.Fatalf("the existing link changed: %q %q %v %v", engine, block, at, dismissed)
	}
}

func TestMigratePortalLinksCrawlEngine_WithoutTheTableIsANoOp(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := migratePortalLinksCrawlEngine(d); err != nil {
		t.Fatal(err)
	}
	if exists, _, err := tableHasColumn(d, "portal_links", "crawl_engine"); err != nil || exists {
		t.Fatalf("the migration created the table: %v %v", exists, err)
	}
}

// A fresh install gets the columns from the base schema, with the defaults.
func TestPortalLinksCrawlEngine_FreshInstall(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()
	if _, err := JobsDB.Exec(`INSERT INTO portals (id, name) VALUES ('p1', 'P')`); err != nil {
		t.Fatal(err)
	}
	if _, err := JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://x')`); err != nil {
		t.Fatal(err)
	}
	var engine string
	if err := JobsDB.QueryRow(`SELECT crawl_engine FROM portal_links WHERE id = 'l1'`).Scan(&engine); err != nil || engine != "" {
		t.Fatalf("default engine %q %v", engine, err)
	}
}
