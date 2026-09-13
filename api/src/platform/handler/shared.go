package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"

	auth_service "github.com/a-digi/cinqo/src/auth/service"
	platform_crypto "github.com/a-digi/cinqo/src/platform/crypto"
	platform_entity "github.com/a-digi/cinqo/src/platform/entity"
)

// diStore mirrors src/auth/handler.diStore and src/tool/handler.diStore
// — Get isn't part of serverdi.Context, so the concrete ContextBag is
// re-asserted to this local interface to reach it. Same idiom, kept
// per-package rather than shared, matching this codebase's existing
// convention (see src/tool/handler/proxy_handler.go).
type diStore interface {
	Get(key string) (any, bool)
}

var errNoToken = errors.New("no token")

// tokenFromRequest matches auth_handler.MeHandler's own established
// approach — CookieSecurityLayer already promoted a cookie into the
// Authorization header before this handler runs, but re-reading
// directly here doesn't assume that.
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

// callerUserID resolves the authenticated caller's own user ID (JWT
// subject) — used as platform_keys.created_by on insert. See step 5's
// "POST /api/v1/platforms/keys, precisely" (step 3).
func callerUserID(reqCtx request.RequestContext) (string, error) {
	token := tokenFromRequest(reqCtx.GetRequest())
	if token == "" {
		return "", errNoToken
	}

	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return "", errNoToken
	}
	raw, ok := storeCtx.Get("jwks_service")
	if !ok {
		return "", errNoToken
	}
	jwksSvc, ok := raw.(*auth_service.JwksService)
	if !ok {
		return "", errNoToken
	}

	sub, _, _, err := jwksSvc.Validate(token)
	if err != nil || sub == "" {
		return "", errNoToken
	}
	return sub, nil
}

// encryptionKey resolves the platform API-key encryption key
// (backendapp.Start loads it once via platform_crypto.LoadOrGenerateKey
// and registers it into DI as "platform_encryption_key" — step 2/5).
func encryptionKey(reqCtx request.RequestContext) ([]byte, error) {
	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return nil, errors.New("encryption key not configured")
	}
	raw, ok := storeCtx.Get("platform_encryption_key")
	if !ok {
		return nil, errors.New("encryption key not configured")
	}
	key, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("encryption key not configured")
	}
	return key, nil
}

// keyResponse is the only shape a platform API key's value is ever
// rendered as — maskedKey is computed fresh from a decrypt on every
// response, never a stored column (step 2/5).
type keyResponse struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Platform  string `json:"platform"`
	MaskedKey string `json:"maskedKey"`
	CreatedAt string `json:"createdAt"`
}

func toKeyResponse(k *platform_entity.Key, encKey []byte) (keyResponse, error) {
	plain, err := platform_crypto.Decrypt(k.EncryptedKey, encKey)
	if err != nil {
		return keyResponse{}, err
	}
	return keyResponse{
		ID:        k.ID,
		Label:     k.Label,
		Platform:  k.Platform,
		MaskedKey: platform_crypto.Mask(plain),
		CreatedAt: k.CreatedAt,
	}, nil
}
