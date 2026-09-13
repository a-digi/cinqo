package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/tool/manager"
	"github.com/a-digi/cinqo/src/tool/manifest"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// EnableHandler handles POST /api/v1/tools/{slug}/enable. See
// plan/ai/tools/step-04-enable-disable-and-tool-manager.md.
func EnableHandler(reqCtx request.RequestContext) {
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

	if err := persistentRepo.SetEnabled(tool.ID, true); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to enable tool: "+err.Error())
		return
	}

	corePort, err := readCorePort()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to determine backend port")
		return
	}

	tool.Enabled = true
	if err := manager.Start(db, *tool, corePort); err != nil {
		// Best-effort, matching the reference: a start failure leaves
		// status "error" (already recorded by manager.Start itself),
		// doesn't fail the enable request — the tool is enabled, just
		// not currently running.
		reqCtx.GetDI().GetLogger().Warning("tool %q enabled but failed to start: %v", slug, err)
	}

	// Re-enabling doesn't re-upload a manifest — the declared "mcp"
	// flag is read back from the already-installed package's own
	// manifest.json rather than duplicated into a new tools column.
	if manifestBytes, readErr := os.ReadFile(filepath.Join(tool.InstallPath, "manifest.json")); readErr == nil {
		if m, parseErr := manifest.Parse(manifestBytes); parseErr == nil {
			discoverMCPToolsIfDeclared(reqCtx, db, m, tool.InstallPath, tool.ID)
		}
	}

	reloaded, err := queryRepo.FindBySlug(slug)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "tool enabled but failed to reload")
		return
	}
	response.SuccessResponse(w, http.StatusOK, reloaded)
}
