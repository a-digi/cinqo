// Package organisation is Cinqo's organisation model: every user belongs to exactly one organisation (decision 2026-11-15). See
// plan/ai/tools/namako/saas/s1-core-organisations.md.
package organisation

import "github.com/a-digi/cinqo/src/security/scopes/entity"

// ScopeGroup is registered with the security engine's scope registry in routes.Init.
var ScopeGroup = entity.ScopeGroup{
	ID:          "cinqo:organisations",
	Description: "Organisations and who belongs to them.",
	Scopes: []entity.Scope{
		{ID: "cinqo:organisations:admin", Description: "Can create, rename and suspend organisations and assign users to them (the operator)."},
	},
}
