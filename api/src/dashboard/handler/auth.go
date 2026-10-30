package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"

	auth_service "github.com/a-digi/cinqo/src/auth/service"
	"github.com/a-digi/cinqo/src/dashboard"
)

// diStore mirrors media/handler.diStore — Get isn't part of serverdi.Context.
type diStore interface {
	Get(key string) (any, bool)
}

var errNoToken = errors.New("no token")

// callerFrom validates the request's token (cookie or Bearer header), the same
// way media/handler.callerIdentity does, and returns who is asking.
func callerFrom(reqCtx request.RequestContext) (dashboard.Caller, error) {
	token := tokenFromRequest(reqCtx.GetRequest())
	if token == "" {
		return dashboard.Caller{}, errNoToken
	}
	store, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return dashboard.Caller{}, errNoToken
	}
	raw, ok := store.Get("jwks_service")
	if !ok {
		return dashboard.Caller{}, errNoToken
	}
	jwks, ok := raw.(*auth_service.JwksService)
	if !ok {
		return dashboard.Caller{}, errNoToken
	}
	sub, scopes, _, err := jwks.Validate(token)
	if err != nil {
		return dashboard.Caller{}, err
	}
	return dashboard.Caller{UserID: sub, Scopes: scopes, Token: token}, nil
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
