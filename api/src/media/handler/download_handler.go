package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	media_query "github.com/a-digi/cinqo/src/media/repository/query"
)

// DownloadHandler handles GET /api/v1/media/{id}/download — the first
// route that serves a media file's own bytes back over HTTP (every
// other resolution, media_query.go's own Resolve, is an in-process Go
// call from conversation/chat.go for an MCP tool argument, never an
// HTTP response body). Added for tools that need to render a stored
// file as a plain, clickable link in their own frontend (e.g. Career's
// AI-generated CV PDFs) — see plan/ai/tools/career/step-XX-cv-pdf.md.
//
// No static scope (route-media.yaml), same dynamic-ownership reasoning
// as DeleteHandler: a caller may download a media file if they hold
// cinqo:super:admin OR are the row's own uploader. This does NOT
// enforce ExpiresAt/ConversationID the way Resolve does — those two
// fields gate an MCP tool's own automatic argument resolution, not a
// human deliberately fetching their own file by a random, unguessable
// id they already have a durable reference to.
func DownloadHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	if r.Method != http.MethodGet {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id is required")
		return
	}

	userID, scopes, err := callerIdentity(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	m, err := media_query.NewMediaQueryRepo(db).FindByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "media not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up media")
		return
	}

	if !hasScope(scopes, "cinqo:super:admin") && userID != m.UploadedByUserID {
		response.ErrorResponse(w, http.StatusForbidden, "not authorized to download this media file")
		return
	}

	w.Header().Set("Content-Type", m.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", m.OriginalFilename))
	http.ServeFile(w, r, m.StoredPath)
}
