// Package query reads the local AI settings. See plan/ai/small-llm-local/step-181-local-ai-small-models.md.
package query

import (
	"database/sql"

	"github.com/a-digi/cinqo/src/localai/entity"
)

type SettingsQueryRepo struct {
	db *sql.DB
}

func NewSettingsQueryRepo(db *sql.DB) *SettingsQueryRepo { return &SettingsQueryRepo{db: db} }

// Load reads the one settings row fresh every time (never cached): an admin can change it at any moment while turns
// are running. The row is created by the migration, so sql.ErrNoRows would mean the migration never ran.
func (r *SettingsQueryRepo) Load() (entity.Settings, error) {
	var s entity.Settings
	var updatedAt sql.NullString
	err := r.db.QueryRow(`SELECT id, enabled, model_id, context_tokens, idle_minutes, updated_at FROM local_ai_settings WHERE id = 1`).
		Scan(&s.ID, &s.Enabled, &s.ModelID, &s.ContextTokens, &s.IdleMinutes, &updatedAt)
	if err != nil {
		return entity.Settings{}, err
	}
	if updatedAt.Valid {
		s.UpdatedAt = &updatedAt.String
	}
	return s, nil
}
