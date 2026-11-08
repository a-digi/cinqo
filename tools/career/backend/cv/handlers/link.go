package cvhandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/cv/cvbuilder"
	"career-tool-backend/jobs"
)

type cvLinkRequest struct {
	JobID        string `json:"jobId"`
	CvDocumentID string `json:"cvDocumentId"`
	Source       string `json:"source"`
}

// CvLinkHandler handles PUT /jobs/cv-link (make an existing CV document the
// job's current CV; source is "manual" for one just built in the CV Builder for
// this job, "existing" for one picked from the library) and DELETE
// /jobs/cv-link?jobId= (remove the job's current CV; the CV itself stays).
// Both are refused with 409 while an AI generation is running for the job. See
// plan/ai/tools/career/step-98-manual-and-existing-cv-for-a-job.md.
func CvLinkHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		var body cvLinkRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.JobID == "" || body.CvDocumentID == "" {
			http.Error(w, "jobId and cvDocumentId are required", http.StatusBadRequest)
			return
		}
		writeCvLinkResult(w, jobs.LinkCv(body.JobID, body.CvDocumentID, body.Source), "attach the CV")
	case http.MethodDelete:
		jobID := r.URL.Query().Get("jobId")
		if jobID == "" {
			http.Error(w, "jobId query parameter is required", http.StatusBadRequest)
			return
		}
		writeCvLinkResult(w, jobs.UnlinkCv(jobID), "remove the CV")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeCvLinkResult(w http.ResponseWriter, err error, action string) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, jobs.ErrUnknownJob):
		http.Error(w, "unknown job id", http.StatusNotFound)
	case errors.Is(err, cvbuilder.ErrUnknownCvDocument):
		http.Error(w, "unknown cv document id", http.StatusNotFound)
	case errors.Is(err, jobs.ErrCvGenerationRunning):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, jobs.ErrInvalidCvLinkSource):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, jobs.ErrCvDocumentHasNoPdf), errors.Is(err, jobs.ErrCvPersonaMissing):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}
