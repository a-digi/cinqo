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

type messageResponse struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

type conversationDetailResponse struct {
	conversationResponse
	Messages []messageResponse `json:"messages"`
}

// GetHandler handles GET /api/v1/conversations/{id} — the caller's own
// conversation only (wrong owner and nonexistent both read as the same
// 404). Returns only the last conversation.ContextTurns turns (step
// 1's own tail-read), not the full retained window — no "load older
// messages" pagination in this design. See
// plan/ai/conversation/step-03-conversation-api.md.
func GetHandler(reqCtx request.RequestContext) {
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

	conv, err := conversation_query.NewConversationQueryRepo(db).FindOwnedByID(id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up conversation")
		return
	}

	turns, err := conversation.ReadRecentTurns(conv.FilePath, conversation.ContextTurns)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read conversation messages")
		return
	}

	messages := make([]messageResponse, 0, len(turns)*2)
	for _, t := range turns {
		messages = append(messages,
			messageResponse{Role: "user", Content: t.UserContent, CreatedAt: t.UserTimestamp},
			messageResponse{Role: "assistant", Content: t.AssistantContent, CreatedAt: t.AssistantTimestamp},
		)
	}

	response.SuccessResponse(w, http.StatusOK, conversationDetailResponse{
		conversationResponse: toConversationResponse(conv),
		Messages:             messages,
	})
}
