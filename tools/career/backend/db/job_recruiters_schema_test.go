package db

import "testing"

// The table that maps recruiters to jobs (plan/ai/tools/career/step-168) exists on a fresh install, and an older jobs.db without it
// gets it at the next start through the real start-up path, with its index and its foreign keys.
func TestStartUpGivesJobsDBTheJobRecruitersTable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	has := func() (table, index bool) {
		var n int
		if err := JobsDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'job_recruiters'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		table = n == 1
		if err := JobsDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'job_recruiters_recruiter_idx'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return table, n == 1
	}
	if table, index := has(); !table || !index {
		t.Fatalf("fresh install: table %v index %v", table, index)
	}
	var fk int
	if err := JobsDB.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_list('job_recruiters')`).Scan(&fk); err != nil || fk != 2 {
		t.Errorf("foreign keys: %d %v, want 2", fk, err)
	}
	// an older install: the table is not there
	if _, err := JobsDB.Exec(`DROP TABLE job_recruiters`); err != nil {
		t.Fatal(err)
	}
	JobsDB.Close()
	CareerDB.Close()
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer func() { JobsDB.Close(); CareerDB.Close() }()
	if table, index := has(); !table || !index {
		t.Errorf("older install: table %v index %v", table, index)
	}
	// starting again changes nothing
	JobsDB.Close()
	CareerDB.Close()
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	if table, _ := has(); !table {
		t.Error("the second start lost the table")
	}
}
