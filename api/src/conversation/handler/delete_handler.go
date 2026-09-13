package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"os"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// DeleteHandler handles DELETE /api/v1/conversations/{id}. Hard
// delete — no soft-delete flag. The database row is removed first,
// then its Markdown file: if the file removal then fails, an orphaned
// file with no owning row is harmless dead weight, not a security or
// consistency problem. See
// plan/ai/conversation/step-03-conversation-api.md.
func DeleteHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id is required")
		return
	}

	userID, err := callerUserID(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	db, err := conversationDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "conversation database not configured")
		return
	}

	queryRepo := conversation_query.NewConversationQueryRepo(db)
	conv, err := queryRepo.FindOwnedByID(id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up conversation")
		return
	}

	if err := conversation_persistent.NewConversationPersistentRepo(db).Delete(id); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to delete conversation")
		return
	}

	if err := os.Remove(conv.FilePath); err != nil && !os.IsNotExist(err) {
		reqCtx.GetDI().GetLogger().Warning("conversation %q deleted but failed to remove its log file: %v", id, err)
	}

	w.WriteHeader(http.StatusNoContent)
}
