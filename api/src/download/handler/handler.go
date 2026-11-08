// Package handler is the generic HTTP API of the download manager: watch the jobs and cancel one. Starting a download is
// not generic (each feature starts its own, behind its own scope); this API only shows and stops them. Every job carries
// the scope needed to see it, so a caller sees exactly the jobs of the features they may use.
package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	auth_service "github.com/a-digi/cinqo/src/auth/service"
	"github.com/a-digi/cinqo/src/download"
)

const superAdmin = "cinqo:super:admin"

type diStore interface {
	Get(key string) (any, bool)
}

var errNoToken = errors.New("no token")

// scopesOf resolves the caller's scopes from their token; a variable so tests can replace it.
var scopesOf = callerScopes

func managerOf(reqCtx request.RequestContext) (*download.Manager, bool) {
	store, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return nil, false
	}
	raw, ok := store.Get("download_manager")
	if !ok {
		return nil, false
	}
	m, ok := raw.(*download.Manager)
	return m, ok && m != nil
}

func canSee(scopes []string, v download.View) bool {
	for _, s := range scopes {
		if s == superAdmin || s == v.Scope {
			return true
		}
	}
	return false
}

// ListHandler handles GET /api/v1/downloads: the jobs the caller may see, newest first.
func ListHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	scopes, err := scopesOf(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	m, ok := managerOf(reqCtx)
	if !ok {
		response.ErrorResponse(w, http.StatusInternalServerError, "downloads are not available")
		return
	}
	out := make([]download.Progress, 0)
	for _, v := range m.List() {
		if canSee(scopes, v) {
			out = append(out, v.Progress)
		}
	}
	response.SuccessResponse(w, http.StatusOK, out)
}

// GetHandler handles GET /api/v1/downloads/{id}. A job the caller may not see is answered exactly like one that does not
// exist.
func GetHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	scopes, err := scopesOf(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	m, ok := managerOf(reqCtx)
	if !ok {
		response.ErrorResponse(w, http.StatusInternalServerError, "downloads are not available")
		return
	}
	v, found := m.Get(reqCtx.GetURI().GetPathVariable("id"))
	if !found || !canSee(scopes, v) {
		response.ErrorResponse(w, http.StatusNotFound, "download not found")
		return
	}
	response.SuccessResponse(w, http.StatusOK, v.Progress)
}

// CancelHandler handles DELETE /api/v1/downloads/{id}: 204 when it was cancelled, 404 for an unknown or invisible job,
// 409 when the job is no longer running.
func CancelHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	scopes, err := scopesOf(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	m, ok := managerOf(reqCtx)
	if !ok {
		response.ErrorResponse(w, http.StatusInternalServerError, "downloads are not available")
		return
	}
	id := reqCtx.GetURI().GetPathVariable("id")
	v, found := m.Get(id)
	if !found || !canSee(scopes, v) {
		response.ErrorResponse(w, http.StatusNotFound, "download not found")
		return
	}
	if !m.Cancel(id) {
		response.ErrorResponse(w, http.StatusConflict, "the download is not running")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func callerScopes(reqCtx request.RequestContext) ([]string, error) {
	token := tokenFromRequest(reqCtx.GetRequest())
	if token == "" {
		return nil, errNoToken
	}
	store, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return nil, errNoToken
	}
	raw, ok := store.Get("jwks_service")
	if !ok {
		return nil, errNoToken
	}
	jwks, ok := raw.(*auth_service.JwksService)
	if !ok {
		return nil, errNoToken
	}
	_, scopes, _, err := jwks.Validate(token)
	if err != nil {
		return nil, errNoToken
	}
	return scopes, nil
}

func tokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		return c.Value
	}
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}
