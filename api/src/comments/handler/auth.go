package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"

	auth_service "github.com/a-digi/cinqo/src/auth/service"
	"github.com/a-digi/cinqo/src/comments/access"
)

// diStore mirrors the other handler packages' own — Get isn't part of
// serverdi.Context, so each re-asserts to this local interface.
type diStore interface {
	Get(key string) (any, bool)
}

var errNoToken = errors.New("no token")

// identity is who is acting: the access-control view plus the display name the
// token carries (stored with a new comment so it still reads right later).
type identity struct {
	access.Caller
	Name string
}

// callerFrom validates the request's token (cookie or Bearer header), the same
// way the media and dashboard handlers do, and returns who is asking.
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
	id, err := jwks.ValidateIdentity(token)
	if err != nil {
		return identity{}, err
	}
	name := id.Name
	if name == "" {
		name = nameFromUserInfoCookie(reqCtx.GetRequest(), id.Subject)
	}
	return identity{Caller: access.Caller{UserID: id.Subject, Scopes: id.Scopes}, Name: name}, nil
}

// userInfoCookie is what login stores in the "user_info" cookie (see the OIDC
// callback handler): the profile the identity provider returned.
type userInfoCookie struct {
	UserID            string `json:"user_id"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}

// nameFromUserInfoCookie is the display-name fallback for a token that carries no
// name (the identity provider's tokens here carry only an id). The cookie is not
// signed, so this is a cosmetic label only: it is never used for identity or for any
// permission, which always come from the verified token. It is ignored if it names a
// different user than the token does.
func nameFromUserInfoCookie(r *http.Request, subject string) string {
	c, err := r.Cookie("user_info")
	if err != nil {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return ""
	}
	var info userInfoCookie
	if json.Unmarshal(decoded, &info) != nil {
		return ""
	}
	if info.UserID != "" && info.UserID != subject {
		return ""
	}
	for _, v := range []string{info.Name, info.PreferredUsername, info.Email} {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
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
