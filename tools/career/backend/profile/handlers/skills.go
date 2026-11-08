package profilehandlers

import (
	"career-tool-backend/handler"
	"encoding/json"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/persona"
)

type skillRequest struct {
	PersonaID string `json:"personaId"`
	Skill     string `json:"skill"`
}

func SkillsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		result, err := persona.FetchPersonaDetails(personaID)
		if err != nil {
			handler.WritePersonaAwareError(w, "read skills", err)
			return
		}
		db.WriteJSON(w, map[string]any{"skills": result.Skills, "skillItems": result.SkillItems})

	case http.MethodPost:
		var body skillRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Skill == "" {
			http.Error(w, "skill is required", http.StatusBadRequest)
			return
		}
		if body.PersonaID == "" {
			http.Error(w, "personaId is required", http.StatusBadRequest)
			return
		}
		if err := persona.AddCareerSkill(body.PersonaID, body.Skill); err != nil {
			handler.WritePersonaAwareError(w, "add skill", err)
			return
		}
		result, err := persona.FetchPersonaDetails(body.PersonaID)
		if err != nil {
			http.Error(w, "skill added but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"skills": result.Skills})

	case http.MethodDelete:
		personaID := r.URL.Query().Get("personaId")
		skill := r.URL.Query().Get("skill")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		if skill == "" {
			http.Error(w, "skill query parameter is required", http.StatusBadRequest)
			return
		}
		if err := persona.RemoveCareerSkill(personaID, skill); err != nil {
			handler.WritePersonaAwareError(w, "remove skill", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /experience ---
