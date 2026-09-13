// Package auth_config holds cinqo's runtime auth configuration — its own
// copy of the mechanism every coco-aim app uses, per
// coco-aim/plan/auth-integration.md §7 (no shared code between apps).
package auth_config

import "encoding/json"

// AppAuthConfig is the runtime auth configuration for this service. It is
// assembled from two sources: config.json's "auth" block (local/dev
// values — hs256_secret, redirect_uri, frontend_callback_url, scopes),
// then optionally overlaid by iam.yaml via IamConfig.Apply once this app
// is registered with a real coco-iam instance (see
// plan/ai/backend/auth/step-01-iam-config-and-registration.md).
type AppAuthConfig struct {
	Issuer              string `json:"issuer"`
	Audience            string `json:"audience"`
	HS256Secret         string `json:"hs256_secret"`
	JwksURL             string `json:"jwks_url"`
	UserinfoURL         string `json:"userinfo_url"`
	AuthorizeURL        string `json:"authorize_url"`
	TokenURL            string `json:"token_url"`
	ClientID            string `json:"client_id"`
	ClientSecret        string `json:"client_secret"`
	RedirectURI         string `json:"redirect_uri"`
	FrontendCallbackURL string `json:"frontend_callback_url"`
	Scopes              string `json:"scopes"`
}

type configWrapper struct {
	Auth AppAuthConfig `json:"auth"`
}

// Load parses the "auth" block out of config.json's raw bytes.
func Load(data []byte) (AppAuthConfig, error) {
	var wrapper configWrapper
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return AppAuthConfig{}, err
	}
	return wrapper.Auth, nil
}
