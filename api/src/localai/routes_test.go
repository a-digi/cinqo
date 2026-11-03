package localai

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const routeFile = "../../config/routes/route-localai.yaml"

// The route file and the handler map must name exactly the same executors: a route without a handler fails when the
// server starts, a handler without a route can never be reached.
func TestTheRouteFileAndTheHandlerMapAgree(t *testing.T) {
	raw, err := os.ReadFile(routeFile)
	if err != nil {
		t.Fatal(err)
	}
	var inFile []string
	for _, m := range regexp.MustCompile(`(?m)^\s*executor:\s*(\S+)\s*$`).FindAllStringSubmatch(string(raw), -1) {
		inFile = append(inFile, m[1])
	}
	var inMap []string
	for name := range Handlers() {
		inMap = append(inMap, name)
	}
	sort.Strings(inFile)
	sort.Strings(inMap)
	if strings.Join(inFile, ",") != strings.Join(inMap, ",") {
		t.Errorf("route file executors %v, handler map %v", inFile, inMap)
	}
}

// Reading needs the platform read scope and every change the manage scope, and every route is authenticated.
func TestEveryRouteIsAuthenticatedAndScoped(t *testing.T) {
	raw, err := os.ReadFile(routeFile)
	if err != nil {
		t.Fatal(err)
	}
	blocks := strings.Split(string(raw), "  - path:")[1:]
	if len(blocks) != 5 {
		t.Fatalf("%d routes, want 5", len(blocks))
	}
	for _, b := range blocks {
		if !strings.Contains(b, "security: authenticated") {
			t.Errorf("route not authenticated:\n%s", b)
		}
		want := "cinqo:platform:read"
		if !strings.Contains(b, "method: GET") {
			want = "cinqo:platform:manage"
		}
		if !strings.Contains(b, want) || !strings.Contains(b, "cinqo:super:admin") {
			t.Errorf("route must require %s or super admin:\n%s", want, b)
		}
		if !strings.Contains(b, "method: GET") && strings.Contains(b, "cinqo:platform:read") {
			t.Errorf("a change must not be allowed with the read scope only:\n%s", b)
		}
	}
}
