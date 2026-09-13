// Package entity holds the conversation feature's plain data shapes —
// see plan/ai/conversation/step-01-data-model-and-separate-database.md.
package entity

// Conversation is metadata only — its message content lives entirely
// in a Markdown log file (see log.go), never in this table. FilePath is
// always server-derived from the conversation's own ID, never
// client-supplied, and json:"-" since the frontend never needs the
// path itself, only content the backend already read out of the file.
//
// PlatformID/Model are set once at creation and never updated by any
// code path afterward — immutability is enforced structurally (no
// UpdatePlatform/UpdateModel method exists anywhere), not by a runtime
// check. See
// plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md.
type Conversation struct {
	_          struct{} `table:"conversations"`
	ID         string   `db:"id" dbtype:"UUID" nullable:"false" json:"id"`
	UserID     string   `db:"user_id" dbtype:"TEXT" nullable:"false" json:"-"`
	Title      string   `db:"title" dbtype:"TEXT" nullable:"false" json:"title"`
	StartedAt  string   `db:"started_at" dbtype:"DATETIME" nullable:"false" json:"startedAt"`
	FilePath   string   `db:"file_path" dbtype:"TEXT" nullable:"false" json:"-"`
	PlatformID string   `db:"platform_id" dbtype:"TEXT" nullable:"false" json:"platformId"`
	Model      string   `db:"model" dbtype:"TEXT" nullable:"false" json:"model"`
}
