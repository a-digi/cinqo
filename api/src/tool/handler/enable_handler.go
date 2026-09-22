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

	// Re-enabling doesn't re-upload a manifest — read the
	// already-installed package's own manifest.json back off disk,
	// same as discoverMCPToolsIfDeclared below already needed to.
	// Parsed once here and reused there, rather than read twice.
	var m manifest.Manifest
	if manifestBytes, readErr := os.ReadFile(filepath.Join(tool.InstallPath, "manifest.json")); readErr == nil {
		if parsed, parseErr := manifest.Parse(manifestBytes); parseErr == nil {
			m = parsed
		}
	}

	// A dependency (browser, say) may have been disabled or removed
	// since this tool was first installed — "cannot be used" means
	// re-enabling must fail now, not just installing originally. See
	// plan/ai/tools/step-14-tool-dependencies.md.
	installedTools, err := installedToolsMap(queryRepo)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to check tool dependencies")
		return
	}
	if err := manifest.ValidateRequiredTools(m, installedTools); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := persistentRepo.SetEnabled(tool.ID, true); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to enable tool: "+err.Error())
		return
	}

	port, err := corePort(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to determine backend port")
		return
	}

	tool.Enabled = true
	if err := manager.Start(db, resolvedDataDir(reqCtx), *tool, port); err != nil {
		// Best-effort, matching the reference: a start failure leaves
		// status "error" (already recorded by manager.Start itself),
		// doesn't fail the enable request — the tool is enabled, just
		// not currently running.
		reqCtx.GetDI().GetLogger().Warning("tool %q enabled but failed to start: %v", slug, err)
	}

	// m was already read+parsed above (for the dependency check) — its
	// own zero value has MCP: false if that read/parse failed, so
	// discoverMCPToolsIfDeclared's own early-return still applies the
	// same as if this were a fresh read here.
	discoverMCPToolsIfDeclared(reqCtx, db, m, tool.InstallPath, tool.ID, tool.ServiceToken)

	reloaded, err := queryRepo.FindBySlug(slug)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "tool enabled but failed to reload")
		return
	}
	mcpTools, err := mcpToolNamesFor(db, reloaded.ID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "tool enabled but failed to load functions")
		return
	}
	response.SuccessResponse(w, http.StatusOK, toToolResponse(reloaded, systemToolsConfigFrom(reqCtx), mcpTools))
}
