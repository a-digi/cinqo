package crawl

import (
	"encoding/json"
	"net/http"
	"strings"

	"career-tool-backend/portal"
)

// window_cleared.go (plan 198): the person's answer "yes, I cleared the check" in the Browser tool's Katana window. The Browser cannot always
// see that a check was cleared, so the screen asks the person; this route relays the answer for the link's own site (or its confirmed job
// site) and nothing else. It starts no read: the screen reads again afterwards.

const (
	browserWindowClearedPath = "/api/v1/tools/browser/proxy/katana-window-cleared"
	browserWindowOpenPath    = "/api/v1/tools/browser/proxy/katana-window-open"
)

// LinkCrawlWindowClearedHandler handles POST /portal-links/link-crawl/window-cleared {portalLinkId, url?}.
func LinkCrawlWindowClearedHandler(w http.ResponseWriter, r *http.Request) {
	relayWindowAnswer(w, r, browserWindowClearedPath)
}

// LinkCrawlWindowOpenHandler handles POST /portal-links/link-crawl/window-open {portalLinkId, url?}: the card's "Keep Katana in the visible
// browser". The Browser opens its Katana window at the link's address for the person to clear a check; the person answers with window-cleared.
func LinkCrawlWindowOpenHandler(w http.ResponseWriter, r *http.Request) {
	relayWindowAnswer(w, r, browserWindowOpenPath)
}

func relayWindowAnswer(w http.ResponseWriter, r *http.Request, browserPath string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		PortalLinkID string `json:"portalLinkId"`
		URL          string `json:"url,omitempty"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "portalLinkId is required (and only portalLinkId and url are accepted)", http.StatusBadRequest)
		return
	}
	cookie, err := r.Cookie("access_token")
	if err != nil {
		http.Error(w, "no active session cookie found", http.StatusUnauthorized)
		return
	}
	linkID := strings.TrimSpace(body.PortalLinkID)
	if linkID == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return
	}
	linkURL, err := portal.GetPortalLinkURL(linkID)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "read the link", err)
		return
	}
	jobHost, _ := portal.LinkJobHost(linkID)
	page, err := jobSitePage(linkURL, jobHost, body.URL) // the link's own site or its confirmed job site, nothing else
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	coreURL, err := coreAPIURL()
	if err != nil {
		http.Error(w, "core API address is not configured", http.StatusInternalServerError)
		return
	}
	payload, _ := json.Marshal(map[string]string{"url": page})
	respBody, status, err := postWithBearer(r.Context(), coreURL+browserPath, payload, cookie.Value)
	if err != nil {
		http.Error(w, "the Browser tool did not answer: "+err.Error(), http.StatusBadGateway)
		return
	}
	if status != http.StatusOK {
		http.Error(w, strings.TrimSpace(string(respBody)), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
