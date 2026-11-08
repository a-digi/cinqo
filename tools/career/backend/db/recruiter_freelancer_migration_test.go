package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// A jobs.db from after plan 169: recruiters without company_id, with the link table, but no is_freelancer yet.
const midRecruitersSchema = `
CREATE TABLE companies (id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE recruiters (
    id TEXT PRIMARY KEY,
    first_name TEXT NOT NULL DEFAULT '', last_name TEXT NOT NULL DEFAULT '', email TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT
);
CREATE TABLE recruiter_companies (
    recruiter_id TEXT NOT NULL REFERENCES recruiters(id) ON DELETE CASCADE,
    company_id TEXT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (recruiter_id, company_id)
);
INSERT INTO companies VALUES ('c1', 'Acme');
INSERT INTO recruiters (id, first_name, last_name, email, created_at, updated_at) VALUES
  ('r1', 'Ada', 'L', 'a@x', '2026-01-01 10:00:00', '2026-02-01 10:00:00'),
  ('r2', 'Bob', 'M', '', '2026-01-02 10:00:00', NULL);
INSERT INTO recruiter_companies (recruiter_id, company_id) VALUES ('r1', 'c1');
`

func startWith(t *testing.T, schema string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	if schema != "" {
		old, err := sql.Open("sqlite", filepath.Join(dir, "jobs.db"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := old.Exec(schema); err != nil {
			t.Fatal(err)
		}
		old.Close()
	}
}

func TestStartUpAddsTheFreelancerFlagAndGuessesNobody(t *testing.T) {
	startWith(t, midRecruitersSchema)
	for start := 1; start <= 2; start++ { // the second start must change nothing
		if err := InitDatabases(); err != nil {
			t.Fatalf("start %d: %v", start, err)
		}
		if has, _ := columnOf("recruiters", "is_freelancer"); !has {
			t.Fatalf("start %d: recruiters has no is_freelancer", start)
		}
		if n := oneInt(t, `SELECT COUNT(*) FROM recruiters WHERE is_freelancer = 0`); n != 2 {
			t.Errorf("start %d: every existing recruiter must stay a non-freelancer, got %d", start, n)
		}
		if n := oneInt(t, `SELECT COUNT(*) FROM recruiters WHERE id = 'r1' AND first_name = 'Ada' AND email = 'a@x' AND created_at = '2026-01-01 10:00:00' AND updated_at = '2026-02-01 10:00:00'`); n != 1 {
			t.Errorf("start %d: r1 fields were not kept", start)
		}
		if n := oneInt(t, `SELECT COUNT(*) FROM recruiter_companies`); n != 1 {
			t.Errorf("start %d: links = %d, want 1", start, n)
		}
		JobsDB.Close()
		CareerDB.Close()
	}
}

func TestTheFreelancerFlagOnlyAcceptsZeroOrOne(t *testing.T) {
	startWith(t, "")
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()
	if _, err := JobsDB.Exec(`INSERT INTO recruiters (id, is_freelancer) VALUES ('ok1', 1), ('ok0', 0)`); err != nil {
		t.Fatalf("0 and 1 must be accepted: %v", err)
	}
	if _, err := JobsDB.Exec(`INSERT INTO recruiters (id, is_freelancer) VALUES ('bad', 2)`); err == nil {
		t.Error("2 must be refused")
	}
	if _, err := JobsDB.Exec(`INSERT INTO recruiters (id) VALUES ('dflt')`); err != nil {
		t.Fatal(err)
	}
	if n := oneInt(t, `SELECT COUNT(*) FROM recruiters WHERE id = 'dflt' AND is_freelancer = 0`); n != 1 {
		t.Error("the default must be 0")
	}
}

// The oldest shape (recruiters.company_id) goes through both migrations in one start: the rebuild creates the table without the
// flag, the next migration adds it.
func TestStartUpOfTheOldestRecruitersShapeEndsWithTheFlag(t *testing.T) {
	startWith(t, oldRecruitersSchema)
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()
	if has, _ := columnOf("recruiters", "company_id"); has {
		t.Error("company_id must be gone")
	}
	if has, _ := columnOf("recruiters", "is_freelancer"); !has {
		t.Fatal("is_freelancer must exist")
	}
	if n := oneInt(t, `SELECT COUNT(*) FROM recruiters WHERE is_freelancer = 0`); n != 3 {
		t.Errorf("non-freelancers = %d, want 3", n)
	}
	if n := oneInt(t, `SELECT COUNT(*) FROM recruiter_companies`); n != 3 {
		t.Errorf("links = %d, want 3", n)
	}
}
