package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"os"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	media_persistent "github.com/a-digi/cinqo/src/media/repository/persistent"
	media_query "github.com/a-digi/cinqo/src/media/repository/query"
)

// DeleteHandler handles DELETE /api/v1/media/{id} — cinqo:super:admin
// only (route-media.yaml's own static scope declaration). Removes the
// on-disk file first (best-effort, log-and-continue — mirrors
// pdf_tools' own pdfCacheStore failure handling: a filesystem error
// here is not a reason to leave the DB row dangling), then the row
// itself. Irreversible; the admin Media page's own delete button
// confirms first (MediaListPage.tsx).
func DeleteHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	if r.Method != http.MethodDelete {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id is required")
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

	if err := os.Remove(m.StoredPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		reqCtx.GetDI().GetLogger().Warning("media: failed to remove file %q for id %q: %v", m.StoredPath, id, err)
	}

	if err := media_persistent.NewMediaPersistentRepo(db).Delete(id); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to delete media")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
