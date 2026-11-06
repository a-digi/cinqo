// engine_status.go (step 51, plan 187) — can each fetch engine actually run on
// this machine right now? Backs GET /engine-status, which the Engine page uses
// to show availability. Rod and Katana both need a Chrome binary (one
// resolver, shared.ChromePath), so both are available exactly when Chrome is
// found. Nothing here starts a browser. See
// plan/ai/tools/browser/step-51-engine-availability.md (the Playwright engine
// that file also covers was removed in plan 187).
package crawler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"browser-tool-backend/shared"
)

// EngineStatus is one engine's availability. Detail is a short sentence
// built from fixed phrases and a path — never raw process output.
type EngineStatus struct {
	Available bool   `json:"available"`
	Detail    string `json:"detail"`
}

// EngineStatusResponse is the body of GET /engine-status.
type EngineStatusResponse struct {
	Rod    EngineStatus `json:"rod"`
	Katana EngineStatus `json:"katana"`
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

// KatanaStatus (plan 184): Katana runs in the same Chrome binary the Rod
// engine uses (the one resolver, shared.ChromePath), so it is available
// exactly when Chrome is found. Nothing is started here.
func KatanaStatus() EngineStatus {
	st := RodStatus()
	if st.Available {
		st.Detail = "Katana will use the Chrome found at " + strings.TrimPrefix(st.Detail, "Chrome found at ")
	}
	return st
}

// EngineStatusHandler handles GET /engine-status. Computed on every
// request, so installing the driver is noticed on the next page load.
func EngineStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(EngineStatusResponse{Rod: RodStatus(), Katana: KatanaStatus()})
}
