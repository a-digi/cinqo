package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// The whole install and update path with unattended use: the target marks routes, the asking tool can only ask for those, an install approves nothing, and an
// update keeps only the approvals that still answer a request.
func TestUnattendedUseThroughRealInstallsAndUpdates(t *testing.T) {
	di, _ := newUploadDI(t)
	di.values["core_port"] = 7030
	origStop, origStart := stopTool, startTool
	stopTool = func(*sql.DB, string) error { return nil }
	startTool = func(*sql.DB, string, tool_entity.Tool, int) error { return nil }
	t.Cleanup(func() { stopTool, startTool = origStop, origStart })

	const base = `"authors":[{"name":"A"}],"license":"MIT"`
	browser := func(version string, listAllowed bool) []byte {
		flag := ""
		if listAllowed {
			flag = `,"allow_background":true`
		}
		return zipOf(t, map[string]string{
			"manifest.json": `{"slug":"browser","name":"Browser","version":"` + version + `",` + base + `,"scopes":[{"scope":"tool:browser:crawl","description":"d"}],
				"routes":[{"method":"POST","path_suffix":"list-links","required_scope":"tool:browser:crawl"` + flag + `},{"method":"POST","path_suffix":"login-window","required_scope":"tool:browser:crawl"}]}`,
			"backend/tool": "x",
		})
	}
	namako := func(version, access string) []byte {
		extra := ""
		if access != "" {
			extra = `,"background_access":` + access
		}
		return zipOf(t, map[string]string{
			"manifest.json": `{"slug":"namako","name":"Namako","version":"` + version + `",` + base + `,"scopes":[{"scope":"tool:namako:a","description":"d"}],
				"requires_tools":[{"slug":"browser"}]` + extra + `}`,
			"backend/tool": "x",
		})
	}
	list := `[{"tool":"browser","routes":[{"method":"POST","path_suffix":"list-links"}]}]`

	if code, out := upload(t, di, namako("1.0.0", list)); code != http.StatusBadRequest {
		t.Fatalf("the target is not installed yet: %d %v", code, out)
	}
	if code, out := upload(t, di, browser("1.0.0", true)); code != http.StatusCreated {
		t.Fatalf("%d %v", code, out)
	}
	var bg, plain int
	_ = di.db.QueryRow(`SELECT allow_background FROM tool_routes WHERE path_suffix = 'list-links'`).Scan(&bg)
	_ = di.db.QueryRow(`SELECT allow_background FROM tool_routes WHERE path_suffix = 'login-window'`).Scan(&plain)
	if bg != 1 || plain != 0 {
		t.Fatalf("the flag is stored per route: %d %d", bg, plain)
	}
	login := `[{"tool":"browser","routes":[{"method":"POST","path_suffix":"login-window"}]}]`
	code, out := upload(t, di, namako("1.0.0", login))
	if code != http.StatusBadRequest || !strings.Contains(toText(out), "does not allow background use of POST login-window") {
		t.Fatalf("a route the target did not mark is refused with the reason: %d %v", code, out)
	}
	if code, out := upload(t, di, namako("1.0.0", list)); code != http.StatusCreated {
		t.Fatalf("%d %v", code, out)
	}
	var namakoID string
	_ = di.db.QueryRow(`SELECT id FROM tools WHERE slug = 'namako'`).Scan(&namakoID)
	q := tool_query.NewToolBackgroundQueryRepo(di.db)
	if r, _ := q.Requests(namakoID); len(r) != 1 || r[0].TargetSlug != "browser" || r[0].PathSuffix != "list-links" {
		t.Fatalf("the install stored the ask: %+v", r)
	}
	if g, _ := q.Grants(namakoID); len(g) != 0 {
		t.Fatalf("an install approves nothing: %+v", g)
	}
	if err := tool_persistent.NewToolBackgroundPersistentRepo(di.db).Approve(namakoID, tool_entity.ToolBackgroundRequest{TargetSlug: "browser", Method: "POST", PathSuffix: "list-links"}, "andet"); err != nil {
		t.Fatal(err)
	}

	if code, out := upload(t, di, namako("1.1.0", list)); code != http.StatusOK {
		t.Fatalf("%d %v", code, out)
	}
	if g, _ := q.Grants(namakoID); len(g) != 1 {
		t.Errorf("an update that asks for the same keeps the approval: %+v", g)
	}
	if code, out := upload(t, di, namako("1.2.0", "")); code != http.StatusOK {
		t.Fatalf("%d %v", code, out)
	}
	if r, _ := q.Requests(namakoID); len(r) != 0 {
		t.Errorf("an update that stops asking clears the ask: %+v", r)
	}
	if g, _ := q.Grants(namakoID); len(g) != 0 {
		t.Errorf("and the approval with it: %+v", g)
	}

	// the browser stops allowing the route: nobody can ask for it any more
	if code, out := upload(t, di, browser("1.1.0", false)); code != http.StatusOK {
		t.Fatalf("%d %v", code, out)
	}
	code, out = upload(t, di, namako("1.3.0", list))
	if code != http.StatusBadRequest || !strings.Contains(toText(out), "does not allow background use of POST list-links") {
		t.Errorf("%d %v", code, out)
	}
}

func toText(m map[string]any) string {
	var b strings.Builder
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			b.WriteString(x + " ")
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(m)
	return b.String()
}
