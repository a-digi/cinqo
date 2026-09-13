package handler

import (
	"net/http"
	"path/filepath"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// FrontendBundleHandler handles GET /api/v1/tools/{slug}/frontend-bundle
// — serves a tool's frontend/bundle.js byte-for-byte. Public: the
// <script> tag request happens before the browser necessarily has a
// warm session, and the bundle itself is static JS, not sensitive data
// — matching coco-mda's own reasoning for its equivalent endpoint,
// which likewise serves a bundle regardless of the tool's enabled
// state (the enforcement boundary is "does the bootstrap choose to
// inject it," not "can the raw file be fetched" — the same category as
// any other public static asset this app already serves). Serving the
// file itself being public doesn't bypass anything else: a tool's own
// registered menu entry still goes through filterVisible's scope
// check, and its backend calls still go through step 5's proxy
// enforcement. See
// plan/ai/tools/step-07-frontend-bridge-and-menu-integration.md.
func FrontendBundleHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	slug := reqCtx.GetURI().GetPathVariable("slug")

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	tool, err := tool_query.NewToolQueryRepo(db).FindBySlug(slug)
	if err != nil {
		response.ErrorResponse(w, http.StatusNotFound, "tool not found")
		return
	}
	if tool.FrontendBundleRelpath == "" {
		response.ErrorResponse(w, http.StatusNotFound, "tool has no frontend bundle")
		return
	}

	within, err := isWithin(toolsRoot, tool.InstallPath)
	if err != nil || !within {
		response.ErrorResponse(w, http.StatusInternalServerError, "invalid install path")
		return
	}

	bundlePath := filepath.Join(tool.InstallPath, tool.FrontendBundleRelpath)
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	http.ServeFile(w, reqCtx.GetRequest(), bundlePath)
}
