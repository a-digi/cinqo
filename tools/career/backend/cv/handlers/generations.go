package cvhandlers

import (
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

// CvGenerationsHandler handles GET /jobs/cv-generations?jobId= — the
// full audit trail of every CV generation attempt ever made for a job
// (job_cv_generations, an append-only table distinct from job_cv_pdfs'
// own single "current state" row — see db.go's own doc comment). See
// plan/ai/career/cv-generation-audit/step-03-backend-wiring.md.
func CvGenerationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId query parameter is required", http.StatusBadRequest)
		return
	}
	generations, err := jobs.ListCvGenerationsForJob(jobID)
	if err != nil {
		if errors.Is(err, jobs.ErrUnknownJob) {
			http.Error(w, "unknown job id", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to list cv generations: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]any{"generations": generations})
}
