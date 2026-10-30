package crawler

// The Browser tool's Home dashboard widget. Cinqo's dashboard engine calls this
// route with the caller's own token (after checking tool:browser:crawl itself)
// and validates the answer against the core's "stat" schema
// (api/src/dashboard/kinds.go); text is shortened to its limits because the engine
// rejects an over-long field outright. See plan/ai/dashboard/home-dashboard.md.

import (
	"encoding/json"
	"net/http"
	"strings"

	"browser-tool-backend/shared"
)

const maxCaptionRunes = 200

type statPayload struct {
	Label   string `json:"label"`
	Value   string `json:"value"`
	Caption string `json:"caption,omitempty"`
	Link    string `json:"link,omitempty"`
}

// browserStatus is "Ready" when Chrome or Chromium can be found (what every
// crawl needs), otherwise "Needs setup" with the reason and a link to the page
// that explains how to fix it.
func browserStatus(chrome shared.ChromeStatus) statPayload {
	if chrome.Installed {
		caption := strings.TrimSpace(chrome.Version)
		if caption == "" {
			caption = "Chrome found"
		}
		return statPayload{Label: "Browser", Value: "Ready", Caption: truncateRunes(caption, maxCaptionRunes)}
	}
	return statPayload{
		Label:   "Browser",
		Value:   "Needs setup",
		Caption: truncateRunes(chrome.Detail, maxCaptionRunes),
		Link:    "/tools/browser/chromium",
	}
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// DashboardStatusHandler handles GET /dashboard/status.
func DashboardStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(browserStatus(shared.CurrentChromeStatus()))
}
