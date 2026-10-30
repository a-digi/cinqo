package service

import (
	"github.com/a-digi/cinqo/src/toolrepository/catalog"
	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
	"github.com/a-digi/cinqo/src/toolrepository/status"
)

// Summary is a repository as the list shows it: the registration plus how its
// tools stand against what is installed.
type Summary struct {
	*repo_entity.Repository
	Counts status.Counts `json:"counts"`
}

// Detail is one repository with every tool's status.
type Detail struct {
	*repo_entity.Repository
	Counts status.Counts `json:"counts"`
	Tools  []status.Tool `json:"tools"`
}

// rowsFor computes the tool rows from the stored catalog. A repository that
// has never loaded, or whose stored file no longer parses, has no rows.
func rowsFor(repo *repo_entity.Repository, env status.Env) []status.Tool {
	if repo.CatalogJSON == "" {
		return []status.Tool{}
	}
	cat, err := catalog.Parse([]byte(repo.CatalogJSON), repo.URL)
	if err != nil {
		return []status.Tool{}
	}
	return status.ForCatalog(cat, env)
}

// Summaries lists every repository with its counts.
func (s *Service) Summaries(env status.Env) ([]Summary, error) {
	repos, err := s.query.List()
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(repos))
	for _, r := range repos {
		out = append(out, Summary{Repository: r, Counts: status.Count(rowsFor(r, env))})
	}
	return out, nil
}

// Detail returns one repository with its tools and their status.
func (s *Service) Detail(id string, env status.Env) (*Detail, error) {
	repo, err := s.find(id)
	if err != nil {
		return nil, err
	}
	rows := rowsFor(repo, env)
	return &Detail{Repository: repo, Counts: status.Count(rows), Tools: rows}, nil
}
