package handler

import (
	"database/sql"
	"errors"
	"fmt"
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
	if slug == "" {
		// Workaround for a real bug in coco-server's own
		// URI.ExtractPathVariables (server/request/uri.go): it bails
		// out WITHOUT extracting any {placeholder} — including {slug}
		// — whenever the real URL has a different segment count than
		// the route pattern. For this route
		// (/api/v1/tools/{slug}/proxy/**), that's true any time the
		// ** remainder itself contains more than one segment (e.g.
		// .../proxy/portal-links/crawl-request — 2 remainder segments,
		// one more than the pattern ever accounts for). Every route
		// this proxy served before step 27 happened to have a single-
		// segment remainder (.../proxy/portals, .../proxy/jobs, ...),
		// so segment counts always coincidentally matched and this was
		// latent until step 27 introduced this repo's first multi-
		// segment proxy remainder. Fixing this properly belongs in
		// coco-server itself (a separately versioned dependency, not
		// this repo) — derived directly from the real URL here instead
		// as a local, scoped workaround. See
		// plan/ai/tools/career/step-27-ai-free-manual-crawl.md.
		slug = slugFromToolsProxyPath(r.URL.Path)
	}
	remainder := reqCtx.GetURI().GetPathRemainder()

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	queryRepo := tool_query.NewToolQueryRepo(db)

	tool, err := queryRepo.FindBySlug(slug)
	if err != nil {
		// Includes the slug itself — this is the caller's OWN request
		// URL echoed back (e.g. /api/v1/tools/{slug}/proxy/...), not new
		// information disclosure, and was worth adding after a real
		// debugging session where "tool not found" alone left it
		// genuinely ambiguous which of several proxy calls in a chain
		// had actually failed.
		response.ErrorResponse(w, http.StatusNotFound, fmt.Sprintf("tool not found: %q", slug))
		return
	}

	if !tool.Enabled || tool.Status != "running" {
		response.ErrorResponse(w, http.StatusServiceUnavailable, fmt.Sprintf("tool %q is not currently enabled and running", slug))
		return
	}

	// Route existence is checked BEFORE the scope check — a 404 for an
	// undeclared path reveals nothing about what scopes exist, the
	// safer ordering (confirmed in the design doc's own open question).
	route, err := queryRepo.FindRoute(tool.ID, r.Method, remainder)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, fmt.Sprintf("tool %q does not declare route %s /%s", slug, r.Method, remainder))
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

// slugFromToolsProxyPath extracts {slug} directly from a real
// /api/v1/tools/{slug}/proxy/** URL — the ProxyHandler-local
// workaround for coco-server's own ExtractPathVariables bug (see
// ProxyHandler's own comment above). Looks for the literal "tools"
// segment rather than hardcoding an index, so it isn't sensitive to
// the exact API version prefix. Returns "" (the same as the buggy
// upstream extraction) if the URL doesn't have the expected shape —
// FindBySlug("") then fails exactly as it already does today.
func slugFromToolsProxyPath(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, s := range segments {
		if s == "tools" && i+1 < len(segments) {
			return segments[i+1]
		}
	}
	return ""
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
