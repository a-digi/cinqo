package auth_handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"

	auth_service "github.com/a-digi/cinqo/src/auth/service"
	"github.com/a-digi/cinqo/src/organisation"
	organisation_entity "github.com/a-digi/cinqo/src/organisation/entity"
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

	var info *userInfoCookie
	if c, err := r.Cookie("user_info"); err == nil {
		var decodedInfo userInfoCookie
		if decoded, err := base64.RawURLEncoding.DecodeString(c.Value); err == nil && json.Unmarshal(decoded, &decodedInfo) == nil {
			info = &decodedInfo
		}
	}
	reqCtx.JSON(http.StatusOK, buildMe(sub, info, scopes, expiresAt.Unix(), organisationOf(reqCtx, sub)))
}

// buildMe is the /auth/me answer. The name, user name and e-mail are only there when the user_info cookie came with the request (tools forward it so they
// can show who did what). The organisation is there whenever it could be read: every user has exactly one (a user who has none is attached to the default
// organisation first).
func buildMe(sub string, info *userInfoCookie, scopes []string, expiresAt int64, org *organisation_entity.Organisation) map[string]any {
	out := map[string]any{"user_id": sub, "scopes": scopes, "expires_at": expiresAt}
	if info != nil {
		out["email"] = info.Email
		out["name"] = info.Name
		out["preferred_username"] = info.PreferredUsername
	}
	if org != nil {
		out["organisation_id"] = org.ID
		out["organisation_name"] = org.Name
		out["organisation_status"] = org.Status
	}
	return out
}

// organisationOf reads (and for a new user creates) the user's organisation. Any failure leaves the fields out instead of breaking the sign-in: the tool proxy
// refuses a user whose organisation it cannot read, so nothing is let through by this.
func organisationOf(reqCtx request.RequestContext, sub string) *organisation_entity.Organisation {
	di := reqCtx.GetDI()
	if di == nil {
		return nil
	}
	dm := di.GetDatabaseManager()
	if dm == nil || dm.Connector == nil || dm.Connector.DB == nil {
		return nil
	}
	org, err := organisation.Of(dm.Connector.DB, sub)
	if err != nil {
		return nil
	}
	return org
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
