package deletedjobs

import (
	"errors"
	"net/http"
	"strconv"

	"career-tool-backend/db"
)

// listResponse is the answer of GET /jobs/deleted. Available is false when the deleted-jobs database could not be
// opened: there is then nothing to list and nothing is being remembered.
type listResponse struct {
	Available bool   `json:"available"`
	Total     int    `json:"total"`
	Items     []Item `json:"items"`
}

// Handler handles GET /jobs/deleted?limit= (the remembered jobs, newest first), DELETE /jobs/deleted?id= (allow one job
// again) and DELETE /jobs/deleted?all=true (allow every job again). Allowing a job again only lets the next crawl add
// it as a new job; it does not bring anything back.
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 0
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				http.Error(w, "limit must be a whole number", http.StatusBadRequest)
				return
			}
			limit = n
		}
		items, total, err := List(limit)
		if errors.Is(err, ErrNoMemory) {
			db.WriteJSON(w, listResponse{Available: false, Items: []Item{}})
			return
		}
		if err != nil {
			http.Error(w, "failed to list the deleted jobs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, listResponse{Available: true, Total: total, Items: items})
	case http.MethodDelete:
		q := r.URL.Query()
		switch {
		case q.Get("all") == "true" && q.Get("id") == "":
			n, err := ForgetAll()
			if writeMemoryError(w, err) {
				return
			}
			db.WriteJSON(w, map[string]int{"released": n})
		case q.Get("id") != "" && q.Get("all") == "":
			id, err := strconv.ParseInt(q.Get("id"), 10, 64)
			if err != nil || id <= 0 {
				http.Error(w, "id must be a positive whole number", http.StatusBadRequest)
				return
			}
			found, err := ForgetByID(id)
			if writeMemoryError(w, err) {
				return
			}
			if !found {
				http.Error(w, "that job is not in the list", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "give either id=<number> or all=true", http.StatusBadRequest)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// writeMemoryError answers an error from the store and reports whether it did.
func writeMemoryError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNoMemory):
		http.Error(w, "the deleted jobs database is not available", http.StatusServiceUnavailable)
	default:
		http.Error(w, "failed: "+err.Error(), http.StatusInternalServerError)
	}
	return true
}
