package config

import (
	"sort"

	"gopkg.in/yaml.v3"
)

// routeNode mirrors just enough of route-*.yaml's shape (see
// routes.Init's merged output) to walk every nested route and collect its
// scopes: routes nest arbitrarily deep via children, and a scope declared
// on a parent has no bearing on its descendants, so every node in the tree
// is inspected independently.
type routeNode struct {
	Scopes   []string    `yaml:"scopes"`
	Children []routeNode `yaml:"children"`
}

type routeDoc struct {
	Routes []routeNode `yaml:"routes"`
}

var enforcedScopes []string

// SetEnforcedScopes parses the merged route YAML routes.Init() loads and
// records the deduplicated, sorted set of every scope any route declares.
// Called once from routes.Init() — see AllEnforcedScopes. Lives in this leaf
// package (not config/routes) so auth_handler can read it back without an
// import cycle (config/routes already imports auth_handler).
func SetEnforcedScopes(mergedYaml []byte) {
	var doc routeDoc
	if err := yaml.Unmarshal(mergedYaml, &doc); err != nil {
		enforcedScopes = nil
		return
	}

	seen := map[string]struct{}{}
	var walk func(nodes []routeNode)
	walk = func(nodes []routeNode) {
		for _, n := range nodes {
			for _, s := range n.Scopes {
				seen[s] = struct{}{}
			}
			walk(n.Children)
		}
	}
	walk(doc.Routes)

	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	enforcedScopes = out
}

// AllEnforcedScopes returns every scope declared across route-*.yaml, as
// captured at startup by routes.Init(). Used to detect drift against the
// static "scopes" string in config.json's auth block (see
// auth_config_handler.go) — a scope enforced here but absent from that
// string can never be requested via the OAuth authorize redirect, so no
// token can ever carry it.
func AllEnforcedScopes() []string {
	return enforcedScopes
}
