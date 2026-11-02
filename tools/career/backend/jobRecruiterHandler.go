package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/jobrecruiters"
	"career-tool-backend/jobs"
	"career-tool-backend/recruiters"
)

// The route that maps recruiters to jobs (plan/ai/tools/career/step-168-recruiters-on-jobs.md, step 2), behind tool:career:jobs.

const maxJobRecruiterBodyBytes = 4 << 10

func writeJobRecruiterError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, jobs.ErrUnknownJob), errors.Is(err, recruiters.ErrUnknownRecruiter):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

// jobRecruitersHandler handles /job-recruiters:
//
//	GET ?jobId=        the recruiters mapped to a job            { recruiters: [...] }
//	GET ?recruiterId=  the jobs a recruiter is mapped to         { jobs: [...] }
//	POST {jobId, recruiterId}   maps one (a repeat is not an error)  { mapped, recruiters: [...] }
//	DELETE ?jobId=&recruiterId=  unmaps one (also when it was not mapped)  204
//
// Exactly one of jobId and recruiterId on a GET; an unknown id on a GET or a POST is a 404.
func jobRecruitersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jobID, recruiterID := r.URL.Query().Get("jobId"), r.URL.Query().Get("recruiterId")
		switch {
		case (jobID == "") == (recruiterID == ""):
			http.Error(w, "give exactly one of the jobId and recruiterId query parameters", http.StatusBadRequest)
		case jobID != "":
			if err := jobrecruiters.RequireJob(jobID); err != nil {
				writeJobRecruiterError(w, "list the recruiters of a job", err)
				return
			}
			list, err := jobrecruiters.ForJob(jobID)
			if err != nil {
				writeJobRecruiterError(w, "list the recruiters of a job", err)
				return
			}
			db.WriteJSON(w, map[string]any{"recruiters": list})
		default:
			if err := jobrecruiters.RequireRecruiter(recruiterID); err != nil {
				writeJobRecruiterError(w, "list the jobs of a recruiter", err)
				return
			}
			list, err := jobrecruiters.ForRecruiter(recruiterID)
			if err != nil {
				writeJobRecruiterError(w, "list the jobs of a recruiter", err)
				return
			}
			db.WriteJSON(w, map[string]any{"jobs": list})
		}

	case http.MethodPost:
		var body struct {
			JobID       string `json:"jobId"`
			RecruiterID string `json:"recruiterId"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJobRecruiterBodyBytes)).Decode(&body); err != nil || body.JobID == "" || body.RecruiterID == "" {
			http.Error(w, "jobId and recruiterId are required", http.StatusBadRequest)
			return
		}
		created, err := jobrecruiters.Map(body.JobID, body.RecruiterID)
		if err != nil {
			writeJobRecruiterError(w, "map the recruiter to the job", err)
			return
		}
		list, err := jobrecruiters.ForJob(body.JobID)
		if err != nil {
			http.Error(w, "recruiter mapped but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"mapped": created, "recruiters": list})

	case http.MethodDelete:
		jobID, recruiterID := r.URL.Query().Get("jobId"), r.URL.Query().Get("recruiterId")
		if jobID == "" || recruiterID == "" {
			http.Error(w, "jobId and recruiterId query parameters are required", http.StatusBadRequest)
			return
		}
		if err := jobrecruiters.Unmap(jobID, recruiterID); err != nil {
			writeJobRecruiterError(w, "unmap the recruiter from the job", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
