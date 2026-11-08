package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/tool/manager"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// DisableHandler handles POST /api/v1/tools/{slug}/disable. Stops the
// process BEFORE flipping the enabled flag — a request landing in the
// tiny window between "flag flipped" and "process actually killed"
// would otherwise still reach a technically-still-alive backend. See
// plan/ai/tools/step-04-enable-disable-and-tool-manager.md.
func DisableHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	slug := reqCtx.GetURI().GetPathVariable("slug")
	if slug == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "slug is required")
		return
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	queryRepo := tool_query.NewToolQueryRepo(db)
	persistentRepo := tool_persistent.NewToolPersistentRepo(db)

	tool, err := queryRepo.FindBySlug(slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "tool not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up tool")
		return
	}

	if err := manager.Stop(db, tool.ID); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to stop tool: "+err.Error())
		return
	}

	if err := persistentRepo.SetEnabled(tool.ID, false); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to disable tool: "+err.Error())
		return
	}

	reloaded, err := queryRepo.FindBySlug(slug)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "tool disabled but failed to reload")
		return
	}
	mcpTools, err := mcpToolNamesFor(db, reloaded.ID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "tool disabled but failed to load functions")
		return
	}
	response.SuccessResponse(w, http.StatusOK, toToolResponse(reloaded, systemToolsConfigFrom(reqCtx), mcpTools))
}
