package profilehandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/profile/profiledetails"
)

// The summary and the goals of a profile (plan 174, step 3). Route profile-details, scope tool:career:profile:
//
//	GET ?profileId=                                                                     { profileId, summary, desiredTitles, desiredLocations, minSalary, remotePreference }
//	PUT {profileId, summary?, desiredTitles?, desiredLocations?, minSalary?, remotePreference?}   the same object, after the change
//
// A persona follows these values unless it has its own. An unknown profile, a remote preference outside "", onsite, open, remote_only and
// a negative minimum salary are 400s with the reason; nothing is written for a refused request.
func writeProfileDetailsError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, profiledetails.ErrUnknownProfile):
		http.Error(w, "unknown profile id", http.StatusBadRequest)
	case errors.Is(err, profiledetails.ErrNegativeSalary), errors.Is(err, profiledetails.ErrInvalidRemotePreference):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

func ProfileDetailsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profileID := r.URL.Query().Get("profileId")
		if profileID == "" {
			http.Error(w, "profileId query parameter is required", http.StatusBadRequest)
			return
		}
		v, err := profiledetails.Get(profileID)
		if err != nil {
			writeProfileDetailsError(w, "read the profile details", err)
			return
		}
		db.WriteJSON(w, v)

	case http.MethodPut:
		var body struct {
			ProfileID string `json:"profileId"`
			profiledetails.Patch
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProfileID == "" {
			http.Error(w, "profileId is required", http.StatusBadRequest)
			return
		}
		v, err := profiledetails.Update(body.ProfileID, body.Patch)
		if err != nil {
			writeProfileDetailsError(w, "update the profile details", err)
			return
		}
		db.WriteJSON(w, v)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
