package auth_handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"

	auth_service "github.com/a-digi/cinqo/src/auth/service"
)

type MeHandler struct{}

func (h *MeHandler) ServeHTTP(reqCtx request.RequestContext) {
	r := reqCtx.GetRequest()
	token := tokenFromRequest(r)
	if token == "" {
		reqCtx.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		reqCtx.JSON(http.StatusInternalServerError, map[string]string{"error": "auth not configured"})
		return
	}
	raw, ok := storeCtx.Get("jwks_service")
	if !ok {
		reqCtx.JSON(http.StatusInternalServerError, map[string]string{"error": "auth not configured"})
		return
	}
	jwksSvc, ok := raw.(*auth_service.JwksService)
	if !ok {
		reqCtx.JSON(http.StatusInternalServerError, map[string]string{"error": "auth not configured"})
		return
	}

	sub, scopes, expiresAt, err := jwksSvc.Validate(token)
	if err != nil {
		reqCtx.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	if c, err := r.Cookie("user_info"); err == nil {
		var info userInfoCookie
		if decoded, err := base64.RawURLEncoding.DecodeString(c.Value); err == nil &&
			json.Unmarshal(decoded, &info) == nil {
			reqCtx.JSON(http.StatusOK, map[string]any{
				"user_id":            sub,
				"email":              info.Email,
				"name":               info.Name,
				"preferred_username": info.PreferredUsername,
				"scopes":             scopes,
				"expires_at":         expiresAt.Unix(),
			})
			return
		}
	}

	reqCtx.JSON(http.StatusOK, map[string]any{
		"user_id":    sub,
		"scopes":     scopes,
		"expires_at": expiresAt.Unix(),
	})
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
