package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// Plan 194: a jobs.db from before has job_recrawl_requests without the worker columns. The migration adds them as NULL, keeps every request,
// and a second run changes nothing.
func TestMigrateJobRecrawlWorkerColumnsKeepsExistingRequests(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, q := range []string{
		`CREATE TABLE job_recrawl_requests (job_id TEXT PRIMARY KEY, batch_id TEXT NOT NULL, requested_at TEXT NOT NULL, state TEXT NOT NULL, outcome TEXT, reason TEXT, finished_at TEXT)`,
		`INSERT INTO job_recrawl_requests (job_id, batch_id, requested_at, state, outcome) VALUES ('a', 'b1', '2026-11-06T10:00:00Z', 'done', 'refreshed'), ('q', 'b1', '2026-11-06T10:00:00Z', 'queued', NULL)`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := migrateJobRecrawlWorkerColumns(database); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}
	var started, phase, message sql.NullString
	var slot sql.NullInt64
	var outcome string
	if err := database.QueryRow(`SELECT started_at, worker_slot, phase, phase_message, outcome FROM job_recrawl_requests WHERE job_id = 'a'`).Scan(&started, &slot, &phase, &message, &outcome); err != nil {
		t.Fatal(err)
	}
	if started.Valid || slot.Valid || phase.Valid || message.Valid || outcome != "refreshed" {
		t.Errorf("an old request changed: %v %v %v %v %q", started, slot, phase, message, outcome)
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM job_recrawl_requests`).Scan(&n); err != nil || n != 2 {
		t.Errorf("requests kept: %d %v", n, err)
	}
}

func TestMigrateJobRecrawlWorkerColumnsWithoutTheTableIsANoOp(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateJobRecrawlWorkerColumns(database); err != nil {
		t.Fatal(err)
	}
}

// A fresh install gets the columns from the base schema.
func TestAFreshJobsDatabaseHasTheWorkerColumns(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { JobsDB.Close(); CareerDB.Close() })
	for _, c := range []string{"started_at", "worker_slot", "phase", "phase_message"} {
		if _, has, err := tableHasColumn(JobsDB, "job_recrawl_requests", c); err != nil || !has {
			t.Errorf("column %s: %v %v", c, has, err)
		}
	}
}
