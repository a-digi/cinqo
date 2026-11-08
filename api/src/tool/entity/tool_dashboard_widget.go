package entity

// ToolDashboardWidget is one Home dashboard widget a tool declared in its
// manifest's widgets[], cached at install/update time (same convention as
// ToolEventListener). DataPath is the path_suffix of one of the tool's own
// declared GET routes; the widget's scope is that route's required_scope, read
// at query time, so the two can never disagree. See
// plan/ai/dashboard/home-dashboard.md.
type ToolDashboardWidget struct {
	_              struct{} `table:"tool_dashboard_widgets"`
	ToolID         string   `db:"tool_id" dbtype:"TEXT" nullable:"false" json:"tool_id"`
	Key            string   `db:"key" dbtype:"TEXT" nullable:"false" json:"key"`
	Title          string   `db:"title" dbtype:"TEXT" nullable:"false" json:"title"`
	Description    string   `db:"description" dbtype:"TEXT" nullable:"false" json:"description"`
	Kind           string   `db:"kind" dbtype:"TEXT" nullable:"false" json:"kind"`
	SchemaVersion  int      `db:"schema_version" dbtype:"INTEGER" nullable:"false" json:"schema_version"`
	DataPath       string   `db:"data_path" dbtype:"TEXT" nullable:"false" json:"data_path"`
	Width          int      `db:"width" dbtype:"INTEGER" nullable:"false" json:"width"`
	Height         int      `db:"height" dbtype:"INTEGER" nullable:"false" json:"height"`
	RefreshSeconds int      `db:"refresh_seconds" dbtype:"INTEGER" nullable:"false" json:"refresh_seconds"`
}
