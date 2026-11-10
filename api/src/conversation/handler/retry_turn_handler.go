package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/conversation"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
	"github.com/a-digi/cinqo/src/platform"
)

type retryTurnRequest struct {
	// Confirmed says the caller was told the turn keeps failing and wants another try anyway.
	Confirmed bool `json:"confirmed"`
	// ReleaseLoadedModel: as on sending a message (plan/ai/conversation/step-241).
	ReleaseLoadedModel bool `json:"releaseLoadedModel"`
}

type retryTurnResponse struct {
	TurnRunID  string `json:"turnRunId"`
	Status     string `json:"status"`
	StartedAt  string `json:"startedAt"`
	RetryCount int    `json:"retryCount"`
}

// retryConfirmationBody is the 409 answer that asks for a confirmation: the usual error body plus a code a client can act on.
type retryConfirmationBody struct {
	Error      bool   `json:"error"`
	Message    string `json:"message"`
	Code       string `json:"code"`
	RetryCount int    `json:"retryCount"`
}

// RetryTurnHandler handles POST /api/v1/conversations/{id}/turns/{turnRunId}/retry — the HTTP wrapper around conversation.StartRetryRun
// (plan/ai/conversation/retry-last-step.md, step 4). Ownership is verified first, exactly as for sending a message: a wrong owner and a
// nonexistent conversation read the same, a 404.
func RetryTurnHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	turnRunID := reqCtx.GetURI().GetPathVariable("turnRunId")
	if id == "" || turnRunID == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id and turnRunId are required")
		return
	}

	// The body is optional ({} or nothing): only {"confirmed": true} means something.
	var body retryTurnRequest
	if r := reqCtx.GetRequest(); r.ContentLength != 0 {
		if err := reqCtx.BindJSON(&body); err != nil {
			response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
			return
		}
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
	conv, err := conversation_query.NewConversationQueryRepo(convDB).FindOwnedByID(id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up conversation")
		return
	}
	// A retry on a local model must not silently displace another model that is loaded either (plan/ai/conversation/step-241).
	if entry, ok := platform.Lookup(conv.PlatformID); ok {
		if status, occupied := occupiedAnswer(entry, conv.Model, userID, scopes, body.ReleaseLoadedModel); occupied != nil {
			writeOccupied(w, status, occupied)
			return
		}
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

	// The scopes are the caller's own, now: a retry never reuses what the failed run was allowed.
	run, err := conversation.StartRetryRun(http.DefaultClient, mainDB, convDB, encKey, id, turnRunID, body.Confirmed, scopes, resolvedDataDir(reqCtx), port)
	if err != nil {
		writeRetryError(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusAccepted, retryTurnResponse{
		TurnRunID: run.ID, Status: run.Status, StartedAt: run.StartedAt, RetryCount: run.RetryCount,
	})
}

func writeRetryError(w http.ResponseWriter, err error) {
	var confirm *conversation.RetryConfirmationRequiredError
	switch {
	case errors.As(err, &confirm):
		response.BuildHeaders(w)
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(retryConfirmationBody{
			Error: true, Code: "retry_confirmation_required", RetryCount: confirm.RetryCount,
			Message: "this turn has failed several retries in a row, confirm to try again",
		})
	case errors.Is(err, conversation.ErrTurnRunNotFound):
		response.ErrorResponse(w, http.StatusNotFound, "turn not found")
	case errors.Is(err, conversation.ErrTurnNotRetryable):
		response.ErrorResponse(w, http.StatusConflict, "this turn cannot be retried, start again instead")
	case errors.Is(err, conversation.ErrTurnAlreadyRunning):
		response.ErrorResponse(w, http.StatusConflict, "a turn is already in progress for this conversation")
	default:
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to start the retry")
	}
}
