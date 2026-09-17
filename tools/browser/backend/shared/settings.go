// settings.go implements the Browser tool's own single settings object
// — currently just the Debug toggle (step 22): crawl logging (see
// crawler/crawl_log.go) only ever happens when DebugEnabled is true,
// and only captures each page's own raw HTML (a much larger, more
// sensitive payload) when DebugLogHTML is also true. Human/frontend-
// only, like crawl_logs itself — never an MCP tool, nothing about this
// needs the AI's own involvement. Lives in shared, not crawler, even
// though the crawler package is its only real reader today
// (paginate.go) — it isn't crawl-specific behavior itself, just a
// tool-wide toggle crawl logging happens to be gated behind. See
// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
package shared

import (
	"encoding/json"
	"net/http"
)

type BrowserSettings struct {
	DebugEnabled bool `json:"debugEnabled"`
	DebugLogHTML bool `json:"debugLogHtml"`
}

// LoadBrowserSettings always reads fresh from the database — never
// cached in a package variable. This is a live toggle a human can flip
// through the frontend at any moment while this long-lived HTTP-mode
// sibling process keeps running; a cached value would go stale the
// moment someone actually used the settings page.
func LoadBrowserSettings() (BrowserSettings, error) {
	var debugEnabled, debugLogHTML int
	err := DB.QueryRow(`SELECT debug_enabled, debug_log_html FROM browser_settings WHERE id = 1`).
		Scan(&debugEnabled, &debugLogHTML)
	if err != nil {
		return BrowserSettings{}, err
	}
	return BrowserSettings{DebugEnabled: debugEnabled != 0, DebugLogHTML: debugLogHTML != 0}, nil
}

func SaveBrowserSettings(s BrowserSettings) error {
	_, err := DB.Exec(
		`UPDATE browser_settings SET debug_enabled = ?, debug_log_html = ?, updated_at = datetime('now') WHERE id = 1`,
		boolToInt(s.DebugEnabled), boolToInt(s.DebugLogHTML),
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
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
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
