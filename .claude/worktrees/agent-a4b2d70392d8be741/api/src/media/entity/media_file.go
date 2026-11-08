// Package entity holds the Media feature's own row shape — a single,
// core-owned file store any tool can upload into and any MCP tool call
// can be resolved against, replacing the ad hoc, per-tool capability-
// token pattern Career's own CV import used to hand-roll. See
// plan/ai/media/step-01-media-feature.md.
package entity

// MediaFile is one uploaded file, namespaced by the tool that uploaded
// it. ConversationID and ExpiresAt are both nullable: a future,
// non-transient use of Media (e.g. a persistent profile photo) sets
// neither; Career's own CV import sets ExpiresAt (mirrors the previous
// cv_import_files.expires_at TTL) but — today — cannot set
// ConversationID, since its own upload happens before the AI
// conversation it will be used in even exists. See media_query.go's
// Resolve for how these two fields gate access.
type MediaFile struct {
	_                struct{} `table:"media_files"`
	ID               string   `db:"id" dbtype:"TEXT" nullable:"false" json:"id"`
	ToolSlug         string   `db:"tool_slug" dbtype:"TEXT" nullable:"false" json:"tool_slug"`
	OriginalFilename string   `db:"original_filename" dbtype:"TEXT" nullable:"false" json:"original_filename"`
	Extension        string   `db:"extension" dbtype:"TEXT" nullable:"false" json:"extension"`
	StoredPath       string   `db:"stored_path" dbtype:"TEXT" nullable:"false" json:"stored_path"`
	ContentType      string   `db:"content_type" dbtype:"TEXT" nullable:"false" json:"content_type"`
	SizeBytes        int64    `db:"size_bytes" dbtype:"INTEGER" nullable:"false" json:"size_bytes"`
	UploadedByUserID string   `db:"uploaded_by_user_id" dbtype:"TEXT" nullable:"false" json:"uploaded_by_user_id"`
	ConversationID   string   `db:"conversation_id" dbtype:"TEXT" nullable:"true" json:"conversation_id"`
	ExpiresAt        string   `db:"expires_at" dbtype:"TEXT" nullable:"true" json:"expires_at"`
	CreatedAt        string   `db:"created_at" dbtype:"DATETIME" nullable:"false" json:"created_at"`
}
