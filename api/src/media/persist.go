// Package media exposes the Media feature's own in-process, HTTP-
// agnostic write path — the symmetric counterpart to
// repository/query's own Resolve, which api/src/conversation/chat.go
// already calls directly (no HTTP, no token) to read a Media file's
// own on-disk location for a tool-call argument. See
// upload_handler.go's own doc comment ("Every other Media operation
// ... happens as a direct, same-process Go call ... never over
// HTTP") for the precedent this follows.
package media

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	media_entity "github.com/a-digi/cinqo/src/media/entity"
	media_persistent "github.com/a-digi/cinqo/src/media/repository/persistent"
)

// PersistLocalFile copies the file already sitting at localPath (e.g.
// another tool's own uploads directory) into toolSlug's own Media
// storage tree — the exact same "<dataDir>/media/tools/<slug>/"
// convention UploadHandler's own mediaRoot uses, so a file written
// this way is indistinguishable from one uploaded over HTTP — and
// records a new, permanent (no ExpiresAt) media_files row.
//
// uploadedByUserID/conversationID are the CALLER'S OWN already-known
// values — this performs no auth or ownership check of its own,
// exactly like Resolve doesn't. It exists only for trusted core code
// that already knows, from its own context, exactly which real user
// and conversation it's acting on behalf of (e.g. a tool call made
// within that user's own conversation, api/src/conversation/chat.go)
// — never for anything an end-user request could reach directly.
func PersistLocalFile(db *sql.DB, dataDir, toolSlug, uploadedByUserID, conversationID, localPath, originalFilename, contentType string) (fileID string, err error) {
	src, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	id := uuid.NewString()
	toolDir := filepath.Join(dataDir, "media", "tools", toolSlug)
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		return "", err
	}
	storedPath := filepath.Join(toolDir, id+"_"+originalFilename)

	dest, err := os.Create(storedPath)
	if err != nil {
		return "", err
	}
	written, copyErr := io.Copy(dest, src)
	dest.Close()
	if copyErr != nil {
		_ = os.Remove(storedPath)
		return "", copyErr
	}

	m := &media_entity.MediaFile{
		ID:               id,
		ToolSlug:         toolSlug,
		OriginalFilename: originalFilename,
		Extension:        filepath.Ext(originalFilename),
		StoredPath:       storedPath,
		ContentType:      contentType,
		SizeBytes:        written,
		UploadedByUserID: uploadedByUserID,
		ConversationID:   conversationID,
		// Title is obligatory (plan/ai/media/step-10-obligatory-title.md)
		// but no meaningful title (e.g. a job/portal name) is available
		// in this generic, tool-agnostic promotion path — originalFilename
		// is the same placeholder every other required-but-uninformed
		// field on this struct already falls back to. A caller with real
		// context (e.g. Career's own save_cv_pdf flow) overwrites this
		// afterward via the new PATCH /api/v1/media/{id}/title
		// service-to-service route.
		Title: originalFilename,
	}
	if err := media_persistent.NewMediaPersistentRepo(db).Insert(m); err != nil {
		_ = os.Remove(storedPath)
		return "", err
	}
	return id, nil
}
