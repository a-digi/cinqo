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
	if ierr := EnableTool(reqCtx, slug); ierr != nil {
		response.ErrorResponse(w, ierr.Status, ierr.Message)
		return
	}

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
	required, err := requiredToolsFor(db, reloaded.ID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "tool enabled but failed to load dependencies")
		return
	}
	response.SuccessResponse(w, http.StatusOK, toToolResponse(reloaded, systemToolsConfigFrom(reqCtx), mcpTools, required))
}

// EnableTool enables an installed tool and starts it: the same work as the enable route, for callers that enable several tools in a row (the ordered
// install of a tool together with its dependencies). Nothing is written when a dependency of the tool cannot be used.
func EnableTool(reqCtx request.RequestContext, slug string) *InstallError {
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	queryRepo := tool_query.NewToolQueryRepo(db)
	persistentRepo := tool_persistent.NewToolPersistentRepo(db)

	tool, err := queryRepo.FindBySlug(slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &InstallError{Status: http.StatusNotFound, Message: "tool not found"}
		}
		return &InstallError{Status: http.StatusInternalServerError, Message: "failed to look up tool"}
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
		return &InstallError{Status: http.StatusInternalServerError, Message: "failed to check tool dependencies"}
	}
	if err := manifest.ValidateRequiredTools(m, installedTools); err != nil {
		return &InstallError{Status: http.StatusBadRequest, Message: err.Error()}
	}

	if err := persistentRepo.SetEnabled(tool.ID, true); err != nil {
		return &InstallError{Status: http.StatusInternalServerError, Message: "failed to enable tool: " + err.Error()}
	}

	port, err := corePort(reqCtx)
	if err != nil {
		return &InstallError{Status: http.StatusInternalServerError, Message: "failed to determine backend port"}
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

	return nil
}
