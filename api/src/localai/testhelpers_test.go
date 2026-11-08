package localai

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

const migrationsDir = "../../config/db/migrations"

// openMainDB applies every real main-database migration, in file-name order, to a fresh in-memory database. The app's
// migration runner cuts a file at every semicolon, comments included, so the helper does exactly that.
func openMainDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found (err %v)", err)
	}
	sort.Strings(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(raw), ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", filepath.Base(f), err)
			}
		}
	}
	return db
}

// saveSettings writes a settings row directly, the way an older or hand-edited database could hold it (no validation).
func saveSettings(t *testing.T, db *sql.DB, enabled bool, model string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE local_ai_settings SET enabled = ?, model_id = ? WHERE id = 1`, enabled, model); err != nil {
		t.Fatal(err)
	}
}

// withEngine replaces the engine for one test.
func withEngine(t *testing.T, e Engine) {
	t.Helper()
	old := engine
	engine = e
	t.Cleanup(func() { engine = old })
}
