package persistent

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolRequiredToolPersistentRepo struct {
	db *sql.DB
}

func NewToolRequiredToolPersistentRepo(db *sql.DB) *ToolRequiredToolPersistentRepo {
	return &ToolRequiredToolPersistentRepo{db: db}
}

// ReplaceAll wholesale-replaces one tool's own declared dependencies —
// called at install (fresh install and update) time, same
// delete-then-reinsert shape as ToolMCPToolPersistentRepo.ReplaceAll.
// See plan/ai/tools/step-14-tool-dependencies.md.
func (r *ToolRequiredToolPersistentRepo) ReplaceAll(toolID string, required []tool_entity.ToolRequiredTool) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM tool_required_tools WHERE tool_id = ?`, toolID); err != nil {
		return err
	}
	for _, rt := range required {
		if _, err := tx.Exec(
			`INSERT INTO tool_required_tools (tool_id, required_slug, min_version) VALUES (?, ?, ?)`,
			toolID, rt.RequiredSlug, rt.MinVersion,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}
