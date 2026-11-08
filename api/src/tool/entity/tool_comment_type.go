package entity

// ToolCommentType is one kind of entity a tool declared in its manifest's
// comments[] as commentable, cached at install/update time (same convention as
// ToolDashboardWidget). The comments themselves are not here: they live in the
// tool's own comments database. See plan/ai/comments/comments-engine.md.
type ToolCommentType struct {
	_          struct{} `table:"tool_comment_types"`
	ToolID     string   `db:"tool_id" dbtype:"TEXT" nullable:"false" json:"tool_id"`
	EntityType string   `db:"entity_type" dbtype:"TEXT" nullable:"false" json:"entity_type"`
	ReadScope  string   `db:"read_scope" dbtype:"TEXT" nullable:"false" json:"read_scope"`
	WriteScope string   `db:"write_scope" dbtype:"TEXT" nullable:"false" json:"write_scope"`
}
