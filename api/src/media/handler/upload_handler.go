// Package handler implements the Media feature's own, single HTTP
// route — uploading a file into a tool's own media namespace. Every
// other Media operation (resolving an id back into real file bytes for
// an MCP tool call) happens as a direct, same-process Go call from
// conversation/chat.go, never over HTTP — see
// media/repository/query/media_query.go's own Resolve, and
// plan/ai/media/step-01-media-feature.md's "API shape" section for why
// there's deliberately no GET-by-id route here.
package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"

	media_entity "github.com/a-digi/cinqo/src/media/entity"
	media_persistent "github.com/a-digi/cinqo/src/media/repository/persistent"
)

// maxMediaUploadBytes bounds a single upload — a plain constant, not
// user-configurable, matching this codebase's own established
// "constants over premature configurability" convention (see
// tools/career/backend/cv_import.go's own maxCVUploadBytes). Generic
// media is allowed a little more headroom than a CV specifically.
const maxMediaUploadBytes = 25 * 1024 * 1024

type uploadResponse struct {
	FileID string `json:"fileId"`
}

// UploadHandler handles POST /api/v1/media/upload — multipart, fields
// "file" (required), "toolSlug" (required), "conversationId"
// (optional), "ttlSeconds" (optional). Returns only { "fileId" } — the
// caller (and, through it, whatever AI turn eventually references this
// file) never sees a path or URL, per the design's own "the AI gets
// only the file id" requirement.
func UploadHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	if r.Method != http.MethodPost {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, callerScopes, err := callerIdentity(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(maxMediaUploadBytes); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "file is required and must be under the size limit")
		return
	}

	toolSlug := strings.TrimSpace(r.FormValue("toolSlug"))
	if toolSlug == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "toolSlug is required")
		return
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	tool, err := tool_query.NewToolQueryRepo(db).FindBySlug(toolSlug)
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("unknown tool %q", toolSlug))
		return
	}

	// Authorization: the caller must hold at least one scope toolSlug
	// itself declared (or cinqo:super:admin) — see
	// plan/ai/media/step-01-media-feature.md's "Scope requirements".
	// This ties the caller-asserted toolSlug back to a real check
	// instead of trusting it outright; Media itself declares no scope
	// of its own.
	toolScopes, err := tool_query.NewToolQueryRepo(db).ScopesForTool(tool.ID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to resolve tool scopes")
		return
	}
	if !hasScope(callerScopes, "cinqo:super:admin") && !hasAnyScope(callerScopes, toolScopes) {
		response.ErrorResponse(w, http.StatusForbidden, "missing required scope")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	// filepath.Base strips any directory component a hostile or buggy
	// client filename could carry (e.g. "../../etc/passwd") before it
	// is ever joined into storedPath — see the design's "Security
	// considerations" (path traversal).
	originalFilename := filepath.Base(header.Filename)
	if originalFilename == "." || originalFilename == string(filepath.Separator) {
		originalFilename = "upload"
	}
	extension := filepath.Ext(originalFilename)

	id := uuid.NewString()
	toolDir := mediaRoot(reqCtx, toolSlug)
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to prepare media storage")
		return
	}
	storedPath := filepath.Join(toolDir, id+"_"+originalFilename)

	dest, err := os.Create(storedPath)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to store file")
		return
	}
	written, err := io.Copy(dest, file)
	dest.Close()
	if err != nil {
		_ = os.Remove(storedPath)
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to store file")
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	m := &media_entity.MediaFile{
		ID:               id,
		ToolSlug:         toolSlug,
		OriginalFilename: originalFilename,
		Extension:        extension,
		StoredPath:       storedPath,
		ContentType:      contentType,
		SizeBytes:        written,
		UploadedByUserID: userID,
		ConversationID:   strings.TrimSpace(r.FormValue("conversationId")),
	}
	if ttlRaw := strings.TrimSpace(r.FormValue("ttlSeconds")); ttlRaw != "" {
		if ttl, convErr := strconv.Atoi(ttlRaw); convErr == nil && ttl > 0 {
			m.ExpiresAt = time.Now().Add(time.Duration(ttl) * time.Second).UTC().Format(time.RFC3339)
		}
	}

	if err := media_persistent.NewMediaPersistentRepo(db).Insert(m); err != nil {
		_ = os.Remove(storedPath)
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to record upload")
		return
	}

	response.SuccessResponse(w, http.StatusCreated, uploadResponse{FileID: id})
}
