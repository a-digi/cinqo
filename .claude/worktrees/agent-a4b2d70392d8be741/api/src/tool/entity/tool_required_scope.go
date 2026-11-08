package entity

// ToolRequiredScope is one existing, core cinqo scope (e.g.
// "cinqo:ping:read") a tool declared a dependency on — informational
// only (surfaced in the admin UI), not independently enforced anywhere
// beyond that.
type ToolRequiredScope struct {
	_      struct{} `table:"tool_required_scopes"`
	ToolID string   `db:"tool_id" dbtype:"TEXT" nullable:"false" json:"tool_id"`
	Scope  string   `db:"scope" dbtype:"TEXT" nullable:"false" json:"scope"`
}
