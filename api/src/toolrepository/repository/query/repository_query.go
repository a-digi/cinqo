package query

import (
	"database/sql"

	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
)

type RepositoryQueryRepo struct {
	db *sql.DB
}

func NewRepositoryQueryRepo(db *sql.DB) *RepositoryQueryRepo {
	return &RepositoryQueryRepo{db: db}
}

const repositoryColumns = `id, url, name, description, catalog_json, last_fetched_at, last_status, last_error, created_at, updated_at, is_local`

func scanRepository(scan func(dest ...any) error) (*repo_entity.Repository, error) {
	var r repo_entity.Repository
	var catalog, fetchedAt, updatedAt sql.NullString
	if err := scan(&r.ID, &r.URL, &r.Name, &r.Description, &catalog, &fetchedAt, &r.LastStatus, &r.LastError, &r.CreatedAt, &updatedAt, &r.IsLocal); err != nil {
		return nil, err
	}
	r.CatalogJSON = catalog.String
	r.LastFetchedAt = fetchedAt.String
	r.UpdatedAt = updatedAt.String
	return &r, nil
}

func (r *RepositoryQueryRepo) List() ([]*repo_entity.Repository, error) {
	rows, err := r.db.Query(`SELECT ` + repositoryColumns + ` FROM tool_repositories ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*repo_entity.Repository, 0)
	for rows.Next() {
		repo, err := scanRepository(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, repo)
	}
	return out, rows.Err()
}

// FindByID returns sql.ErrNoRows when there is no such repository.
func (r *RepositoryQueryRepo) FindByID(id string) (*repo_entity.Repository, error) {
	return scanRepository(r.db.QueryRow(`SELECT `+repositoryColumns+` FROM tool_repositories WHERE id = ?`, id).Scan)
}

// FindByURL returns sql.ErrNoRows when the link is not registered.
func (r *RepositoryQueryRepo) FindByURL(url string) (*repo_entity.Repository, error) {
	return scanRepository(r.db.QueryRow(`SELECT `+repositoryColumns+` FROM tool_repositories WHERE url = ?`, url).Scan)
}

func (r *RepositoryQueryRepo) Count() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM tool_repositories`).Scan(&n)
	return n, err
}
