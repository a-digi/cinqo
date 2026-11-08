package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"

	auth_service "github.com/a-digi/cinqo/src/auth/service"
)

// diStore mirrors tool/handler.diStore and auth/handler.diStore — Get
// isn't part of serverdi.Context, so every handler package re-asserts
// to this local interface to reach it. Same small, deliberate
// duplication this codebase already establishes (see
// tool/handler/proxy_handler.go's own diStore doc comment) rather than
// a shared cross-domain import for one interface.
type diStore interface {
	Get(key string) (any, bool)
}

var errNoToken = errors.New("no token")

// callerIdentity mirrors tool/handler.proxy_handler.go's own
// callerScopes — validates the request's own token (cookie or Bearer
// header) and returns both the caller's subject (for
// MediaFile.UploadedByUserID) and scopes (for the tool_scopes
// intersection check in scope.go). Duplicated rather than imported
// from tool/handler for the same reason proxy_handler.go's own
// callerScopes isn't shared further: it's a small, self-contained
// function, and tool/handler doesn't export it.
func callerIdentity(reqCtx request.RequestContext) (userID string, scopes []string, err error) {
	r := reqCtx.GetRequest()
	token := tokenFromRequest(r)
	if token == "" {
		return "", nil, errNoToken
	}

	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return "", nil, errNoToken
	}
	raw, ok := storeCtx.Get("jwks_service")
	if !ok {
		return "", nil, errNoToken
	}
	jwksSvc, ok := raw.(*auth_service.JwksService)
	if !ok {
		return "", nil, errNoToken
	}

	sub, callerScopes, _, err := jwksSvc.Validate(token)
	if err != nil {
		return "", nil, err
	}
	return sub, callerScopes, nil
}

func tokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		return c.Value
	}
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

func hasAnyScope(scopes []string, wanted []string) bool {
	for _, w := range wanted {
		if hasScope(scopes, w) {
			return true
		}
	}
	return false
}
