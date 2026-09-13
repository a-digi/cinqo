// Package scopes is the security engine's scope registry: the bridge
// through which this application, and any future plugin, declares the
// scopes it defines. Replaces api/config/scopes/*.json (dead — read by
// nothing at runtime, confirmed before this package was written) as the
// single source of truth. See plan/ai/security/security.md.
package scopes

import (
	"fmt"
	"sort"
	"sync"

	"github.com/a-digi/cinqo/src/security/scopes/entity"
)

// CoreScopeGroup is the security engine's own cross-cutting group —
// not owned by any single feature domain, unlike every other group
// (e.g. ping.ScopeGroup), which is why it lives here instead of in a
// domain package. Registered the same explicit way as any other group
// (see routes.Init) rather than auto-registering itself via init() —
// one registration style, not two.
var CoreScopeGroup = entity.ScopeGroup{
	ID:          "cinqo:super",
	Description: "Cross-cutting administrative scopes not tied to any single domain.",
	Scopes: []entity.Scope{
		{
			ID:          "cinqo:super:admin",
			Description: "Bypasses every cinqo:* scope check — full access to all cinqo resources and actions. Assign to very few identities.",
		},
	},
}

var (
	mu     sync.Mutex
	groups = map[string]entity.ScopeGroup{}
)

// Register adds a scope group to the registry. Called once per domain
// (or, later, per plugin) during routes.Init, before the server accepts
// any request — see plan/ai/security/security.md's "Where registration
// happens."
//
// Panics on a duplicate group ID: two domains registering the same
// group is always a programming error caught at startup, never runtime
// data to degrade gracefully around — same fail-fast reasoning this
// codebase already applies to a route with no declared scope.
func Register(group entity.ScopeGroup) {
	mu.Lock()
	defer mu.Unlock()

	if _, exists := groups[group.ID]; exists {
		panic(fmt.Sprintf("scopes: group %q already registered", group.ID))
	}
	groups[group.ID] = group
}

// Groups returns every registered group, sorted by ID.
func Groups() []entity.ScopeGroup {
	mu.Lock()
	defer mu.Unlock()

	out := make([]entity.ScopeGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AllScopeIDs returns every registered scope ID across every group,
// flattened, sorted, deduplicated.
func AllScopeIDs() []string {
	mu.Lock()
	defer mu.Unlock()

	seen := map[string]struct{}{}
	for _, g := range groups {
		for _, s := range g.Scopes {
			seen[s.ID] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
