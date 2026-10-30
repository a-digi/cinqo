package persistent

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolDashboardWidgetPersistentRepo struct {
	db *sql.DB
}

func NewToolDashboardWidgetPersistentRepo(db *sql.DB) *ToolDashboardWidgetPersistentRepo {
	return &ToolDashboardWidgetPersistentRepo{db: db}
}

// ReplaceAll wholesale-replaces one tool's cached widgets — called at
// install/update time, with an empty list when the new version declares none (so
// a widget removed in an update disappears). Same shape as
// ToolEventListenerPersistentRepo.ReplaceAll.
func (r *ToolDashboardWidgetPersistentRepo) ReplaceAll(toolID string, widgets []tool_entity.ToolDashboardWidget) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM tool_dashboard_widgets WHERE tool_id = ?`, toolID); err != nil {
		return err
	}
	for _, w := range widgets {
		if _, err := tx.Exec(
			`INSERT INTO tool_dashboard_widgets
				(tool_id, key, title, description, kind, schema_version, data_path, width, height, refresh_seconds)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			toolID, w.Key, w.Title, w.Description, w.Kind, w.SchemaVersion, w.DataPath, w.Width, w.Height, w.RefreshSeconds,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
