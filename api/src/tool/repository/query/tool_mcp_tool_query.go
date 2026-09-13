package query

import (
	"database/sql"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type ToolMCPToolQueryRepo struct {
	db *sql.DB
}

func NewToolMCPToolQueryRepo(db *sql.DB) *ToolMCPToolQueryRepo {
	return &ToolMCPToolQueryRepo{db: db}
}

// FindByToolID returns every MCP tool cached for one installed tool —
// what Discover last found at that tool's own most recent install/
// enable, not a live re-query.
func (r *ToolMCPToolQueryRepo) FindByToolID(toolID string) ([]tool_entity.ToolMCPTool, error) {
	rows, err := r.db.Query(
		`SELECT tool_id, name, description, input_schema FROM tool_mcp_tools WHERE tool_id = ?`,
		toolID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]tool_entity.ToolMCPTool, 0)
	for rows.Next() {
		var t tool_entity.ToolMCPTool
		if err := rows.Scan(&t.ToolID, &t.Name, &t.Description, &t.InputSchema); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// FindToolByMCPToolName resolves an MCP tool_call's own Name (which
// only identifies the tool within its own MCP server) back to the
// installed tool that owns it — the invocation path's own
// spawn-target lookup. Returns sql.ErrNoRows (unwrapped) if no
// installed tool currently declares this MCP tool name. Columns
// explicitly prefixed "t." — tools.name and tool_mcp_tools.name would
// otherwise collide once joined. Reuses tool_query.go's own scanTool
// (same package).
func (r *ToolMCPToolQueryRepo) FindToolByMCPToolName(name string) (*tool_entity.Tool, error) {
	row := r.db.QueryRow(
		`SELECT t.id, t.slug, t.name, t.version, t.kind, t.enabled, t.status, t.install_path,
			t.backend_executable_relpath, t.frontend_bundle_relpath, t.min_app_version, t.max_app_version,
			t.pid, t.created_at, t.updated_at
		FROM tools t
		JOIN tool_mcp_tools m ON m.tool_id = t.id
		WHERE m.name = ?
		LIMIT 1`,
		name,
	)
	return scanTool(row.Scan)
}
