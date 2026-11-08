package portalhandlers

import (
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

// ListingMethodForRequest names the method the Browser's selected engine crawls a link's job list with ("links" under Katana, "instructions"
// under Rod, "" when it cannot be read), read with the caller's own session. A var set by main.go (this package cannot reach the crawl
// package); the default reads nothing.
var ListingMethodForRequest = func(*http.Request) string { return "" }

// PortalPipelineHandler handles GET /portal-links/pipeline (plan/ai/tools/career/step-163-job-detail-instructions-pipeline.md, step 1), behind
// tool:career:portals: where every link is in the chain from its listing instructions to a complete set of jobs. It only reads.
//
//	GET  { links: [{ portalLinkId, stage, waitingFor, since?, detail: { jobs, jobsWithDescription, detailFailedRuns } }] }
func PortalPipelineHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	links, err := portal.PipelineForLinks(ListingMethodForRequest(r))
	if err != nil {
		http.Error(w, "failed to read the pipeline: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]any{"links": links})
}
