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

const toolMCPToolColumns = `tool_id, name, description, input_schema, required_scope, media_param`

func scanToolMCPTool(scan func(dest ...any) error) (tool_entity.ToolMCPTool, error) {
	var t tool_entity.ToolMCPTool
	err := scan(&t.ToolID, &t.Name, &t.Description, &t.InputSchema, &t.RequiredScope, &t.MediaParam)
	return t, err
}

// FindByToolID returns every MCP tool cached for one installed tool —
// what Discover last found at that tool's own most recent install/
// enable, not a live re-query.
func (r *ToolMCPToolQueryRepo) FindByToolID(toolID string) ([]tool_entity.ToolMCPTool, error) {
	rows, err := r.db.Query(`SELECT `+toolMCPToolColumns+` FROM tool_mcp_tools WHERE tool_id = ?`, toolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]tool_entity.ToolMCPTool, 0)
	for rows.Next() {
		t, err := scanToolMCPTool(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// FindAll returns every cached MCP tool for every installed tool,
// regardless of that tool's own enabled state — the admin tools list
// (ListHandler) reads this, not FindAllEnabled, since an admin
// viewing installed tools should see what a disabled tool declares
// too, not only what's currently offerable to a live conversation.
func (r *ToolMCPToolQueryRepo) FindAll() ([]tool_entity.ToolMCPTool, error) {
	rows, err := r.db.Query(`SELECT ` + toolMCPToolColumns + ` FROM tool_mcp_tools`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]tool_entity.ToolMCPTool, 0)
	for rows.Next() {
		t, err := scanToolMCPTool(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// FindAllEnabled returns every cached MCP tool belonging to a
// currently-enabled installed tool — the pre-scope-filter candidate
// set "offering tools to the model" reads from. Per-caller scope
// filtering happens in Go (the same hasScope-with-super-admin-bypass
// pattern already used by proxy_handler.go), not here — keeps this
// query free of any caller-identity concept.
func (r *ToolMCPToolQueryRepo) FindAllEnabled() ([]tool_entity.ToolMCPTool, error) {
	rows, err := r.db.Query(
		`SELECT m.tool_id, m.name, m.description, m.input_schema, m.required_scope, m.media_param
		FROM tool_mcp_tools m
		JOIN tools t ON t.id = m.tool_id
		WHERE t.enabled = 1`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]tool_entity.ToolMCPTool, 0)
	for rows.Next() {
		t, err := scanToolMCPTool(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// FindMCPToolByName resolves an MCP tool_call's own Name (which only
// identifies the tool within its own MCP server) to its cached row —
// the invocation path's own re-check: RequiredScope for the
// defense-in-depth scope check, ToolID to then look up the parent
// Tool's InstallPath/BackendExecutableRelpath/Enabled via
// ToolQueryRepo.FindByID (same package, different table — kept as two
// plain queries rather than one join, since the caller needs both
// rows as their own separate, already-established shapes). Returns
// sql.ErrNoRows (unwrapped) if no installed tool currently declares
// this MCP tool name.
func (r *ToolMCPToolQueryRepo) FindMCPToolByName(name string) (tool_entity.ToolMCPTool, error) {
	row := r.db.QueryRow(`SELECT `+toolMCPToolColumns+` FROM tool_mcp_tools WHERE name = ? LIMIT 1`, name)
	return scanToolMCPTool(row.Scan)
}
