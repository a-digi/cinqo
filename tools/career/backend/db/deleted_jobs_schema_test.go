package db

import (
	"os"
	"path/filepath"
	"testing"
)

// InitDatabases creates deleted_jobs.db next to the other two with its table, empty, and opening again keeps what is in it.
func TestDeletedJobsDatabaseIsCreatedAndKept(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { JobsDB.Close(); CareerDB.Close() })
	if DeletedJobsDB == nil {
		t.Fatal("DeletedJobsDB is nil")
	}
	if _, err := os.Stat(filepath.Join(dir, "deleted_jobs.db")); err != nil {
		t.Errorf("the file is missing: %v", err)
	}
	var n int
	if err := DeletedJobsDB.QueryRow(`SELECT COUNT(*) FROM deleted_jobs`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("fresh table: %d, %v", n, err)
	}
	if _, err := DeletedJobsDB.Exec(`INSERT INTO deleted_jobs (url_hash, key_version, source_url, title, reason) VALUES ('h', 1, 'u', 't', 'deleted')`); err != nil {
		t.Fatal(err)
	}
	JobsDB.Close()
	CareerDB.Close()
	first := DeletedJobsDB
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	if err := first.Ping(); err == nil {
		t.Error("the earlier handle was left open")
	}
	if err := DeletedJobsDB.QueryRow(`SELECT COUNT(*) FROM deleted_jobs`).Scan(&n); err != nil || n != 1 {
		t.Errorf("after reopening: %d, %v", n, err)
	}
}

// The memory is optional: when its file cannot be opened the tool still starts, the other databases work, and the
// handle is nil.
func TestAnUnopenableDeletedJobsDatabaseDoesNotStopTheTool(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "deleted_jobs.db"), 0o755); err != nil { // a directory where the file should be
		t.Fatal(err)
	}
	t.Setenv("TOOL_DB_DIR", dir)
	if err := InitDatabases(); err != nil {
		t.Fatalf("InitDatabases failed because of the optional database: %v", err)
	}
	t.Cleanup(func() { JobsDB.Close(); CareerDB.Close() })
	if DeletedJobsDB != nil {
		t.Error("DeletedJobsDB should be nil")
	}
	var n int
	if err := JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil {
		t.Errorf("jobs.db is unusable: %v", err)
	}
	if err := CareerDB.QueryRow(`SELECT COUNT(*) FROM profiles`).Scan(&n); err != nil {
		t.Errorf("career.db is unusable: %v", err)
	}
}
