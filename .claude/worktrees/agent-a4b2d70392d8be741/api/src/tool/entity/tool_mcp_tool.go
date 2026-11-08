package entity

// ToolMCPTool is one tool an installed tool's own MCP server declared
// via tools/list, cached at install/enable time so an MCP-capable
// tool's schema doesn't need a live spawn just to be looked up. See
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md.
type ToolMCPTool struct {
	_           struct{} `table:"tool_mcp_tools"`
	ToolID      string   `db:"tool_id" dbtype:"TEXT" nullable:"false" json:"tool_id"`
	Name        string   `db:"name" dbtype:"TEXT" nullable:"false" json:"name"`
	Description string   `db:"description" dbtype:"TEXT" nullable:"false" json:"description"`
	InputSchema string   `db:"input_schema" dbtype:"TEXT" nullable:"false" json:"input_schema"`
	// RequiredScope comes from the manifest's own mcp_tools mapping
	// (never from live MCP discovery, which has no scope concept at
	// all) — see manifest.MCPToolDecl.
	RequiredScope string `db:"required_scope" dbtype:"TEXT" nullable:"false" json:"required_scope"`
	// MediaParam mirrors manifest.MCPToolDecl's own field of the same
	// name — see its doc comment. Empty string means "not applicable".
	MediaParam string `db:"media_param" dbtype:"TEXT" nullable:"false" json:"media_param"`
}
