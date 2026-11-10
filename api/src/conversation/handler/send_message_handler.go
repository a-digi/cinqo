package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/conversation"
	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
	"github.com/a-digi/cinqo/src/platform"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// Content — platformId/model are no longer per-message; they're fixed on the conversation itself at creation (step 7). Fresh and AllowedTools
// are optional settings of this one message (plan/ai/small-llm-local/step-210-import-progress-in-the-chat.md): fresh sends the model this
// message only (no earlier turn), allowedTools narrows the tools this turn is offered. Omitted, nothing changes.
type sendMessageRequest struct {
	Content      string   `json:"content"`
	Fresh        bool     `json:"fresh"`
	AllowedTools []string `json:"allowedTools"`
	// ReleaseLoadedModel: as on creating a conversation (plan/ai/conversation/step-241) — unload another model nobody uses, then start the turn.
	ReleaseLoadedModel bool `json:"releaseLoadedModel"`
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

	conv, err := conversation_query.NewConversationQueryRepo(convDB).FindOwnedByID(id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up conversation")
		return
	}

	allowedTools, err := conversation_entity.ParseAllowedTools(body.AllowedTools)
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	encKey, err := encryptionKey(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "encryption key not configured")
		return
	}

	mainDB := reqCtx.GetDI().GetDatabaseManager().Connector.DB

	if status, message := checkTurnAllowedTools(mainDB, conv, allowedTools); status != 0 {
		response.ErrorResponse(w, status, message)
		return
	}

	// A turn on a local model must not silently displace another model that is loaded (plan/ai/conversation/step-241); checked once, here, never inside
	// the tool loop. A platform that has gone is the turn's to report.
	if entry, ok := platform.Lookup(conv.PlatformID); ok {
		if status, occupied := occupiedAnswer(entry, conv.Model, userID, scopes, body.ReleaseLoadedModel); occupied != nil {
			writeOccupied(w, status, occupied)
			return
		}
	}

	port, err := corePort(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "core port not configured")
		return
	}

	// http.DefaultClient, not reqCtx.GetRequest()'s own client — there
	// is no such thing on the server side; this is the same shared
	// client the run itself will keep using for the entirety of its own
	// detached lifetime, well past this handler returning.
	run, err := conversation.StartTurnRunWith(http.DefaultClient, mainDB, convDB, encKey, id, body.Content, scopes, resolvedDataDir(reqCtx), port,
		conversation.TurnOptions{Fresh: body.Fresh, AllowedTools: allowedTools})
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

// checkTurnAllowedTools checks the allowedTools of a message the way the create handler checks them for a conversation: only in direct mode, never on a
// platform or model that offers no tools, and every name must be a tool that exists. It returns the status and the message to answer with, or 0 when the
// list is fine (or there is none). A platform that has gone is not checked here: the turn itself says so in the conversation.
func checkTurnAllowedTools(mainDB *sql.DB, conv *conversation_entity.Conversation, allowedTools []string) (int, string) {
	if allowedTools == nil {
		return 0, ""
	}
	if conv.OrchestrationMode != conversation_entity.OrchestrationModeDirect {
		return http.StatusBadRequest, "allowedTools is only supported in direct mode"
	}
	if entry, ok := platform.Lookup(conv.PlatformID); ok && entry.ToolPolicyFor(conv.Model) == platform.ToolsNone {
		return http.StatusBadRequest, "this platform or model offers no tools: do not send allowedTools"
	}
	if own := conv.AllowedToolNames(); own != nil {
		for _, name := range allowedTools {
			if !slices.Contains(own, name) {
				return http.StatusBadRequest, fmt.Sprintf("allowedTools: %q is not one of this conversation's tools (a message can narrow them, never widen)", name)
			}
		}
	}
	toolRepo := tool_query.NewToolMCPToolQueryRepo(mainDB)
	for _, name := range allowedTools {
		if _, err := toolRepo.FindMCPToolByName(name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return http.StatusBadRequest, fmt.Sprintf("allowedTools: unknown tool %q", name)
			}
			return http.StatusInternalServerError, "failed to check allowedTools"
		}
	}
	return 0, ""
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
