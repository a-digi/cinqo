package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/conversation"
	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// activeTurnSummaryResponse is the lean subset of activeTurnResponse
// (below) that's cheap enough to include once per item in a
// conversation-list response — see ListHandler
// (list_handler.go) and
// plan/ai/conversation/step-26-list-endpoint-active-turn-summary.md.
// ServerNow is this handler's own clock at response time — never
// stored, computed fresh on every request. Lets a client compute a
// one-time clockOffset = serverNow - Date.now() and display elapsed
// time as (Date.now() + clockOffset) - startedAt, so a turn's elapsed
// timer stays correct even when the viewer's own system clock
// disagrees with the server's. See
// plan/ai/conversation/step-24-server-tracked-turn-elapsed-time.md.
type activeTurnSummaryResponse struct {
	TurnRunID string `json:"turnRunId"`
	Status    string `json:"status"`
	StartedAt string `json:"startedAt"`
	ServerNow string `json:"serverNow"`
}

// activeTurnResponse is the fuller shape both GetActiveTurnHandler and
// GetHandler's own `activeTurn` field use — see
// plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
// Embeds activeTurnSummaryResponse (encoding/json flattens an embedded
// anonymous struct's fields into the parent object, so this is not a
// wire-format change from before this embedding was introduced). Log
// is split into lines here (rather than the raw stored string) since
// that's the shape a poller actually wants to render as a list.
type activeTurnResponse struct {
	activeTurnSummaryResponse
	// UserContent is the message that started this run — included
	// (unlike the TurnRun entity's own json:"-" on this field, which
	// only prevents an accidental full-entity dump elsewhere) because
	// a page reopened mid-turn has no other way to show the user's own
	// just-sent message: it isn't appended to the conversation's
	// Markdown log until the whole run finishes.
	UserContent string   `json:"userContent"`
	Log         []string `json:"log"`
	// PromptTokens/CompletionTokens/TotalTokens (step 34) are read
	// straight off the TurnRun row — already correct and up-to-date
	// while a turn is still "running" (AddTokenUsage increments them
	// live, once per tool-loop iteration), not only once it finishes.
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
	// SubAgents (step 41 — Sub Agents) — every sub-agent this turn has
	// spawned so far, oldest first, live-updating on every poll exactly
	// like the parent turn's own Log/token fields already do. Omitted
	// (never an empty array) when this turn has spawned none, so older
	// clients that don't render it see no change in the response shape.
	SubAgents []subAgentRunResponse `json:"subAgents,omitempty"`
}

// subAgentRunResponse mirrors activeTurnResponse's own shape for one
// sub_agent_runs row — Task instead of UserContent, Result instead of
// "the conversation's own Markdown log gains it once finished" (a
// sub-agent has no Markdown log of its own to gain it in).
type subAgentRunResponse struct {
	ID               string   `json:"id"`
	Task             string   `json:"task"`
	Status           string   `json:"status"`
	Result           string   `json:"result,omitempty"`
	Log              []string `json:"log"`
	StartedAt        string   `json:"startedAt"`
	PromptTokens     int      `json:"promptTokens"`
	CompletionTokens int      `json:"completionTokens"`
	TotalTokens      int      `json:"totalTokens"`
}

func toSubAgentRunResponse(s *conversation_entity.SubAgentRun) subAgentRunResponse {
	lines := strings.Split(strings.TrimRight(s.Log, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{}
	}
	result := ""
	if s.Result != nil {
		result = *s.Result
	}
	return subAgentRunResponse{
		ID:               s.ID,
		Task:             s.Task,
		Status:           s.Status,
		Result:           result,
		Log:              lines,
		StartedAt:        s.StartedAt,
		PromptTokens:     s.PromptTokens,
		CompletionTokens: s.CompletionTokens,
		TotalTokens:      s.TotalTokens,
	}
}

func toActiveTurnSummaryResponse(t *conversation_entity.TurnRun) activeTurnSummaryResponse {
	return activeTurnSummaryResponse{
		TurnRunID: t.ID,
		Status:    t.Status,
		StartedAt: t.StartedAt,
		ServerNow: time.Now().UTC().Format(time.RFC3339),
	}
}

func toActiveTurnResponse(t *conversation_entity.TurnRun, subAgents []*conversation_entity.SubAgentRun) activeTurnResponse {
	lines := strings.Split(strings.TrimRight(t.Log, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{}
	}
	var subAgentResponses []subAgentRunResponse
	if len(subAgents) > 0 {
		subAgentResponses = make([]subAgentRunResponse, 0, len(subAgents))
		for _, s := range subAgents {
			subAgentResponses = append(subAgentResponses, toSubAgentRunResponse(s))
		}
	}
	return activeTurnResponse{
		activeTurnSummaryResponse: toActiveTurnSummaryResponse(t),
		UserContent:               t.UserContent,
		Log:                       lines,
		PromptTokens:              t.PromptTokens,
		CompletionTokens:          t.CompletionTokens,
		TotalTokens:               t.TotalTokens,
		SubAgents:                 subAgentResponses,
	}
}

// GetActiveTurnHandler handles GET
// /api/v1/conversations/{id}/turns/active — the caller's own
// conversation only (wrong owner and nonexistent both read as the same
// 404, matching every other handler in this feature). Reports the
// conversation's own most recent turn run regardless of status —
// deliberately NOT restricted to "running" — so a poller's request
// racing the exact moment a run finishes still observes the real
// terminal status instead of a 404 it could otherwise misread as "the
// run vanished." A poller is expected to stop once it sees a
// non-"running" status; nothing here ever deletes a terminal row, so
// a poller that keeps calling after that just keeps seeing the same
// finished run until a new one starts. 404 only when no turn has ever
// been started for this conversation at all.
func GetActiveTurnHandler(reqCtx request.RequestContext) {
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

	if _, err := conversation_query.NewConversationQueryRepo(db).FindOwnedByID(id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up conversation")
		return
	}

	run, err := conversation_query.NewTurnRunQueryRepo(db).FindMostRecentByConversationID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "no turn has been started for this conversation")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up active turn")
		return
	}

	subAgents, err := conversation_query.NewSubAgentRunQueryRepo(db).FindByParentTurnRunID(run.ID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up sub-agent runs")
		return
	}

	response.SuccessResponse(w, http.StatusOK, toActiveTurnResponse(run, subAgents))
}

// GetTurnSubAgentsHandler handles GET
// /api/v1/conversations/{id}/turns/{turnRunId}/subagents — historical
// sub-agent detail for ANY past turn of this conversation, not just
// its current/most-recent one (GetActiveTurnHandler's own scope).
// turnRunId is caller-supplied, so its own conversation_id is verified
// against {id} before returning anything — otherwise a caller could
// probe another conversation's turn_runs by guessing/copying an id
// (ownership of {id} alone isn't enough, since a valid, owned {id}
// paired with an arbitrary turnRunId would otherwise still work).
func GetTurnSubAgentsHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	turnRunID := reqCtx.GetURI().GetPathVariable("turnRunId")
	if id == "" || turnRunID == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id and turnRunId are required")
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

	if _, err := conversation_query.NewConversationQueryRepo(db).FindOwnedByID(id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up conversation")
		return
	}

	run, err := conversation_query.NewTurnRunQueryRepo(db).FindByID(turnRunID)
	if err != nil || run.ConversationID != id {
		response.ErrorResponse(w, http.StatusNotFound, "turn not found")
		return
	}

	subAgents, err := conversation_query.NewSubAgentRunQueryRepo(db).FindByParentTurnRunID(turnRunID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up sub-agent runs")
		return
	}

	out := make([]subAgentRunResponse, 0, len(subAgents))
	for _, s := range subAgents {
		out = append(out, toSubAgentRunResponse(s))
	}
	response.SuccessResponse(w, http.StatusOK, out)
}

type stopTurnResponse struct {
	Status string `json:"status"`
}

// StopActiveTurnHandler handles POST
// /api/v1/conversations/{id}/turns/active/stop — the caller's own
// conversation only (wrong owner and nonexistent both read as the same
// 404, matching every other handler in this feature). Idempotent: a
// doubled click, or a stop arriving once there's genuinely nothing left
// running (turn finished AND no sub-agents still going), both 404
// rather than erroring. Looks up the conversation's most recent turn
// regardless of its own status (not "running only") — since step 41's
// "the reply returns early" change, that turn's own sub-agents can
// still be running well after it itself shows terminal, and stopping
// those is still a real, meaningful action even though the turn's own
// reply already came back. Cancellation itself is asynchronous (see
// conversation.CancelActiveTurn's own doc comment): this responds 202
// the moment the signal is sent, not once everything has actually
// stopped. See plan/ai/conversation/step-25-cancel-in-progress-turn.md
// and plan/ai/conversation/step-41-sub-agents.md.
func StopActiveTurnHandler(reqCtx request.RequestContext) {
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

	if _, err := conversation_query.NewConversationQueryRepo(db).FindOwnedByID(id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up conversation")
		return
	}

	// FindMostRecentByConversationID, not FindActiveByConversationID —
	// since step 41's "the reply returns early" change, a turn's own
	// sub-agents can still be running well after the turn itself already
	// shows terminal, and there's still something real to stop in that
	// case. CancelActiveTurn itself now decides what, if anything, is
	// actually cancellable (the turn, its sub-agents, both, or neither).
	run, err := conversation_query.NewTurnRunQueryRepo(db).FindMostRecentByConversationID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "no turn has been started for this conversation")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up active turn")
		return
	}

	if run.Status == "running" {
		if err := conversation_persistent.NewTurnRunPersistentRepo(db).SetCancelRequested(run.ID); err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to record cancel request")
			return
		}
	}

	if !conversation.CancelActiveTurn(db, run.ID) {
		response.ErrorResponse(w, http.StatusNotFound, "nothing is currently running for this conversation")
		return
	}

	response.SuccessResponse(w, http.StatusAccepted, stopTurnResponse{Status: "cancelling"})
}
