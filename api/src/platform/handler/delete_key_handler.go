package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	platform_persistent "github.com/a-digi/cinqo/src/platform/repository/persistent"
)

// DeleteKeyHandler handles DELETE /api/v1/platforms/keys/{id}. Hard
// delete — no soft-delete flag, matching this table's own small,
// low-stakes nature; nothing references a key by foreign key in this
// design. 404 if the ID doesn't exist. See
// plan/ai/platform/step-05-platform-and-key-api.md.
func DeleteKeyHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id is required")
		return
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	if err := platform_persistent.NewKeyPersistentRepo(db).Delete(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "key not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to delete key")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
