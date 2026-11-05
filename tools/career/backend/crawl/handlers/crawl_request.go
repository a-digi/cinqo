package crawlhandlers

import (
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

// CrawlRequestHandler handles GET /portal-links/crawl-request?id=... —
// the frontend's first step in the deterministic "Crawl now" flow:
// turn the link's own stored crawl_instructions into the JSON shape
// browser's own POST /crawl-paginated (called directly by the
// frontend, via that tool's own proxy route) expects.
func CrawlRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id query parameter is required", http.StatusBadRequest)
		return
	}
	req, err := portal.BuildCrawlRequest(id)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "build crawl request", err)
		return
	}
	db.WriteJSON(w, req)
}
