package query

import (
	"database/sql"
)

// EnabledDashboardWidget is a cached widget joined with its tool and with the
// declared route it reads from.
type EnabledDashboardWidget struct {
	ToolID         string
	ToolSlug       string
	Key            string
	Title          string
	Description    string
	Kind           string
	SchemaVersion  int
	DataPath       string
	Width          int
	Height         int
	RefreshSeconds int
	// RequiredScope is the scope of the widget's data route.
	RequiredScope string
}

type ToolDashboardWidgetQueryRepo struct {
	db *sql.DB
}

func NewToolDashboardWidgetQueryRepo(db *sql.DB) *ToolDashboardWidgetQueryRepo {
	return &ToolDashboardWidgetQueryRepo{db: db}
}

// FindAllEnabled returns the widgets of every ENABLED tool (a stopped-but-enabled
// tool still lists its widgets; they answer "unavailable"). The join on
// tool_routes is what makes the widget inherit its route's scope; a widget whose
// route no longer exists is not returned.
func (r *ToolDashboardWidgetQueryRepo) FindAllEnabled() ([]EnabledDashboardWidget, error) {
	rows, err := r.db.Query(
		`SELECT w.tool_id, t.slug, w.key, w.title, w.description, w.kind, w.schema_version,
			w.data_path, w.width, w.height, w.refresh_seconds, r.required_scope
		FROM tool_dashboard_widgets w
		JOIN tools t ON t.id = w.tool_id
		JOIN tool_routes r ON r.tool_id = w.tool_id AND r.method = 'GET' AND r.path_suffix = w.data_path
		WHERE t.enabled = 1
		ORDER BY t.slug ASC, w.key ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]EnabledDashboardWidget, 0)
	for rows.Next() {
		var w EnabledDashboardWidget
		if err := rows.Scan(&w.ToolID, &w.ToolSlug, &w.Key, &w.Title, &w.Description, &w.Kind, &w.SchemaVersion,
			&w.DataPath, &w.Width, &w.Height, &w.RefreshSeconds, &w.RequiredScope); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
