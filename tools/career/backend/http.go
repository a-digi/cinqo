// http.go is the plain HTTP mirror of profile.go/persona.go/jobs.go's
// own MCP tools — needed because a human's own browser session and
// the AI's own MCP tool-calling loop are two different callers
// needing two different transports to reach the same underlying data
// (the same reason browser's own /login-credentials route exists
// alongside its MCP tools). Every handler here calls the exact same
// fetchCareerProfile/updateCareerProfile/addCareerSkill/saveJob/etc.
// functions profile.go/persona.go/jobs.go already built — never
// independently reimplemented. Deliberately not MCP-exposed
// themselves (the AI already has the real MCP tools) — scope
// enforcement happens at the host's own reverse proxy via
// manifest.json's routes[], same as every other tool's own HTTP
// routes; nothing here re-checks it. See
// plan/ai/tools/career/step-05-react-tailwind-frontend.md and
// plan/ai/tools/career/step-08-persona.md (personaId threading).
package main

import (
	"encoding/json"
	"errors"
	"net/http"
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writePersonaAwareError maps errUnknownPersona to a 400 (a caller
// mistake) and everything else to a 500, matching profile.go's own
// personaAwareErrResult reasoning on the MCP side.
func writePersonaAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, errUnknownPersona) {
		http.Error(w, "unknown persona id", http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

// --- /personas ---

type personaRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type personaUpdateRequest struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func personasHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personas, err := listPersonas()
		if err != nil {
			http.Error(w, "failed to list personas: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"personas": personas})

	case http.MethodPost:
		var body personaRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		id, err := createPersona(body.Name, body.Description)
		if err != nil {
			http.Error(w, "failed to create persona: "+err.Error(), http.StatusInternalServerError)
			return
		}
		personas, err := listPersonas()
		if err != nil {
			http.Error(w, "persona created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"id": id, "personas": personas})

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
		personas, err := listPersonas()
		if err != nil {
			http.Error(w, "persona updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"personas": personas})

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

// --- /profile ---

func profileHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		result, err := fetchCareerProfile(personaID)
		if err != nil {
			writePersonaAwareError(w, "read profile", err)
			return
		}
		writeJSON(w, result)

	case http.MethodPost:
		var body updateCareerProfileArgs
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if body.PersonaID == "" {
			http.Error(w, "personaId is required", http.StatusBadRequest)
			return
		}
		if err := updateCareerProfile(body); err != nil {
			writePersonaAwareError(w, "update profile", err)
			return
		}
		result, err := fetchCareerProfile(body.PersonaID)
		if err != nil {
			http.Error(w, "profile updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, result)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /skills ---

type skillRequest struct {
	PersonaID string `json:"personaId"`
	Skill     string `json:"skill"`
}

func skillsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		result, err := fetchCareerProfile(personaID)
		if err != nil {
			writePersonaAwareError(w, "read skills", err)
			return
		}
		writeJSON(w, map[string]any{"skills": result.Skills})

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
		result, err := fetchCareerProfile(body.PersonaID)
		if err != nil {
			http.Error(w, "skill added but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"skills": result.Skills})

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

func experienceHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		result, err := fetchCareerProfile(personaID)
		if err != nil {
			writePersonaAwareError(w, "read experience", err)
			return
		}
		writeJSON(w, map[string]any{"experience": result.Experience})

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
		result, err := fetchCareerProfile(body.PersonaID)
		if err != nil {
			http.Error(w, "experience added but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"experience": result.Experience})

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
		result, err := fetchCareerProfile(body.PersonaID)
		if err != nil {
			http.Error(w, "experience updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"experience": result.Experience})

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

// --- /jobs ---

// jobsHandler's own GET always calls searchJobs — deliberately not a
// separate list-vs-search branch: searchJobs already degrades to
// "return everything" when both query and location are empty, the
// same overlap this step's own design doc calls out on the MCP side
// (list_jobs/search_jobs). No POST here at all — jobs are populated
// only by the AI's own crawling workflow (step 4), never hand-entered
// through this UI. Jobs stay persona-independent — step 8 deliberately
// did not touch this table (see step-08-persona.md's own open
// question 1).
func jobsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		limit := atoiOrZero(q.Get("limit"))
		offset := atoiOrZero(q.Get("offset"))
		result, err := searchJobs(q.Get("query"), q.Get("location"), clampLimit(limit), offset)
		if err != nil {
			http.Error(w, "failed to list jobs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, result)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := deleteJob(id); err != nil {
			http.Error(w, "failed to delete job: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func atoiOrZero(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
