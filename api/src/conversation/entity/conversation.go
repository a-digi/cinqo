// Package entity holds the conversation feature's plain data shapes —
// see plan/ai/conversation/step-01-data-model-and-separate-database.md.
package entity

// Conversation is metadata only — its message content lives entirely
// in a Markdown log file (see log.go), never in this table. FilePath is
// always server-derived from the conversation's own ID, never
// client-supplied, and json:"-" since the frontend never needs the
// path itself, only content the backend already read out of the file.
type Conversation struct {
	_         struct{} `table:"conversations"`
	ID        string   `db:"id" dbtype:"UUID" nullable:"false" json:"id"`
	UserID    string   `db:"user_id" dbtype:"TEXT" nullable:"false" json:"-"`
	Title     string   `db:"title" dbtype:"TEXT" nullable:"false" json:"title"`
	StartedAt string   `db:"started_at" dbtype:"DATETIME" nullable:"false" json:"startedAt"`
	FilePath  string   `db:"file_path" dbtype:"TEXT" nullable:"false" json:"-"`
}
