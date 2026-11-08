package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// A career.db from before step 165 has cv_import_runs without draft_json: the migration adds it as NULL, keeps every run, and a
// second run changes nothing.
func TestMigrateCVImportRunsDraftKeepsExistingRuns(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, q := range []string{
		oldCVImportRunsTable,
		`INSERT INTO cv_import_runs (id, media_file_id, original_filename, conversation_id, ai_proposal_json) VALUES ('r1', 'm1', 'cv.pdf', 'c1', '{"a":1}')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := migrateCVImportRunsDraft(database); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}
	var draft sql.NullString
	var proposal string
	if err := database.QueryRow(`SELECT draft_json, ai_proposal_json FROM cv_import_runs WHERE id = 'r1'`).Scan(&draft, &proposal); err != nil {
		t.Fatal(err)
	}
	if draft.Valid || proposal != `{"a":1}` {
		t.Errorf("draft %v, proposal %q", draft, proposal)
	}
}

func TestMigrateCVImportRunsDraftWithoutTheTableIsANoOp(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateCVImportRunsDraft(database); err != nil {
		t.Fatal(err)
	}
}

func TestStartUpGivesCVImportRunsTheirDraftColumn(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer func() { JobsDB.Close(); CareerDB.Close() }()
	if _, has, err := tableHasColumn(CareerDB, "cv_import_runs", "draft_json"); err != nil || !has {
		t.Errorf("fresh install: %v %v", has, err)
	}
}

// An existing install gets the column through the real start-up path, and its run is kept.
func TestStartUpAddsTheDraftColumnToAnOlderInstall(t *testing.T) {
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
	defer func() { JobsDB.Close(); CareerDB.Close() }()
	if _, has, err := tableHasColumn(CareerDB, "cv_import_runs", "draft_json"); err != nil || !has {
		t.Errorf("older install: %v %v", has, err)
	}
	var n int
	if err := CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_import_runs WHERE id = 'r1'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("the existing run: %d %v", n, err)
	}
}
