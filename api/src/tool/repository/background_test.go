package repository_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	"github.com/a-digi/cinqo/src/tool/repository/persistent"
	"github.com/a-digi/cinqo/src/tool/repository/query"
)

func openDB(t *testing.T) *sql.DB {
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
		raw, _ := os.ReadFile(f)
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

func addTool(t *testing.T, db *sql.DB, id, slug string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at) VALUES (?, ?, ?, '1.0.0', 'backend_only', 1, 'running', '/x', '2026-11-15T00:00:00Z')`, id, slug, slug); err != nil {
		t.Fatal(err)
	}
}

func req(target, method, path string) tool_entity.ToolBackgroundRequest {
	return tool_entity.ToolBackgroundRequest{TargetSlug: target, Method: method, PathSuffix: path}
}

func TestRequestsAreReplacedWholesaleAndApprovalsNeverAppearByThemselves(t *testing.T) {
	db := openDB(t)
	addTool(t, db, "t1", "namako")
	p, q := persistent.NewToolBackgroundPersistentRepo(db), query.NewToolBackgroundQueryRepo(db)
	if err := p.ReplaceRequests("t1", []tool_entity.ToolBackgroundRequest{req("browser", "POST", "list-links"), req("browser", "POST", "fetch-frames"), req("browser", "POST", "list-links")}); err != nil {
		t.Fatal(err)
	}
	got, _ := q.Requests("t1")
	if len(got) != 2 || got[0].PathSuffix != "fetch-frames" || got[1].PathSuffix != "list-links" || got[0].ToolID != "t1" {
		t.Errorf("a duplicate is stored once, in a fixed order: %+v", got)
	}
	if g, _ := q.Grants("t1"); len(g) != 0 {
		t.Errorf("a request is not an approval: %+v", g)
	}
	if ok, _ := q.IsGranted("t1", "browser", "POST", "list-links"); ok {
		t.Error("nothing is approved yet")
	}
	if other, _ := q.Requests("t2"); len(other) != 0 {
		t.Errorf("%+v", other)
	}
}

func TestApprovalsAnswerRequestsAndAnUpdateDropsWhatIsNoLongerAsked(t *testing.T) {
	db := openDB(t)
	addTool(t, db, "t1", "namako")
	p, q := persistent.NewToolBackgroundPersistentRepo(db), query.NewToolBackgroundQueryRepo(db)
	clock := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	p.SetClock(func() time.Time { return clock })
	_ = p.ReplaceRequests("t1", []tool_entity.ToolBackgroundRequest{req("browser", "POST", "list-links"), req("browser", "POST", "fetch-frames")})
	if err := p.Approve("t1", req("browser", "POST", "list-links"), "andet"); err != nil {
		t.Fatal(err)
	}
	if err := p.Approve("t1", req("browser", "POST", "list-links"), "someone-else"); err != nil { // twice: nothing changes
		t.Fatal(err)
	}
	g, _ := q.Grants("t1")
	if len(g) != 1 || g[0].ApprovedBy != "andet" || g[0].ApprovedAt != "2026-11-15T10:00:00Z" || g[0].TargetSlug != "browser" {
		t.Errorf("%+v", g)
	}
	if ok, _ := q.IsGranted("t1", "browser", "POST", "list-links"); !ok {
		t.Error("approved")
	}
	for name, c := range map[string]struct{ target, method, path string }{
		"another method": {"browser", "GET", "list-links"}, "another path": {"browser", "POST", "list-link"}, "another target": {"pdf", "POST", "list-links"},
	} {
		if ok, _ := q.IsGranted("t1", c.target, c.method, c.path); ok {
			t.Errorf("%s must not be granted", name)
		}
	}
	if ok, _ := q.IsGranted("t2", "browser", "POST", "list-links"); ok {
		t.Error("another tool holds no such approval")
	}
	if err := p.Approve("t1", req("browser", "POST", "something-else"), "andet"); !errors.Is(err, persistent.ErrNotRequested) {
		t.Errorf("an approval can only answer a request: %v", err)
	}
	if err := p.Approve("t1", req("browser", "POST", "fetch-frames"), "  "); err == nil {
		t.Error("an approval needs a person")
	}
	// an update that asks for something else: the approval of the route it dropped goes, a new request is not approved
	_ = p.ReplaceRequests("t1", []tool_entity.ToolBackgroundRequest{req("browser", "POST", "fetch-frames"), req("browser", "POST", "capture-network")})
	if ok, _ := q.IsGranted("t1", "browser", "POST", "list-links"); ok {
		t.Error("an update that stops asking for a route loses its approval")
	}
	if g, _ := q.Grants("t1"); len(g) != 0 {
		t.Errorf("%+v", g)
	}
	_ = p.Approve("t1", req("browser", "POST", "fetch-frames"), "andet")
	_ = p.ReplaceRequests("t1", []tool_entity.ToolBackgroundRequest{req("browser", "POST", "fetch-frames"), req("browser", "POST", "capture-network"), req("browser", "POST", "extra")})
	if g, _ := q.Grants("t1"); len(g) != 1 || g[0].PathSuffix != "fetch-frames" {
		t.Errorf("an update that asks for more keeps what was approved and approves nothing new: %+v", g)
	}
	_ = p.ReplaceRequests("t1", nil)
	if r, _ := q.Requests("t1"); len(r) != 0 {
		t.Errorf("%+v", r)
	}
	if g, _ := q.Grants("t1"); len(g) != 0 {
		t.Errorf("a tool that asks for nothing holds nothing: %+v", g)
	}
}

func TestRevokeTakesOneApprovalBack(t *testing.T) {
	db := openDB(t)
	addTool(t, db, "t1", "namako")
	p, q := persistent.NewToolBackgroundPersistentRepo(db), query.NewToolBackgroundQueryRepo(db)
	_ = p.ReplaceRequests("t1", []tool_entity.ToolBackgroundRequest{req("browser", "POST", "a"), req("browser", "POST", "b")})
	_ = p.Approve("t1", req("browser", "POST", "a"), "u")
	_ = p.Approve("t1", req("browser", "POST", "b"), "u")
	if ok, err := p.Revoke("t1", req("browser", "POST", "a")); err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if ok, _ := p.Revoke("t1", req("browser", "POST", "a")); ok {
		t.Error("taking back what is gone says so")
	}
	if g, _ := q.Grants("t1"); len(g) != 1 || g[0].PathSuffix != "b" {
		t.Errorf("only that one: %+v", g)
	}
	if r, _ := q.Requests("t1"); len(r) != 2 {
		t.Errorf("the request stays, only the approval goes: %+v", r)
	}
}

func TestDeletingAToolRemovesItsRequestsItsApprovalsAndApprovalsOthersHoldForIt(t *testing.T) {
	db := openDB(t)
	addTool(t, db, "t1", "namako")
	addTool(t, db, "t2", "browser")
	addTool(t, db, "t3", "career")
	p, q := persistent.NewToolBackgroundPersistentRepo(db), query.NewToolBackgroundQueryRepo(db)
	for _, c := range []struct{ tool, target string }{{"t1", "browser"}, {"t3", "browser"}, {"t3", "pdf"}} {
		_ = p.ReplaceRequests(c.tool, append(mustReqs(t, q, c.tool), req(c.target, "POST", "x")))
		_ = p.Approve(c.tool, req(c.target, "POST", "x"), "u")
	}
	if err := persistent.NewToolPersistentRepo(db).Delete("t2"); err != nil { // the browser goes
		t.Fatal(err)
	}
	if g, _ := q.Grants("t1"); len(g) != 0 {
		t.Errorf("approvals for a deleted tool's routes must go: %+v", g)
	}
	if r, _ := q.Requests("t1"); len(r) != 0 {
		t.Errorf("so must the requests for it: %+v", r)
	}
	if g, _ := q.Grants("t3"); len(g) != 1 || g[0].TargetSlug != "pdf" {
		t.Errorf("what concerns other tools stays: %+v", g)
	}
	if err := persistent.NewToolPersistentRepo(db).Delete("t3"); err != nil {
		t.Fatal(err)
	}
	if g, _ := q.Grants("t3"); len(g) != 0 {
		t.Errorf("the tool's own approvals go with it: %+v", g)
	}
	if r, _ := q.Requests("t3"); len(r) != 0 {
		t.Errorf("%+v", r)
	}
	// a tool installed again under the same slug starts with no approvals
	addTool(t, db, "t4", "browser")
	if ok, _ := q.IsGranted("t1", "browser", "POST", "x"); ok {
		t.Error("a new browser inherits nothing")
	}
}

func mustReqs(t *testing.T, q *query.ToolBackgroundQueryRepo, tool string) []tool_entity.ToolBackgroundRequest {
	t.Helper()
	r, err := q.Requests(tool)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestARouteRemembersWhetherItMayBeCalledInTheBackground(t *testing.T) {
	db := openDB(t)
	addTool(t, db, "t2", "browser")
	if _, err := db.Exec(`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope) VALUES ('t2', 'POST', 'plain', 's')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope, allow_background) VALUES ('t2', 'POST', 'list-links', 's', 1)`); err != nil {
		t.Fatal(err)
	}
	q := query.NewToolQueryRepo(db)
	plain, err := q.FindRoute("t2", "POST", "plain")
	if err != nil || plain.AllowBackground {
		t.Errorf("a route is not background by default: %+v %v", plain, err)
	}
	bg, err := q.FindRoute("t2", "POST", "list-links")
	if err != nil || !bg.AllowBackground {
		t.Errorf("%+v %v", bg, err)
	}
	by, err := q.BackgroundRoutesBySlug()
	if err != nil || len(by) != 1 || len(by["browser"]) != 1 {
		t.Fatalf("%v %v", by, err)
	}
	if _, ok := by["browser"]["POST list-links"]; !ok {
		t.Errorf("%v", by)
	}
}

func TestCallsAndRefusalsAreCountedPerToolOrganisationAndUTCDay(t *testing.T) {
	db := openDB(t)
	p, q := persistent.NewToolBackgroundPersistentRepo(db), query.NewToolBackgroundQueryRepo(db)
	clock := time.Date(2026, 11, 15, 23, 59, 59, 0, time.UTC)
	p.SetClock(func() time.Time { return clock })
	for i := 0; i < 3; i++ {
		if err := p.CountCall("t1", "org-a", false); err != nil {
			t.Fatal(err)
		}
	}
	_ = p.CountCall("t1", "org-a", true)
	_ = p.CountCall("t1", "org-b", false)
	_ = p.CountCall("t2", "org-a", false)
	clock = clock.Add(time.Second) // midnight UTC: a new day, a new row
	_ = p.CountCall("t1", "org-a", false)
	_ = p.CountCall("t1", "", true) // refused with no usable organisation
	got, err := q.Usage("t1")
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		day, org      string
		calls, denied int
	}
	var rows []row
	for _, u := range got {
		rows = append(rows, row{u.Day, u.OrgID, u.Calls, u.Denied})
	}
	want := []row{{"2026-11-16", "", 0, 1}, {"2026-11-16", "org-a", 1, 0}, {"2026-11-15", "org-a", 3, 1}, {"2026-11-15", "org-b", 1, 0}}
	if len(rows) != len(want) {
		t.Fatalf("%+v", rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d: %+v, want %+v (newest day first)", i, rows[i], want[i])
		}
	}
	if other, _ := q.Usage("t2"); len(other) != 1 || other[0].Calls != 1 {
		t.Errorf("%+v", other)
	}
	if none, _ := q.Usage("nobody"); none == nil || len(none) != 0 {
		t.Errorf("%+v", none)
	}
}

func TestUsageSinceListsTheCountersFromADayOnNewestFirstWithTheToolNamed(t *testing.T) {
	db := openDB(t)
	addTool(t, db, "t1", "namako")
	addTool(t, db, "t2", "career")
	p, q := persistent.NewToolBackgroundPersistentRepo(db), query.NewToolBackgroundQueryRepo(db)
	clock := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	p.SetClock(func() time.Time { return clock })
	_ = p.CountCall("t1", "org-b", false)
	_ = p.CountCall("t1", "org-a", true)
	_ = p.CountCall("t2", "org-a", false)
	_ = p.CountCall("gone", "org-a", false)
	clock = clock.Add(24 * time.Hour)
	_ = p.CountCall("t1", "org-a", false)
	got, err := q.UsageSince("2026-11-11")
	if err != nil || len(got) != 1 || got[0].ToolSlug != "namako" || got[0].ToolName != "namako" || got[0].Day != "2026-11-11" || got[0].Calls != 1 {
		t.Fatalf("a day is inclusive and earlier days are left out: %+v %v", got, err)
	}
	all, _ := q.UsageSince("2026-11-10")
	var lines []string
	for _, u := range all {
		lines = append(lines, u.Day+" "+u.ToolSlug+" "+u.OrgID)
	}
	want := []string{"2026-11-11 namako org-a", "2026-11-10 career org-a", "2026-11-10 gone org-a", "2026-11-10 namako org-a", "2026-11-10 namako org-b"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Errorf("%v\nwant %v", lines, want)
	}
	if none, err := q.UsageSince("2030-01-01"); err != nil || none == nil || len(none) != 0 {
		t.Errorf("%+v %v", none, err)
	}
}
