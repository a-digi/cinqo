// settings.go implements the Browser tool's own single settings object
// — currently the Debug toggle (step 22) plus two independent,
// Debug-gated sub-toggles: crawl logging (see crawler/crawl_log.go)
// only ever happens when DebugEnabled is true; DebugLogHTML additionally
// captures each logged crawl page's own raw HTML (a much larger, more
// sensitive payload); DebugLogChallenge (step 68) separately gates the
// Cloudflare human-wait loop's own raw-HTML dump
// (waitForHumanToClearCloudflare, crawler/crawl.go) — a distinct
// concern from DebugLogHTML, not implied by it, since one is about
// regular crawl results and the other is about debugging a stuck
// challenge. Human/frontend-only, like crawl_logs itself — never an
// MCP tool, nothing about this needs the AI's own involvement. Lives
// in shared, not crawler, even though the crawler package is its only
// real reader today (paginate.go, crawl.go) — it isn't crawl-specific
// behavior itself, just a tool-wide toggle crawl logging happens to be
// gated behind. See
// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
package shared

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// The two engines that can serve the primary single-page fetch (step 50;
// see crawler/fetch_engine.go). Rod is the default.
const (
	FetchEngineRod        = "rod"
	FetchEnginePlaywright = "playwright"
	DefaultFetchEngine    = FetchEngineRod
)

// ErrInvalidFetchEngine is a caller mistake (a 400), never a server failure.
var ErrInvalidFetchEngine = errors.New(`fetchEngine must be "rod" or "playwright"`)

// ValidFetchEngine reports whether v is exactly one of the two engines.
func ValidFetchEngine(v string) bool {
	return v == FetchEngineRod || v == FetchEnginePlaywright
}

// fetchEngineOrDefault reads a stored value through the allow-list: a
// value that is somehow not one of the two engines (the column has a
// CHECK constraint, so only a hand-edited or foreign database could hold
// one) is treated as the default, never used as-is.
func fetchEngineOrDefault(stored string) string {
	if ValidFetchEngine(stored) {
		return stored
	}
	return DefaultFetchEngine
}

type BrowserSettings struct {
	DebugEnabled      bool `json:"debugEnabled"`
	DebugLogHTML      bool `json:"debugLogHtml"`
	DebugLogChallenge bool `json:"debugLogChallenge"`
	// FetchEngine (step 50) is "rod" or "playwright". On a PUT, an empty
	// or missing value means "leave the stored engine as it is" — so a
	// client that predates this field (or the Debug page saving the rest of
	// the object) can never reset it — and any other value is rejected.
	FetchEngine string `json:"fetchEngine"`
	// HumanAssist (step 96) — when an AI call meets a challenge only a person
	// can clear, open a browser window for that person instead of just
	// failing. On by default. On a PUT, a body without the key leaves the
	// stored value alone (a client that predates it must never switch it off).
	HumanAssist bool `json:"humanAssist"`
}

// LoadBrowserSettings always reads fresh from the database — never
// cached in a package variable. This is a live toggle a human can flip
// through the frontend at any moment while this long-lived HTTP-mode
// sibling process keeps running; a cached value would go stale the
// moment someone actually used the settings page.
func LoadBrowserSettings() (BrowserSettings, error) {
	var debugEnabled, debugLogHTML, debugLogChallenge, humanAssist int
	var fetchEngine string
	err := DB.QueryRow(`SELECT debug_enabled, debug_log_html, debug_log_challenge, fetch_engine, human_assist FROM browser_settings WHERE id = 1`).
		Scan(&debugEnabled, &debugLogHTML, &debugLogChallenge, &fetchEngine, &humanAssist)
	if err != nil {
		return BrowserSettings{}, err
	}
	return BrowserSettings{
		DebugEnabled:      debugEnabled != 0,
		DebugLogHTML:      debugLogHTML != 0,
		DebugLogChallenge: debugLogChallenge != 0,
		FetchEngine:       fetchEngineOrDefault(fetchEngine),
		HumanAssist:       humanAssist != 0,
	}, nil
}

// SaveBrowserSettings stores every field. FetchEngine must be one of the
// two engines (ErrInvalidFetchEngine otherwise — nothing is written);
// callers that mean "keep the stored engine" resolve that BEFORE calling
// (see the PUT handler).
func SaveBrowserSettings(s BrowserSettings) error {
	if !ValidFetchEngine(s.FetchEngine) {
		return ErrInvalidFetchEngine
	}
	_, err := DB.Exec(
		`UPDATE browser_settings SET debug_enabled = ?, debug_log_html = ?, debug_log_challenge = ?, fetch_engine = ?, human_assist = ?, updated_at = datetime('now') WHERE id = 1`,
		boolToInt(s.DebugEnabled), boolToInt(s.DebugLogHTML), boolToInt(s.DebugLogChallenge), s.FetchEngine, boolToInt(s.HumanAssist),
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// BrowserSettingsHandler handles GET/PUT /browser-settings — the
// Debug page's own read/write endpoint.
func BrowserSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := LoadBrowserSettings()
		if err != nil {
			http.Error(w, "failed to load settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeBrowserSettings(w, settings)

	case http.MethodPut:
		var settings BrowserSettings
		raw, err := io.ReadAll(r.Body)
		if err != nil || json.Unmarshal(raw, &settings) != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		// Step 96: a body without humanAssist keeps the stored value.
		var present map[string]json.RawMessage
		_ = json.Unmarshal(raw, &present)
		if _, ok := present["humanAssist"]; !ok {
			current, err := LoadBrowserSettings()
			if err != nil {
				http.Error(w, "failed to load settings: "+err.Error(), http.StatusInternalServerError)
				return
			}
			settings.HumanAssist = current.HumanAssist
		}
		// A missing/empty fetchEngine keeps the stored one (a client that
		// predates the field must never reset it); anything else must be
		// one of the two engines.
		if settings.FetchEngine == "" {
			current, err := LoadBrowserSettings()
			if err != nil {
				http.Error(w, "failed to load settings: "+err.Error(), http.StatusInternalServerError)
				return
			}
			settings.FetchEngine = current.FetchEngine
		} else if !ValidFetchEngine(settings.FetchEngine) {
			http.Error(w, ErrInvalidFetchEngine.Error(), http.StatusBadRequest)
			return
		}
		if err := SaveBrowserSettings(settings); err != nil {
			http.Error(w, "failed to save settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		saved, err := LoadBrowserSettings()
		if err != nil {
			http.Error(w, "failed to reload settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeBrowserSettings(w, saved)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeBrowserSettings(w http.ResponseWriter, settings BrowserSettings) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(settings)
}
