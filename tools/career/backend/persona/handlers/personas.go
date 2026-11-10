package personahandlers

import (
	"career-tool-backend/handler"
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

type personaRequest struct {
	ProfileID   string `json:"profileId"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type personaUpdateRequest struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func PersonasHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personas, err := persona.ListPersonas(r.URL.Query().Get("profileId"))
		if err != nil {
			http.Error(w, "failed to list target roles: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"personas": personas})

	case http.MethodPost:
		var body personaRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if body.ProfileID == "" {
			http.Error(w, "profileId is required", http.StatusBadRequest)
			return
		}
		id, err := persona.CreatePersona(body.ProfileID, body.Name, body.Description)
		if err != nil {
			if errors.Is(err, profile.ErrUnknownProfile) {
				http.Error(w, "unknown profile id", http.StatusBadRequest)
				return
			}
			http.Error(w, "failed to create target role: "+err.Error(), http.StatusInternalServerError)
			return
		}
		personas, err := persona.ListPersonas("")
		if err != nil {
			http.Error(w, "target role created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"id": id, "personas": personas})

	case http.MethodPut:
		var body personaUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := persona.UpdatePersona(body.ID, body.Name, body.Description); err != nil {
			handler.WritePersonaAwareError(w, "update target role", err)
			return
		}
		personas, err := persona.ListPersonas("")
		if err != nil {
			http.Error(w, "target role updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"personas": personas})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := persona.DeletePersona(id); err != nil {
			http.Error(w, "failed to delete target role: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /persona-details ---
