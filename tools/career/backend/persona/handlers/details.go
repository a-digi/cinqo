package personahandlers

import (
	"career-tool-backend/handler"
	"encoding/json"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/persona"
)

func PersonaDetailsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		result, err := persona.FetchPersonaDetails(personaID)
		if err != nil {
			handler.WritePersonaAwareError(w, "read persona details", err)
			return
		}
		db.WriteJSON(w, result)

	case http.MethodPost:
		var body persona.UpdatePersonaDetailsArgs
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if body.PersonaID == "" {
			http.Error(w, "personaId is required", http.StatusBadRequest)
			return
		}
		if err := persona.UpdatePersonaDetails(body); err != nil {
			handler.WritePersonaAwareError(w, "update persona details", err)
			return
		}
		result, err := persona.FetchPersonaDetails(body.PersonaID)
		if err != nil {
			http.Error(w, "persona details updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, result)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /skills ---
