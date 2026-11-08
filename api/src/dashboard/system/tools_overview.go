// Package system holds the dashboard widgets Cinqo itself provides.
package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/a-digi/cinqo/src/dashboard"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// ToolsOverview is "cinqo.tools_overview": how many tools are installed and how
// many of those are enabled. Admin-only, like the Tools page it links to.
type ToolsOverview struct {
	db *sql.DB
}

func NewToolsOverview(db *sql.DB) *ToolsOverview { return &ToolsOverview{db: db} }

func (*ToolsOverview) Definition() dashboard.Definition {
	return dashboard.Definition{
		Provider:       "cinqo",
		Key:            "tools_overview",
		Title:          "Tools",
		Description:    "Installed tools and how many are enabled.",
		Kind:           dashboard.KindStat,
		Size:           dashboard.Size{W: 1, H: 1},
		RefreshSeconds: dashboard.DefaultRefreshSeconds,
		RequiredScope:  dashboard.AdminScope,
	}
}

func (t *ToolsOverview) Data(_ context.Context, _ dashboard.Caller) ([]byte, error) {
	tools, err := tool_query.NewToolQueryRepo(t.db).List()
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	enabled := 0
	for _, tool := range tools {
		if tool.Enabled {
			enabled++
		}
	}
	return json.Marshal(dashboard.StatData{
		Label:   "Installed tools",
		Value:   dashboard.StatValue{IsNumber: true, Number: float64(len(tools))},
		Caption: fmt.Sprintf("%d enabled", enabled),
		Link:    "/admin/tools",
	})
}
