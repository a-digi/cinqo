package query

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolRequiredToolQueryRepo struct {
	db *sql.DB
}

func NewToolRequiredToolQueryRepo(db *sql.DB) *ToolRequiredToolQueryRepo {
	return &ToolRequiredToolQueryRepo{db: db}
}

// FindAll returns the declared dependencies of every installed tool (one query, for the admin tools list), in a stable order: by tool, then by
// the order they were stored. See plan/ai/tools/step-217-tools-card-description-and-dependencies.md.
func (r *ToolRequiredToolQueryRepo) FindAll() ([]tool_entity.ToolRequiredTool, error) {
	rows, err := r.db.Query(`SELECT tool_id, required_slug, COALESCE(min_version, '') FROM tool_required_tools ORDER BY tool_id, rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]tool_entity.ToolRequiredTool, 0)
	for rows.Next() {
		var t tool_entity.ToolRequiredTool
		if err := rows.Scan(&t.ToolID, &t.RequiredSlug, &t.MinVersion); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// FindByToolID returns one tool's declared dependencies, in the order they were stored (for the single-tool answers of install, enable, disable).
func (r *ToolRequiredToolQueryRepo) FindByToolID(toolID string) ([]tool_entity.ToolRequiredTool, error) {
	rows, err := r.db.Query(`SELECT tool_id, required_slug, COALESCE(min_version, '') FROM tool_required_tools WHERE tool_id = ? ORDER BY rowid`, toolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]tool_entity.ToolRequiredTool, 0)
	for rows.Next() {
		var t tool_entity.ToolRequiredTool
		if err := rows.Scan(&t.ToolID, &t.RequiredSlug, &t.MinVersion); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
