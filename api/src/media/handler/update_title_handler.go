package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	media_persistent "github.com/a-digi/cinqo/src/media/repository/persistent"
	media_query "github.com/a-digi/cinqo/src/media/repository/query"
)

type updateMediaTitleRequest struct {
	Title string `json:"title"`
}

// UpdateTitleHandler handles PATCH /api/v1/media/{id} — no static
// scope (route-media.yaml), same dynamic ownership check as
// DeleteHandler/ReplaceImageHandler: cinqo:super:admin OR the row's own
// uploader. Title is the only field this route can change; it is
// required (rejects empty/blank — see
// plan/ai/media/step-10-obligatory-title.md), not a "clear it back to
// not set" mechanism. See
// plan/ai/media/step-09-title-metadata-and-preview.md.
func UpdateTitleHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	if r.Method != http.MethodPatch {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id is required")
		return
	}

	var body updateMediaTitleRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "title is required")
		return
	}

	userID, scopes, err := callerIdentity(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	queryRepo := media_query.NewMediaQueryRepo(db)

	m, err := queryRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "media not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up media")
		return
	}

	if !hasScope(scopes, "cinqo:super:admin") && userID != m.UploadedByUserID {
		response.ErrorResponse(w, http.StatusForbidden, "not authorized to update this media file")
		return
	}

	if err := media_persistent.NewMediaPersistentRepo(db).UpdateTitle(id, body.Title); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to update media")
		return
	}

	m.Title = body.Title
	response.SuccessResponse(w, http.StatusOK, toMediaFileResponse(m))
}
