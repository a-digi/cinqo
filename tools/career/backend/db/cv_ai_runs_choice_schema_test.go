package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// plan 256: cv_ai_runs gets a choice_json column (the choice a model-driven run handed over).
const oldCvAiRunsTable = `
CREATE TABLE personas (id TEXT PRIMARY KEY);
CREATE TABLE cv_documents (id TEXT PRIMARY KEY);
CREATE TABLE cv_ai_runs (
    id              TEXT PRIMARY KEY,
    persona_id      TEXT NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL,
    focus           TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'generating' CHECK (status IN ('generating','ready','failed')),
    cv_document_id  TEXT REFERENCES cv_documents(id) ON DELETE SET NULL,
    error           TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at     TEXT
);`

func TestMigrateCvAiRunsChoiceKeepsExistingRuns(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, q := range []string{oldCvAiRunsTable, `INSERT INTO personas (id) VALUES ('p1')`, `INSERT INTO cv_ai_runs (id, persona_id, conversation_id, focus) VALUES ('r1', 'p1', 'c1', 'for banks')`} {
		if _, err := database.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := migrateCvAiRunsChoice(database); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}
	var choice sql.NullString
	var focus string
	if err := database.QueryRow(`SELECT choice_json, focus FROM cv_ai_runs WHERE id = 'r1'`).Scan(&choice, &focus); err != nil {
		t.Fatal(err)
	}
	if choice.Valid || focus != "for banks" {
		t.Errorf("choice %v, focus %q", choice, focus)
	}
}

func TestMigrateCvAiRunsChoiceWithoutTheTableIsANoOp(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateCvAiRunsChoice(database); err != nil {
		t.Fatal(err)
	}
}

func TestStartUpGivesCvAiRunsTheirChoiceColumn(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer func() { JobsDB.Close(); CareerDB.Close() }()
	if _, has, err := tableHasColumn(CareerDB, "cv_ai_runs", "choice_json"); err != nil || !has {
		t.Errorf("fresh install: %v %v", has, err)
	}
}

func TestStartUpAddsTheChoiceColumnToAnOlderInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	// an install from before plan 256: start once, then take the column away again and leave one run behind
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	JobsDB.Close()
	CareerDB.Close()
	old, err := sql.Open("sqlite", filepath.Join(dir, "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO profiles (id, first_name) VALUES ('pr1', 'Ada')`,
		`INSERT INTO personas (id, profile_id, name) VALUES ('p1', 'pr1', 'Backend')`,
		`INSERT INTO cv_ai_runs (id, persona_id, conversation_id, focus) VALUES ('r1', 'p1', 'c1', 'for banks')`,
		`ALTER TABLE cv_ai_runs DROP COLUMN choice_json`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, has, _ := tableHasColumn(old, "cv_ai_runs", "choice_json"); has {
		t.Fatal("the test did not make an older install")
	}
	old.Close()
	if err := InitDatabases(); err != nil {
		t.Fatalf("start-up on an older install: %v", err)
	}
	defer func() { JobsDB.Close(); CareerDB.Close() }()
	if _, has, err := tableHasColumn(CareerDB, "cv_ai_runs", "choice_json"); err != nil || !has {
		t.Errorf("older install: %v %v", has, err)
	}
	var focus string
	var choice sql.NullString
	if err := CareerDB.QueryRow(`SELECT focus, choice_json FROM cv_ai_runs WHERE id = 'r1'`).Scan(&focus, &choice); err != nil || focus != "for banks" || choice.Valid {
		t.Errorf("the run must be kept with no choice: %q %v %v", focus, choice, err)
	}
}
