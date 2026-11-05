package profilehandlers

import (
	"career-tool-backend/handler"
	"encoding/json"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/persona"
)

type experienceRequest struct {
	PersonaID   string `json:"personaId"`
	Company     string `json:"company"`
	Title       string `json:"title"`
	StartDate   string `json:"startDate"`
	EndDate     string `json:"endDate"`
	Description string `json:"description"`
}

type experienceUpdateRequest struct {
	PersonaID   string  `json:"personaId"`
	ID          string  `json:"id"`
	Company     *string `json:"company,omitempty"`
	Title       *string `json:"title,omitempty"`
	StartDate   *string `json:"startDate,omitempty"`
	EndDate     *string `json:"endDate,omitempty"`
	Description *string `json:"description,omitempty"`
}

func ExperienceHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		result, err := persona.FetchPersonaDetails(personaID)
		if err != nil {
			handler.WritePersonaAwareError(w, "read experience", err)
			return
		}
		db.WriteJSON(w, map[string]any{"experience": result.Experience})

	case http.MethodPost:
		var body experienceRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Company == "" || body.Title == "" {
			http.Error(w, "company and title are both required", http.StatusBadRequest)
			return
		}
		if body.PersonaID == "" {
			http.Error(w, "personaId is required", http.StatusBadRequest)
			return
		}
		if _, err := persona.AddCareerExperience(body.PersonaID, body.Company, body.Title, body.StartDate, body.EndDate, body.Description); err != nil {
			handler.WritePersonaAwareError(w, "add experience", err)
			return
		}
		result, err := persona.FetchPersonaDetails(body.PersonaID)
		if err != nil {
			http.Error(w, "experience added but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"experience": result.Experience})

	case http.MethodPut:
		var body experienceUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if body.PersonaID == "" {
			http.Error(w, "personaId is required", http.StatusBadRequest)
			return
		}
		if err := persona.UpdateCareerExperience(persona.UpdateCareerExperienceArgs{
			PersonaID:   body.PersonaID,
			ID:          body.ID,
			Company:     body.Company,
			Title:       body.Title,
			StartDate:   body.StartDate,
			EndDate:     body.EndDate,
			Description: body.Description,
		}); err != nil {
			handler.WritePersonaAwareError(w, "update experience", err)
			return
		}
		result, err := persona.FetchPersonaDetails(body.PersonaID)
		if err != nil {
			http.Error(w, "experience updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"experience": result.Experience})

	case http.MethodDelete:
		personaID := r.URL.Query().Get("personaId")
		id := r.URL.Query().Get("id")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := persona.RemoveCareerExperience(personaID, id); err != nil {
			handler.WritePersonaAwareError(w, "remove experience", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /jobs ---
