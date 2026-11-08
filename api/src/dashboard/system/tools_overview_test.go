package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/dashboard"
)

func coreDB(t *testing.T) *sql.DB { return migratedDB(t, "migrations") }

// migratedDB opens an in-memory database with every migration in config/db/<dir>.
func migratedDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	files, _ := filepath.Glob(filepath.Join("..", "..", "..", "config", "db", dir, "*.sql"))
	sort.Strings(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(raw), "/***Statement***/") {
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

func TestToolsOverview(t *testing.T) {
	db := coreDB(t)
	for i, row := range []struct {
		slug    string
		enabled int
	}{{"alpha", 1}, {"beta", 0}, {"gamma", 1}} {
		if _, err := db.Exec(
			`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at)
			 VALUES (?, ?, ?, '1.0.0', 'backend_only', ?, 'stopped', '/x', '2026-10-30T00:00:00Z')`,
			string(rune('a'+i)), row.slug, row.slug, row.enabled,
		); err != nil {
			t.Fatal(err)
		}
	}

	w := NewToolsOverview(db)
	if err := w.Definition().Validate(); err != nil {
		t.Fatalf("definition: %v", err)
	}
	if w.Definition().ID() != "cinqo.tools_overview" || w.Definition().RequiredScope != dashboard.AdminScope {
		t.Errorf("definition = %+v", w.Definition())
	}

	raw, err := w.Data(context.Background(), dashboard.Caller{})
	if err != nil {
		t.Fatal(err)
	}
	clean, err := dashboard.Validate(dashboard.KindStat, dashboard.SchemaVersion, raw)
	if err != nil {
		t.Fatalf("payload does not satisfy its own kind: %v", err)
	}
	var got dashboard.StatData
	if err := json.Unmarshal(clean, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Value.IsNumber || got.Value.Number != 3 || got.Caption != "2 enabled" || got.Link != "/admin/tools" {
		t.Errorf("stat = %+v", got)
	}
}
