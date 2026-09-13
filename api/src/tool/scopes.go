package tool

import "github.com/a-digi/cinqo/src/security/scopes/entity"

// ScopeGroup is registered with the security engine's scope registry
// in routes.Init — see plan/ai/security/security.md. tool:manage is a
// cross-cutting scope for administering the tool system itself
// (install/update/delete/enable/disable) — distinct from any single
// tool's own tool:{slug}:* namespace (see
// plan/ai/tools/step-01-data-model-and-migration.md).
var ScopeGroup = entity.ScopeGroup{
	ID:          "tool",
	Description: "Administration of the tool (plugin) system itself.",
	Scopes: []entity.Scope{
		{ID: "tool:manage", Description: "Can install, update, enable, disable, and delete tools."},
	},
}
