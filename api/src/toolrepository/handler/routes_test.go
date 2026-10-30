package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Every tool-repository route must be authenticated and require tool:manage or
// cinqo:super:admin — reads included — and its executor must be wired in routes.go.
func TestEveryRouteRequiresToolManage(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "routes", "route-toolrepository.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Routes []struct {
			Path     string   `yaml:"path"`
			Method   string   `yaml:"method"`
			Executor string   `yaml:"executor"`
			Security string   `yaml:"security"`
			Scopes   []string `yaml:"scopes"`
		} `yaml:"routes"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Routes) != 6 {
		t.Fatalf("expected 6 routes, got %d", len(doc.Routes))
	}
	routesGo, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "routes", "routes.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range doc.Routes {
		id := r.Method + " " + r.Path
		if !strings.HasPrefix(r.Path, "/api/v1/tool-repositories") {
			t.Errorf("%s: unexpected path", id)
		}
		if r.Security != "authenticated" {
			t.Errorf("%s: security %q", id, r.Security)
		}
		if strings.Join(r.Scopes, ",") != "tool:manage,cinqo:super:admin" {
			t.Errorf("%s: scopes %v", id, r.Scopes)
		}
		if !strings.Contains(string(routesGo), `"`+r.Executor+`"`) {
			t.Errorf("%s: executor %s not registered in routes.go", id, r.Executor)
		}
	}
}
