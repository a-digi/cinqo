package conversation

import "github.com/a-digi/cinqo/src/security/scopes/entity"

// ScopeGroup is registered with the security engine's scope registry
// in routes.Init — see plan/ai/security/security.md and
// plan/ai/conversation/step-03-conversation-api.md.
//
// route-conversation.yaml now also accepts cinqo:super:admin on every
// route (plan/ai/conversation/step-06-superadmin-route-access.md,
// revising step-03's original "no admin bypass" decision) — that only
// restores route-level access for a superadmin's OWN conversations; it
// grants no visibility into other users' conversations, since every
// handler still filters by the caller's own resolved user_id
// unconditionally (repository/query.FindOwnedByID/ListOwnedBy),
// regardless of scope.
var ScopeGroup = entity.ScopeGroup{
	ID:          "cinqo:conversation",
	Description: "AI conversations — create, view, and send messages in your own conversations.",
	Scopes: []entity.Scope{
		{ID: "cinqo:conversation:use", Description: "Can create, view, rename, delete, and send messages in your own conversations."},
	},
}
