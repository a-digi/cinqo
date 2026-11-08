package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// An older jobs.db: recruiters carry one required company_id with the cascade, and job_recruiters points at them.
const oldRecruitersSchema = `
CREATE TABLE companies (id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE jobs (id TEXT PRIMARY KEY, source_url TEXT NOT NULL UNIQUE, title TEXT NOT NULL);
CREATE TABLE recruiters (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    first_name TEXT NOT NULL DEFAULT '', last_name TEXT NOT NULL DEFAULT '', email TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT
);
CREATE TABLE job_recruiters (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    recruiter_id TEXT NOT NULL REFERENCES recruiters(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (job_id, recruiter_id)
);
INSERT INTO companies VALUES ('c1', 'Acme'), ('c2', 'Beta');
INSERT INTO jobs VALUES ('j1', 'https://e/1', 'Engineer');
INSERT INTO recruiters (id, company_id, first_name, last_name, email, created_at, updated_at) VALUES
  ('r1', 'c1', 'Ada', 'L', 'a@x', '2026-01-01 10:00:00', '2026-02-01 10:00:00'),
  ('r2', 'c2', 'Bob', 'M', '', '2026-01-02 10:00:00', NULL),
  ('r3', 'c1', 'Cy', 'N', 'c@x', '2026-01-03 10:00:00', NULL);
INSERT INTO job_recruiters (job_id, recruiter_id) VALUES ('j1', 'r1'), ('j1', 'r2');
`

func oneInt(t *testing.T, q string) int {
	t.Helper()
	var n int
	if err := JobsDB.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestStartUpConvertsOldRecruitersToTheLinkTable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	old, err := sql.Open("sqlite", filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(oldRecruitersSchema); err != nil {
		t.Fatal(err)
	}
	old.Close()

	for start := 1; start <= 2; start++ { // the second start must change nothing
		if err := InitDatabases(); err != nil {
			t.Fatalf("start %d: %v", start, err)
		}
		if has, _ := columnOf("recruiters", "company_id"); has {
			t.Fatalf("start %d: recruiters still has company_id", start)
		}
		if n := oneInt(t, `SELECT COUNT(*) FROM recruiters`); n != 3 {
			t.Errorf("start %d: recruiters = %d, want 3", start, n)
		}
		if n := oneInt(t, `SELECT COUNT(*) FROM recruiter_companies`); n != 3 {
			t.Errorf("start %d: links = %d, want 3", start, n)
		}
		for r, c := range map[string]string{"r1": "c1", "r2": "c2", "r3": "c1"} {
			if n := oneInt(t, `SELECT COUNT(*) FROM recruiter_companies WHERE recruiter_id = '`+r+`' AND company_id = '`+c+`'`); n != 1 {
				t.Errorf("start %d: %s -> %s link missing", start, r, c)
			}
		}
		if n := oneInt(t, `SELECT COUNT(*) FROM recruiters WHERE id = 'r1' AND first_name = 'Ada' AND email = 'a@x' AND created_at = '2026-01-01 10:00:00' AND updated_at = '2026-02-01 10:00:00'`); n != 1 {
			t.Errorf("start %d: r1 fields were not kept", start)
		}
		if n := oneInt(t, `SELECT COUNT(*) FROM job_recruiters`); n != 2 {
			t.Errorf("start %d: job mappings = %d, want 2", start, n)
		}
		if n := oneInt(t, `SELECT COUNT(*) FROM pragma_foreign_key_list('job_recruiters') WHERE "table" = 'recruiters'`); n != 1 {
			t.Errorf("start %d: job_recruiters must still point at recruiters", start)
		}
		if n := oneInt(t, `SELECT COUNT(*) FROM pragma_foreign_key_list('recruiter_companies')`); n != 2 {
			t.Errorf("start %d: link foreign keys = %d, want 2", start, n)
		}
		if oneInt(t, `PRAGMA foreign_keys`) != 1 {
			t.Errorf("start %d: foreign keys must be on again", start)
		}
		JobsDB.Close()
		CareerDB.Close()
	}

	// after the conversion, deleting a company keeps the recruiter and its job mapping
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer func() { JobsDB.Close(); CareerDB.Close() }()
	if _, err := JobsDB.Exec(`DELETE FROM companies WHERE id = 'c2'`); err != nil {
		t.Fatal(err)
	}
	if oneInt(t, `SELECT COUNT(*) FROM recruiters`) != 3 || oneInt(t, `SELECT COUNT(*) FROM job_recruiters`) != 2 || oneInt(t, `SELECT COUNT(*) FROM recruiter_companies`) != 2 {
		t.Error("deleting a company must keep its recruiter and the recruiter's job mappings")
	}
}

func columnOf(table, column string) (bool, error) {
	_, has, err := tableHasColumn(JobsDB, table, column)
	return has, err
}

// A link to a company that no longer exists is not copied, and a failed conversion leaves the old table untouched.
func TestConversionSkipsOrphanCompaniesAndRollsBackOnFailure(t *testing.T) {
	dir := t.TempDir()
	d, err := sql.Open("sqlite", filepath.Join(dir, "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.SetMaxOpenConns(1)
	if _, err := d.Exec(oldRecruitersSchema + `INSERT INTO recruiters (id, company_id) VALUES ('r9', 'gone');`); err != nil {
		t.Fatal(err)
	}
	// make the rebuild fail: a leftover recruiters_new
	if _, err := d.Exec(`CREATE TABLE recruiters_new (x)`); err != nil {
		t.Fatal(err)
	}
	if err := migrateRecruiterCompanies(d); err == nil {
		t.Fatal("expected the conversion to fail")
	}
	var n int
	_ = d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('recruiters') WHERE name = 'company_id'`).Scan(&n)
	if n != 1 {
		t.Error("a failed conversion must leave the old recruiters table")
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'recruiter_companies'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("a failed conversion must not leave the link table: %d %v", n, err)
	}
	if _, err := d.Exec(`DROP TABLE recruiters_new`); err != nil {
		t.Fatal(err)
	}
	if err := migrateRecruiterCompanies(d); err != nil {
		t.Fatal(err)
	}
	_ = d.QueryRow(`SELECT COUNT(*) FROM recruiter_companies`).Scan(&n)
	if n != 3 {
		t.Errorf("links = %d, want 3 (the orphan r9 has no company to link)", n)
	}
	_ = d.QueryRow(`SELECT COUNT(*) FROM recruiters`).Scan(&n)
	if n != 4 {
		t.Errorf("recruiters = %d, want 4 (r9 itself is kept)", n)
	}
}
