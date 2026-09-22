package persistent

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolMCPToolPersistentRepo struct {
	db *sql.DB
}

func NewToolMCPToolPersistentRepo(db *sql.DB) *ToolMCPToolPersistentRepo {
	return &ToolMCPToolPersistentRepo{db: db}
}

// ReplaceAll wholesale-replaces one tool's cached MCP tool schema —
// called after a successful Discover, at install/enable time. Not
// cleared on disable (see
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md's own
// resolved open question): a disabled tool's cached rows are simply
// never read, since the caller who offers tools to the model already
// filters on the parent tool's own Enabled flag.
func (r *ToolMCPToolPersistentRepo) ReplaceAll(toolID string, tools []tool_entity.ToolMCPTool) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM tool_mcp_tools WHERE tool_id = ?`, toolID); err != nil {
		return err
	}
	for _, t := range tools {
		if _, err := tx.Exec(
			`INSERT INTO tool_mcp_tools (tool_id, name, description, input_schema, required_scope, media_param, promote_media_param) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			toolID, t.Name, t.Description, t.InputSchema, t.RequiredScope, t.MediaParam, t.PromoteMediaParam,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}
