package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/a-digi/coco-server/server/request"
)

// Plan dependency-install, step 9: disabling or deleting a tool that enabled tools need is refused with their names, unless the person confirmed (force).
func TestDisableAndDeleteAreRefusedWhileEnabledToolsNeedTheTool(t *testing.T) {
	di, dir := newUploadDI(t)
	di.values["core_port"] = 7026
	base := `"authors":[{"name":"A"}],"license":"MIT"`
	for _, m := range []string{
		`{"slug":"browser","name":"Browser","version":"1.0.0",` + base + `}`,
		`{"slug":"career","name":"Career","version":"1.0.0",` + base + `,"requires_tools":[{"slug":"browser"}]}`,
	} {
		if code, out := upload(t, di, zipOf(t, map[string]string{"manifest.json": m, "frontend/bundle.js": "x"})); code != http.StatusCreated {
			t.Fatalf("%d %v", code, out)
		}
	}
	call := func(h func(request.RequestContext), method, slug, query string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		ctx := request.NewContext(rec, httptest.NewRequest(method, "/api/v1/tools/"+slug+query, nil), di)
		ctx.GetURI().SetPathVariable("slug", slug)
		h(ctx)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	enabled := func(slug string) bool {
		var n int
		di.db.QueryRow(`SELECT enabled FROM tools WHERE slug = ?`, slug).Scan(&n)
		return n == 1
	}
	exists := func(slug string) bool {
		var n int
		di.db.QueryRow(`SELECT COUNT(*) FROM tools WHERE slug = ?`, slug).Scan(&n)
		return n == 1
	}

	for name, h := range map[string]func(request.RequestContext){"disable": DisableHandler, "delete": DeleteHandler} {
		method := "POST"
		if name == "delete" {
			method = "DELETE"
		}
		code, out := call(h, method, "browser", "")
		msg, _ := out["message"].(map[string]any)
		if code != http.StatusConflict || msg["code"] != "has_dependents" {
			t.Fatalf("%s: %d %v", name, code, out)
		}
		deps := msg["dependents"].([]any)
		if len(deps) != 1 || deps[0].(map[string]any)["name"] != "Career" || deps[0].(map[string]any)["slug"] != "career" {
			t.Fatalf("%s lists Career: %v", name, deps)
		}
		if !enabled("browser") || !exists("browser") {
			t.Fatalf("%s: a refused request changes nothing", name)
		}
	}
	// a tool that nothing needs is not guarded, and the one that needs others is not either
	if code, _ := call(DisableHandler, "POST", "career", ""); code != http.StatusOK || enabled("career") {
		t.Fatalf("career can be disabled: %d", code)
	}
	// a disabled dependent no longer counts
	if code, out := call(DisableHandler, "POST", "browser", ""); code != http.StatusOK || enabled("browser") {
		t.Fatalf("a disabled dependent does not hold it: %d %v", code, out)
	}
	// enable both again; force goes through after the list was seen
	for _, s := range []string{"browser", "career"} {
		if code, out := call(EnableHandler, "POST", s, ""); code != http.StatusOK {
			t.Fatalf("%d %v", code, out)
		}
	}
	if code, _ := call(DisableHandler, "POST", "browser", "?force=true"); code != http.StatusOK || enabled("browser") {
		t.Fatalf("force disables: %d", code)
	}
	if code, _ := call(DeleteHandler, "DELETE", "browser", "?force=true"); code != http.StatusOK && code != http.StatusNoContent || exists("browser") {
		t.Fatalf("force deletes: %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "tools", "browser")); err == nil {
		t.Error("the files are gone")
	}
}
