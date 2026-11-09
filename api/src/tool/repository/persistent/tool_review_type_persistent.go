package persistent

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolReviewTypePersistentRepo struct {
	db *sql.DB
}

func NewToolReviewTypePersistentRepo(db *sql.DB) *ToolReviewTypePersistentRepo {
	return &ToolReviewTypePersistentRepo{db: db}
}

// ReplaceAll wholesale-replaces one tool's cached review types, called at install/update time with an empty list when the new
// version declares none (so a type removed in an update stops accepting reviews). Same shape as ToolCommentTypePersistentRepo.
func (r *ToolReviewTypePersistentRepo) ReplaceAll(toolID string, types []tool_entity.ToolReviewType) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM tool_review_types WHERE tool_id = ?`, toolID); err != nil {
		return err
	}
	for _, t := range types {
		half := 0
		if t.HalfStars {
			half = 1
		}
		if _, err := tx.Exec(
			`INSERT INTO tool_review_types (tool_id, entity_type, scale, half_stars, read_scope, write_scope) VALUES (?, ?, ?, ?, ?, ?)`,
			toolID, t.EntityType, t.Scale, half, t.ReadScope, t.WriteScope,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
