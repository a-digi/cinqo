package entity

// ToolRequiredTool is one other tool (by slug) a tool declared it
// cannot function without — enforced at install and enable time
// (manifest.ValidateRequiredTools), unlike ToolRequiredScope's own
// "informational only" cinqo-scope dependency. See
// plan/ai/tools/step-14-tool-dependencies.md.
type ToolRequiredTool struct {
	_            struct{} `table:"tool_required_tools"`
	ToolID       string   `db:"tool_id" dbtype:"TEXT" nullable:"false" json:"tool_id"`
	RequiredSlug string   `db:"required_slug" dbtype:"TEXT" nullable:"false" json:"required_slug"`
	MinVersion   string   `db:"min_version" dbtype:"TEXT" nullable:"true" json:"min_version"`
	// MaxVersion is the highest version of RequiredSlug the tool works with (inclusive); empty means no upper bound.
	MaxVersion string `db:"max_version" dbtype:"TEXT" nullable:"true" json:"max_version"`
}
