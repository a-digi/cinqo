package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	auth_service "github.com/a-digi/cinqo/src/auth/service"
	"github.com/a-digi/cinqo/src/tool/manager"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// diStore mirrors the pattern already established in auth_handler and
// security/scopes/handler — Get isn't part of serverdi.Context, so
// every handler package re-asserts to this local interface.
type diStore interface {
	Get(key string) (any, bool)
}

// ProxyHandler handles ANY /api/v1/tools/{slug}/proxy/** — the one,
// permanently-registered catch-all route (see route-tool.yaml, which
// deliberately declares no static scopes: for it). Re-derives
// enabled/running, the declared route, and the caller's own scope on
// every single request — nothing here is cached. See
// plan/ai/tools/step-05-reverse-proxy-and-request-enforcement.md.
func ProxyHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	slug := reqCtx.GetURI().GetPathVariable("slug")
	remainder := reqCtx.GetURI().GetPathRemainder()

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	queryRepo := tool_query.NewToolQueryRepo(db)

	tool, err := queryRepo.FindBySlug(slug)
	if err != nil {
		response.ErrorResponse(w, http.StatusNotFound, "tool not found")
		return
	}

	if !tool.Enabled || tool.Status != "running" {
		response.ErrorResponse(w, http.StatusServiceUnavailable, "tool is not currently enabled and running")
		return
	}

	// Route existence is checked BEFORE the scope check — a 404 for an
	// undeclared path reveals nothing about what scopes exist, the
	// safer ordering (confirmed in the design doc's open question).
	route, err := queryRepo.FindRoute(tool.ID, r.Method, remainder)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "tool does not declare this route")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up tool route")
		return
	}

	scopes, err := callerScopes(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !hasScope(scopes, "cinqo:super:admin") && !hasScope(scopes, route.RequiredScope) {
		response.ErrorResponse(w, http.StatusForbidden, "missing required scope")
		return
	}

	port, running := manager.Port(tool.ID)
	if !running {
		response.ErrorResponse(w, http.StatusServiceUnavailable, "tool is not currently running")
		return
	}

	target, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(port))
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to build proxy target")
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	r.URL.Path = "/" + remainder
	proxy.ServeHTTP(w, r)
}

// callerScopes validates the request's own token (cookie or Bearer
// header — CookieSecurityLayer already promoted a cookie into the
// header before this handler ever runs, but re-reading directly here
// matches auth_handler.MeHandler's own established approach rather
// than assuming that promotion always already happened) and returns
// its scopes. This is the one place in cinqo a handler re-validates a
// scope dynamically instead of relying on a static route-YAML
// declaration — see the design doc's "Security considerations" for why
// this route structurally can't declare one.
func callerScopes(reqCtx request.RequestContext) ([]string, error) {
	r := reqCtx.GetRequest()
	token := tokenFromRequest(r)
	if token == "" {
		return nil, errNoToken
	}

	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return nil, errNoToken
	}
	raw, ok := storeCtx.Get("jwks_service")
	if !ok {
		return nil, errNoToken
	}
	jwksSvc, ok := raw.(*auth_service.JwksService)
	if !ok {
		return nil, errNoToken
	}

	_, scopes, _, err := jwksSvc.Validate(token)
	if err != nil {
		return nil, err
	}
	return scopes, nil
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

var errNoToken = errors.New("no token")
