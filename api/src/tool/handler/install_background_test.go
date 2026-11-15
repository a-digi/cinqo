package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/a-digi/coco-server/server/request"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	"github.com/a-digi/cinqo/src/tool/manifest"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

func bgDecl(tool string, routes ...[2]string) manifest.BackgroundAccessDecl {
	d := manifest.BackgroundAccessDecl{Tool: tool}
	for _, r := range routes {
		d.Routes = append(d.Routes, manifest.BackgroundRouteDecl{Method: r[0], PathSuffix: r[1]})
	}
	return d
}

func TestCacheBackgroundRequestsStoresTheAskAndNeverApprovesAndAnUpdateClearsWhatIsNotAskedAnyMore(t *testing.T) {
	di, _ := newUploadDI(t)
	if _, err := di.db.Exec(`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at) VALUES ('t1', 'namako', 'N', '1.0.0', 'backend_only', 1, 'running', '/x', '2026-11-15T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	ctx := request.NewContext(httptest.NewRecorder(), httptest.NewRequest("POST", "/x", nil), di)
	q := tool_query.NewToolBackgroundQueryRepo(di.db)

	m := manifest.Manifest{Slug: "namako", BackgroundAccess: []manifest.BackgroundAccessDecl{
		bgDecl("browser", [2]string{"POST", "list-links"}, [2]string{"POST", "fetch-frames"}),
		bgDecl("pdf_tools", [2]string{"POST", "render"}),
	}}
	cacheBackgroundRequests(ctx, di.db, m, "t1")
	got, _ := q.Requests("t1")
	if len(got) != 3 || got[0].TargetSlug != "browser" || got[2].TargetSlug != "pdf_tools" {
		t.Fatalf("%+v", got)
	}
	if g, _ := q.Grants("t1"); len(g) != 0 {
		t.Errorf("an install approves nothing: %+v", g)
	}

	p := tool_persistent.NewToolBackgroundPersistentRepo(di.db)
	_ = p.Approve("t1", tool_entity.ToolBackgroundRequest{TargetSlug: "browser", Method: "POST", PathSuffix: "list-links"}, "andet")
	_ = p.Approve("t1", tool_entity.ToolBackgroundRequest{TargetSlug: "pdf_tools", Method: "POST", PathSuffix: "render"}, "andet")

	// an update keeps asking for list-links only: the approval of the other two is gone, the one that is still asked for stays
	cacheBackgroundRequests(ctx, di.db, manifest.Manifest{Slug: "namako", BackgroundAccess: []manifest.BackgroundAccessDecl{bgDecl("browser", [2]string{"POST", "list-links"})}}, "t1")
	if r, _ := q.Requests("t1"); len(r) != 1 || r[0].PathSuffix != "list-links" {
		t.Errorf("%+v", r)
	}
	if g, _ := q.Grants("t1"); len(g) != 1 || g[0].PathSuffix != "list-links" {
		t.Errorf("%+v", g)
	}
	// and an update without any declaration clears both
	cacheBackgroundRequests(ctx, di.db, manifest.Manifest{Slug: "namako"}, "t1")
	if r, _ := q.Requests("t1"); len(r) != 0 {
		t.Errorf("%+v", r)
	}
	if g, _ := q.Grants("t1"); len(g) != 0 {
		t.Errorf("%+v", g)
	}
}

func TestInstalledToolsMapTellsWhichRoutesAToolAllowsInTheBackground(t *testing.T) {
	di, _ := newUploadDI(t)
	for _, q := range []string{
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at) VALUES ('t2', 'browser', 'B', '1.0.0', 'backend_only', 1, 'running', '/x', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at) VALUES ('t3', 'career', 'C', '1.0.0', 'backend_only', 0, 'stopped', '/x', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope, allow_background) VALUES ('t2', 'POST', 'list-links', 's', 1)`,
		`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope, allow_background) VALUES ('t2', 'POST', 'login-window', 's', 0)`,
		`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope) VALUES ('t3', 'GET', 'jobs', 's')`,
	} {
		if _, err := di.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := installedToolsMap(tool_query.NewToolQueryRepo(di.db))
	if err != nil {
		t.Fatal(err)
	}
	b := got["browser"]
	if !b.Enabled || len(b.BackgroundRoutes) != 1 {
		t.Fatalf("%+v", b)
	}
	if _, ok := b.BackgroundRoutes["POST list-links"]; !ok {
		t.Errorf("%v", b.BackgroundRoutes)
	}
	if _, ok := b.BackgroundRoutes["POST login-window"]; ok {
		t.Error("a route that is not marked is not offered")
	}
	if c := got["career"]; c.Enabled || len(c.BackgroundRoutes) != 0 {
		t.Errorf("%+v", c)
	}
}

func TestTheRouteFlagTravelsFromTheManifestIntoTheStoredRoute(t *testing.T) {
	m := manifest.Manifest{Slug: "browser", Routes: []manifest.RouteDecl{
		{Method: "POST", PathSuffix: "list-links", RequiredScope: "s", AllowBackground: true},
		{Method: "POST", PathSuffix: "login-window", RequiredScope: "s"},
	}}
	_, routes, _, _ := childRowsFromManifest(m)
	if len(routes) != 2 || !routes[0].AllowBackground || routes[1].AllowBackground {
		t.Fatalf("%+v", routes)
	}
	di, _ := newUploadDI(t)
	tool := &tool_entity.Tool{ID: "t9", Slug: "browser", Name: "B", Version: "1.0.0", Kind: "backend_only", Enabled: true, Status: "running", InstallPath: "/x"}
	if err := tool_persistent.NewToolPersistentRepo(di.db).InsertFresh(tool, nil, routes, nil); err != nil {
		t.Fatal(err)
	}
	q := tool_query.NewToolQueryRepo(di.db)
	if r, err := q.FindRoute("t9", "POST", "list-links"); err != nil || !r.AllowBackground {
		t.Errorf("%+v %v", r, err)
	}
	if r, err := q.FindRoute("t9", "POST", "login-window"); err != nil || r.AllowBackground {
		t.Errorf("%+v %v", r, err)
	}
}
