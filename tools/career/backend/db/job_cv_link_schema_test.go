package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// A jobs.db from before step 98 has job_cv_pdfs without cv_document_id/source:
// the migration adds both, existing rows become 'ai', and running it again (a
// restart) changes nothing.
func TestMigrateJobCvPdfsLinkKeepsExistingRowsAsAI(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, q := range []string{
		`CREATE TABLE job_cv_pdfs (job_id TEXT PRIMARY KEY, profile_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'generating',
			conversation_id TEXT, media_file_id TEXT, error TEXT, updated_at TEXT NOT NULL DEFAULT (datetime('now')))`,
		`INSERT INTO job_cv_pdfs (job_id, profile_id, status, media_file_id) VALUES ('j1', 'p1', 'completed', 'media-1')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 2; i++ { // twice: idempotent
		if err := migrateJobCvPdfsLink(database); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}

	var source string
	var documentID sql.NullString
	if err := database.QueryRow(`SELECT source, cv_document_id FROM job_cv_pdfs WHERE job_id = 'j1'`).Scan(&source, &documentID); err != nil {
		t.Fatal(err)
	}
	if source != "ai" || documentID.Valid {
		t.Errorf("source %q, document %v", source, documentID)
	}
	if _, err := database.Exec(`INSERT INTO job_cv_pdfs (job_id, profile_id, status, source) VALUES ('j2', 'p1', 'completed', 'manual')`); err != nil {
		t.Errorf("manual must be allowed: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO job_cv_pdfs (job_id, profile_id, status, source) VALUES ('j3', 'p1', 'completed', 'bogus')`); err == nil {
		t.Error("the CHECK must refuse an unknown source")
	}
}

func TestMigrateJobCvPdfsLinkWithoutTheTableIsANoOp(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateJobCvPdfsLink(database); err != nil {
		t.Fatal(err)
	}
}

// A brand-new install already has both columns from the inline CREATE TABLE, and
// a restart over an existing database keeps working.
func TestFreshInstallHasTheLinkColumns(t *testing.T) {
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
	for _, col := range []string{"cv_document_id", "source"} {
		if _, has, err := tableHasColumn(JobsDB, "job_cv_pdfs", col); err != nil || !has {
			t.Errorf("job_cv_pdfs.%s missing (err %v)", col, err)
		}
	}
}
