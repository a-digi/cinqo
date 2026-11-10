// Package mcpserver is Cinqo as an MCP server for outside clients (plan 244): the foundations of step 1 are here (the switch, the access tokens and
// their admin routes); the endpoint itself, the tools it offers and the admin page follow in later steps. The module is optional and removable: only
// config/routes/routes.go imports it (a boundary test keeps it so), and deleting the package, its routes file and that import removes it.
package mcpserver

import "github.com/a-digi/cinqo/src/security/scopes/entity"

// ManageScope is the one scope of the module: the switch, the tokens, what is offered and the log.
const ManageScope = "cinqo:mcp:manage"

// ScopeGroup is registered with the security engine's scope registry in routes.Init.
var ScopeGroup = entity.ScopeGroup{
	ID:          "cinqo:mcp",
	Description: "Cinqo as an MCP server for outside clients.",
	Scopes: []entity.Scope{
		{ID: ManageScope, Description: "Can switch the MCP server on and off, create and revoke its access tokens, and see what an outside client may call."},
	},
}
