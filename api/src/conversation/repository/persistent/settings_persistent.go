// Package persistent — conversation_settings table writes. See
// plan/ai/conversation/step-37-ai-debug-logging-toggle.md.
package persistent

import "database/sql"

type SettingsPersistentRepo struct {
	db *sql.DB
}

func NewSettingsPersistentRepo(db *sql.DB) *SettingsPersistentRepo {
	return &SettingsPersistentRepo{db: db}
}

// SetAITraceLogsEnabled updates the single global settings row (always
// id=1, seeded by this feature's own migration — never inserted here).
func (r *SettingsPersistentRepo) SetAITraceLogsEnabled(enabled bool) error {
	_, err := r.db.Exec(
		`UPDATE conversation_settings SET ai_trace_logs_enabled = ?, updated_at = datetime('now') WHERE id = 1`,
		enabled,
	)
	return err
}
