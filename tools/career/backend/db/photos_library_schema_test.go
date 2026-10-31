package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// Photos already linked by profiles become library entries, once each, and the
// backfill can run again without changing anything.
func TestMigratePhotosLibrary_BackfillsLinkedPhotos(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()

	for _, row := range [][]any{
		{"a", "media-1"}, // two profiles share one file
		{"b", "media-1"},
		{"c", "media-2"},
		{"d", nil}, // no photo
		{"e", ""},  // an empty id is not a photo
	} {
		if _, err := CareerDB.Exec(`INSERT INTO profiles (id, image_media_file_id) VALUES (?, ?)`, row...); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := migratePhotosLibrary(CareerDB); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	rows, err := CareerDB.Query(`SELECT media_file_id FROM photos ORDER BY media_file_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	if len(got) != 2 || got[0] != "media-1" || got[1] != "media-2" {
		t.Fatalf("library = %v, want [media-1 media-2]", got)
	}
}

// Every profile's current photo becomes one of its photos (and stays its default), shared
// files once per profile, and the backfill can run again without changing anything.
func TestMigrateProfilePhotos_BackfillsTheDefaults(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()

	for _, row := range [][]any{{"a", "media-1"}, {"b", "media-1"}, {"c", "media-2"}, {"d", nil}, {"e", ""}} {
		if _, err := CareerDB.Exec(`INSERT INTO profiles (id, image_media_file_id) VALUES (?, ?)`, row...); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := migrateProfilePhotos(CareerDB); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	rows, err := CareerDB.Query(`SELECT profile_id, media_file_id FROM profile_photos ORDER BY profile_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var p, m string
		if err := rows.Scan(&p, &m); err != nil {
			t.Fatal(err)
		}
		got = append(got, p+":"+m)
	}
	want := []string{"a:media-1", "b:media-1", "c:media-2"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("profile_photos = %v, want %v", got, want)
	}
}

// An installed cv_documents table without the photo column gets it (NULL for every existing
// row); running it again, or on a database with no such table, changes nothing.
func TestMigrateCvDocumentsPhoto_AddsTheColumnToAnOldTable(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.SetMaxOpenConns(1)

	if err := migrateCvDocumentsPhoto(d); err != nil {
		t.Fatalf("no table: %v", err)
	}
	if _, err := d.Exec(`CREATE TABLE cv_documents (id TEXT PRIMARY KEY, title TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO cv_documents (id, title) VALUES ('d1', 'Old')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateCvDocumentsPhoto(d); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	var photo sql.NullString
	if err := d.QueryRow(`SELECT photo_media_file_id FROM cv_documents WHERE id = 'd1'`).Scan(&photo); err != nil || photo.Valid {
		t.Fatalf("photo = %v err=%v, want NULL", photo, err)
	}
}

// An installed personas table without ai_run_id gets it (NULL for every existing persona);
// running it again, or on a database with no such table, changes nothing.
func TestMigratePersonasAiRunID_AddsTheColumnToAnOldTable(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.SetMaxOpenConns(1)

	if err := migratePersonasAiRunID(d); err != nil {
		t.Fatalf("no table: %v", err)
	}
	if _, err := d.Exec(`CREATE TABLE personas (id TEXT PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO personas (id, name) VALUES ('p1', 'Old')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migratePersonasAiRunID(d); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	var runID sql.NullString
	if err := d.QueryRow(`SELECT ai_run_id FROM personas WHERE id = 'p1'`).Scan(&runID); err != nil || runID.Valid {
		t.Fatalf("ai_run_id = %v err=%v, want NULL", runID, err)
	}
}

// An installed career.db from before step 112 (a personas table without ai_run_id, one persona in
// it) is upgraded by the normal start: the column and the runs table appear, the persona is kept
// with NULL, and a second start changes nothing.
func TestInitDatabases_UpgradesAnInstallFromBeforeThePersonaAiRuns(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TOOL_DB_DIR", dir)
	old, err := sql.Open("sqlite", filepath.Join(dir, "career.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE profiles (id TEXT PRIMARY KEY, first_name TEXT NOT NULL DEFAULT '', last_name TEXT NOT NULL DEFAULT '', image_media_file_id TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT)`,
		`CREATE TABLE personas (id TEXT PRIMARY KEY, profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE, name TEXT NOT NULL, description TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')), updated_at TEXT)`,
		`INSERT INTO profiles (id, first_name) VALUES ('pr1', 'Ada')`,
		`INSERT INTO personas (id, profile_id, name) VALUES ('pe1', 'pr1', 'Old persona')`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()

	for i := 0; i < 2; i++ {
		if err := InitDatabases(); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		var name string
		var runID sql.NullString
		if err := CareerDB.QueryRow(`SELECT name, ai_run_id FROM personas WHERE id = 'pe1'`).Scan(&name, &runID); err != nil || name != "Old persona" || runID.Valid {
			t.Fatalf("start %d: name=%q ai_run_id=%v err=%v", i, name, runID, err)
		}
		if _, err := CareerDB.Exec(`INSERT INTO persona_ai_runs (id, profile_id, conversation_id) VALUES ('r', 'pr1', 'c')`); err != nil {
			t.Fatalf("start %d: the runs table is missing: %v", i, err)
		}
		if _, err := CareerDB.Exec(`UPDATE personas SET ai_run_id = 'r' WHERE id = 'pe1'`); err != nil {
			t.Fatalf("start %d: cannot link a persona to a run: %v", i, err)
		}
		// Deleting the run keeps the persona (the foreign key is live).
		if _, err := CareerDB.Exec(`DELETE FROM persona_ai_runs WHERE id = 'r'`); err != nil {
			t.Fatal(err)
		}
		if err := CareerDB.QueryRow(`SELECT ai_run_id FROM personas WHERE id = 'pe1'`).Scan(&runID); err != nil || runID.Valid {
			t.Fatalf("start %d: ai_run_id after the run was deleted = %v err=%v", i, runID, err)
		}
		JobsDB.Close()
		CareerDB.Close()
	}
}
