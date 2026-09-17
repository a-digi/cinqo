package query

import (
	"database/sql"
	"errors"
	"time"

	media_entity "github.com/a-digi/cinqo/src/media/entity"
)

type MediaQueryRepo struct {
	db *sql.DB
}

func NewMediaQueryRepo(db *sql.DB) *MediaQueryRepo {
	return &MediaQueryRepo{db: db}
}

const mediaFileColumns = `id, tool_slug, original_filename, extension, stored_path, content_type, ` +
	`size_bytes, uploaded_by_user_id, conversation_id, expires_at, created_at`

func scanMediaFile(scan func(dest ...any) error) (*media_entity.MediaFile, error) {
	var m media_entity.MediaFile
	var conversationID, expiresAt sql.NullString

	if err := scan(&m.ID, &m.ToolSlug, &m.OriginalFilename, &m.Extension, &m.StoredPath, &m.ContentType,
		&m.SizeBytes, &m.UploadedByUserID, &conversationID, &expiresAt, &m.CreatedAt); err != nil {
		return nil, err
	}

	m.ConversationID = conversationID.String
	m.ExpiresAt = expiresAt.String

	return &m, nil
}

// FindByID returns sql.ErrNoRows (unwrapped) when no media file has
// this id — matches this codebase's own established convention (see
// tool_query.go's FindBySlug).
func (r *MediaQueryRepo) FindByID(id string) (*media_entity.MediaFile, error) {
	row := r.db.QueryRow(`SELECT `+mediaFileColumns+` FROM media_files WHERE id = ? LIMIT 1`, id)
	return scanMediaFile(row.Scan)
}

// FindAll returns every media_files row, newest first, optionally
// filtered to one tool's own namespace — the admin Media page's own
// data source (MediaListPage.tsx). Unlike Resolve, this never checks
// expiry or conversation scoping: an admin browsing/cleaning up media
// needs to see expired rows too, not have them silently disappear.
func (r *MediaQueryRepo) FindAll(toolSlug string) ([]*media_entity.MediaFile, error) {
	query := `SELECT ` + mediaFileColumns + ` FROM media_files`
	args := []any{}
	if toolSlug != "" {
		query += ` WHERE tool_slug = ?`
		args = append(args, toolSlug)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*media_entity.MediaFile, 0)
	for rows.Next() {
		m, err := scanMediaFile(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ErrNotResolvable collapses every reason a media reference can't be
// resolved (missing row, expired, wrong conversation) into one error —
// deliberately indistinguishable from the caller's own point of view,
// same reasoning cv_import_files' own serveCVFileHandler already
// established: which specific reason it failed for is not information
// worth exposing back through a tool-call result.
var ErrNotResolvable = errors.New("media file not resolvable")

// Resolve returns the real, absolute on-disk path for id, enforcing
// both of MediaFile's own optional access-control fields:
//   - expiresAt, if set, must not have passed (lazy expiry check —
//     mirrors cv_import_files' own previous behavior, no separate
//     reaper required for correctness).
//   - conversationID, if the row has one recorded, must match the
//     calling turn's own conversation — a file uploaded for one
//     conversation can never be resolved from another. A row with no
//     conversation_id recorded (e.g. Career's own CV import, whose
//     upload happens before the conversation exists) skips this check
//     entirely; the random, unguessable id plus the TTL above are that
//     row's own security boundary instead.
//
// This is called directly (same-process Go call, not HTTP) from
// conversation/chat.go's invokeToolCall — see
// plan/ai/media/step-02-career-media-migration.md.
func (r *MediaQueryRepo) Resolve(id, conversationID string) (*media_entity.MediaFile, error) {
	m, err := r.FindByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotResolvable
		}
		return nil, err
	}

	if m.ExpiresAt != "" {
		expiry, err := time.Parse(time.RFC3339, m.ExpiresAt)
		if err != nil || time.Now().UTC().After(expiry) {
			return nil, ErrNotResolvable
		}
	}

	if m.ConversationID != "" && m.ConversationID != conversationID {
		return nil, ErrNotResolvable
	}

	return m, nil
}
