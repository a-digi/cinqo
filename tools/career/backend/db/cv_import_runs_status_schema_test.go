package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

const oldCVImportRunsTable = `CREATE TABLE cv_import_runs (
	id TEXT PRIMARY KEY, media_file_id TEXT NOT NULL, original_filename TEXT NOT NULL, conversation_id TEXT NOT NULL,
	ai_proposal_json TEXT NOT NULL, save_summary_json TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT)`

// A career.db from before step 155 has cv_import_runs without status and error: the migration adds them, every existing run (a
// completed analysis) becomes 'ready' with no error and keeps all its data, a second run changes nothing, and the CHECK refuses
// a status outside the three.
func TestMigrateCVImportRunsStatusKeepsExistingRunsReady(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, q := range []string{
		oldCVImportRunsTable,
		`INSERT INTO cv_import_runs (id, media_file_id, original_filename, conversation_id, ai_proposal_json, save_summary_json)
		 VALUES ('r1', 'm1', 'cv.pdf', 'c1', '{"profile":{}}', '{"saved":true}')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ { // twice: idempotent
		if err := migrateCVImportRunsStatus(database); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}
	var status, proposal, summary string
	var errText sql.NullString
	if err := database.QueryRow(`SELECT status, error, ai_proposal_json, save_summary_json FROM cv_import_runs WHERE id = 'r1'`).Scan(&status, &errText, &proposal, &summary); err != nil {
		t.Fatal(err)
	}
	if status != "ready" || errText.Valid || proposal != `{"profile":{}}` || summary != `{"saved":true}` {
		t.Errorf("status %q, error %v, proposal %q, summary %q", status, errText, proposal, summary)
	}
	for _, v := range []string{"generating", "ready", "failed"} {
		if _, err := database.Exec(`UPDATE cv_import_runs SET status = ? WHERE id = 'r1'`, v); err != nil {
			t.Errorf("%q must be allowed: %v", v, err)
		}
	}
	if _, err := database.Exec(`UPDATE cv_import_runs SET status = 'done' WHERE id = 'r1'`); err == nil {
		t.Error("the CHECK must refuse an unknown status")
	}
}

func TestMigrateCVImportRunsStatusWithoutTheTableIsANoOp(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateCVImportRunsStatus(database); err != nil {
		t.Fatal(err)
	}
}

// The error column is added even when only status exists (a start-up that stopped between the two statements).
func TestMigrateCVImportRunsStatusAddsAMissingErrorColumn(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(oldCVImportRunsTable); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`ALTER TABLE cv_import_runs ADD COLUMN status TEXT NOT NULL DEFAULT 'ready' CHECK (status IN ('generating','ready','failed'))`); err != nil {
		t.Fatal(err)
	}
	if err := migrateCVImportRunsStatus(database); err != nil {
		t.Fatal(err)
	}
	if _, has, err := tableHasColumn(database, "cv_import_runs", "error"); err != nil || !has {
		t.Errorf("error column: %v %v", has, err)
	}
}

// A fresh install has both columns from the base schema; an existing install gets them through the real start-up path.
func TestStartUpGivesCVImportRunsTheirStatus(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{"status", "error"} {
		if _, has, err := tableHasColumn(CareerDB, "cv_import_runs", col); err != nil || !has {
			t.Errorf("fresh install: %s: %v %v", col, has, err)
		}
	}
	// a row that names no status is a completed analysis (what every row written before the column was)
	if _, err := CareerDB.Exec(`INSERT INTO cv_import_runs (id, media_file_id, original_filename, conversation_id, ai_proposal_json) VALUES ('d1', 'm', 'f.pdf', 'c', '{}')`); err != nil {
		t.Fatal(err)
	}
	var def string
	if err := CareerDB.QueryRow(`SELECT status FROM cv_import_runs WHERE id = 'd1'`).Scan(&def); err != nil || def != "ready" {
		t.Errorf("default status %q %v", def, err)
	}
	JobsDB.Close()
	CareerDB.Close()

	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	old, err := sql.Open("sqlite", filepath.Join(dir, "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(oldCVImportRunsTable); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO cv_import_runs (id, media_file_id, original_filename, conversation_id, ai_proposal_json) VALUES ('r1', 'm', 'f.pdf', 'c', '{}')`); err != nil {
		t.Fatal(err)
	}
	old.Close()
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()
	var status string
	if err := CareerDB.QueryRow(`SELECT status FROM cv_import_runs WHERE id = 'r1'`).Scan(&status); err != nil || status != "ready" {
		t.Errorf("existing run: %q %v", status, err)
	}
}
