package auth_handler

import (
	"net/http"

	lift_security "github.com/a-digi/coco-lift/security"
	serverdi "github.com/a-digi/coco-server/server/di"
	server_security "github.com/a-digi/coco-server/server/security"
)

// CookieSecurityLayer wraps a ScopeSecurityLayer and promotes the
// access_token cookie to an Authorization header before the wrapped
// layer validates it, so httpOnly cookie-based auth works transparently
// against routes declared security: authenticated.
type CookieSecurityLayer struct {
	inner *lift_security.ScopeSecurityLayer
}

func NewCookieSecurityLayer(inner *lift_security.ScopeSecurityLayer) server_security.SecurityLayer {
	return &CookieSecurityLayer{inner: inner}
}

func (c *CookieSecurityLayer) Authorize(w http.ResponseWriter, r *http.Request, ctx serverdi.Context, route *server_security.Route) error {
	if r.Header.Get("Authorization") == "" {
		if cookie, err := r.Cookie("access_token"); err == nil {
			r.Header.Set("Authorization", "Bearer "+cookie.Value)
		}
	}
	return c.inner.Authorize(w, r, ctx, route)
}
