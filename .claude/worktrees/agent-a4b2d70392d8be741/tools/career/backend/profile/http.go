// http.go is the plain HTTP mirror of this package's own MCP tools
// (profile.go/persona.go/persona_details.go) — needed because a
// human's own browser session and the AI's own MCP tool-calling loop
// are two different callers needing two different transports to reach
// the same underlying data (the same reason browser's own
// /login-credentials route exists alongside its MCP tools). Every
// handler here calls the exact same functions those files already
// built — never independently reimplemented. Deliberately not
// MCP-exposed themselves (the AI already has the real MCP tools) —
// scope enforcement happens at the host's own reverse proxy via
// manifest.json's routes[], same as every other tool's own HTTP
// routes; nothing here re-checks it.
//
// Originally these handlers shared one flat http.go with every other
// domain's own; each now lives beside the business logic it calls, in
// that domain's own package. See
// plan/ai/tools/career/step-05-react-tailwind-frontend.md,
// plan/ai/tools/career/step-08-persona.md (personaId threading), and
// plan/ai/tools/career/step-10-job-seeker-profile.md (/profiles,
// /profile-links, /profile renamed to /persona-details).
package profile

import (
	"encoding/json"
	"net/http"

	"career-tool-backend/shared"
)

// writePersonaAwareError maps errUnknownPersona to a 400 (a caller
// mistake) and everything else to a 500, matching persona_details.go's
// own personaAwareErrResult reasoning on the MCP side.
func writePersonaAwareError(w http.ResponseWriter, action string, err error) {
	shared.WriteKnownError(w, err, errUnknownPersona, "unknown persona id", action)
}

// writeProfileAwareError is the same idea for errUnknownProfile.
func writeProfileAwareError(w http.ResponseWriter, action string, err error) {
	shared.WriteKnownError(w, err, errUnknownProfile, "unknown profile id", action)
}

// --- /profiles ---

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
		profiles, err := listProfiles()
		if err != nil {
			http.Error(w, "failed to list profiles: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"profiles": profiles})

	case http.MethodPost:
		var body profileRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		id, err := createProfile(body.FirstName, body.LastName)
		if err != nil {
			http.Error(w, "failed to create profile: "+err.Error(), http.StatusInternalServerError)
			return
		}
		profiles, err := listProfiles()
		if err != nil {
			http.Error(w, "profile created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"id": id, "profiles": profiles})

	case http.MethodPut:
		var body profileUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := updateProfile(body.ID, body.FirstName, body.LastName); err != nil {
			writeProfileAwareError(w, "update profile", err)
			return
		}
		profiles, err := listProfiles()
		if err != nil {
			http.Error(w, "profile updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"profiles": profiles})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := deleteProfile(id); err != nil {
			http.Error(w, "failed to delete profile: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /profile-links ---

type profileLinkRequest struct {
	ProfileID string `json:"profileId"`
	Platform  string `json:"platform"`
	URL       string `json:"url"`
}

func ProfileLinksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profileID := r.URL.Query().Get("profileId")
		if profileID == "" {
			http.Error(w, "profileId query parameter is required", http.StatusBadRequest)
			return
		}
		profiles, err := listProfiles()
		if err != nil {
			http.Error(w, "failed to load external links: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for _, p := range profiles {
			if p.ID == profileID {
				shared.WriteJSON(w, map[string]any{"externalLinks": p.ExternalLinks})
				return
			}
		}
		http.Error(w, "unknown profile id", http.StatusBadRequest)

	case http.MethodPost:
		var body profileLinkRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Platform == "" || body.URL == "" {
			http.Error(w, "platform and url are both required", http.StatusBadRequest)
			return
		}
		if body.ProfileID == "" {
			http.Error(w, "profileId is required", http.StatusBadRequest)
			return
		}
		if err := upsertProfileExternalLink(body.ProfileID, body.Platform, body.URL); err != nil {
			writeProfileAwareError(w, "add external link", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		profileID := r.URL.Query().Get("profileId")
		platform := r.URL.Query().Get("platform")
		if profileID == "" {
			http.Error(w, "profileId query parameter is required", http.StatusBadRequest)
			return
		}
		if platform == "" {
			http.Error(w, "platform query parameter is required", http.StatusBadRequest)
			return
		}
		if err := removeProfileExternalLink(profileID, platform); err != nil {
			writeProfileAwareError(w, "remove external link", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /personas ---

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
		personas, err := listPersonas(r.URL.Query().Get("profileId"))
		if err != nil {
			http.Error(w, "failed to list personas: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"personas": personas})

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
		id, err := createPersona(body.ProfileID, body.Name, body.Description)
		if err != nil {
			writeProfileAwareError(w, "create persona", err)
			return
		}
		personas, err := listPersonas("")
		if err != nil {
			http.Error(w, "persona created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"id": id, "personas": personas})

	case http.MethodPut:
		var body personaUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := updatePersona(body.ID, body.Name, body.Description); err != nil {
			writePersonaAwareError(w, "update persona", err)
			return
		}
		personas, err := listPersonas("")
		if err != nil {
			http.Error(w, "persona updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"personas": personas})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := deletePersona(id); err != nil {
			http.Error(w, "failed to delete persona: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /persona-details ---

func PersonaDetailsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		result, err := FetchPersonaDetails(personaID)
		if err != nil {
			writePersonaAwareError(w, "read persona details", err)
			return
		}
		shared.WriteJSON(w, result)

	case http.MethodPost:
		var body updatePersonaDetailsArgs
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if body.PersonaID == "" {
			http.Error(w, "personaId is required", http.StatusBadRequest)
			return
		}
		if err := updatePersonaDetails(body); err != nil {
			writePersonaAwareError(w, "update persona details", err)
			return
		}
		result, err := FetchPersonaDetails(body.PersonaID)
		if err != nil {
			http.Error(w, "persona details updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, result)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /skills ---

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
		result, err := FetchPersonaDetails(personaID)
		if err != nil {
			writePersonaAwareError(w, "read skills", err)
			return
		}
		shared.WriteJSON(w, map[string]any{"skills": result.Skills})

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
		if err := addCareerSkill(body.PersonaID, body.Skill); err != nil {
			writePersonaAwareError(w, "add skill", err)
			return
		}
		result, err := FetchPersonaDetails(body.PersonaID)
		if err != nil {
			http.Error(w, "skill added but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"skills": result.Skills})

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
		if err := removeCareerSkill(personaID, skill); err != nil {
			writePersonaAwareError(w, "remove skill", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /experience ---

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
		result, err := FetchPersonaDetails(personaID)
		if err != nil {
			writePersonaAwareError(w, "read experience", err)
			return
		}
		shared.WriteJSON(w, map[string]any{"experience": result.Experience})

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
		if _, err := addCareerExperience(body.PersonaID, body.Company, body.Title, body.StartDate, body.EndDate, body.Description); err != nil {
			writePersonaAwareError(w, "add experience", err)
			return
		}
		result, err := FetchPersonaDetails(body.PersonaID)
		if err != nil {
			http.Error(w, "experience added but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"experience": result.Experience})

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
		if err := updateCareerExperience(updateCareerExperienceArgs{
			PersonaID:   body.PersonaID,
			ID:          body.ID,
			Company:     body.Company,
			Title:       body.Title,
			StartDate:   body.StartDate,
			EndDate:     body.EndDate,
			Description: body.Description,
		}); err != nil {
			writePersonaAwareError(w, "update experience", err)
			return
		}
		result, err := FetchPersonaDetails(body.PersonaID)
		if err != nil {
			http.Error(w, "experience updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"experience": result.Experience})

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
		if err := removeCareerExperience(personaID, id); err != nil {
			writePersonaAwareError(w, "remove experience", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
