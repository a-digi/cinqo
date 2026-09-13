package handler

import (
	"github.com/a-digi/coco-server/server/request"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	"github.com/a-digi/cinqo/src/tool/systemtools"
)

// toolResponse is the shared JSON shape every tool list/mutation
// endpoint returns — an explicit field-by-field DTO, not struct
// embedding, matching conversation/handler/shared.go's own established
// convention. IsSystemTool is computed fresh from system-tools.yaml on
// every response, never persisted anywhere — see
// plan/ai/tools/step-10-system-tools.md.
type toolResponse struct {
	ID                    string `json:"id"`
	Slug                  string `json:"slug"`
	Name                  string `json:"name"`
	Version               string `json:"version"`
	Kind                  string `json:"kind"`
	Enabled               bool   `json:"enabled"`
	Status                string `json:"status"`
	FrontendBundleRelpath string `json:"frontend_bundle_relpath"`
	MinAppVersion         string `json:"min_app_version"`
	MaxAppVersion         string `json:"max_app_version"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
	IsSystemTool          bool   `json:"is_system_tool"`
}

func toToolResponse(t *tool_entity.Tool, cfg systemtools.Config) toolResponse {
	protected, _ := cfg.IsSystemTool(t.Slug)
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
	}
}

func toToolResponses(tools []*tool_entity.Tool, cfg systemtools.Config) []toolResponse {
	out := make([]toolResponse, 0, len(tools))
	for _, t := range tools {
		out = append(out, toToolResponse(t, cfg))
	}
	return out
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
