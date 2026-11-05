package profilehandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/profile"
)

// writeProfileAwareError is the same idea for
// profile.ErrUnknownProfile.
func writeProfileAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, profile.ErrUnknownProfile) {
		http.Error(w, "unknown profile id", http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

type profileRequest struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

type profileUpdateRequest struct {
	ID        string  `json:"id"`
	FirstName *string `json:"firstName,omitempty"`
	LastName  *string `json:"lastName,omitempty"`
}

func ProfilesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profiles, err := profile.ListProfiles()
		if err != nil {
			http.Error(w, "failed to list profiles: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"profiles": profiles})

	case http.MethodPost:
		var body profileRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		id, err := profile.CreateProfile(body.FirstName, body.LastName)
		if err != nil {
			http.Error(w, "failed to create profile: "+err.Error(), http.StatusInternalServerError)
			return
		}
		profiles, err := profile.ListProfiles()
		if err != nil {
			http.Error(w, "profile created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"id": id, "profiles": profiles})

	case http.MethodPut:
		var body profileUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := profile.UpdateProfile(body.ID, body.FirstName, body.LastName); err != nil {
			writeProfileAwareError(w, "update profile", err)
			return
		}
		profiles, err := profile.ListProfiles()
		if err != nil {
			http.Error(w, "profile updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"profiles": profiles})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		// The profile's photo is not deleted with it: photos live in the
		// library and other profiles may link the same one (step 110).
		if err := profile.DeleteProfile(id); err != nil {
			http.Error(w, "failed to delete profile: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /profile-links ---
