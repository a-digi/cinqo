package handler

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	routeFile   = "../../../config/routes/route-livelog.yaml"
	routerFile  = "../../../config/routes/routes.go"
	appFile     = "../../cinqo/app.go"
	composeFile = "../../cinqo/livelog.go"
)

// The route file and the executors the router registers must name exactly the same names: a route without a handler fails when the server
// starts, a handler without a route can never be reached.
func TestTheRouteFileAndTheRouterAgree(t *testing.T) {
	raw, err := os.ReadFile(routeFile)
	if err != nil {
		t.Fatal(err)
	}
	var inFile []string
	for _, m := range regexp.MustCompile(`(?m)^\s*executor:\s*(\S+)\s*$`).FindAllStringSubmatch(string(raw), -1) {
		inFile = append(inFile, m[1])
	}
	router, err := os.ReadFile(routerFile)
	if err != nil {
		t.Fatal(err)
	}
	var inRouter []string
	for _, m := range regexp.MustCompile(`"(LiveLogs\w+)":\s+local_routing\.HandlerFunc\(livelog_handler\.(\w+)\)`).FindAllStringSubmatch(string(router), -1) {
		inRouter = append(inRouter, m[1])
	}
	sort.Strings(inFile)
	sort.Strings(inRouter)
	if strings.Join(inFile, ",") != "LiveLogsPoll,LiveLogsProviders" || strings.Join(inFile, ",") != strings.Join(inRouter, ",") {
		t.Errorf("route file executors %v, router %v", inFile, inRouter)
	}
	for exec, fn := range map[string]string{"LiveLogsPoll": "PollHandler", "LiveLogsProviders": "ProvidersHandler"} {
		if !regexp.MustCompile(`"` + exec + `":\s+local_routing\.HandlerFunc\(livelog_handler\.` + fn + `\)`).Match(router) {
			t.Errorf("%s is not wired to %s", exec, fn)
		}
	}
}

// Both routes are authenticated GETs with no static scope: the provider's own scope is checked per request, inside the handler.
func TestEveryRouteIsAnAuthenticatedGetWithTheScopeCheckedPerProvider(t *testing.T) {
	raw, err := os.ReadFile(routeFile)
	if err != nil {
		t.Fatal(err)
	}
	blocks := strings.Split(string(raw), "  - path:")[1:]
	if len(blocks) != 2 {
		t.Fatalf("%d routes, want 2", len(blocks))
	}
	for _, b := range blocks {
		if !strings.Contains(b, "method: GET") || !strings.Contains(b, "security: authenticated") {
			t.Errorf("route must be an authenticated GET:\n%s", b)
		}
		if strings.Contains(b, "scopes:") || strings.Contains(b, "public") {
			t.Errorf("route must not carry a static scope (the provider's own is checked in the handler) nor be public:\n%s", b)
		}
	}
	if !strings.Contains(string(raw), "/api/v1/live-logs/providers") || !strings.Contains(string(raw), "/api/v1/live-logs/{provider}/poll") {
		t.Error("the paths changed")
	}
}

// Start must compose the module, and only through the composition root: one call in app.go, the providers named in livelog.go.
func TestStartComposesTheModuleThroughTheCompositionRoot(t *testing.T) {
	app, err := os.ReadFile(appFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(app), "configureLiveLogs(dataDir,") {
		t.Error("app.go does not call configureLiveLogs")
	}
	if strings.Contains(string(app), "src/livelog") {
		t.Error("app.go must not import the module: only the composition root does")
	}
	root, err := os.ReadFile(composeFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"livelog_providers.NewAITrace(filepath.Join(dataDir, \"conversations\"))",
		"livelog_providers.NewSystemLog(filepath.Join(dataDir, \"logs\"))",
		"livelog_handler.Configure(reg, logf)",
		"reg.Register(p)",
	} {
		if !strings.Contains(string(root), want) {
			t.Errorf("the composition root no longer contains %q", want)
		}
	}
}
