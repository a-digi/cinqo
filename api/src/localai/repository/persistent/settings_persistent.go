// Package persistent writes the local AI settings. See plan/ai/small-llm-local/step-181-local-ai-small-models.md.
package persistent

import (
	"database/sql"

	"github.com/a-digi/cinqo/src/localai/entity"
)

type SettingsPersistentRepo struct {
	db *sql.DB
}

func NewSettingsPersistentRepo(db *sql.DB) *SettingsPersistentRepo {
	return &SettingsPersistentRepo{db: db}
}

// Save overwrites the single settings row. The caller must have validated s (entity.Validate): nothing is checked here.
func (r *SettingsPersistentRepo) Save(s entity.Settings) error {
	_, err := r.db.Exec(
		`UPDATE local_ai_settings SET enabled = ?, model_id = ?, context_tokens = ?, idle_minutes = ?, updated_at = datetime('now') WHERE id = 1`,
		s.Enabled, s.ModelID, s.ContextTokens, s.IdleMinutes,
	)
	return err
}
