package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/jobboard"
	"career-tool-backend/profile"
	"career-tool-backend/profilerules"
)

// The auto-add rules routes (step 140), behind tool:career:jobs like the board routes the settings page also uses. See
// plan/ai/tools/career/step-140-profile-job-match-consumer.md. They live here, not in the profilerules package, for the
// reason the board routes do: they answer for profiles.

// writeProfileRulesError maps a profilerules, jobboard or profile error to its HTTP answer: what the caller got wrong
// is a 400 (the percentage, an unknown or foreign status, an unknown profile on a write, too many rules), a second rule for
// one percentage is a 409, a rule or a profile that is not there on a read or a change of a rule is a 404.
func writeProfileRulesError(w http.ResponseWriter, action string, err error, unknownProfileIs404 bool) {
	switch {
	case errors.Is(err, profile.ErrUnknownProfile):
		code := http.StatusBadRequest
		if unknownProfileIs404 {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
	case errors.Is(err, profilerules.ErrInvalidPercent), errors.Is(err, profilerules.ErrTooManyRules),
		errors.Is(err, jobboard.ErrUnknownStatus), errors.Is(err, jobboard.ErrStatusNotOnBoard):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, profilerules.ErrDuplicatePercent):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, profilerules.ErrUnknownRule):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

// profileMatchRulesHandler handles GET (?profileId=, one profile's rules, highest percentage first), POST (add a rule),
// PUT (change a rule's percentage and status) and DELETE (?id=) on /profile-match-rules.
func profileMatchRulesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profileID := r.URL.Query().Get("profileId")
		if profileID == "" {
			http.Error(w, "profileId query parameter is required", http.StatusBadRequest)
			return
		}
		rules, err := profilerules.List(profileID)
		if err != nil {
			writeProfileRulesError(w, "list the rules", err, true)
			return
		}
		db.WriteJSON(w, map[string]any{"rules": rules})

	case http.MethodPost:
		var body struct {
			ProfileID  string `json:"profileId"`
			MinPercent int    `json:"minPercent"`
			StatusID   string `json:"statusId"`
		}
		if !decodeJobBoardBody(w, r, &body) {
			return
		}
		if body.ProfileID == "" {
			http.Error(w, "profileId is required", http.StatusBadRequest)
			return
		}
		rule, err := profilerules.Create(body.ProfileID, body.MinPercent, body.StatusID)
		if err != nil {
			writeProfileRulesError(w, "add the rule", err, false)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(rule)

	case http.MethodPut:
		var body struct {
			ID         string `json:"id"`
			MinPercent int    `json:"minPercent"`
			StatusID   string `json:"statusId"`
		}
		if !decodeJobBoardBody(w, r, &body) {
			return
		}
		if body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		rule, err := profilerules.Update(body.ID, body.MinPercent, body.StatusID)
		if err != nil {
			writeProfileRulesError(w, "change the rule", err, false)
			return
		}
		db.WriteJSON(w, rule)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := profilerules.Delete(id); err != nil {
			writeProfileRulesError(w, "delete the rule", err, false)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
