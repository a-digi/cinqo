package service

import (
	"github.com/a-digi/cinqo/src/toolrepository/catalog"
	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
	"github.com/a-digi/cinqo/src/toolrepository/resolver"
)

// PlanDependencies is the read-only preview of installing or updating one tool together with what it needs (plan
// plan/ai/tools/repository/dependency-install.md, step 4). It looks at the repository the user chose, checks that the tool and version exist there (the
// same answers as PlanInstall), then gives the resolver every registered repository's last good file and the installed tools, and returns its plan.
// Nothing is downloaded, installed or changed, and no repository is contacted: only the stored files are read.
func (s *Service) PlanDependencies(id, toolID, versionID string, installed []resolver.InstalledTool, env resolver.Env) (*resolver.Plan, error) {
	in, err := s.planInput(id, toolID, versionID, installed, env)
	if err != nil {
		return nil, err
	}
	plan := resolver.Resolve(in)
	return &plan, nil
}

// planInput checks that the repository, the tool and the version the user chose exist (the same answers as PlanInstall) and assembles what the
// resolver looks at: every registered repository's last good file and the installed tools.
func (s *Service) planInput(id, toolID, versionID string, installed []resolver.InstalledTool, env resolver.Env) (resolver.Input, error) {
	repo, err := s.find(id)
	if err != nil {
		return resolver.Input{}, err
	}
	if repo.CatalogJSON == "" {
		return resolver.Input{}, &Error{ErrToolNotFound, "this repository has not been loaded yet"}
	}
	cat, err := catalog.Parse([]byte(repo.CatalogJSON), repo.URL)
	if err != nil {
		return resolver.Input{}, &Error{ErrInvalidFile, "the stored repository file is no longer valid; refresh the repository"}
	}
	tool, ok := cat.Tool(toolID)
	if !ok {
		return resolver.Input{}, &Error{ErrToolNotFound, "tool not found in this repository"}
	}
	if versionID != "" {
		if _, ok := tool.Version(versionID); !ok {
			return resolver.Input{}, &Error{ErrVersionMissing, "version not found"}
		}
	}
	all, err := s.query.List()
	if err != nil {
		return resolver.Input{}, err
	}
	return resolver.Input{
		Installed: installed,
		Sources:   sourcesFrom(all),
		Env:       env,
		Request:   resolver.Request{RepositoryID: id, ToolID: toolID, VersionID: versionID},
	}, nil
}

// sourcesFrom turns the stored repositories into resolver sources: a repository whose stored file cannot be read any more is left without a catalog (the
// resolver skips it), one whose last refresh failed is stale (its file is from an earlier refresh).
func sourcesFrom(repos []*repo_entity.Repository) []resolver.Source {
	out := make([]resolver.Source, 0, len(repos))
	for _, r := range repos {
		src := resolver.Source{ID: r.ID, Name: r.Name, Local: r.IsLocal, Stale: r.LastStatus == repo_entity.StatusError, RefreshedAt: r.LastFetchedAt}
		if r.CatalogJSON != "" {
			if cat, err := catalog.Parse([]byte(r.CatalogJSON), r.URL); err == nil {
				src.Catalog = cat
			}
		}
		out = append(out, src)
	}
	return out
}
