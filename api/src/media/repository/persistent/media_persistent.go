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
// the literal string "". Width/Height are the same convention for an
// INTEGER column (zero means "not set" — see MediaFile's own doc
// comment): a non-image row (every existing caller of Insert before
// step 3) simply never sets them, and nullIfZero below stores a real
// SQL NULL for them, not a literal 0. updated_at is never set here —
// it stays NULL until UpdateImage (below) first replaces this row's
// own image bytes.
func (r *MediaPersistentRepo) Insert(m *media_entity.MediaFile) error {
	_, err := r.db.Exec(
		`INSERT INTO media_files (id, tool_slug, original_filename, extension, stored_path, content_type,
			size_bytes, uploaded_by_user_id, conversation_id, expires_at, created_at, width, height)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?)`,
		m.ID, m.ToolSlug, m.OriginalFilename, m.Extension, m.StoredPath, m.ContentType,
		m.SizeBytes, m.UploadedByUserID, nullIfEmpty(m.ConversationID), nullIfEmpty(m.ExpiresAt),
		nullIfZero(m.Width), nullIfZero(m.Height),
	)
	return err
}

func nullIfEmpty(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func nullIfZero(n int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(n), Valid: n != 0}
}

// UpdateImage overwrites an existing row's own stored_path/
// content_type/extension/size_bytes/width/height and sets updated_at
// to now — the row's own id/tool_slug/uploaded_by_user_id/created_at
// never change. The caller (replace_image_handler.go) has already
// written the new file to a fresh path and confirmed the row is an
// image before calling this; deleting the OLD file on disk is also the
// caller's own responsibility, done only AFTER this succeeds (see that
// handler's own "old file cleanup" ordering). See
// plan/ai/media/step-03-image-upload-and-replace-endpoints.md.
func (r *MediaPersistentRepo) UpdateImage(id, storedPath, contentType, extension string, sizeBytes int64, width, height int) error {
	_, err := r.db.Exec(
		`UPDATE media_files SET stored_path = ?, content_type = ?, extension = ?, size_bytes = ?,
			width = ?, height = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		storedPath, contentType, extension, sizeBytes, width, height, id,
	)
	return err
}

// UpdateTitle sets a row's own plain, non-localized title — the only
// field a caller can rename after upload. See
// plan/ai/media/step-09-title-metadata-and-preview.md.
func (r *MediaPersistentRepo) UpdateTitle(id, title string) error {
	_, err := r.db.Exec(`UPDATE media_files SET title = ? WHERE id = ?`, nullIfEmpty(title), id)
	return err
}

// Delete removes one media_files row. Deleting the underlying file
// itself is the caller's own responsibility (MediaDeleteHandler) — a
// row and its file are two separate resources, and the handler already
// needs the row's own StoredPath (via FindByID) before this is even
// called.
func (r *MediaPersistentRepo) Delete(id string) error {
	_, err := r.db.Exec(`DELETE FROM media_files WHERE id = ?`, id)
	return err
}
