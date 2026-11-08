package entity

// ToolScope is one scope a tool defines for itself (its own permission
// vocabulary, e.g. "tool:invoice_export:read"). Scope is globally
// unique across every installed tool (see the migration's
// tool_scopes_scope_unique index) — one tool can never shadow
// another's declared permission meaning.
type ToolScope struct {
	_           struct{} `table:"tool_scopes"`
	ToolID      string   `db:"tool_id" dbtype:"TEXT" nullable:"false" json:"tool_id"`
	Scope       string   `db:"scope" dbtype:"TEXT" nullable:"false" json:"scope"`
	Description string   `db:"description" dbtype:"TEXT" nullable:"false" json:"description"`
}
