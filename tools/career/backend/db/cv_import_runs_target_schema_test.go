package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// A career.db from before plan 261 has cv_import_runs without target_profile_id: the migration adds it as NULL, keeps every run, and a second run changes nothing.
func TestMigrateCVImportRunsTargetKeepsExistingRuns(t *testing.T) {
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
		if err := migrateCVImportRunsTarget(database); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}
	var target sql.NullString
	var proposal string
	if err := database.QueryRow(`SELECT target_profile_id, ai_proposal_json FROM cv_import_runs WHERE id = 'r1'`).Scan(&target, &proposal); err != nil {
		t.Fatal(err)
	}
	if target.Valid || proposal != `{"a":1}` {
		t.Errorf("target %v, proposal %q", target, proposal)
	}
	if _, err := database.Exec(`UPDATE cv_import_runs SET target_profile_id = 'p1' WHERE id = 'r1'`); err != nil {
		t.Errorf("the column is writable: %v", err)
	}
}

func TestMigrateCVImportRunsTargetWithoutTheTableIsANoOp(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateCVImportRunsTarget(database); err != nil {
		t.Fatal(err)
	}
}

func TestStartUpGivesCVImportRunsTheirTargetColumn(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer func() { JobsDB.Close(); CareerDB.Close() }()
	if _, has, err := tableHasColumn(CareerDB, "cv_import_runs", "target_profile_id"); err != nil || !has {
		t.Errorf("fresh install: %v %v", has, err)
	}
}

// An installed database from before the column gets it when the tool starts, with its runs kept.
func TestStartUpAddsTheTargetColumnToAnInstalledDatabase(t *testing.T) {
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
	var target sql.NullString
	if err := CareerDB.QueryRow(`SELECT target_profile_id FROM cv_import_runs WHERE id = 'r1'`).Scan(&target); err != nil || target.Valid {
		t.Errorf("target %v, err %v", target, err)
	}
}
