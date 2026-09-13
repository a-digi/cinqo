// Package handler exposes the scope registry over HTTP for admins to
// review — see plan/ai/security/security.md.
package handler

import (
	"net/http"
	"sort"
	"strings"

	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/config"
	auth_config "github.com/a-digi/cinqo/src/auth/config"
	"github.com/a-digi/cinqo/src/security/scopes"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// diStore mirrors auth_handler's own diStore — Get isn't part of
// serverdi.Context, so every handler package needing it re-asserts to
// this local interface off reqCtx.GetDI(), same pattern used throughout
// this codebase (see api/src/auth/handler/utils.go's own comment).
type diStore interface {
	Get(key string) (any, bool)
}

type scopeView struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Enforced    bool   `json:"enforced"`
	Requested   bool   `json:"requested"`
}

type groupView struct {
	ID          string      `json:"id"`
	Description string      `json:"description"`
	Scopes      []scopeView `json:"scopes"`
	// Source and ToolEnabled are only ever set for a tool-contributed
	// group (see plan/ai/tools/step-06-scope-registry-integration.md) —
	// absent for every existing, static (Go-code-registered) group, so
	// the response shape is unchanged for a caller only looking at
	// those. ToolEnabled is a pointer specifically so a disabled tool's
	// real "false" value is actually present in the JSON rather than
	// omitted as a zero value.
	Source      string `json:"source,omitempty"`
	ToolEnabled *bool  `json:"tool_enabled,omitempty"`
}

type scopesResponse struct {
	Groups                     []groupView `json:"groups"`
	UnregisteredEnforcedScopes []string    `json:"unregistered_enforced_scopes"`
	UnrequestedScopes          []string    `json:"unrequested_scopes"`
}

// ScopesListHandler handles GET /api/v1/security/scopes.
type ScopesListHandler struct{}

func (h *ScopesListHandler) ServeHTTP(reqCtx request.RequestContext) {
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	toolQueryRepo := tool_query.NewToolQueryRepo(db)

	enforced := map[string]struct{}{}
	for _, s := range config.AllEnforcedScopes() {
		enforced[s] = struct{}{}
	}
	// Fold in every ENABLED tool's own declared route scopes —
	// tool_routes are never part of any route-*.yaml, so
	// config.AllEnforcedScopes() can't see them on its own. A disabled
	// tool's routes are unreachable (step 5), so they're excluded here
	// too. See plan/ai/tools/step-06-scope-registry-integration.md.
	if toolRouteScopes, err := toolQueryRepo.EnabledRouteScopes(); err == nil {
		for _, s := range toolRouteScopes {
			enforced[s] = struct{}{}
		}
	}

	requested := map[string]struct{}{}
	if storeCtx, ok := reqCtx.GetDI().(diStore); ok {
		if raw, ok := storeCtx.Get("auth_config"); ok {
			if authCfg, ok := raw.(auth_config.AppAuthConfig); ok {
				for _, s := range strings.Fields(authCfg.Scopes) {
					requested[s] = struct{}{}
				}
			}
		}
	}

	registered := map[string]struct{}{}
	groups := make([]groupView, 0, len(scopes.Groups()))
	for _, g := range scopes.Groups() {
		gv := groupView{ID: g.ID, Description: g.Description, Scopes: make([]scopeView, 0, len(g.Scopes))}
		for _, s := range g.Scopes {
			registered[s.ID] = struct{}{}
			_, isEnforced := enforced[s.ID]
			_, isRequested := requested[s.ID]
			gv.Scopes = append(gv.Scopes, scopeView{
				ID:          s.ID,
				Description: s.Description,
				Enforced:    isEnforced,
				Requested:   isRequested,
			})
		}
		groups = append(groups, gv)
	}

	// One additional group per INSTALLED tool, regardless of enabled
	// state — a disabled tool's declared scopes are still "registered,"
	// just currently unusable, same distinction this page already
	// draws for core scopes. See
	// plan/ai/tools/step-06-scope-registry-integration.md.
	if tools, err := toolQueryRepo.List(); err == nil {
		if toolScopesByTool, err := toolQueryRepo.AllScopesGroupedByTool(); err == nil {
			for _, t := range tools {
				toolScopes := toolScopesByTool[t.ID]
				enabled := t.Enabled
				gv := groupView{
					ID:          t.Slug,
					Description: t.Name,
					Source:      "tool",
					ToolEnabled: &enabled,
					Scopes:      make([]scopeView, 0, len(toolScopes)),
				}
				for _, s := range toolScopes {
					registered[s.Scope] = struct{}{}
					_, isEnforced := enforced[s.Scope]
					_, isRequested := requested[s.Scope]
					gv.Scopes = append(gv.Scopes, scopeView{
						ID:          s.Scope,
						Description: s.Description,
						Enforced:    isEnforced,
						Requested:   isRequested,
					})
				}
				groups = append(groups, gv)
			}
		}
	}

	// make(..., 0), not a nil slice: a nil slice marshals to JSON null,
	// not [] — already bit this codebase once (jwks_service.go's scopes
	// field), fixed the same way there.
	unregisteredEnforced := make([]string, 0)
	unrequested := make([]string, 0)
	for s := range enforced {
		if _, ok := registered[s]; !ok {
			unregisteredEnforced = append(unregisteredEnforced, s)
		}
		if _, ok := requested[s]; !ok {
			unrequested = append(unrequested, s)
		}
	}
	// Ranging over the (now tool-scope-merged) enforced map has no
	// deterministic order — sort for stable, reproducible API output.
	sort.Strings(unregisteredEnforced)
	sort.Strings(unrequested)

	reqCtx.JSON(http.StatusOK, scopesResponse{
		Groups:                     groups,
		UnregisteredEnforcedScopes: unregisteredEnforced,
		UnrequestedScopes:          unrequested,
	})
}
