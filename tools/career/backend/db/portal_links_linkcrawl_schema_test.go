package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func linkCrawlColumns(t *testing.T, d *sql.DB) (method, rules bool) {
	t.Helper()
	_, method, err := tableHasColumn(d, "portal_links", "crawl_method")
	if err != nil {
		t.Fatal(err)
	}
	_, rules, err = tableHasColumn(d, "portal_links", "link_rules")
	if err != nil {
		t.Fatal(err)
	}
	return method, rules
}

// A fresh install gets the two columns from the base schema: method "instructions", no rule, and a restart changes nothing.
func TestPortalLinksLinkCrawlSchema_FreshInstall(t *testing.T) {
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

	if m, r := linkCrawlColumns(t, JobsDB); !m || !r {
		t.Fatalf("portal_links lacks the link crawler columns (%v %v)", m, r)
	}
	for _, q := range []string{
		`INSERT INTO portals (id, name) VALUES ('p1', 'P')`,
		`INSERT INTO portal_links (id, portal_id, url) VALUES ('l1', 'p1', 'https://x.example/jobs')`,
	} {
		if _, err := JobsDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	var method string
	var rules sql.NullString
	if err := JobsDB.QueryRow(`SELECT crawl_method, link_rules FROM portal_links WHERE id = 'l1'`).Scan(&method, &rules); err != nil {
		t.Fatal(err)
	}
	if method != "instructions" || rules.Valid {
		t.Fatalf("defaults: %q %v, want instructions and NULL", method, rules)
	}
	// the column only holds the two methods
	if _, err := JobsDB.Exec(`UPDATE portal_links SET crawl_method = 'colly' WHERE id = 'l1'`); err == nil {
		t.Error("an unknown method was accepted by the CHECK")
	}
	if _, err := JobsDB.Exec(`UPDATE portal_links SET crawl_method = 'links' WHERE id = 'l1'`); err != nil {
		t.Errorf("'links' refused: %v", err)
	}
}

func oldLinksTable(t *testing.T, extra ...string) *sql.DB {
	t.Helper()
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	qs := append([]string{
		`CREATE TABLE portals (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE portal_links (id TEXT PRIMARY KEY, portal_id TEXT NOT NULL, url TEXT NOT NULL, title TEXT, requires_login INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO portals (id, name) VALUES ('p1', 'Old')`,
		`INSERT INTO portal_links (id, portal_id, url, title) VALUES ('l1', 'p1', 'https://old.example', 'Old link')`,
	}, extra...)
	for _, q := range qs {
		if _, err := d.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return d
}

// An install from before plan 185 keeps its links, which get the method they always had; the migration can run again.
func TestMigratePortalLinksLinkCrawl_ExistingInstall(t *testing.T) {
	d := oldLinksTable(t)
	if m, r := linkCrawlColumns(t, d); m || r {
		t.Fatal("the old table already has the columns")
	}
	for i := 0; i < 2; i++ {
		if err := migratePortalLinksLinkCrawl(d); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	if m, r := linkCrawlColumns(t, d); !m || !r {
		t.Fatalf("columns missing after the migration (%v %v)", m, r)
	}
	var url, title, method string
	var rules sql.NullString
	if err := d.QueryRow(`SELECT url, title, crawl_method, link_rules FROM portal_links WHERE id = 'l1'`).Scan(&url, &title, &method, &rules); err != nil {
		t.Fatal(err)
	}
	if url != "https://old.example" || title != "Old link" || method != "instructions" || rules.Valid {
		t.Fatalf("the existing link changed: %q %q %q %v", url, title, method, rules)
	}
	if _, err := d.Exec(`UPDATE portal_links SET crawl_method = 'colly'`); err == nil {
		t.Error("the added column does not carry the CHECK")
	}
}

func TestMigratePortalLinksLinkCrawl_WithoutTheTableIsANoOp(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := migratePortalLinksLinkCrawl(d); err != nil {
		t.Fatal(err)
	}
	if exists, _, err := tableHasColumn(d, "portal_links", "crawl_method"); err != nil || exists {
		t.Fatalf("the migration created the table: exists=%v err=%v", exists, err)
	}
}

func TestMigratePortalLinksLinkCrawl_AddsAMissingColumn(t *testing.T) {
	d := oldLinksTable(t, `ALTER TABLE portal_links ADD COLUMN link_rules TEXT`, `UPDATE portal_links SET link_rules = '{"host":"old.example"}'`)
	if err := migratePortalLinksLinkCrawl(d); err != nil {
		t.Fatal(err)
	}
	var method, rules string
	if err := d.QueryRow(`SELECT crawl_method, link_rules FROM portal_links WHERE id = 'l1'`).Scan(&method, &rules); err != nil {
		t.Fatal(err)
	}
	if method != "instructions" || rules != `{"host":"old.example"}` {
		t.Fatalf("got %q %q, want the existing rule kept", method, rules)
	}
}

// A link that needs a login is never crawled with the link crawler: at every start it goes back to "instructions" (the rule stays).
func TestMigratePortalLinksLinkCrawl_PutsLoginLinksBack(t *testing.T) {
	d := oldLinksTable(t,
		`INSERT INTO portal_links (id, portal_id, url, requires_login) VALUES ('login', 'p1', 'https://g.example', 1)`,
		`INSERT INTO portal_links (id, portal_id, url, requires_login) VALUES ('plain', 'p1', 'https://p.example', 0)`,
	)
	if err := migratePortalLinksLinkCrawl(d); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE portal_links SET crawl_method = 'links', link_rules = '{"host":"g.example"}' WHERE id = 'login'`, // as an older build could have left it
		`UPDATE portal_links SET crawl_method = 'links', link_rules = '{"host":"p.example"}' WHERE id = 'plain'`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ { // twice: idempotent
		if err := migratePortalLinksLinkCrawl(d); err != nil {
			t.Fatal(err)
		}
	}
	read := func(id string) (method, rules string) {
		if err := d.QueryRow(`SELECT crawl_method, link_rules FROM portal_links WHERE id = ?`, id).Scan(&method, &rules); err != nil {
			t.Fatal(err)
		}
		return
	}
	if m, r := read("login"); m != "instructions" || r != `{"host":"g.example"}` {
		t.Errorf("login link: %q %q", m, r)
	}
	if m, r := read("plain"); m != "links" || r != `{"host":"p.example"}` {
		t.Errorf("link without a login: %q %q", m, r)
	}
}

// The start-up path itself: an install with a jobs.db from before plan 185 gets the columns when the tool starts, and keeps its link.
func TestStartUpGivesAnOldInstallTheLinkCrawlColumns(t *testing.T) {
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
	if m, r := linkCrawlColumns(t, JobsDB); !m || !r {
		t.Fatalf("start-up did not add the columns (%v %v)", m, r)
	}
	var title, method string
	if err := JobsDB.QueryRow(`SELECT title, crawl_method FROM portal_links WHERE id = 'l1'`).Scan(&title, &method); err != nil {
		t.Fatal(err)
	}
	if title != "Old link" || method != "instructions" {
		t.Errorf("the old link changed: %q %q", title, method)
	}
}

// ---- plan 186, step 9: the origin of a link's job-detail instructions ----

func hasOriginColumn(t *testing.T, d *sql.DB) bool {
	t.Helper()
	_, has, err := tableHasColumn(d, "portal_links", "job_detail_instructions_origin")
	if err != nil {
		t.Fatal(err)
	}
	return has
}

func TestMigratePortalLinksJobDetailOrigin_ExistingInstall(t *testing.T) {
	d := oldLinksTable(t)
	if hasOriginColumn(t, d) {
		t.Fatal("the old table already has the column")
	}
	for i := 0; i < 2; i++ { // idempotent
		if err := migratePortalLinksJobDetailOrigin(d); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	if !hasOriginColumn(t, d) {
		t.Fatal("column missing after the migration")
	}
	var origin, title string
	if err := d.QueryRow(`SELECT job_detail_instructions_origin, title FROM portal_links WHERE id = 'l1'`).Scan(&origin, &title); err != nil {
		t.Fatal(err)
	}
	if origin != "" || title != "Old link" {
		t.Fatalf("the existing link changed: origin %q title %q", origin, title)
	}
	if _, err := d.Exec(`UPDATE portal_links SET job_detail_instructions_origin = NULL`); err == nil {
		t.Error("the column must not accept NULL")
	}
}

func TestMigratePortalLinksJobDetailOrigin_WithoutTheTableIsANoOp(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := migratePortalLinksJobDetailOrigin(d); err != nil {
		t.Fatal(err)
	}
	if exists, _, err := tableHasColumn(d, "portal_links", "job_detail_instructions_origin"); err != nil || exists {
		t.Fatalf("the migration created the table: exists=%v err=%v", exists, err)
	}
}
