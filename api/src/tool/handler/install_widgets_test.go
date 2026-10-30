package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/tool/manifest"
)

func TestCacheDashboardWidgets(t *testing.T) {
	di, _ := newUploadDI(t)
	if _, err := di.db.Exec(
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at)
		 VALUES ('t1', 'demo', 'Demo', '1.0.0', 'backend_only', 1, 'running', '/x', '2026-10-30T00:00:00Z')`,
	); err != nil {
		t.Fatal(err)
	}
	ctx := request.NewContext(httptest.NewRecorder(), httptest.NewRequest("POST", "/x", nil), di)

	m := manifest.Manifest{Slug: "demo", Widgets: []manifest.WidgetDecl{
		{Key: "jobs", Title: "Jobs", Kind: "stat", DataPath: "dashboard/jobs"},
		{Key: "recent", Title: "Recent", Description: "Latest", Kind: "list", DataPath: "dashboard/recent", SchemaVersion: 1, Width: 2, Height: 2, RefreshSeconds: 120},
	}}
	cacheDashboardWidgets(ctx, di.db, m, "t1")

	type row struct {
		key, kind, path        string
		version, w, h, refresh int
	}
	read := func() map[string]row {
		rows, err := di.db.Query(`SELECT key, kind, data_path, schema_version, width, height, refresh_seconds FROM tool_dashboard_widgets WHERE tool_id = 't1'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]row{}
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.key, &r.kind, &r.path, &r.version, &r.w, &r.h, &r.refresh); err != nil {
				t.Fatal(err)
			}
			out[r.key] = r
		}
		return out
	}

	got := read()
	if len(got) != 2 {
		t.Fatalf("rows = %+v", got)
	}
	if g := got["jobs"]; g.kind != "stat" || g.path != "dashboard/jobs" || g.version != 1 || g.w != 1 || g.h != 1 || g.refresh != 60 {
		t.Errorf("defaults not applied: %+v", g)
	}
	if g := got["recent"]; g.kind != "list" || g.w != 2 || g.h != 2 || g.refresh != 120 {
		t.Errorf("declared values lost: %+v", g)
	}

	// An update that declares fewer widgets (here: none) drops the old ones.
	cacheDashboardWidgets(ctx, di.db, manifest.Manifest{Slug: "demo"}, "t1")
	if got := read(); len(got) != 0 {
		t.Errorf("rows after an update without widgets: %+v", got)
	}
}
