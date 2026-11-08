package auth_handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/a-digi/coco-server/server/request"
)

type TokenRenewHandler struct{}

func (h *TokenRenewHandler) ServeHTTP(reqCtx request.RequestContext) {
	r := reqCtx.GetRequest()

	refreshToken := ""
	if c, err := r.Cookie("refresh_token"); err == nil && c.Value != "" {
		refreshToken = c.Value
	}

	cfg, ok := getAuthConfig(reqCtx)
	if !ok {
		reqCtx.JSON(http.StatusInternalServerError, map[string]string{"error": "auth not configured"})
		return
	}

	if refreshToken == "" {
		reqCtx.JSON(http.StatusUnauthorized, map[string]string{"error": "no refresh token"})
		return
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)

	resp, err := http.Post(cfg.TokenURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		reqCtx.JSON(http.StatusBadGateway, map[string]string{"error": "token renewal failed"})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		reqCtx.JSON(http.StatusUnauthorized, map[string]string{"error": "refresh rejected"})
		return
	}

	var upstream struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &upstream); err != nil {
		reqCtx.JSON(http.StatusInternalServerError, map[string]string{"error": "invalid token response"})
		return
	}

	secure := r.TLS != nil
	w := reqCtx.GetWriter()

	// SameSite=Lax on both cookies, matching callback.go's cookie contract.
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    upstream.AccessToken,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		MaxAge:   upstream.ExpiresIn,
	})
	if upstream.RefreshToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    upstream.RefreshToken,
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
			Path:     "/api/v1/auth/renew",
			MaxAge:   30 * 24 * 3600,
		})
	}

	reqCtx.JSON(http.StatusOK, map[string]any{"status": "ok", "expires_in": upstream.ExpiresIn})
}
