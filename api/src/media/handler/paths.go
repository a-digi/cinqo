package handler

import (
	"path/filepath"

	"github.com/a-digi/coco-server/server/request"
)

// resolvedDataDir mirrors tool/handler.resolvedDataDir exactly — same
// "data_dir" DI key cinqo.Start registers, same small, deliberate
// duplication convention already established for handler packages that
// need this one value without importing tool/handler wholesale (see
// conversation/handler/shared.go's own copy).
func resolvedDataDir(reqCtx request.RequestContext) string {
	dataDir := defaultDataDirFallback
	if storeCtx, ok := reqCtx.GetDI().(diStore); ok {
		if raw, ok := storeCtx.Get("data_dir"); ok {
			if s, ok := raw.(string); ok && s != "" {
				dataDir = s
			}
		}
	}
	return dataDir
}

const defaultDataDirFallback = "data"

// mediaRoot is where every tool's own uploaded media lives — under the
// backend's own resolved data directory, deliberately separate from
// data/uploads/tools/{slug} (a tool's own private, TOOL_UPLOADS_DIR-
// addressed scratch space): Media is core-owned storage a tool never
// writes to directly, so giving it its own top-level tree keeps the
// two concepts from blurring together on disk. See
// plan/ai/media/step-01-media-feature.md.
func mediaRoot(reqCtx request.RequestContext, toolSlug string) string {
	return filepath.Join(resolvedDataDir(reqCtx), "media", "tools", toolSlug)
}
