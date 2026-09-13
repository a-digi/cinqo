package platform

import "github.com/a-digi/cinqo/src/security/scopes/entity"

// ScopeGroup is registered with the security engine's scope registry
// in routes.Init — see plan/ai/security/security.md and
// plan/ai/platform/step-05-platform-and-key-api.md.
var ScopeGroup = entity.ScopeGroup{
	ID:          "cinqo:platform",
	Description: "AI platform and API key management.",
	Scopes: []entity.Scope{
		{ID: "cinqo:platform:read", Description: "Can view registered platforms and existing API keys (masked)."},
		{ID: "cinqo:platform:manage", Description: "Can add and remove API keys."},
	},
}
