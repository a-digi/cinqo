package handler

import (
	"net/http"
	"os"

	"github.com/google/uuid"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/conversation"
	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

type createConversationRequest struct {
	Title string `json:"title"`
}

// CreateHandler handles POST /api/v1/conversations. title defaults to
// "New conversation" when omitted — the user renames it via PATCH once
// they've actually seen what it's about. See
// plan/ai/conversation/step-03-conversation-api.md.
func CreateHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()

	var body createConversationRequest
	_ = reqCtx.BindJSON(&body) // title is optional — an empty/missing body is fine

	title := body.Title
	if title == "" {
		title = "New conversation"
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

	if err := os.MkdirAll(conversation.LogsRoot, 0o755); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to prepare conversation storage")
		return
	}

	id := uuid.NewString()
	c := &conversation_entity.Conversation{
		ID:       id,
		UserID:   userID,
		Title:    title,
		FilePath: conversation.LogPath(conversation.LogsRoot, id),
	}

	if err := conversation_persistent.NewConversationPersistentRepo(db).Insert(c); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to create conversation")
		return
	}

	created, err := conversation_query.NewConversationQueryRepo(db).FindOwnedByID(id, userID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "conversation created but failed to reload")
		return
	}
	response.SuccessResponse(w, http.StatusCreated, toConversationResponse(created))
}
