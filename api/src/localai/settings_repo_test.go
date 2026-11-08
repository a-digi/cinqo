package localai

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/repository/persistent"
	"github.com/a-digi/cinqo/src/localai/repository/query"
)

func TestTheMigrationsCreateTheOffByDefaultRow(t *testing.T) {
	db := openMainDB(t)
	s, err := query.NewSettingsQueryRepo(db).Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != 1 || s.Enabled || s.ModelID != "" || s.ContextTokens != 4096 || s.IdleMinutes != 10 || s.UpdatedAt != nil {
		t.Errorf("got %+v, want the default row: off, no model, 4096 tokens, 10 minutes", s)
	}
	if s.Configured() {
		t.Error("the default row must not be configured: the local AI is off until an admin turns it on")
	}
}

func columnsOf(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM pragma_table_info('local_ai_settings') ORDER BY cid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, n)
	}
	return cols
}

func TestTheTableHasNoTraceOfTheOwnServerMode(t *testing.T) {
	cols := strings.Join(columnsOf(t, openMainDB(t)), ",")
	if cols != "id,enabled,model_id,context_tokens,idle_minutes,updated_at" {
		t.Errorf("columns = %s", cols)
	}
}

// An installation that already ran the first migration (with the own-server columns and an admin's choices) must come
// through the second one with its row intact. The two files are run by name, exactly as the app cuts them.
func TestAnExistingDatabaseKeepsItsChoicesWhenTheOwnServerColumnsGo(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	run := func(name string) {
		raw, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(raw), ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	run("31_10_2026_20_00_00.sql")
	if _, err := db.Exec(`UPDATE local_ai_settings SET enabled = 1, mode = 'external', external_base_url = 'http://127.0.0.1:8080/v1', model_id = 'phi-4-mini-instruct-q4_k_m', context_tokens = 8192, idle_minutes = 3 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	run("31_10_2026_21_00_00.sql")

	got, err := query.NewSettingsQueryRepo(db).Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.ModelID != "phi-4-mini-instruct-q4_k_m" || got.ContextTokens != 8192 || got.IdleMinutes != 3 {
		t.Errorf("the admin's choices were lost: %+v", got)
	}
	if cols := strings.Join(columnsOf(t, db), ","); cols != "id,enabled,model_id,context_tokens,idle_minutes,updated_at" {
		t.Errorf("columns = %s, want the own-server columns gone and nothing else", cols)
	}
}

func TestTheTableRefusesASecondRow(t *testing.T) {
	if _, err := openMainDB(t).Exec(`INSERT INTO local_ai_settings (id) VALUES (2)`); err == nil {
		t.Error("a second settings row must be refused by the CHECK")
	}
}

func TestSavedSettingsAreReadBack(t *testing.T) {
	db := openMainDB(t)
	in := entity.Settings{Enabled: true, ModelID: "qwen2.5-1.5b-instruct-q4_k_m", ContextTokens: 8192, IdleMinutes: 0}
	if err := persistent.NewSettingsPersistentRepo(db).Save(in); err != nil {
		t.Fatal(err)
	}
	got, err := query.NewSettingsQueryRepo(db).Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.ModelID != in.ModelID || got.ContextTokens != 8192 || got.IdleMinutes != 0 {
		t.Errorf("got %+v, want %+v", got, in)
	}
	if got.UpdatedAt == nil {
		t.Error("a save must stamp updated_at")
	}

	// A save is a full overwrite, never a merge.
	in.Enabled = false
	in.ModelID = ""
	if err := persistent.NewSettingsPersistentRepo(db).Save(in); err != nil {
		t.Fatal(err)
	}
	if got, _ := query.NewSettingsQueryRepo(db).Load(); got.Enabled || got.ModelID != "" {
		t.Errorf("got %+v, want switched off with no model", got)
	}
}
