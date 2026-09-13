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
	"github.com/a-digi/cinqo/src/platform"
)

type createConversationRequest struct {
	Title      string `json:"title"`
	PlatformID string `json:"platformId"`
	Model      string `json:"model"`
}

// CreateHandler handles POST /api/v1/conversations. title defaults to
// "New conversation" when omitted. platformId/model are fixed for the
// conversation's entire lifetime — no endpoint ever changes them after
// this. See
// plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md.
func CreateHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()

	var body createConversationRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	title := body.Title
	if title == "" {
		title = "New conversation"
	}

	entry, ok := platform.Lookup(body.PlatformID)
	if !ok {
		response.ErrorResponse(w, http.StatusBadRequest, "platformId is required and must be a registered platform")
		return
	}

	model := body.Model
	if len(entry.SelectableModels) > 0 {
		if model == "" || !platform.IsSelectableModel(body.PlatformID, model) {
			response.ErrorResponse(w, http.StatusBadRequest, "model is required for this platform and must be one of its selectable models")
			return
		}
	} else {
		if model != "" {
			response.ErrorResponse(w, http.StatusBadRequest, "this platform has no selectable models — omit model")
			return
		}
		model = entry.DefaultModel
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
		ID:         id,
		UserID:     userID,
		Title:      title,
		FilePath:   conversation.LogPath(conversation.LogsRoot, id),
		PlatformID: body.PlatformID,
		Model:      model,
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
