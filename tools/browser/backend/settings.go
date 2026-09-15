// settings.go implements the Browser tool's own single settings object
// — currently just the Debug toggle (step 22): crawl logging (see
// crawl_log.go) only ever happens when DebugEnabled is true, and only
// captures each page's own raw HTML (a much larger, more sensitive
// payload) when DebugLogHTML is also true. Human/frontend-only, like
// crawl_logs itself — never an MCP tool, nothing about this needs the
// AI's own involvement. See
// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
package main

import (
	"encoding/json"
	"net/http"
)

type browserSettings struct {
	DebugEnabled bool `json:"debugEnabled"`
	DebugLogHTML bool `json:"debugLogHtml"`
}

// loadBrowserSettings always reads fresh from the database — never
// cached in a package variable. This is a live toggle a human can flip
// through the frontend at any moment while this long-lived HTTP-mode
// sibling process keeps running; a cached value would go stale the
// moment someone actually used the settings page.
func loadBrowserSettings() (browserSettings, error) {
	var debugEnabled, debugLogHTML int
	err := browserDB.QueryRow(`SELECT debug_enabled, debug_log_html FROM browser_settings WHERE id = 1`).
		Scan(&debugEnabled, &debugLogHTML)
	if err != nil {
		return browserSettings{}, err
	}
	return browserSettings{DebugEnabled: debugEnabled != 0, DebugLogHTML: debugLogHTML != 0}, nil
}

func saveBrowserSettings(s browserSettings) error {
	_, err := browserDB.Exec(
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

// browserSettingsHandler handles GET/PUT /browser-settings — the
// Debug page's own read/write endpoint.
func browserSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := loadBrowserSettings()
		if err != nil {
			http.Error(w, "failed to load settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeBrowserSettings(w, settings)

	case http.MethodPut:
		var settings browserSettings
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := saveBrowserSettings(settings); err != nil {
			http.Error(w, "failed to save settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		saved, err := loadBrowserSettings()
		if err != nil {
			http.Error(w, "failed to reload settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeBrowserSettings(w, saved)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeBrowserSettings(w http.ResponseWriter, settings browserSettings) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(settings)
}
