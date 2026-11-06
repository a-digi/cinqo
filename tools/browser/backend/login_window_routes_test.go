package main

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Plan 182, step 1: the four login-window routes are declared in the manifest with the login scope, and main.go serves each of them.
func TestTheLoginWindowRoutesAreDeclaredWithTheLoginScopeAndServed(t *testing.T) {
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []struct {
			Method        string `json:"method"`
			PathSuffix    string `json:"path_suffix"`
			RequiredScope string `json:"required_scope"`
		} `json:"routes"`
		MCPTools []struct {
			Name string `json:"name"`
		} `json:"mcp_tools"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"login-window/open":    "POST",
		"login-window/status":  "GET",
		"login-window/confirm": "POST",
		"login-window/cancel":  "POST",
	}
	seen := map[string]int{}
	for _, r := range m.Routes {
		method, ok := want[r.PathSuffix]
		if !ok {
			continue
		}
		seen[r.PathSuffix]++
		if r.Method != method {
			t.Errorf("%s declared as %s, want %s", r.PathSuffix, r.Method, method)
		}
		if r.RequiredScope != "tool:browser:login" {
			t.Errorf("%s has scope %q, want tool:browser:login", r.PathSuffix, r.RequiredScope)
		}
	}
	for path := range want {
		if seen[path] != 1 {
			t.Errorf("manifest.json declares %s %d times, want once", path, seen[path])
		}
	}
	// never an AI tool: the AI cannot open, confirm or end a login
	for _, tool := range m.MCPTools {
		if strings.Contains(tool.Name, "login_window") || strings.Contains(tool.Name, "login-window") {
			t.Errorf("%s must not be an MCP tool", tool.Name)
		}
	}

	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for path, handler := range map[string]string{
		"/login-window/open":    "auth.LoginWindowOpenHandler",
		"/login-window/status":  "auth.LoginWindowStatusHandler",
		"/login-window/confirm": "auth.LoginWindowConfirmHandler",
		"/login-window/cancel":  "auth.LoginWindowCancelHandler",
	} {
		re := regexp.MustCompile(`(?m)^\s*http\.HandleFunc\("` + regexp.QuoteMeta(path) + `", ` + regexp.QuoteMeta(handler) + `\)`)
		if !re.Match(src) {
			t.Errorf("main.go does not serve %s with %s (a commented-out line does not count)", path, handler)
		}
	}
	// the sweep at start and the close at shutdown are wired
	for _, needle := range []string{"shared.SweepLoginWindowFolders()", "auth.StopLoginWindows()"} {
		if !strings.Contains(string(src), needle) {
			t.Errorf("main.go does not call %s", needle)
		}
	}
	// the MCP server never registers a login-window tool
	if strings.Contains(string(src), "RegisterLoginWindow") {
		t.Error("main.go registers a login window MCP tool")
	}
}

// Plan 195, step 1: the saved-session routes are declared under the login scope only, and no MCP tool or route can create or read cookies.
func TestLoginSessionRoutesAreDeclaredUnderTheLoginScopeOnly(t *testing.T) {
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []struct {
			M string `json:"method"`
			P string `json:"path_suffix"`
			S string `json:"required_scope"`
		} `json:"routes"`
		MCPTools []struct {
			Name string `json:"name"`
		} `json:"mcp_tools"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"login-session/state": "GET", "login-session": "DELETE"}
	for _, r := range m.Routes {
		if strings.HasPrefix(r.P, "login-session") {
			if want[r.P] != r.M || r.S != "tool:browser:login" {
				t.Errorf("route %s %s has scope %q", r.M, r.P, r.S)
			}
			delete(want, r.P)
		}
	}
	if len(want) != 0 {
		t.Errorf("routes missing from manifest.json: %v", want)
	}
	for _, tool := range m.MCPTools {
		if strings.Contains(tool.Name, "session") || strings.Contains(tool.Name, "cookie") {
			t.Errorf("MCP tool %s would let the AI reach a login session", tool.Name)
		}
	}
}
