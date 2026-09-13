package auth_handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/a-digi/coco-server/server/request"
)

type userInfoCookie struct {
	UserID            string `json:"user_id"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}

type OidcCallbackHandler struct{}

func (h *OidcCallbackHandler) ServeHTTP(reqCtx request.RequestContext) {
	r := reqCtx.GetRequest()

	code := r.URL.Query().Get("code")
	codeVerifier := r.URL.Query().Get("code_verifier")
	redirectURI := r.URL.Query().Get("redirect_uri")

	if code == "" {
		reqCtx.JSON(http.StatusBadRequest, map[string]string{"error": "code required"})
		return
	}

	// Browser redirect straight from coco-iam: pass the code to the
	// frontend so it can complete the PKCE exchange with the verifier
	// it stashed in sessionStorage.
	if codeVerifier == "" {
		cfg, ok := getAuthConfig(reqCtx)
		if !ok {
			reqCtx.JSON(http.StatusInternalServerError, map[string]string{"error": "auth not configured"})
			return
		}
		if cfg.FrontendCallbackURL == "" {
			reqCtx.JSON(http.StatusInternalServerError, map[string]string{"error": "frontend_callback_url not configured"})
			return
		}
		target := cfg.FrontendCallbackURL + "?code=" + url.QueryEscape(code)
		if state := r.URL.Query().Get("state"); state != "" {
			target += "&state=" + url.QueryEscape(state)
		}
		http.Redirect(reqCtx.GetWriter(), r, target, http.StatusFound)
		return
	}

	cfg, ok := getAuthConfig(reqCtx)
	if !ok {
		reqCtx.JSON(http.StatusInternalServerError, map[string]string{"error": "auth not configured"})
		return
	}
	if redirectURI == "" {
		redirectURI = cfg.RedirectURI
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", codeVerifier)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)

	resp, err := http.Post(cfg.TokenURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		reqCtx.JSON(http.StatusBadGateway, map[string]string{"error": "token exchange failed"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		reqCtx.GetDI().GetLogger().Warning("token exchange rejected by %s: status=%d body=%s", cfg.TokenURL, resp.StatusCode, string(body))
		reqCtx.JSON(http.StatusBadGateway, map[string]string{"error": "token exchange rejected"})
		return
	}

	var upstream struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &upstream); err != nil {
		reqCtx.JSON(http.StatusInternalServerError, map[string]string{"error": "invalid token response"})
		return
	}

	secure := r.TLS != nil
	w := reqCtx.GetWriter()

	// SameSite=Lax on both cookies, consistently — per the cookie
	// contract in plan/ai/backend/auth/step-03-auth-endpoints-and-cookies.md.
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

	if cfg.UserinfoURL != "" {
		if info, err := fetchUserInfo(cfg.UserinfoURL, upstream.AccessToken); err == nil {
			if infoJSON, err := json.Marshal(info); err == nil {
				http.SetCookie(w, &http.Cookie{
					Name:     "user_info",
					Value:    base64.RawURLEncoding.EncodeToString(infoJSON),
					HttpOnly: true,
					Secure:   secure,
					SameSite: http.SameSiteLaxMode,
					Path:     "/",
					MaxAge:   30 * 24 * 3600,
				})
			}
		}
	}

	reqCtx.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func fetchUserInfo(userinfoURL, accessToken string) (*userInfoCookie, error) {
	req, err := http.NewRequest(http.MethodGet, userinfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo: %d", resp.StatusCode)
	}

	// Handle both a flat {"sub":...} response and coco-iam's wrapped
	// {"success":true,"message":{...}}.
	var raw struct {
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Message           *struct {
			Sub               string `json:"sub"`
			Email             string `json:"email"`
			Name              string `json:"name"`
			PreferredUsername string `json:"preferred_username"`
		} `json:"message"`
	}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return nil, err
	}
	if raw.Sub == "" && raw.Message != nil {
		raw.Sub = raw.Message.Sub
		raw.Email = raw.Message.Email
		raw.Name = raw.Message.Name
		raw.PreferredUsername = raw.Message.PreferredUsername
	}

	return &userInfoCookie{
		UserID:            raw.Sub,
		Email:             raw.Email,
		Name:              raw.Name,
		PreferredUsername: raw.PreferredUsername,
	}, nil
}
