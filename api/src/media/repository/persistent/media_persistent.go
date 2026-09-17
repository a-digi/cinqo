package persistent

import (
	"database/sql"

	media_entity "github.com/a-digi/cinqo/src/media/entity"
)

type MediaPersistentRepo struct {
	db *sql.DB
}

func NewMediaPersistentRepo(db *sql.DB) *MediaPersistentRepo {
	return &MediaPersistentRepo{db: db}
}

// Insert stores a new media_files row. ConversationID/ExpiresAt are
// passed through as sql.NullString so an empty string on the entity
// (the "not set" convention this whole package uses — see
// media_query.go's scanMediaFile) round-trips as a real SQL NULL, not
// the literal string "".
func (r *MediaPersistentRepo) Insert(m *media_entity.MediaFile) error {
	_, err := r.db.Exec(
		`INSERT INTO media_files (id, tool_slug, original_filename, extension, stored_path, content_type,
			size_bytes, uploaded_by_user_id, conversation_id, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		m.ID, m.ToolSlug, m.OriginalFilename, m.Extension, m.StoredPath, m.ContentType,
		m.SizeBytes, m.UploadedByUserID, nullIfEmpty(m.ConversationID), nullIfEmpty(m.ExpiresAt),
	)
	return err
}

func nullIfEmpty(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
