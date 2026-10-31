package service

import (
	"github.com/a-digi/cinqo/src/toolmeta"
	"github.com/a-digi/cinqo/src/toolrepository/catalog"
	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
	"github.com/a-digi/cinqo/src/toolrepository/status"
)

// Summary is a repository as the list shows it: the registration plus how its
// tools stand against what is installed.
type Summary struct {
	*repo_entity.Repository
	Counts status.Counts `json:"counts"`
	// Publisher is who maintains the repository, as its file states it; null for a file that states nothing
	// (the older format), which the page shows as "developer not stated".
	Publisher *toolmeta.Publisher `json:"publisher"`
}

// Detail is one repository with every tool's status.
type Detail struct {
	*repo_entity.Repository
	Counts    status.Counts       `json:"counts"`
	Publisher *toolmeta.Publisher `json:"publisher"`
	Tools     []status.Tool       `json:"tools"`
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

// publisherOf reads who maintains the repository out of the stored file. A repository that has never loaded, whose
// stored file no longer parses, or whose file states no authors, company or links has none.
func publisherOf(repo *repo_entity.Repository) *toolmeta.Publisher {
	if repo.CatalogJSON == "" {
		return nil
	}
	cat, err := catalog.Parse([]byte(repo.CatalogJSON), repo.URL)
	if err != nil || (len(cat.Authors) == 0 && cat.Company == nil && cat.Links == nil) {
		return nil
	}
	authors := cat.Authors
	if authors == nil {
		authors = []toolmeta.Author{}
	}
	return &toolmeta.Publisher{Authors: authors, Company: cat.Company, Links: cat.Links, GeneratedAt: cat.GeneratedAt}
}

// Summaries lists every repository with its counts.
func (s *Service) Summaries(env status.Env) ([]Summary, error) {
	repos, err := s.query.List()
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(repos))
	for _, r := range repos {
		out = append(out, Summary{Repository: r, Counts: status.Count(rowsFor(r, env)), Publisher: publisherOf(r)})
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
	return &Detail{Repository: repo, Counts: status.Count(rows), Publisher: publisherOf(repo), Tools: rows}, nil
}
