package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"os"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/tool/manager"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// DeleteHandler handles DELETE /api/v1/tools/{slug}. Irreversible —
// files removed before the DB row, so a failure partway through is
// safely retryable rather than leaving a DB row with no matching
// files. See plan/ai/tools/step-03-install-update-uninstall.md.
//
// A system tool (system-tools.yaml) is refused outright, before even
// looking the tool up in the database — the config file alone is
// enough to reject the request, and a rejected delete must never stop
// a system tool's running process as an unwanted side effect. No
// scope-based override exists: even cinqo:super:admin cannot delete a
// system tool through this endpoint. See
// plan/ai/tools/step-10-system-tools.md.
func DeleteHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	slug := reqCtx.GetURI().GetPathVariable("slug")
	if slug == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "slug is required")
		return
	}

	if protected, reason := systemToolsConfigFrom(reqCtx).IsSystemTool(slug); protected {
		msg := "system tools cannot be deleted — disable it instead"
		if reason != "" {
			msg += ": " + reason
		}
		response.ErrorResponse(w, http.StatusForbidden, msg)
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
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to stop tool before uninstalling: "+err.Error())
		return
	}

	within, err := isWithin(toolsRoot(reqCtx), tool.InstallPath)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to validate install path")
		return
	}
	if !within {
		// Defense in depth against a corrupted DB value — never
		// RemoveAll a path outside the tools sandbox root.
		response.ErrorResponse(w, http.StatusInternalServerError, "refusing to delete: install path is outside the tools directory")
		return
	}

	if err := os.RemoveAll(tool.InstallPath); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to remove tool files: "+err.Error())
		return
	}

	if err := persistentRepo.Delete(tool.ID); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "tool files removed but failed to delete database record: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
