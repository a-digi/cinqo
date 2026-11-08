package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupJobsDBMakesAReadableCopy(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { JobsDB.Close(); CareerDB.Close() })
	if _, err := JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, description, crawled_at) VALUES ('j1', 'https://x/1', 'T', 'text', datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	path, err := BackupJobsDB("test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(path), "jobs-before-test-") || filepath.Ext(path) != ".db" || filepath.Dir(path) != jobsDir {
		t.Errorf("path %q", path)
	}
	copyDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	var d string
	if err := copyDB.QueryRow(`SELECT description FROM jobs WHERE id = 'j1'`).Scan(&d); err != nil || d != "text" {
		t.Errorf("copy has %q (%v)", d, err)
	}
}

func TestTwoBackupsInTheSameSecondKeepTheirOwnFiles(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { JobsDB.Close(); CareerDB.Close() })
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		path, err := BackupJobsDB("same")
		if err != nil {
			t.Fatal(err)
		}
		if seen[path] {
			t.Fatalf("%q was given twice", path)
		}
		seen[path] = true
	}
}

func TestBackupJobsDBWithoutAnOpenDatabaseIsAnError(t *testing.T) {
	old, oldDir := JobsDB, jobsDir
	JobsDB, jobsDir = nil, ""
	t.Cleanup(func() { JobsDB, jobsDir = old, oldDir })
	if path, err := BackupJobsDB("none"); err == nil {
		t.Errorf("expected an error, got %q", path)
	}
}
