package handler

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// ListHandler handles GET /api/v1/tools. authenticated (not admin-only,
// unlike install/enable/disable/delete) — every signed-in user needs to
// know which tools exist and are enabled to build their own menu
// correctly. Tool.InstallPath/PID are json:"-" (step 1), so nothing
// server-internal leaks here. See
// plan/ai/tools/step-07-frontend-bridge-and-menu-integration.md.
func ListHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB

	tools, err := tool_query.NewToolQueryRepo(db).List()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to list tools")
		return
	}
	response.SuccessResponse(w, http.StatusOK, tools)
}
