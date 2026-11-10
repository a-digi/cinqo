package hobbieshandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/hobbies"
)

// The profile's hobbies (plan 231). Route hobbies, scope tool:career:profile:
//
//	GET    ?profileId=                     { hobbies: [...] }
//	POST   {profileId, name, description?} the profile's list
//	PUT    {id, name?, description?}       the profile's list
//	DELETE ?id=                            204
//
// Every unknown id and every refusal of a caller (no name, too long, a hobby already listed) is a 400 with the reason; anything else a 500.
func writeError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, hobbies.ErrUnknownProfile):
		http.Error(w, "unknown profile id", http.StatusBadRequest)
	case errors.Is(err, hobbies.ErrUnknownHobby):
		http.Error(w, "unknown hobby id", http.StatusBadRequest)
	case errors.Is(err, hobbies.ErrNameRequired), errors.Is(err, hobbies.ErrNameTooLong), errors.Is(err, hobbies.ErrTextTooLong), errors.Is(err, hobbies.ErrDuplicate):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

func HobbiesHandler(w http.ResponseWriter, r *http.Request) {
	reload := func(profileID string) {
		list, err := hobbies.List(profileID)
		if err != nil {
			http.Error(w, "saved but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"hobbies": list})
	}
	switch r.Method {
	case http.MethodGet:
		profileID := r.URL.Query().Get("profileId")
		if profileID == "" {
			http.Error(w, "profileId query parameter is required", http.StatusBadRequest)
			return
		}
		reload(profileID)

	case http.MethodPost:
		var body struct {
			ProfileID   string `json:"profileId"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProfileID == "" {
			http.Error(w, "profileId and name are required", http.StatusBadRequest)
			return
		}
		if _, err := hobbies.Add(body.ProfileID, hobbies.Input{Name: body.Name, Description: body.Description}); err != nil {
			writeError(w, "add hobby", err)
			return
		}
		reload(body.ProfileID)

	case http.MethodPut:
		var body struct {
			ID          string  `json:"id"`
			Name        *string `json:"name,omitempty"`
			Description *string `json:"description,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		updated, err := hobbies.Update(body.ID, hobbies.Patch{Name: body.Name, Description: body.Description})
		if err != nil {
			writeError(w, "update hobby", err)
			return
		}
		reload(updated.ProfileID)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := hobbies.Delete(id); err != nil {
			writeError(w, "delete hobby", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
