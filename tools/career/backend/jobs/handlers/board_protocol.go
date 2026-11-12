package jobshandlers

import (
	"log"
	"net/http"

	"career-tool-backend/applicationlog"
	"career-tool-backend/db"
	"career-tool-backend/jobs/jobboard"
	"career-tool-backend/profile"
)

// attachProtocolSummaries fills the Protocol of every card that has entries in the profile's protocol (step 176). It is a side view,
// like the persona scores: when the entries cannot be read it logs that and leaves the cards as they are, so the board never fails
// because of it. A card without entries keeps no Protocol, so it looks as before.
func attachProtocolSummaries(board *jobboard.Board, profileID string) {
	summaries, err := applicationlog.Summaries(profileID)
	if err != nil {
		log.Printf("job board: could not read the protocol of profile %s: %v", profileID, err)
		return
	}
	for ci := range board.Columns {
		for ji := range board.Columns[ci].Jobs {
			card := &board.Columns[ci].Jobs[ji]
			if s, ok := summaries[card.JobID]; ok {
				card.Protocol = &s
			}
		}
	}
}

// JobApplicationsHandler handles GET /job-applications?profileId= — the overview of one profile's applications (step 176, step 5):
// the jobs in its Applied status and the jobs with entries in its protocol, the most recent activity first.
func JobApplicationsHandler(w http.ResponseWriter, r *http.Request) {
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
		writeJobBoardError(w, "read the applications", err)
		return
	}
	apps, err := jobboard.Applications(profileID)
	if err != nil {
		writeJobBoardError(w, "read the applications", err)
		return
	}
	db.WriteJSON(w, map[string]any{"profileId": profileID, "applications": apps})
}

// ApplicationCountHandler handles GET /application-count — how many applications there are in all profiles, as {"total": N, "profiles": [...]}
// (plan 238, plan 265). The menu reads total to decide whether to show its Applications item; the Profiles page reads profiles, the numbers of
// each profile (total, applied, waiting). Both use the same rule as the Applications page.
func ApplicationCountHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	total, profiles, err := jobboard.ApplicationCounts()
	if err != nil {
		http.Error(w, "failed to count the applications", http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, struct {
		Total    int                      `json:"total"`
		Profiles []jobboard.ProfileCounts `json:"profiles"`
	}{total, profiles})
}
