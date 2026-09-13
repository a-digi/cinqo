package handler

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// ListHandler handles GET /api/v1/conversations — the caller's own
// conversations only, most recently started first. See
// plan/ai/conversation/step-03-conversation-api.md.
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

	out := make([]conversationResponse, 0, len(conversations))
	for _, c := range conversations {
		out = append(out, toConversationResponse(c))
	}
	response.SuccessResponse(w, http.StatusOK, out)
}
