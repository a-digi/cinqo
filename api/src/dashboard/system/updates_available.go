package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/a-digi/cinqo/src/dashboard"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
	"github.com/a-digi/cinqo/src/toolrepository/status"
	"github.com/a-digi/cinqo/src/toolrepository/updatecheck"
)

// RepositoryLister is what the widget needs from the tool repository service.
type RepositoryLister interface {
	List() ([]*repo_entity.Repository, error)
}

// UpdatesAvailable is "cinqo.updates_available": how many installed tools have a
// newer version in a tool repository. Computed from the stored catalogs, with no
// network call — the same source as the Tools page's "Update available". Admin-only.
type UpdatesAvailable struct {
	db         *sql.DB
	repos      RepositoryLister
	appVersion string
}

func NewUpdatesAvailable(db *sql.DB, repos RepositoryLister, appVersion string) *UpdatesAvailable {
	return &UpdatesAvailable{db: db, repos: repos, appVersion: appVersion}
}

func (*UpdatesAvailable) Definition() dashboard.Definition {
	return dashboard.Definition{
		Provider:       "cinqo",
		Key:            "updates_available",
		Title:          "Updates",
		Description:    "Installed tools that have a newer version in a repository.",
		Kind:           dashboard.KindStat,
		Size:           dashboard.Size{W: 1, H: 1},
		RefreshSeconds: dashboard.DefaultRefreshSeconds,
		RequiredScope:  dashboard.AdminScope,
	}
}

func (u *UpdatesAvailable) Data(_ context.Context, _ dashboard.Caller) ([]byte, error) {
	tools, err := tool_query.NewToolQueryRepo(u.db).List()
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	env := status.Env{OS: runtime.GOOS, Arch: runtime.GOARCH, AppVersion: u.appVersion, Installed: make(map[string]string, len(tools))}
	for _, t := range tools {
		env.Installed[t.Slug] = t.Version
	}
	repos, err := u.repos.List()
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	return json.Marshal(updatesStat(updatecheck.Updates(repos, env)))
}

// updatesStat is the widget's payload for a list of updates.
func updatesStat(updates []updatecheck.Update) dashboard.StatData {
	stat := dashboard.StatData{
		Label: "Tool updates",
		Value: dashboard.StatValue{IsNumber: true, Number: float64(len(updates))},
		Link:  "/admin/tools",
	}
	if len(updates) == 0 {
		stat.Caption = "All tools are up to date"
		return stat
	}
	const shown = 3
	names := make([]string, 0, shown)
	for i, up := range updates {
		if i == shown {
			break
		}
		names = append(names, up.Name)
	}
	caption := strings.Join(names, ", ")
	if more := len(updates) - shown; more > 0 {
		caption += fmt.Sprintf(" +%d more", more)
	}
	stat.Caption = truncate(caption, 200)
	return stat
}
