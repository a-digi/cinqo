package conversation

import "github.com/a-digi/cinqo/src/security/scopes/entity"

// ScopeGroup is registered with the security engine's scope registry
// in routes.Init — see plan/ai/security/security.md and
// plan/ai/conversation/step-03-conversation-api.md. Deliberately never
// listed alongside cinqo:super:admin in route-conversation.yaml — a
// conversation's content is closer to personal correspondence than
// operational data, so this feature has no admin bypass at all,
// breaking from this codebase's otherwise-universal cinqo:super:admin
// pattern (see that step's own security considerations).
var ScopeGroup = entity.ScopeGroup{
	ID:          "cinqo:conversation",
	Description: "AI conversations — create, view, and send messages in your own conversations.",
	Scopes: []entity.Scope{
		{ID: "cinqo:conversation:use", Description: "Can create, view, rename, delete, and send messages in your own conversations."},
	},
}
