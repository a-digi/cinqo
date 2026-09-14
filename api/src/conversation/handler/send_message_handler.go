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

// Content only — platformId/model are no longer per-message; they're
// fixed on the conversation itself at creation (step 7).
type sendMessageRequest struct {
	Content string `json:"content"`
}

type sendMessageResponse struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	CreatedAt  string `json:"createdAt"`
	DurationMs *int64 `json:"durationMs,omitempty"`
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

	userID, err := callerUserID(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	scopes, err := callerScopes(reqCtx)
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

	port, err := corePort(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "core port not configured")
		return
	}

	turn, err := conversation.SendMessage(reqCtx.GetRequest().Context(), http.DefaultClient, mainDB, convDB, encKey, id, body.Content, scopes, port)
	if err != nil {
		// Logged here, the handler layer, not inside SendMessage itself
		// — matches this codebase's own established convention
		// (install_handler.go's own Warning calls) of domain functions
		// just returning errors, never touching a logger. The real
		// failure detail is also persisted into the conversation's own
		// log (step 8) for the end user to inspect via the info
		// button; this is the separate, operator-facing record of it.
		if errors.Is(err, conversation.ErrProviderCallFailed) {
			reqCtx.GetDI().GetLogger().Warning("conversation %q: send failed: %v", id, err)
		}
		writeSendMessageError(w, err)
		return
	}

	response.SuccessResponse(w, http.StatusCreated, sendMessageResponse{
		Role:       "assistant",
		Content:    turn.AssistantContent,
		CreatedAt:  turn.AssistantTimestamp,
		DurationMs: turn.DurationMs(),
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
	case errors.Is(err, conversation.ErrToolIterationLimitReached):
		response.ErrorResponse(w, http.StatusInternalServerError, "the model kept calling tools without producing a final reply")
	default:
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to send message")
	}
}
