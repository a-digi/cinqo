package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"

	auth_service "github.com/a-digi/cinqo/src/auth/service"
)

// diStore mirrors the other handler packages' own — Get isn't part of serverdi.Context.
type diStore interface {
	Get(key string) (any, bool)
}

var errNoToken = errors.New("no token")

// identity is who is asking: the verified token's subject and scopes.
type identity struct {
	UserID string
	Scopes []string
}

// callerFrom validates the request's token (cookie or Bearer header) and returns who is asking.
func callerFrom(reqCtx request.RequestContext) (identity, error) {
	token := tokenFromRequest(reqCtx.GetRequest())
	if token == "" {
		return identity{}, errNoToken
	}
	store, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return identity{}, errNoToken
	}
	raw, ok := store.Get("jwks_service")
	if !ok {
		return identity{}, errNoToken
	}
	jwks, ok := raw.(*auth_service.JwksService)
	if !ok {
		return identity{}, errNoToken
	}
	sub, scopes, _, err := jwks.Validate(token)
	if err != nil || sub == "" {
		return identity{}, errNoToken
	}
	return identity{UserID: sub, Scopes: scopes}, nil
}

func tokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		return c.Value
	}
	h := r.Header.Get("Authorization")
	parts := strings.SplitN(h, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}
