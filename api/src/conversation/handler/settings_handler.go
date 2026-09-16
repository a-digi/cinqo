// settings_handler.go serves the conversation feature's own single
// global settings object — currently just the AI trace-logging toggle
// (step 37). Deliberately scoped to cinqo:super:admin only, unlike
// every other handler in this package: this is a system-wide switch
// affecting every user's conversations at once (turning it on captures
// raw model I/O for everyone, not just the caller's own data), not a
// per-conversation, per-owner concern the usual
// cinqo:conversation:use scope is meant for. See
// plan/ai/conversation/step-37-ai-debug-logging-toggle.md.
package handler

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

type settingsResponse struct {
	AITraceLogsEnabled bool `json:"aiTraceLogsEnabled"`
}

// GetSettingsHandler handles GET /api/v1/conversations/settings.
func GetSettingsHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	db, err := conversationDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "conversation database not configured")
		return
	}

	settings, err := conversation_query.NewSettingsQueryRepo(db).Load()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to load settings")
		return
	}

	response.SuccessResponse(w, http.StatusOK, settingsResponse{AITraceLogsEnabled: settings.AITraceLogsEnabled})
}

type updateSettingsRequest struct {
	AITraceLogsEnabled bool `json:"aiTraceLogsEnabled"`
}

// UpdateSettingsHandler handles PUT /api/v1/conversations/settings.
func UpdateSettingsHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	var body updateSettingsRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	db, err := conversationDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "conversation database not configured")
		return
	}

	if err := conversation_persistent.NewSettingsPersistentRepo(db).SetAITraceLogsEnabled(body.AITraceLogsEnabled); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to save settings")
		return
	}

	settings, err := conversation_query.NewSettingsQueryRepo(db).Load()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to reload settings")
		return
	}

	response.SuccessResponse(w, http.StatusOK, settingsResponse{AITraceLogsEnabled: settings.AITraceLogsEnabled})
}
