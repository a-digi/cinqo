package auth_config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// IamConfig holds the coco-iam connection settings needed to derive all
// OAuth/OIDC endpoint URLs. Lives in iam.yaml, which is currently a
// template only — cinqo has no application record registered in any
// coco-iam instance yet (org/workspace placement, app_id, OAuth client
// credentials are all "data, not code" per
// plan/ai/backend/auth/step-01-iam-config-and-registration.md's open
// question). If that file is ever absent or unpopulated,
// LoadIamConfig/Apply are simply never called, and AppAuthConfig runs on
// config.json's dev HS256 values alone.
type IamConfig struct {
	// Issuer is the backend API base URL (e.g. https://coco-iam-api.example.com).
	// Used to derive the token, userinfo, and JWKS endpoints.
	Issuer string `yaml:"issuer"`

	// LoginURL is the frontend/login base URL. Used to derive the
	// authorize endpoint and the JWT iss claim. Falls back to Issuer
	// when empty.
	LoginURL string `yaml:"login_url"`

	OrgSlug       string `yaml:"org_slug"`
	WorkspaceSlug string `yaml:"workspace_slug"`
	AppSlug       string `yaml:"app_slug"`
	AppID         string `yaml:"app_id"`
	ClientID      string `yaml:"client_id"`
	ClientSecret  string `yaml:"client_secret"`
	Audience      string `yaml:"audience"`

	// FrontendURL is the origin of this app's frontend dev/prod server.
	// The backend redirects the browser here after receiving the OAuth
	// code from coco-iam.
	FrontendURL string `yaml:"frontend_url"`

	// Optional explicit overrides — derived automatically when empty.
	AuthorizeURL string `yaml:"authorize_url"`
	TokenURL     string `yaml:"token_url"`
	UserinfoURL  string `yaml:"userinfo_url"`
	JwksURL      string `yaml:"jwks_url"`
}

func LoadIamConfig(data []byte) (IamConfig, error) {
	var cfg IamConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return IamConfig{}, fmt.Errorf("parse iam.yaml: %w", err)
	}
	return cfg, nil
}

// Apply merges IAM-derived values into an existing AppAuthConfig. Fields
// already set on dst are preserved where IAM doesn't own them; explicit
// URL overrides in iam.yaml take priority over derived ones.
func (iam IamConfig) Apply(dst *AppAuthConfig) {
	if iam.Issuer == "" {
		return
	}

	// apiBase is used for token, userinfo, and JWKS — server-to-server calls.
	apiBase := strings.TrimRight(iam.Issuer, "/")

	// loginBase is used for the authorize endpoint and the JWT iss claim.
	// coco-iam issues tokens with iss = loginBase + appPath.
	loginBase := apiBase
	if iam.LoginURL != "" {
		loginBase = strings.TrimRight(iam.LoginURL, "/")
	}

	oauthPath := fmt.Sprintf("/a/%s/%s/%s/oauth", iam.OrgSlug, iam.WorkspaceSlug, iam.AppSlug)
	appPath := fmt.Sprintf("/a/%s/%s/%s", iam.OrgSlug, iam.WorkspaceSlug, iam.AppSlug)

	dst.Issuer = loginBase + appPath
	dst.ClientID = iam.ClientID
	dst.ClientSecret = iam.ClientSecret
	dst.Audience = iam.Audience

	dst.AuthorizeURL = firstNonEmpty(iam.AuthorizeURL, apiBase+oauthPath+"/authorize")
	dst.TokenURL = firstNonEmpty(iam.TokenURL, apiBase+oauthPath+"/token")
	dst.UserinfoURL = firstNonEmpty(iam.UserinfoURL, apiBase+oauthPath+"/userinfo")

	if iam.JwksURL != "" {
		dst.JwksURL = iam.JwksURL
	} else if iam.AppID != "" {
		dst.JwksURL = fmt.Sprintf("%s/api/v1/applications/%s/.well-known/jwks.json", apiBase, iam.AppID)
	}

	if iam.FrontendURL != "" {
		// NOT /auth/callback: that path collides with the Vite dev-server
		// proxy (proxies the whole /auth prefix to this backend), which
		// swallows the frontend's own callback page and causes an
		// infinite redirect loop — confirmed in coco-mda's own build.
		// See frontend/step-01-project-scaffold.md's vite.config.ts proxy.
		dst.FrontendCallbackURL = strings.TrimRight(iam.FrontendURL, "/") + "/login/callback"
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
