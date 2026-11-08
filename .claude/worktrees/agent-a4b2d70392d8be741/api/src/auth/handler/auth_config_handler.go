package auth_handler

import (
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/config"
)

type AuthConfigHandler struct{}

// publicAuthConfig deliberately excludes ClientSecret.
type publicAuthConfig struct {
	AuthorizeURL      string   `json:"authorize_url"`
	ClientID          string   `json:"client_id"`
	RedirectURI       string   `json:"redirect_uri"`
	Scopes            string   `json:"scopes"`
	UnrequestedScopes []string `json:"unrequested_scopes,omitempty"`
}

func (h *AuthConfigHandler) ServeHTTP(reqCtx request.RequestContext) {
	cfg, ok := getAuthConfig(reqCtx)
	if !ok {
		reqCtx.JSON(http.StatusInternalServerError, map[string]string{"error": "auth not configured"})
		return
	}
	reqCtx.JSON(http.StatusOK, publicAuthConfig{
		AuthorizeURL:      cfg.AuthorizeURL,
		ClientID:          cfg.ClientID,
		RedirectURI:       cfg.RedirectURI,
		Scopes:            cfg.Scopes,
		UnrequestedScopes: unrequestedScopes(cfg.Scopes),
	})
}

// unrequestedScopes returns every scope route-*.yaml enforces that is
// missing from requestedScopes (config.json's space-separated auth.scopes
// string). A scope in this gap can never be requested via the OAuth
// authorize redirect, so no user token can ever carry it — no role
// assignment in coco-iam can fix that on its own.
func unrequestedScopes(requestedScopes string) []string {
	requested := map[string]struct{}{}
	for _, s := range strings.Fields(requestedScopes) {
		requested[s] = struct{}{}
	}

	var missing []string
	for _, s := range config.AllEnforcedScopes() {
		if _, ok := requested[s]; !ok {
			missing = append(missing, s)
		}
	}
	return missing
}
