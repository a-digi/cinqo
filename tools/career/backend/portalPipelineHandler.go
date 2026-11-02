package main

import (
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

// portalPipelineHandler handles GET /portal-links/pipeline (plan/ai/tools/career/step-163-job-detail-instructions-pipeline.md, step 1), behind
// tool:career:portals: where every link is in the chain from its listing instructions to a complete set of jobs. It only reads.
//
//	GET  { links: [{ portalLinkId, stage, waitingFor, since?, detail: { jobs, jobsWithDescription, detailFailedRuns } }] }
func portalPipelineHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	links, err := portal.PipelineForLinks()
	if err != nil {
		http.Error(w, "failed to read the pipeline: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]any{"links": links})
}
