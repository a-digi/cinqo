package ping

import "github.com/a-digi/cinqo/src/security/scopes/entity"

// ScopeGroup is registered with the security engine's scope registry in
// routes.Init — see plan/ai/security/security.md. Replaces
// api/config/scopes/ping.json's content, now colocated with the domain
// it describes instead of a separate, hand-synced catalog file.
var ScopeGroup = entity.ScopeGroup{
	ID:          "cinqo:ping",
	Description: "Reference/demo domain proving the scope-enforcement pattern end to end. Not a real feature — delete this file when src/ping/ is deleted.",
	Scopes: []entity.Scope{
		{ID: "cinqo:ping:read", Description: "Can view pings."},
		{ID: "cinqo:ping:create", Description: "Can create pings."},
	},
}
