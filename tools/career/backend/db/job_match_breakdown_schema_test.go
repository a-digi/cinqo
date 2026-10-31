package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// A jobs.db from before step 129 has job_matches without breakdown_json: the migration adds it, existing rows
// keep their AI meaning (NULL breakdown, kind 'ai'), and running it again (a restart) changes nothing.
func TestMigrateJobMatchesBreakdownKeepsExistingRowsAsAI(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, q := range []string{
		`CREATE TABLE job_matches (job_id TEXT PRIMARY KEY, profile_id TEXT NOT NULL, persona_id TEXT, score INTEGER,
			status TEXT NOT NULL DEFAULT 'matching', conversation_id TEXT, error TEXT, updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			kind TEXT NOT NULL DEFAULT 'ai', semantic_model_id TEXT)`,
		`INSERT INTO job_matches (job_id, profile_id, score, status) VALUES ('j1', 'p1', 77, 'completed')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := migrateJobMatchesBreakdown(database); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}
	var kind string
	var breakdown sql.NullString
	var score int
	if err := database.QueryRow(`SELECT kind, breakdown_json, score FROM job_matches WHERE job_id = 'j1'`).Scan(&kind, &breakdown, &score); err != nil {
		t.Fatal(err)
	}
	if kind != "ai" || breakdown.Valid || score != 77 {
		t.Errorf("kind %q breakdown %v score %d", kind, breakdown, score)
	}
}

func TestMigrateJobMatchesBreakdownWithoutTheTableIsANoOp(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateJobMatchesBreakdown(database); err != nil {
		t.Fatal(err)
	}
}

// A fresh install has the column and the requirements table straight from the schema.
func TestFreshSchemaHasBreakdownAndRequirements(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()
	if _, has, err := tableHasColumn(JobsDB, "job_matches", "breakdown_json"); err != nil || !has {
		t.Errorf("breakdown_json: %v %v", has, err)
	}
	if _, err := JobsDB.Exec(`INSERT INTO jobs (id, source_url, title) VALUES ('j', 'u', 't')`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO job_requirements (id, job_id, term, tier) VALUES ('1', 'j', 'Go', 3)`,
	} {
		if _, err := JobsDB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for name, q := range map[string]string{
		"a tier of 4":            `INSERT INTO job_requirements (id, job_id, term, tier) VALUES ('2', 'j', 'x', 4)`,
		"a tier of 0":            `INSERT INTO job_requirements (id, job_id, term, tier) VALUES ('3', 'j', 'x', 0)`,
		"the same term, recased": `INSERT INTO job_requirements (id, job_id, term, tier) VALUES ('4', 'j', 'GO', 2)`,
		"an unknown job":         `INSERT INTO job_requirements (id, job_id, term, tier) VALUES ('5', 'nope', 'x', 2)`,
	} {
		if _, err := JobsDB.Exec(q); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	if _, err := JobsDB.Exec(`DELETE FROM jobs WHERE id = 'j'`); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = JobsDB.QueryRow(`SELECT COUNT(*) FROM job_requirements`).Scan(&n)
	if n != 0 {
		t.Errorf("%d requirements left after the job was deleted", n)
	}
}
