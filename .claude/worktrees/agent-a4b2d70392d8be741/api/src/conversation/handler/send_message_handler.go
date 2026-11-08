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

// startTurnRunResponse — this route no longer waits for the AI to
// finish (see plan/ai/conversation/step-23-detach-turn-execution-from-request.md,
// which supersedes step 12's "bounded only by the incoming request's
// own context" conclusion): it starts a detached turn run and returns
// immediately. The caller polls GET .../turns/active (turn_handler.go)
// until Status leaves "running", then re-fetches the conversation
// (GetHandler) for the real, finished message.
type startTurnRunResponse struct {
	TurnRunID string `json:"turnRunId"`
	Status    string `json:"status"`
	StartedAt string `json:"startedAt"`
}

// SendMessageHandler handles POST /api/v1/conversations/{id}/messages
// — the HTTP wrapper around conversation.StartTurnRun. Verifies
// ownership first (wrong owner and nonexistent both read as the same
// 404) before ever calling StartTurnRun, which has no ownership
// concept of its own. See
// plan/ai/conversation/step-03-conversation-api.md and
// plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
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

	// http.DefaultClient, not reqCtx.GetRequest()'s own client — there
	// is no such thing on the server side; this is the same shared
	// client the run itself will keep using for the entirety of its own
	// detached lifetime, well past this handler returning.
	run, err := conversation.StartTurnRun(http.DefaultClient, mainDB, convDB, encKey, id, body.Content, scopes, resolvedDataDir(reqCtx), port)
	if err != nil {
		writeStartTurnRunError(w, err)
		return
	}

	response.SuccessResponse(w, http.StatusAccepted, startTurnRunResponse{
		TurnRunID: run.ID,
		Status:    run.Status,
		StartedAt: run.StartedAt,
	})
}

func writeStartTurnRunError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, conversation.ErrEmptyContent),
		errors.Is(err, conversation.ErrContentTooLong):
		response.ErrorResponse(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, conversation.ErrConversationNotFound):
		response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
	case errors.Is(err, conversation.ErrTurnAlreadyRunning):
		response.ErrorResponse(w, http.StatusConflict, "a turn is already in progress for this conversation")
	default:
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to start turn")
	}
}
