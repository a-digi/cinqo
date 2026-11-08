package handler

import (
	"net/http"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// conversationListItemResponse is list-only — deliberately NOT added
// to the shared conversationResponse struct that create/get/rename
// also use (shared.go), since that would silently collide with
// GetHandler's own separate, fuller `activeTurn` field
// (conversationDetailResponse.ActiveTurn): both would serialize to the
// same "activeTurn" JSON key at different struct depths, and Go's
// field-promotion rule (shallowest field wins) would make this one
// silently unreachable through that response with no compile error to
// catch it. Keeping list-only enrichment in its own wrapper type here
// avoids that trap and leaves every other conversation-metadata
// response's shape untouched. See
// plan/ai/conversation/step-26-list-endpoint-active-turn-summary.md.
type conversationListItemResponse struct {
	conversationResponse
	ActiveTurn *activeTurnSummaryResponse `json:"activeTurn,omitempty"`
}

// ListHandler handles GET /api/v1/conversations — the caller's own
// conversations only, most recently started first. See
// plan/ai/conversation/step-03-conversation-api.md. Each item's own
// activeTurn (step 26) is a lean summary, not the fuller shape
// GET .../turns/active returns — this list may be polled every few
// seconds (step-27-frontend-periodic-list-refresh.md) to drive live
// "still running" badges across the whole list, so it deliberately
// never pulls a running turn's own log/user_content in here.
func ListHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()

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

	conversations, err := conversation_query.NewConversationQueryRepo(db).ListOwnedBy(userID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to list conversations")
		return
	}

	ids := make([]string, len(conversations))
	for i, c := range conversations {
		ids[i] = c.ID
	}
	activeByConversationID, err := conversation_query.NewTurnRunQueryRepo(db).FindActiveSummariesByConversationIDs(ids)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up active turns")
		return
	}

	serverNow := time.Now().UTC().Format(time.RFC3339)
	out := make([]conversationListItemResponse, 0, len(conversations))
	for _, c := range conversations {
		item := conversationListItemResponse{conversationResponse: toConversationResponse(c)}
		if active, ok := activeByConversationID[c.ID]; ok {
			item.ActiveTurn = &activeTurnSummaryResponse{
				TurnRunID: active.TurnRunID,
				Status:    "running",
				StartedAt: active.StartedAt,
				ServerNow: serverNow,
			}
		}
		out = append(out, item)
	}
	response.SuccessResponse(w, http.StatusOK, out)
}
