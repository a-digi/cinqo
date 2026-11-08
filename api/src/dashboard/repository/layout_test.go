package repository_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/dashboard"
	"github.com/a-digi/cinqo/src/dashboard/repository/persistent"
	"github.com/a-digi/cinqo/src/dashboard/repository/query"
)

func coreDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	files, _ := filepath.Glob(filepath.Join("..", "..", "..", "config", "db", "migrations", "*.sql"))
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

func TestLayoutRoundTripAndIsolation(t *testing.T) {
	db := coreDB(t)
	w, r := persistent.NewLayoutPersistentRepo(db), query.NewLayoutQueryRepo(db)

	alice := []dashboard.LayoutEntry{{WidgetID: "cinqo.b", Hidden: true}, {WidgetID: "cinqo.a"}, {WidgetID: "career.x"}}
	bob := []dashboard.LayoutEntry{{WidgetID: "cinqo.a", Hidden: true}}
	if err := w.ReplaceAll("alice", alice); err != nil {
		t.Fatal(err)
	}
	if err := w.ReplaceAll("bob", bob); err != nil {
		t.Fatal(err)
	}

	if got, err := r.Find("alice"); err != nil || !reflect.DeepEqual(got, alice) {
		t.Errorf("alice = %v, %v", got, err)
	}
	if got, _ := r.Find("bob"); !reflect.DeepEqual(got, bob) {
		t.Errorf("bob = %v", got)
	}
	if got, err := r.Find("nobody"); err != nil || len(got) != 0 {
		t.Errorf("nobody = %v, %v", got, err)
	}

	// Saving replaces, it does not merge, and one user's save never touches another's.
	if err := w.ReplaceAll("alice", []dashboard.LayoutEntry{{WidgetID: "cinqo.a"}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Find("alice"); len(got) != 1 || got[0].WidgetID != "cinqo.a" {
		t.Errorf("alice after replace = %v", got)
	}
	if got, _ := r.Find("bob"); !reflect.DeepEqual(got, bob) {
		t.Errorf("bob changed by alice's save: %v", got)
	}

	// An empty list clears the layout (back to the default arrangement).
	if err := w.ReplaceAll("alice", nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Find("alice"); len(got) != 0 {
		t.Errorf("alice after clear = %v", got)
	}
}
