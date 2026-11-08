package toolwidgets

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/dashboard"
	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
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

func addTool(t *testing.T, db *sql.DB, id, slug string, enabled bool) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at)
		 VALUES (?, ?, ?, '1.0.0', 'backend_only', ?, 'running', '/x', '2026-10-30T00:00:00Z')`,
		id, slug, slug, enabled,
	); err != nil {
		t.Fatal(err)
	}
}

func addRoute(t *testing.T, db *sql.DB, toolID, method, path, scope string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope) VALUES (?, ?, ?, ?)`,
		toolID, method, path, scope,
	); err != nil {
		t.Fatal(err)
	}
}

func addWidget(t *testing.T, db *sql.DB, toolID, key, kind, path string) {
	t.Helper()
	err := tool_persistent.NewToolDashboardWidgetPersistentRepo(db).ReplaceAll(toolID, append(existing(t, db, toolID), tool_entity.ToolDashboardWidget{
		Key: key, Title: strings.ToUpper(key), Kind: kind, SchemaVersion: 1, DataPath: path, Width: 2, Height: 1, RefreshSeconds: 30,
	}))
	if err != nil {
		t.Fatal(err)
	}
}

func existing(t *testing.T, db *sql.DB, toolID string) []tool_entity.ToolDashboardWidget {
	rows, err := db.Query(`SELECT key, title, kind, schema_version, data_path, width, height, refresh_seconds FROM tool_dashboard_widgets WHERE tool_id = ?`, toolID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []tool_entity.ToolDashboardWidget
	for rows.Next() {
		var w tool_entity.ToolDashboardWidget
		if err := rows.Scan(&w.Key, &w.Title, &w.Kind, &w.SchemaVersion, &w.DataPath, &w.Width, &w.Height, &w.RefreshSeconds); err != nil {
			t.Fatal(err)
		}
		out = append(out, w)
	}
	return out
}

// fakeTool serves one route and records what it was asked.
type fakeTool struct {
	*httptest.Server
	mu   sync.Mutex
	auth string
	path string
	hits int
}

func newFakeTool(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *fakeTool {
	ft := &fakeTool{}
	ft.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ft.mu.Lock()
		ft.auth, ft.path = r.Header.Get("Authorization"), r.URL.Path
		ft.hits++
		ft.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(ft.Close)
	return ft
}

func (f *fakeTool) portFunc(running bool) PortFunc {
	u, _ := url.Parse(f.URL)
	port, _ := strconv.Atoi(u.Port())
	return func(string) (int, bool) { return port, running }
}

func setup(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*dashboard.Engine, *fakeTool, *sql.DB) {
	t.Helper()
	db := coreDB(t)
	addTool(t, db, "t1", "demo", true)
	addRoute(t, db, "t1", "GET", "dashboard/jobs", "tool:demo:read")
	addWidget(t, db, "t1", "jobs", "stat", "dashboard/jobs")
	ft := newFakeTool(t, handler)
	return dashboard.NewEngine(NewSource(db, ft.portFunc(true))), ft, db
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(`{"label":"Open jobs","value":12,"unexpected":true}`))
}

var reader = dashboard.Caller{UserID: "u1", Scopes: []string{"tool:demo:read"}, Token: "tok-123"}

func TestWidgetInheritsItsRoutesScope(t *testing.T) {
	e, _, db := setup(t, okHandler)

	if got := e.Widgets(reader); len(got) != 1 || got[0].ID() != "demo.jobs" || got[0].RequiredScope != "tool:demo:read" || got[0].Size.W != 2 {
		t.Fatalf("reader sees %+v", got)
	}
	if got := e.Widgets(dashboard.Caller{UserID: "u2"}); len(got) != 0 {
		t.Errorf("a caller without the route's scope must not see the widget: %+v", got)
	}
	if got := e.Widgets(dashboard.Caller{UserID: "root", Scopes: []string{dashboard.AdminScope}}); len(got) != 1 {
		t.Errorf("super admin should see it: %+v", got)
	}

	if _, err := db.Exec(`UPDATE tools SET enabled = 0 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if got := e.Widgets(reader); len(got) != 0 {
		t.Errorf("a disabled tool's widgets must be hidden: %+v", got)
	}
	if _, err := db.Exec(`UPDATE tools SET enabled = 1 WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM tool_routes WHERE tool_id = 't1'`); err != nil {
		t.Fatal(err)
	}
	if got := e.Widgets(reader); len(got) != 0 {
		t.Errorf("a widget whose route is gone must not be served: %+v", got)
	}
}

func TestDataIsFetchedWithTheCallersTokenAndValidated(t *testing.T) {
	e, ft, _ := setup(t, okHandler)
	d, ok := e.Data(context.Background(), reader, "demo.jobs")
	if !ok || d.State != dashboard.StateOK {
		t.Fatalf("got %+v ok=%v", d, ok)
	}
	if ft.auth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want the caller's bearer token", ft.auth)
	}
	if ft.path != "/dashboard/jobs" {
		t.Errorf("path = %q", ft.path)
	}
	if strings.Contains(string(d.Data), "unexpected") {
		t.Errorf("payload not normalised: %s", d.Data)
	}
}

func TestForbiddenCallerNeverReachesTheTool(t *testing.T) {
	e, ft, _ := setup(t, okHandler)
	if _, ok := e.Data(context.Background(), dashboard.Caller{UserID: "u2", Token: "x"}, "demo.jobs"); ok {
		t.Error("found")
	}
	if ft.hits != 0 {
		t.Errorf("the tool was called %d times for a forbidden caller", ft.hits)
	}
}

func TestFailureModes(t *testing.T) {
	for name, tc := range map[string]struct {
		handler func(w http.ResponseWriter, r *http.Request)
		want    dashboard.State
	}{
		"503 from the tool": {func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }, dashboard.StateUnavailable},
		"500 from the tool": {func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }, dashboard.StateError},
		"403 from the tool": {func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(403) }, dashboard.StateError},
		"redirect is not followed": {func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://127.0.0.1:1/elsewhere", http.StatusFound)
		}, dashboard.StateError},
		"payload over the cap": {func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"label":"x","value":"%s"}`, strings.Repeat("a", dashboard.MaxPayloadBytes))
		}, dashboard.StateError},
		"invalid payload": {func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"label":"x"}`)) }, dashboard.StateError},
		"not JSON":        {func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`<html>`)) }, dashboard.StateError},
	} {
		e, _, _ := setup(t, tc.handler)
		d, ok := e.Data(context.Background(), reader, "demo.jobs")
		if !ok || d.State != tc.want {
			t.Errorf("%s: state = %q, want %q", name, d.State, tc.want)
		}
	}
}

func TestStoppedToolIsUnavailableWithoutACall(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "demo", true)
	addRoute(t, db, "t1", "GET", "dashboard/jobs", "tool:demo:read")
	addWidget(t, db, "t1", "jobs", "stat", "dashboard/jobs")
	ft := newFakeTool(t, okHandler)
	e := dashboard.NewEngine(NewSource(db, ft.portFunc(false)))

	d, ok := e.Data(context.Background(), reader, "demo.jobs")
	if !ok || d.State != dashboard.StateUnavailable {
		t.Fatalf("got %+v", d)
	}
	if ft.hits != 0 {
		t.Error("a stopped tool must not be called")
	}
}

func TestRefusedConnectionIsUnavailable(t *testing.T) {
	e, ft, _ := setup(t, okHandler)
	ft.Close() // the port now refuses connections
	d, ok := e.Data(context.Background(), reader, "demo.jobs")
	if !ok || d.State != dashboard.StateUnavailable {
		t.Errorf("state = %q, want unavailable", d.State)
	}
}

func TestInvalidStoredWidgetIsSkipped(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "demo", true)
	addRoute(t, db, "t1", "GET", "dashboard/jobs", "tool:demo:read")
	addWidget(t, db, "t1", "good", "stat", "dashboard/jobs")
	addWidget(t, db, "t1", "badkind", "table", "dashboard/jobs")
	if _, err := db.Exec(`UPDATE tool_dashboard_widgets SET width = 99 WHERE key = 'good'`); err != nil {
		t.Fatal(err)
	}
	addWidget(t, db, "t1", "fine", "list", "dashboard/jobs")
	ft := newFakeTool(t, okHandler)
	e := dashboard.NewEngine(NewSource(db, ft.portFunc(true)))
	var ids []string
	for _, d := range e.Widgets(reader) {
		ids = append(ids, d.ID())
	}
	if len(ids) != 1 || ids[0] != "demo.fine" {
		t.Errorf("served %v, want only demo.fine", ids)
	}
}

func TestReplaceAllClearsWhenEmpty(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "demo", true)
	addWidget(t, db, "t1", "jobs", "stat", "dashboard/jobs")
	repo := tool_persistent.NewToolDashboardWidgetPersistentRepo(db)
	if err := repo.ReplaceAll("t1", nil); err != nil {
		t.Fatal(err)
	}
	if got := existing(t, db, "t1"); len(got) != 0 {
		t.Errorf("rows left: %+v", got)
	}
}

func TestToolDeleteRemovesItsWidgets(t *testing.T) {
	db := coreDB(t)
	addTool(t, db, "t1", "demo", true)
	addWidget(t, db, "t1", "jobs", "stat", "dashboard/jobs")
	if err := tool_persistent.NewToolPersistentRepo(db).Delete("t1"); err != nil {
		t.Fatal(err)
	}
	if got := existing(t, db, "t1"); len(got) != 0 {
		t.Errorf("rows left after delete: %+v", got)
	}
}
