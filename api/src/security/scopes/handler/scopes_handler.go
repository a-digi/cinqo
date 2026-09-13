// Package handler exposes the scope registry over HTTP for admins to
// review — see plan/ai/security/security.md.
package handler

import (
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/config"
	auth_config "github.com/a-digi/cinqo/src/auth/config"
	"github.com/a-digi/cinqo/src/security/scopes"
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
}

type scopesResponse struct {
	Groups                     []groupView `json:"groups"`
	UnregisteredEnforcedScopes []string    `json:"unregistered_enforced_scopes"`
	UnrequestedScopes          []string    `json:"unrequested_scopes"`
}

// ScopesListHandler handles GET /api/v1/security/scopes.
type ScopesListHandler struct{}

func (h *ScopesListHandler) ServeHTTP(reqCtx request.RequestContext) {
	enforced := map[string]struct{}{}
	for _, s := range config.AllEnforcedScopes() {
		enforced[s] = struct{}{}
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

	// make(..., 0), not a nil slice: a nil slice marshals to JSON null,
	// not [] — already bit this codebase once (jwks_service.go's scopes
	// field), fixed the same way there.
	unregisteredEnforced := make([]string, 0)
	unrequested := make([]string, 0)
	for _, s := range config.AllEnforcedScopes() {
		if _, ok := registered[s]; !ok {
			unregisteredEnforced = append(unregisteredEnforced, s)
		}
		if _, ok := requested[s]; !ok {
			unrequested = append(unrequested, s)
		}
	}

	reqCtx.JSON(http.StatusOK, scopesResponse{
		Groups:                     groups,
		UnregisteredEnforcedScopes: unregisteredEnforced,
		UnrequestedScopes:          unrequested,
	})
}
