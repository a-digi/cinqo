package persistent

import (
	"database/sql"

	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
)

type RepositoryPersistentRepo struct {
	db *sql.DB
}

func NewRepositoryPersistentRepo(db *sql.DB) *RepositoryPersistentRepo {
	return &RepositoryPersistentRepo{db: db}
}

// Insert stores a repository whose file has just been fetched and validated.
func (r *RepositoryPersistentRepo) Insert(repo *repo_entity.Repository) error {
	_, err := r.db.Exec(
		`INSERT INTO tool_repositories (id, url, name, description, catalog_json, last_fetched_at, last_status, last_error, is_local)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', ?)`,
		repo.ID, repo.URL, repo.Name, repo.Description, repo.CatalogJSON, repo.LastFetchedAt, repo_entity.StatusOK, repo.IsLocal,
	)
	return err
}

// RecordSuccess replaces the stored catalog with a freshly validated one.
func (r *RepositoryPersistentRepo) RecordSuccess(id, name, description, catalogJSON, fetchedAt string, isLocal bool) error {
	_, err := r.db.Exec(
		`UPDATE tool_repositories SET name = ?, description = ?, catalog_json = ?, last_fetched_at = ?,
		last_status = 'ok', last_error = '', is_local = ?, updated_at = ? WHERE id = ?`,
		name, description, catalogJSON, fetchedAt, isLocal, fetchedAt, id,
	)
	return err
}

// RecordError keeps the last good catalog and only notes that the refresh failed.
func (r *RepositoryPersistentRepo) RecordError(id, message, fetchedAt string) error {
	_, err := r.db.Exec(
		`UPDATE tool_repositories SET last_fetched_at = ?, last_status = 'error', last_error = ?, updated_at = ? WHERE id = ?`,
		fetchedAt, message, fetchedAt, id,
	)
	return err
}

// Delete removes only the registration; tools installed from the repository stay.
func (r *RepositoryPersistentRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM tool_repositories WHERE id = ?`, id)
	return err
}
