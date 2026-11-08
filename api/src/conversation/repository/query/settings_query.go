// Package query — conversation_settings table reads. See
// plan/ai/conversation/step-37-ai-debug-logging-toggle.md.
package query

import (
	"database/sql"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

type SettingsQueryRepo struct {
	db *sql.DB
}

func NewSettingsQueryRepo(db *sql.DB) *SettingsQueryRepo {
	return &SettingsQueryRepo{db: db}
}

// Load always reads fresh from the database — never cached in a
// package variable. This is a live toggle an admin can flip at any
// moment while this long-running process keeps executing turns; a
// cached value would go stale the moment someone actually used the
// settings page. The row always exists (seeded by this feature's own
// migration), so sql.ErrNoRows here would mean the migration itself
// never ran — a real, surfaced error, not silently defaulted.
func (r *SettingsQueryRepo) Load() (*conversation_entity.Settings, error) {
	var s conversation_entity.Settings
	var updatedAt sql.NullString
	err := r.db.QueryRow(`SELECT id, ai_trace_logs_enabled, updated_at FROM conversation_settings WHERE id = 1`).
		Scan(&s.ID, &s.AITraceLogsEnabled, &updatedAt)
	if err != nil {
		return nil, err
	}
	if updatedAt.Valid {
		s.UpdatedAt = &updatedAt.String
	}
	return &s, nil
}
