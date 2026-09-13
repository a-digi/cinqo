package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

type updateTitleRequest struct {
	Title string `json:"title"`
}

// UpdateTitleHandler handles PATCH /api/v1/conversations/{id}. See
// plan/ai/conversation/step-03-conversation-api.md.
func UpdateTitleHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id is required")
		return
	}

	var body updateTitleRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Title == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "title is required")
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
	if _, err := queryRepo.FindOwnedByID(id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up conversation")
		return
	}

	if err := conversation_persistent.NewConversationPersistentRepo(db).UpdateTitle(id, body.Title); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to update conversation")
		return
	}

	updated, err := queryRepo.FindOwnedByID(id, userID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "conversation updated but failed to reload")
		return
	}
	response.SuccessResponse(w, http.StatusOK, toConversationResponse(updated))
}
