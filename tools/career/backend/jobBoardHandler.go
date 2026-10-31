package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/jobboard"
	"career-tool-backend/profile"
)

// The job board routes (step 126), all behind tool:career:jobs. See
// plan/ai/tools/career/step-126-job-board-statuses.md. They live here, not in the jobboard
// package, because they check profiles and the profile package imports jobboard.

const maxJobBoardBodyBytes = 16 << 10

// writeJobBoardError maps a jobboard or profile error to its HTTP answer.
func writeJobBoardError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, jobboard.ErrInvalidName), errors.Is(err, jobboard.ErrDuplicateName),
		errors.Is(err, jobboard.ErrFixedStatus), errors.Is(err, jobboard.ErrStatusNotOnBoard):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, jobboard.ErrTooManyStatuses):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, jobboard.ErrUnknownJob), errors.Is(err, jobboard.ErrUnknownStatus), errors.Is(err, profile.ErrUnknownProfile):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

func decodeJobBoardBody(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJobBoardBodyBytes)).Decode(into); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

// jobBoardHandler handles GET /job-board?profileId= — one profile's board, every column.
func jobBoardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	profileID := r.URL.Query().Get("profileId")
	if profileID == "" {
		http.Error(w, "profileId query parameter is required", http.StatusBadRequest)
		return
	}
	if err := profile.RequireProfileExists(profileID); err != nil {
		writeJobBoardError(w, "read the job board", err)
		return
	}
	board, err := jobboard.GetBoard(profileID)
	if err != nil {
		writeJobBoardError(w, "read the job board", err)
		return
	}
	db.WriteJSON(w, board)
}

// jobStatusesHandler handles GET (every status: the fixed ones and every profile's own), POST (add a
// custom status), PUT (rename) and DELETE (?id=, its jobs move to Not Reviewed) on /job-statuses.
func jobStatusesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := jobboard.ListStatuses()
		if err != nil {
			writeJobBoardError(w, "list the statuses", err)
			return
		}
		db.WriteJSON(w, map[string]any{"statuses": list})

	case http.MethodPost:
		var body struct {
			ProfileID string `json:"profileId"`
			Name      string `json:"name"`
		}
		if !decodeJobBoardBody(w, r, &body) {
			return
		}
		if body.ProfileID == "" {
			http.Error(w, "profileId is required", http.StatusBadRequest)
			return
		}
		if err := profile.RequireProfileExists(body.ProfileID); err != nil {
			writeJobBoardError(w, "add the status", err)
			return
		}
		s, err := jobboard.CreateStatus(body.ProfileID, body.Name)
		if err != nil {
			writeJobBoardError(w, "add the status", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(s)

	case http.MethodPut:
		var body struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if !decodeJobBoardBody(w, r, &body) {
			return
		}
		if body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		s, err := jobboard.RenameStatus(body.ID, body.Name)
		if err != nil {
			writeJobBoardError(w, "rename the status", err)
			return
		}
		db.WriteJSON(w, s)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := jobboard.DeleteStatus(id); err != nil {
			writeJobBoardError(w, "delete the status", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// jobBoardEntriesHandler handles PUT (put a job on a board, or move it: one upsert) and
// DELETE (?profileId=&jobId=, take it off) on /job-board-entries.
func jobBoardEntriesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		var body struct {
			ProfileID string `json:"profileId"`
			JobID     string `json:"jobId"`
			StatusID  string `json:"statusId"`
		}
		if !decodeJobBoardBody(w, r, &body) {
			return
		}
		if body.ProfileID == "" || body.JobID == "" || body.StatusID == "" {
			http.Error(w, "profileId, jobId and statusId are required", http.StatusBadRequest)
			return
		}
		if err := profile.RequireProfileExists(body.ProfileID); err != nil {
			writeJobBoardError(w, "move the job", err)
			return
		}
		e, err := jobboard.SetEntry(body.ProfileID, body.JobID, body.StatusID)
		if err != nil {
			writeJobBoardError(w, "move the job", err)
			return
		}
		db.WriteJSON(w, e)

	case http.MethodDelete:
		q := r.URL.Query()
		if q.Get("profileId") == "" || q.Get("jobId") == "" {
			http.Error(w, "profileId and jobId query parameters are required", http.StatusBadRequest)
			return
		}
		if err := jobboard.RemoveEntry(q.Get("profileId"), q.Get("jobId")); err != nil {
			writeJobBoardError(w, "remove the job from the board", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// jobBoardUsageHandler handles GET /job-board-usage?jobId= — the job's status on every board
// that has it, with each profile's name. A profile that no longer exists is left out.
func jobBoardUsageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId query parameter is required", http.StatusBadRequest)
		return
	}
	usage, err := jobboard.GetUsage(jobID)
	if err != nil {
		writeJobBoardError(w, "read the job's boards", err)
		return
	}
	names, err := profile.Names()
	if err != nil {
		writeJobBoardError(w, "read the job's boards", err)
		return
	}
	type usageRow struct {
		jobboard.Usage
		profile.Name
	}
	rows := []usageRow{}
	for _, u := range usage {
		if n, ok := names[u.ProfileID]; ok {
			rows = append(rows, usageRow{Usage: u, Name: n})
		}
	}
	db.WriteJSON(w, map[string]any{"jobId": jobID, "profiles": rows})
}
