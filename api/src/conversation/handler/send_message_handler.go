package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/conversation"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

type sendMessageRequest struct {
	PlatformID string `json:"platformId"`
	Model      string `json:"model"`
	Content    string `json:"content"`
}

type sendMessageResponse struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

// SendMessageHandler handles POST /api/v1/conversations/{id}/messages
// — the HTTP wrapper around step 2's SendMessage. Verifies ownership
// first (wrong owner and nonexistent both read as the same 404) before
// ever calling SendMessage, which has no ownership concept of its own.
// See plan/ai/conversation/step-03-conversation-api.md.
func SendMessageHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id is required")
		return
	}

	var body sendMessageRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.PlatformID == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "platformId is required")
		return
	}

	userID, err := callerUserID(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	convDB, err := conversationDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "conversation database not configured")
		return
	}

	if _, err := conversation_query.NewConversationQueryRepo(convDB).FindOwnedByID(id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up conversation")
		return
	}

	encKey, err := encryptionKey(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "encryption key not configured")
		return
	}

	mainDB := reqCtx.GetDI().GetDatabaseManager().Connector.DB

	turn, err := conversation.SendMessage(reqCtx.GetRequest().Context(), http.DefaultClient, mainDB, convDB, encKey, id, body.PlatformID, body.Model, body.Content)
	if err != nil {
		writeSendMessageError(w, err)
		return
	}

	response.SuccessResponse(w, http.StatusCreated, sendMessageResponse{
		Role:      "assistant",
		Content:   turn.AssistantContent,
		CreatedAt: turn.AssistantTimestamp,
	})
}

func writeSendMessageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, conversation.ErrEmptyContent),
		errors.Is(err, conversation.ErrContentTooLong),
		errors.Is(err, conversation.ErrPlatformNotFound),
		errors.Is(err, conversation.ErrPlatformUnsupported),
		errors.Is(err, conversation.ErrNoKeyForPlatform):
		response.ErrorResponse(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, conversation.ErrConversationNotFound):
		response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
	case errors.Is(err, conversation.ErrProviderCallFailed):
		response.ErrorResponse(w, http.StatusBadGateway, "the AI platform request failed")
	default:
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to send message")
	}
}
