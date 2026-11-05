package jobshandlers

import (
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

// JobLocationsHandler handles GET /job-locations — the Jobs page's own
// Location filter dropdown's data source (step — see
// jobs.ListDistinctJobLocations's own doc comment). Read-only, no
// query params: every distinct location is always returned, the same
// "fetch everything, no pagination" shape fetchCompanies/fetchPortals
// already use for their own filter dropdowns.
func JobLocationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	locations, err := jobs.ListDistinctJobLocations()
	if err != nil {
		http.Error(w, "failed to list job locations: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]any{"locations": locations})
}

// --- /companies ---
