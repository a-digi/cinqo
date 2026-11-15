package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/a-digi/coco-server/server/request"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
	"github.com/a-digi/cinqo/src/tool/systemtools"
)

// seedBackground installs (in the database only) a browser that allows two routes and does not allow a third, and a namako that asks for three of them
// plus one the browser never had.
func seedBackground(t *testing.T) *sql.DB {
	t.Helper()
	di, _ := newUploadDI(t)
	db := di.db
	for _, q := range []string{
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at) VALUES ('tb', 'browser', 'Browser', '1.0.0', 'backend_only', 1, 'running', '/x', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at) VALUES ('tn', 'namako', 'Namako', '1.0.0', 'backend_only', 1, 'running', '/x', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope, allow_background) VALUES ('tb', 'POST', 'list-links', 's', 1)`,
		`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope, allow_background) VALUES ('tb', 'POST', 'fetch-frames', 's', 1)`,
		`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope, allow_background) VALUES ('tb', 'POST', 'login-window', 's', 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	reqs := []tool_entity.ToolBackgroundRequest{
		{TargetSlug: "browser", Method: "POST", PathSuffix: "list-links"},
		{TargetSlug: "browser", Method: "POST", PathSuffix: "fetch-frames"},
		{TargetSlug: "browser", Method: "POST", PathSuffix: "login-window"}, // asked, but the browser never allowed it
		{TargetSlug: "ghost", Method: "GET", PathSuffix: "x"},               // a tool that is not installed
	}
	if err := tool_persistent.NewToolBackgroundPersistentRepo(db).ReplaceRequests("tn", reqs); err != nil {
		t.Fatal(err)
	}
	return db
}

func route(target, method, path string) backgroundRoute {
	return backgroundRoute{TargetSlug: target, Method: method, PathSuffix: path}
}

func TestTheListShowsEachAskWithItsApprovalAndWhetherTheTargetStillAllowsIt(t *testing.T) {
	db := seedBackground(t)
	items, err := backgroundItems(db, "namako")
	if err != nil || len(items) != 4 {
		t.Fatalf("%+v %v", items, err)
	}
	by := map[string]backgroundItem{}
	for _, it := range items {
		by[it.TargetSlug+"/"+it.PathSuffix] = it
	}
	if it := by["browser/list-links"]; !it.AllowedNow || it.Approved || it.TargetName != "Browser" || it.Method != "POST" {
		t.Errorf("%+v", it)
	}
	if it := by["browser/login-window"]; it.AllowedNow {
		t.Errorf("the browser never allowed it: %+v", it)
	}
	if it := by["ghost/x"]; it.AllowedNow || it.TargetName != "ghost" {
		t.Errorf("a tool that is not installed is shown by its slug and allows nothing: %+v", it)
	}
	if code, msg := applyBackground(db, "namako", "andet", true, []backgroundRoute{route("browser", "POST", "list-links")}); code != 0 {
		t.Fatalf("%d %s", code, msg)
	}
	items, _ = backgroundItems(db, "namako")
	for _, it := range items {
		if it.PathSuffix == "list-links" && (!it.Approved || it.ApprovedBy != "andet" || it.ApprovedAt == "") {
			t.Errorf("%+v", it)
		}
		if it.PathSuffix != "list-links" && it.Approved {
			t.Errorf("only that one: %+v", it)
		}
	}
	if _, err := backgroundItems(db, "nobody"); err != errUnknownTool {
		t.Errorf("%v", err)
	}
	if empty, err := backgroundItems(db, "browser"); err != nil || len(empty) != 0 || empty == nil {
		t.Errorf("a tool that asks for nothing has an empty list (not null): %+v %v", empty, err)
	}
}

func approvedCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	g, err := tool_query.NewToolBackgroundQueryRepo(db).Grants("tn")
	if err != nil {
		t.Fatal(err)
	}
	return len(g)
}

func TestApprovingOrRevokingIsAllOrNothingAndRefusesWhatIsNotAskedOrNotAllowed(t *testing.T) {
	db := seedBackground(t)
	good := route("browser", "POST", "list-links")
	for name, c := range map[string]struct {
		routes  []backgroundRoute
		slug    string
		want    int
		contain string
	}{
		"nothing named":             {nil, "namako", http.StatusBadRequest, "at least one"},
		"unknown tool":              {[]backgroundRoute{good}, "nobody", http.StatusNotFound, "not found"},
		"a route that is not asked": {[]backgroundRoute{good, route("browser", "POST", "capture")}, "namako", http.StatusBadRequest, "does not ask"},
		"the wrong method":          {[]backgroundRoute{route("browser", "GET", "list-links")}, "namako", http.StatusBadRequest, "does not ask"},
		"asked but not allowed":     {[]backgroundRoute{good, route("browser", "POST", "login-window")}, "namako", http.StatusConflict, "does not allow background use of POST login-window"},
		"a tool that is gone":       {[]backgroundRoute{good, route("ghost", "GET", "x")}, "namako", http.StatusConflict, "ghost does not allow"},
	} {
		code, msg := applyBackground(db, c.slug, "andet", true, c.routes)
		if code != c.want || !strings.Contains(msg, c.contain) {
			t.Errorf("%s: %d %q", name, code, msg)
		}
		if n := approvedCount(t, db); n != 0 {
			t.Errorf("%s: a refused call approved %d routes, it must change nothing", name, n)
		}
	}
	tooMany := make([]backgroundRoute, maxBackgroundCall+1)
	for i := range tooMany {
		tooMany[i] = good
	}
	if code, _ := applyBackground(db, "namako", "andet", true, tooMany); code != http.StatusBadRequest {
		t.Errorf("%d", code)
	}
	exact := make([]backgroundRoute, maxBackgroundCall)
	for i := range exact {
		exact[i] = good
	}
	if code, msg := applyBackground(db, "namako", "andet", true, exact); code != 0 {
		t.Errorf("exactly the limit is fine: %d %s", code, msg)
	}
	// two good routes together, twice (idempotent)
	both := []backgroundRoute{good, route("browser", "POST", "fetch-frames")}
	for i := 0; i < 2; i++ {
		if code, msg := applyBackground(db, "namako", "andet", true, both); code != 0 {
			t.Fatalf("%d %s", code, msg)
		}
	}
	if n := approvedCount(t, db); n != 2 {
		t.Errorf("%d approvals", n)
	}
	// revoking: a route not asked is refused and nothing changes; one that is asked is taken back
	if code, _ := applyBackground(db, "namako", "andet", false, []backgroundRoute{good, route("browser", "POST", "capture")}); code != http.StatusBadRequest || approvedCount(t, db) != 2 {
		t.Errorf("%d %d", code, approvedCount(t, db))
	}
	if code, msg := applyBackground(db, "namako", "andet", false, []backgroundRoute{good}); code != 0 {
		t.Fatalf("%d %s", code, msg)
	}
	if n := approvedCount(t, db); n != 1 {
		t.Errorf("%d approvals after one revoke", n)
	}
	// the browser stops allowing the second route: it can still be taken back, and cannot be approved again
	if _, err := db.Exec(`UPDATE tool_routes SET allow_background = 0 WHERE path_suffix = 'fetch-frames'`); err != nil {
		t.Fatal(err)
	}
	if code, _ := applyBackground(db, "namako", "andet", true, []backgroundRoute{route("browser", "POST", "fetch-frames")}); code != http.StatusConflict {
		t.Errorf("%d", code)
	}
	if code, msg := applyBackground(db, "namako", "andet", false, []backgroundRoute{route("browser", "POST", "fetch-frames")}); code != 0 {
		t.Errorf("a revoke needs no permission of the target: %d %s", code, msg)
	}
	if n := approvedCount(t, db); n != 0 {
		t.Errorf("%d", n)
	}
}

func TestTheHTTPHandlersListAndRefuseACallerWithoutAValidToken(t *testing.T) {
	di, _ := newUploadDI(t)
	for _, q := range []string{
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at) VALUES ('tn', 'namako', 'Namako', '1.0.0', 'backend_only', 1, 'running', '/x', '2026-11-15T00:00:00Z')`,
	} {
		if _, err := di.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	_ = tool_persistent.NewToolBackgroundPersistentRepo(di.db).ReplaceRequests("tn", []tool_entity.ToolBackgroundRequest{{TargetSlug: "browser", Method: "POST", PathSuffix: "list-links"}})
	call := func(h func(request.RequestContext), method, slug, body string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		ctx := request.NewContext(rec, httptest.NewRequest(method, "/x", strings.NewReader(body)), di)
		ctx.GetURI().SetPathVariable("slug", slug)
		h(ctx)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	code, out := call(BackgroundListHandler, "GET", "namako", "")
	items := out["message"].(map[string]any)["items"].([]any)
	if code != 200 || len(items) != 1 || items[0].(map[string]any)["target_slug"] != "browser" || items[0].(map[string]any)["approved"] != false {
		t.Fatalf("%d %v", code, out)
	}
	if code, _ := call(BackgroundListHandler, "GET", "nobody", ""); code != http.StatusNotFound {
		t.Errorf("%d", code)
	}
	body := `{"routes":[{"target_slug":"browser","method":"POST","path_suffix":"list-links"}]}`
	for name, h := range map[string]func(request.RequestContext){"approve": BackgroundApproveHandler, "revoke": BackgroundRevokeHandler} {
		if code, _ := call(h, "POST", "namako", body); code != http.StatusUnauthorized {
			t.Errorf("%s without a token: %d", name, code)
		}
	}
	if approvedCount(t, di.db) != 0 {
		t.Error("nothing was approved")
	}
}

func TestTheToolListCarriesTheSummaryOnlyForAToolThatAsks(t *testing.T) {
	db := seedBackground(t)
	_ = applyBackgroundOK(t, db)
	tools, err := tool_query.NewToolQueryRepo(db).List()
	if err != nil {
		t.Fatal(err)
	}
	out, err := toToolResponses(db, tools, systemtools.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range out {
		switch r.Slug {
		case "namako":
			if r.Background == nil || r.Background.Requested != 4 || r.Background.Approved != 1 {
				t.Errorf("%+v", r.Background)
			}
		case "browser":
			if r.Background != nil {
				t.Errorf("a tool that asks for nothing has no summary: %+v", r.Background)
			}
		}
	}
	raw, _ := json.Marshal(out)
	if strings.Count(string(raw), `"background"`) != 1 {
		t.Errorf("the key is left out when empty: %s", raw)
	}
}

func applyBackgroundOK(t *testing.T, db *sql.DB) int {
	t.Helper()
	code, msg := applyBackground(db, "namako", "andet", true, []backgroundRoute{route("browser", "POST", "list-links")})
	if code != 0 {
		t.Fatalf("%d %s", code, msg)
	}
	return code
}

func TestTheRoutesAreDeclaredWithTheToolManageScopeAndRegistered(t *testing.T) {
	yaml, err := os.ReadFile("../../../config/routes/route-tool.yaml")
	if err != nil {
		t.Fatal(err)
	}
	routesGo, _ := os.ReadFile("../../../config/routes/routes.go")
	for path, want := range map[string][2]string{
		"/api/v1/tools/{slug}/background":         {"GET", "ToolBackgroundList"},
		"/api/v1/tools/{slug}/background/approve": {"POST", "ToolBackgroundApprove"},
		"/api/v1/tools/{slug}/background/revoke":  {"POST", "ToolBackgroundRevoke"},
	} {
		i := strings.Index(string(yaml), "path: "+path+"\n")
		if i < 0 {
			t.Errorf("%s is not declared", path)
			continue
		}
		block := string(yaml)[i:]
		if j := strings.Index(block[1:], "- path:"); j >= 0 {
			block = block[:j+1]
		}
		if !strings.Contains(block, "method: "+want[0]) || !strings.Contains(block, "executor: "+want[1]) || !strings.Contains(block, "security: authenticated") ||
			!strings.Contains(block, "- tool:manage") || !strings.Contains(block, "- cinqo:super:admin") {
			t.Errorf("%s: %s", path, block)
		}
		if !strings.Contains(string(routesGo), `"`+want[1]+`":`) {
			t.Errorf("%s is declared but not registered", want[1])
		}
	}
}

func TestTheCallLimitIsTwoHundredRoutes(t *testing.T) {
	if maxBackgroundCall != 200 {
		t.Errorf("%d", maxBackgroundCall)
	}
}

// The approve route approves and the revoke route revokes: a wrong flag could not be seen without a token, so the two calls are read from the source.
func TestTheTwoHTTPHandlersPassTheRightFlag(t *testing.T) {
	raw, err := os.ReadFile("background_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	approve := src[strings.Index(src, "func BackgroundApproveHandler"):strings.Index(src, "func BackgroundRevokeHandler")]
	revoke := src[strings.Index(src, "func BackgroundRevokeHandler"):strings.Index(src, "func changeBackground")]
	if !strings.Contains(approve, "changeBackground(reqCtx, sub, true)") || strings.Contains(approve, "false)") {
		t.Errorf("approve: %s", approve)
	}
	if !strings.Contains(revoke, "changeBackground(reqCtx, sub, false)") || strings.Contains(revoke, ", true)") {
		t.Errorf("revoke: %s", revoke)
	}
	for _, h := range []string{approve, revoke} {
		if !strings.Contains(h, "callerIdentity(reqCtx)") || !strings.Contains(h, "http.StatusUnauthorized") || !strings.Contains(h, `sub == ""`) {
			t.Errorf("a caller without a valid identity must be refused: %s", h)
		}
	}
}

func TestChangeBackgroundAnswersTheWholeListAfterApprovingAndRevoking(t *testing.T) {
	db := seedBackground(t)
	run := func(approve bool, body string) (int, []any) {
		rec := httptest.NewRecorder()
		ctx := request.NewContext(rec, httptest.NewRequest("POST", "/x", strings.NewReader(body)), &uploadDI{db: db, values: map[string]any{}})
		ctx.GetURI().SetPathVariable("slug", "namako")
		changeBackground(ctx, "andet", approve)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		msg, _ := out["message"].(map[string]any)
		items, _ := msg["items"].([]any)
		return rec.Code, items
	}
	one := `{"routes":[{"target_slug":"browser","method":"POST","path_suffix":"list-links"}]}`
	code, items := run(true, one)
	approved := 0
	for _, it := range items {
		if it.(map[string]any)["approved"] == true {
			approved++
		}
	}
	if code != 200 || len(items) != 4 || approved != 1 {
		t.Fatalf("%d %d items, %d approved", code, len(items), approved)
	}
	code, items = run(false, one)
	for _, it := range items {
		if it.(map[string]any)["approved"] == true {
			t.Errorf("revoked: %v", it)
		}
	}
	if code != 200 {
		t.Errorf("%d", code)
	}
	if code, _ := run(true, `nope`); code != http.StatusBadRequest {
		t.Errorf("%d", code)
	}
	if code, _ := run(true, `{"routes":[{"target_slug":"browser","method":"POST","path_suffix":"login-window"}]}`); code != http.StatusConflict {
		t.Errorf("%d", code)
	}
}

func TestTheSummaryCountsEachToolsOwnApprovalsOnly(t *testing.T) {
	db := seedBackground(t)
	if _, err := db.Exec(`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at) VALUES ('tc', 'career', 'Career', '1.0.0', 'backend_only', 1, 'running', '/x', '2026-11-15T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	// career asks for the very route namako gets approved: its own count must stay 0
	if err := tool_persistent.NewToolBackgroundPersistentRepo(db).ReplaceRequests("tc", []tool_entity.ToolBackgroundRequest{{TargetSlug: "browser", Method: "POST", PathSuffix: "list-links"}}); err != nil {
		t.Fatal(err)
	}
	applyBackgroundOK(t, db)
	sum, err := tool_query.NewToolBackgroundQueryRepo(db).Summaries()
	if err != nil {
		t.Fatal(err)
	}
	if sum["tn"].Requested != 4 || sum["tn"].Approved != 1 || sum["tc"].Requested != 1 || sum["tc"].Approved != 0 || len(sum) != 2 {
		t.Errorf("%+v", sum)
	}
}
