package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/a-digi/coco-server/server"
)

// toolsRoot is where every installed tool's own directory lives —
// relative to the backend's CWD (api/, same convention as data/db,
// data/logs — see backendapp.Start()). Mirrors coco-mda's
// data/plugins/ layout, renamed.
const toolsRoot = "data/tools"

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

// readCurrentAppVersion reads the running backend's own VERSION file
// off CWD — both api/main.go and cmd/app/main.go run with CWD=api/, so
// this is always api/VERSION regardless of which binary is hosting the
// request. Simpler than distinguishing which binary is running (the
// two are otherwise indistinguishable from a request handler's own
// vantage point, and both expose the exact same HTTP API a tool talks
// to) — a deliberate simplification over step 2's originally-recommended
// "whichever binary is running," discovered to need new cross-cutting
// plumbing this step doesn't otherwise require. See
// plan/ai/tools/step-02-manifest-and-safe-zip-extraction.md's confirmed
// open question.
func readCurrentAppVersion() (string, error) {
	data, err := os.ReadFile("VERSION")
	if err != nil {
		return "", fmt.Errorf("read VERSION: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// readCorePort reads the backend's own listening port straight from
// config.json (the same file server.StartServer itself reads) — reused
// here rather than threading cfg.Port through DI, since it's already
// on disk and this is the only place outside main.go that needs it.
func readCorePort() (int, error) {
	cfg, err := server.LoadConfig("config.json")
	if err != nil {
		return 0, err
	}
	return cfg.Port, nil
}
