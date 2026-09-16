package handler

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/a-digi/coco-server/server/request"
)

// resolvedDataDir resolves the backend's own resolved data directory —
// backendapp.Start's "data_dir", the same one data/db, data/logs, and
// every tool subprocess's own TOOL_DB_DIR/TOOL_UPLOADS_DIR/TOOL_TMP_DIR
// (manager.ToolEnvVars) derive from — defaults to "data", next to the
// running executable, unless overridden via --data. Extracted out of
// toolsRoot so callers that need the bare data directory itself (not
// just its "tools" subdirectory) — install/enable handlers passing it
// into manager.Start/manager.ToolEnvVars — don't each re-duplicate the
// same DI lookup. See
// plan/ai/build/app/step-17-configurable-data-directory.md and
// plan/ai/build/app/step-22-data-dir-always-executable-relative.md.
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

// toolsRoot is where every installed tool's own directory lives —
// under the backend's own resolved data directory. Mirrors coco-mda's
// data/plugins/ layout, renamed. See
// plan/ai/build/app/step-17-configurable-data-directory.md.
func toolsRoot(reqCtx request.RequestContext) string {
	return filepath.Join(resolvedDataDir(reqCtx), "tools")
}

// defaultDataDirFallback mirrors backendapp.ResolveDataDir's own
// fallback ("data") for the case "data_dir" isn't in DI at all —
// should never happen once backendapp.Start has run, but a handler
// package staying self-contained (not importing backendapp just for
// one string constant) is worth a one-line duplicated literal.
const defaultDataDirFallback = "data"

// Fixed on-disk convention for a tool package — never configurable,
// never trusted from the manifest itself. See
// plan/ai/tools/step-02-manifest-and-safe-zip-extraction.md.
const (
	frontendBundleFilename    = "frontend/bundle.js"
	backendExecutableFilename = "backend/tool"
)

// isWithin reports whether path resolves to somewhere under root —
// defense in depth against a corrupted DB value before a destructive
// filesystem operation, matching coco-mda's own equivalent guard.
func isWithin(root, path string) (bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	return absPath == absRoot || strings.HasPrefix(absPath, absRoot+string(os.PathSeparator)), nil
}

// chmodBackendExecutableIfPresent makes a tool's own backend binary
// executable after extraction — zip entries don't reliably preserve
// the execute bit across platforms/zip tools.
func chmodBackendExecutableIfPresent(installDir string) error {
	execPath := filepath.Join(installDir, backendExecutableFilename)
	if _, err := os.Stat(execPath); err != nil {
		return nil // no backend executable in this package — nothing to do
	}
	return os.Chmod(execPath, 0o755)
}

// errAppVersionUnavailable is returned by appVersion whenever
// "app_version" isn't resolvable via DI — either it was never
// registered (shouldn't happen once backendapp.Start has run) or it
// resolved to an empty string (api/main.go's own non-fatal
// api/VERSION read failure, step 20). Both collapse to the same
// install_handler.go error text this tool-install path has always
// shown, preserving today's exact failure scope: a version-resolution
// problem breaks tool installation specifically, never app startup.
var errAppVersionUnavailable = errors.New("app version unavailable")

// appVersion resolves the running app's own version — registered into
// DI once, at boot, by backendapp.Start (embedded and always present
// for cmd/app; read fresh from api/VERSION, possibly empty on
// failure, for the dev binary) — rather than re-reading a bare
// "VERSION" file off CWD on every tool-install request the way
// readCurrentAppVersion used to. See
// plan/ai/build/app/step-20-app-version-via-di.md.
func appVersion(reqCtx request.RequestContext) (string, error) {
	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return "", errAppVersionUnavailable
	}
	raw, ok := storeCtx.Get("app_version")
	if !ok {
		return "", errAppVersionUnavailable
	}
	v, ok := raw.(string)
	if !ok || v == "" {
		return "", errAppVersionUnavailable
	}
	return v, nil
}

// errCorePortUnavailable mirrors errAppVersionUnavailable for
// corePort below — same "resolved once at boot, via DI" fix for the
// exact same class of bug readCorePort used to have (a bare
// server.LoadConfig("config.json") read, CWD-relative, no fallback —
// found sitting immediately adjacent to the reported "failed to
// determine the running app version" bug and fixed alongside it,
// since leaving it would have meant a fresh-elsewhere tool install
// hit the identical class of failure on its very next step). See
// plan/ai/build/app/step-20-app-version-via-di.md.
var errCorePortUnavailable = errors.New("core port unavailable")

// corePort resolves the backend's own listening port — registered
// into DI once, at boot, by backendapp.Start (it already loads
// config.json via the correctly-resolved configPath for its own
// tool_manager.StartAllEnabled call; this reuses that same value
// rather than re-reading config.json a second time here).
func corePort(reqCtx request.RequestContext) (int, error) {
	storeCtx, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return 0, errCorePortUnavailable
	}
	raw, ok := storeCtx.Get("core_port")
	if !ok {
		return 0, errCorePortUnavailable
	}
	p, ok := raw.(int)
	if !ok || p == 0 {
		return 0, errCorePortUnavailable
	}
	return p, nil
}
