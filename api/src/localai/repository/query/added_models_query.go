package query

import (
	"database/sql"
	"strings"

	"github.com/a-digi/cinqo/src/localai/entity"
)

type AddedModelsQueryRepo struct {
	db *sql.DB
}

func NewAddedModelsQueryRepo(db *sql.DB) *AddedModelsQueryRepo { return &AddedModelsQueryRepo{db: db} }

const addedModelColumns = `id, repo, file, revision, url, sha256, size_bytes, quantization, license, name, publisher, context_tokens, why, suggested_by, status, created_at, updated_at`

// ListByStatus returns the models with this status, oldest first (a stable order for the list and for the tests).
func (r *AddedModelsQueryRepo) ListByStatus(status string) ([]entity.AddedModel, error) {
	rows, err := r.db.Query(`SELECT `+addedModelColumns+` FROM local_ai_models WHERE status = ? ORDER BY created_at, id`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.AddedModel{}
	for rows.Next() {
		var m entity.AddedModel
		var updated sql.NullString
		if err := rows.Scan(&m.ID, &m.Repo, &m.File, &m.Revision, &m.URL, &m.SHA256, &m.SizeBytes, &m.Quantization, &m.License, &m.Name, &m.Publisher,
			&m.ContextTokens, &m.Why, &m.SuggestedBy, &m.Status, &m.CreatedAt, &updated); err != nil {
			return nil, err
		}
		if updated.Valid {
			m.UpdatedAt = &updated.String
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListAdded returns the models that are part of the list of models (status added).
func (r *AddedModelsQueryRepo) ListAdded() ([]entity.AddedModel, error) {
	return r.ListByStatus(entity.AddedStatusAdded)
}

// Find returns one model by id (sql.ErrNoRows when there is none).
func (r *AddedModelsQueryRepo) Find(id string) (entity.AddedModel, error) {
	var m entity.AddedModel
	var updated sql.NullString
	err := r.db.QueryRow(`SELECT `+addedModelColumns+` FROM local_ai_models WHERE id = ?`, id).
		Scan(&m.ID, &m.Repo, &m.File, &m.Revision, &m.URL, &m.SHA256, &m.SizeBytes, &m.Quantization, &m.License, &m.Name, &m.Publisher,
			&m.ContextTokens, &m.Why, &m.SuggestedBy, &m.Status, &m.CreatedAt, &updated)
	if err != nil {
		return entity.AddedModel{}, err
	}
	if updated.Valid {
		m.UpdatedAt = &updated.String
	}
	return m, nil
}

// CountByStatus says how many models have this status.
func (r *AddedModelsQueryRepo) CountByStatus(status string) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM local_ai_models WHERE status = ?`, status).Scan(&n)
	return n, err
}

// Suggesters returns, for each model id asked about, the tools that suggested it, first suggester first.
func (r *AddedModelsQueryRepo) Suggesters(ids ...string) (map[string][]entity.Suggester, error) {
	out := map[string][]entity.Suggester{}
	if len(ids) == 0 {
		return out, nil
	}
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i], args[i] = "?", id
	}
	rows, err := r.db.Query(`SELECT model_id, tool, why, created_at FROM local_ai_model_suggesters WHERE model_id IN (`+strings.Join(marks, ",")+`) ORDER BY created_at, rowid`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var s entity.Suggester
		if err := rows.Scan(&id, &s.Tool, &s.Why, &s.At); err != nil {
			return nil, err
		}
		out[id] = append(out[id], s)
	}
	return out, rows.Err()
}
