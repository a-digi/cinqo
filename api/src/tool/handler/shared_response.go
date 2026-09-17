package handler

import (
	"database/sql"

	"github.com/a-digi/coco-server/server/request"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
	"github.com/a-digi/cinqo/src/tool/systemtools"
)

// toolResponse is the shared JSON shape every tool list/mutation
// endpoint returns — an explicit field-by-field DTO, not struct
// embedding, matching conversation/handler/shared.go's own established
// convention. IsSystemTool is computed fresh from system-tools.yaml on
// every response, never persisted anywhere — see
// plan/ai/tools/step-10-system-tools.md. MCPTools is names only (not
// description/input_schema/required_scope) — the admin Tools page's
// own pills just need a label, no reason to also ship each tool's full
// schema to the browser for a display-only feature.
type toolResponse struct {
	ID                    string   `json:"id"`
	Slug                  string   `json:"slug"`
	Name                  string   `json:"name"`
	Version               string   `json:"version"`
	Kind                  string   `json:"kind"`
	Enabled               bool     `json:"enabled"`
	Status                string   `json:"status"`
	FrontendBundleRelpath string   `json:"frontend_bundle_relpath"`
	MinAppVersion         string   `json:"min_app_version"`
	MaxAppVersion         string   `json:"max_app_version"`
	CreatedAt             string   `json:"created_at"`
	UpdatedAt             string   `json:"updated_at"`
	IsSystemTool          bool     `json:"is_system_tool"`
	MCPTools              []string `json:"mcp_tools"`
}

func toToolResponse(t *tool_entity.Tool, cfg systemtools.Config, mcpTools []string) toolResponse {
	protected, _ := cfg.IsSystemTool(t.Slug)
	if mcpTools == nil {
		mcpTools = []string{}
	}
	return toolResponse{
		ID:                    t.ID,
		Slug:                  t.Slug,
		Name:                  t.Name,
		Version:               t.Version,
		Kind:                  t.Kind,
		Enabled:               t.Enabled,
		Status:                t.Status,
		FrontendBundleRelpath: t.FrontendBundleRelpath,
		MinAppVersion:         t.MinAppVersion,
		MaxAppVersion:         t.MaxAppVersion,
		CreatedAt:             t.CreatedAt,
		UpdatedAt:             t.UpdatedAt,
		IsSystemTool:          protected,
		MCPTools:              mcpTools,
	}
}

// toToolResponses batches its own MCP-tool-name lookup with a single
// ToolMCPToolQueryRepo.FindAll() call (not one query per tool) and
// groups the result by tool_id in Go — avoids an N+1 query regardless
// of how many tools are installed. Deliberately reads FindAll, not
// FindAllEnabled: an admin viewing the installed-tools list should see
// what a disabled tool declares too, not only what's currently
// offerable to a live conversation.
func toToolResponses(db *sql.DB, tools []*tool_entity.Tool, cfg systemtools.Config) ([]toolResponse, error) {
	mcpTools, err := tool_query.NewToolMCPToolQueryRepo(db).FindAll()
	if err != nil {
		return nil, err
	}
	namesByToolID := make(map[string][]string, len(tools))
	for _, mt := range mcpTools {
		namesByToolID[mt.ToolID] = append(namesByToolID[mt.ToolID], mt.Name)
	}

	out := make([]toolResponse, 0, len(tools))
	for _, t := range tools {
		out = append(out, toToolResponse(t, cfg, namesByToolID[t.ID]))
	}
	return out, nil
}

// mcpToolNamesFor looks up one tool's own currently-cached MCP tool
// names — used by the single-tool mutation handlers (install/enable/
// disable), which return just the one affected tool rather than a full
// list, so its own pills stay populated in that response instead of
// silently disappearing until the next full list refetch.
func mcpToolNamesFor(db *sql.DB, toolID string) ([]string, error) {
	mcpTools, err := tool_query.NewToolMCPToolQueryRepo(db).FindByToolID(toolID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(mcpTools))
	for _, mt := range mcpTools {
		names = append(names, mt.Name)
	}
	return names, nil
}

// systemToolsConfigFrom resolves the DI-registered system-tools config,
// falling back to an empty Config (nothing protected) if it isn't
// there — mirrors this package's own established diStore lookup
// pattern (proxy_handler.go's callerScopes).
func systemToolsConfigFrom(reqCtx request.RequestContext) systemtools.Config {
	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return systemtools.Config{}
	}
	raw, ok := storeCtx.Get("system_tools_config")
	if !ok {
		return systemtools.Config{}
	}
	cfg, ok := raw.(systemtools.Config)
	if !ok {
		return systemtools.Config{}
	}
	return cfg
}
