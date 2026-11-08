package entity

// ToolEventListener is one Domain Event topic an installed tool
// declared (manifest.go's own EventListenerDecl) that it wants
// delivered to its own already-running HTTP server, cached at
// install/update time — mirrors ToolMCPTool's own caching convention
// exactly. See
// plan/ai/domain-events/step-03-tool-listener-manifest-and-cache.md.
type ToolEventListener struct {
	_          struct{} `table:"tool_event_listeners"`
	ToolID     string   `db:"tool_id" dbtype:"TEXT" nullable:"false" json:"tool_id"`
	Topic      string   `db:"topic" dbtype:"TEXT" nullable:"false" json:"topic"`
	PathSuffix string   `db:"path_suffix" dbtype:"TEXT" nullable:"false" json:"path_suffix"`
}
