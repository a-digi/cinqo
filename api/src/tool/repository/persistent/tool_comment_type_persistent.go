package persistent

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolCommentTypePersistentRepo struct {
	db *sql.DB
}

func NewToolCommentTypePersistentRepo(db *sql.DB) *ToolCommentTypePersistentRepo {
	return &ToolCommentTypePersistentRepo{db: db}
}

// ReplaceAll wholesale-replaces one tool's cached comment types — called at
// install/update time, with an empty list when the new version declares none (so a
// type removed in an update stops accepting comments). Same shape as
// ToolDashboardWidgetPersistentRepo.ReplaceAll.
func (r *ToolCommentTypePersistentRepo) ReplaceAll(toolID string, types []tool_entity.ToolCommentType) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM tool_comment_types WHERE tool_id = ?`, toolID); err != nil {
		return err
	}
	for _, t := range types {
		if _, err := tx.Exec(
			`INSERT INTO tool_comment_types (tool_id, entity_type, read_scope, write_scope) VALUES (?, ?, ?, ?)`,
			toolID, t.EntityType, t.ReadScope, t.WriteScope,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
