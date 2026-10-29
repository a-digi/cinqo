// engine_status.go (step 51) — can each fetch engine actually run on this
// machine right now? Backs GET /engine-status, which the Engine page uses to
// show availability and to lock the choice to Rod when Playwright is not
// installed. See plan/ai/tools/browser/step-51-engine-availability.md.
//
// Nothing here starts a browser or Playwright. Rod needs a Chrome binary.
// Playwright does NOT use the Node installed in the OS: playwright-go runs
// its own bundled Node, placed next to its driver, and Run() never
// downloads anything — it requires the driver (and the browsers) to have
// been installed beforehand and fails with "please install the driver
// first" otherwise. So "available" for Playwright means exactly what Run()
// checks before starting: the driver directory holds the CLI and the
// bundled node, and `node cli.js --version` reports the expected version.
package crawler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mxschmitt/playwright-go"

	"browser-tool-backend/shared"
)

// engineStatusTimeout bounds the one process call the Playwright check
// makes. A var so a test can shrink it.
var engineStatusTimeout = 5 * time.Second

// EngineStatus is one engine's availability. Detail is a short sentence
// built from fixed phrases and a path — never raw process output.
type EngineStatus struct {
	Available bool   `json:"available"`
	Detail    string `json:"detail"`
}

// EngineStatusResponse is the body of GET /engine-status.
type EngineStatusResponse struct {
	Rod        EngineStatus `json:"rod"`
	Playwright EngineStatus `json:"playwright"`
}

// RodStatus: rod is available when a Chrome binary is found — the same
// lookup the engine itself uses (rodChromePath).
func RodStatus() EngineStatus {
	path, err := rodChromePath()
	switch {
	case err == nil:
		return EngineStatus{Available: true, Detail: "Chrome found at " + path}
	case errors.Is(err, errRodNoChrome):
		return EngineStatus{Detail: "Chrome was not found (install Chrome, or set " + shared.ChromePathEnv + ")"}
	default:
		return EngineStatus{Detail: fmt.Sprintf("%s is set but not usable (%s)", shared.ChromePathEnv, strings.TrimSpace(os.Getenv(shared.ChromePathEnv)))}
	}
}

// playwrightDriverDir is where playwright-go keeps its driver for version:
// PLAYWRIGHT_DRIVER_PATH if set, else <cache>/ms-playwright-go/<version>
// with the library's own per-OS cache directory.
func playwrightDriverDir(version string) (string, error) {
	if p := strings.TrimSpace(os.Getenv("PLAYWRIGHT_DRIVER_PATH")); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	var cache string
	switch runtime.GOOS {
	case "windows":
		cache = filepath.Join(home, "AppData", "Local")
	case "darwin":
		cache = filepath.Join(home, "Library", "Caches")
	default:
		cache = filepath.Join(home, ".cache")
	}
	return filepath.Join(cache, "ms-playwright-go", version), nil
}

// PlaywrightStatus — see the file comment. Read-only: unlike Run(), it
// never creates the driver directory.
func PlaywrightStatus() EngineStatus {
	probe, err := playwright.NewDriver() // only to learn the version the library expects
	if err != nil {
		return EngineStatus{Detail: "Playwright could not be initialised"}
	}
	dir, err := playwrightDriverDir(probe.Version)
	if err != nil {
		return EngineStatus{Detail: "Playwright's driver directory could not be determined"}
	}
	driver, err := playwright.NewDriver(&playwright.RunOptions{DriverDirectory: dir})
	if err != nil {
		return EngineStatus{Detail: "Playwright could not be initialised"}
	}

	cli := filepath.Join(dir, "package", "cli.js")
	if p := strings.TrimSpace(os.Getenv("PLAYWRIGHT_CLI_PATH")); p != "" {
		cli = p
	}
	if _, err := os.Stat(cli); err != nil {
		return EngineStatus{Detail: fmt.Sprintf("Playwright driver is not installed (expected in %s; install it with the Playwright installer first)", dir)}
	}

	node := filepath.Join(dir, "node")
	if runtime.GOOS == "windows" {
		node += ".exe"
	}
	if p := strings.TrimSpace(os.Getenv("PLAYWRIGHT_NODEJS_PATH")); p != "" {
		node = p
	}
	if _, err := os.Stat(node); err != nil {
		return EngineStatus{Detail: fmt.Sprintf("Playwright driver is incomplete: its Node runtime is missing (%s)", node)}
	}

	cmd := driver.Command("--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return EngineStatus{Detail: "Playwright driver could not be run"}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return EngineStatus{Detail: "Playwright driver could not be run"}
		}
	case <-time.After(engineStatusTimeout):
		_ = cmd.Process.Kill()
		<-done
		return EngineStatus{Detail: "Playwright driver did not answer in time"}
	}
	if !bytes.Contains(out.Bytes(), []byte(driver.Version)) {
		return EngineStatus{Detail: fmt.Sprintf("Playwright driver in %s is not the version this tool needs (%s)", dir, driver.Version)}
	}
	return EngineStatus{Available: true, Detail: fmt.Sprintf("Playwright driver %s installed in %s", driver.Version, dir)}
}

// EngineStatusHandler handles GET /engine-status. Computed on every
// request, so installing the driver is noticed on the next page load.
func EngineStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(EngineStatusResponse{Rod: RodStatus(), Playwright: PlaywrightStatus()})
}
