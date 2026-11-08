package crawlhandlers

import (
	"encoding/json"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

type ingestCrawlResultsRequest struct {
	PortalLinkID string                   `json:"portalLinkId"`
	Pages        []portal.CrawlResultPage `json:"pages"`
}

// IngestCrawlResultsHandler handles POST /portal-links/ingest-crawl-
// results — the deterministic flow's last step: the frontend forwards
// browser's own /crawl-paginated response body (plus which portal
// link it was crawling) here unmodified, and this tool maps it onto
// job rows via portal.IngestCrawlResults' own fixed label vocabulary.
func IngestCrawlResultsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body ingestCrawlResultsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PortalLinkID == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return
	}
	result, err := portal.IngestCrawlResults(body.PortalLinkID, body.Pages)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "ingest crawl results", err)
		return
	}
	db.WriteJSON(w, result)
}
