package persistent

import (
	"database/sql"

	"github.com/a-digi/cinqo/src/localai/entity"
)

type AddedModelsPersistentRepo struct {
	db *sql.DB
}

func NewAddedModelsPersistentRepo(db *sql.DB) *AddedModelsPersistentRepo {
	return &AddedModelsPersistentRepo{db: db}
}

// Insert stores a model. The unique (repo, file, revision) refuses the same file twice.
func (r *AddedModelsPersistentRepo) Insert(m *entity.AddedModel) error {
	_, err := r.db.Exec(`INSERT INTO local_ai_models (id, repo, file, revision, url, sha256, size_bytes, quantization, license, name, publisher, context_tokens, why, suggested_by, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Repo, m.File, m.Revision, m.URL, m.SHA256, m.SizeBytes, m.Quantization, m.License, m.Name, m.Publisher, m.ContextTokens, m.Why, m.SuggestedBy, m.Status, m.CreatedAt)
	return err
}

// Delete removes a model's row.
func (r *AddedModelsPersistentRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM local_ai_models WHERE id = ?`, id)
	return err
}

// SetStatus changes a model's status.
func (r *AddedModelsPersistentRepo) SetStatus(id, status, now string) error {
	_, err := r.db.Exec(`UPDATE local_ai_models SET status = ?, updated_at = ? WHERE id = ?`, status, now, id)
	return err
}
