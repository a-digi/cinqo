package profilehandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile/profileitems"
)

// The profile's library of skills and experience, and what a persona uses of it (plan 173, step 2). Routes: profile-skills,
// profile-experience (the library of one profile) and persona-skills, persona-experience (what one persona uses, with the library to
// choose from and the persona's own items). All scope tool:career:profile. The older skills and experience routes keep their paths and
// shapes and write the persona's OWN items only.

// writeProfileItemsError: every unknown id and every refusal of a caller is a 400 (a duplicate skill a 409), like the other routes of this
// tool; anything else a 500.
func writeProfileItemsError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, profileitems.ErrUnknownProfile):
		http.Error(w, "unknown profile id", http.StatusBadRequest)
	case errors.Is(err, profileitems.ErrUnknownPersona), errors.Is(err, persona.ErrUnknownPersona):
		http.Error(w, "unknown target role id", http.StatusBadRequest)
	case errors.Is(err, profileitems.ErrUnknownItem):
		http.Error(w, "unknown skill or experience id", http.StatusBadRequest)
	case errors.Is(err, profileitems.ErrDuplicateSkill):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, profileitems.ErrOtherProfile), errors.Is(err, profileitems.ErrSkillRequired),
		errors.Is(err, profileitems.ErrExperienceRequired), errors.Is(err, profileitems.ErrTitleOverrideEmpty):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

func ProfileSkillsHandler(w http.ResponseWriter, r *http.Request) {
	reload := func(profileID string) {
		list, err := profileitems.ListSkills(profileID)
		if err != nil {
			http.Error(w, "saved but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"skills": list})
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
			ProfileID string `json:"profileId"`
			Skill     string `json:"skill"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProfileID == "" {
			http.Error(w, "profileId and skill are required", http.StatusBadRequest)
			return
		}
		if _, _, err := profileitems.AddSkill(body.ProfileID, body.Skill); err != nil {
			writeProfileItemsError(w, "add skill", err)
			return
		}
		reload(body.ProfileID)

	case http.MethodPut:
		var body struct {
			ID    string `json:"id"`
			Skill string `json:"skill"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id and skill are required", http.StatusBadRequest)
			return
		}
		existing, err := profileitems.SkillByID(body.ID)
		if err != nil {
			writeProfileItemsError(w, "rename skill", err)
			return
		}
		if err := profileitems.RenameSkill(body.ID, body.Skill); err != nil {
			writeProfileItemsError(w, "rename skill", err)
			return
		}
		reload(existing.ProfileID)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := profileitems.DeleteSkill(id); err != nil {
			writeProfileItemsError(w, "delete skill", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func ProfileExperienceHandler(w http.ResponseWriter, r *http.Request) {
	reload := func(profileID string) {
		list, err := profileitems.ListExperience(profileID)
		if err != nil {
			http.Error(w, "saved but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"experience": list})
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
			Company     string `json:"company"`
			Title       string `json:"title"`
			StartDate   string `json:"startDate"`
			EndDate     string `json:"endDate"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProfileID == "" {
			http.Error(w, "profileId, company and title are required", http.StatusBadRequest)
			return
		}
		if _, err := profileitems.AddExperience(body.ProfileID, profileitems.ExperienceInput{
			Company: body.Company, Title: body.Title, StartDate: body.StartDate, EndDate: body.EndDate, Description: body.Description,
		}); err != nil {
			writeProfileItemsError(w, "add experience", err)
			return
		}
		reload(body.ProfileID)

	case http.MethodPut:
		var body struct {
			ID          string  `json:"id"`
			Company     *string `json:"company,omitempty"`
			Title       *string `json:"title,omitempty"`
			StartDate   *string `json:"startDate,omitempty"`
			EndDate     *string `json:"endDate,omitempty"`
			Description *string `json:"description,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		existing, err := profileitems.ExperienceByID(body.ID)
		if err != nil {
			writeProfileItemsError(w, "update experience", err)
			return
		}
		if err := profileitems.UpdateExperience(body.ID, profileitems.ExperiencePatch{
			Company: body.Company, Title: body.Title, StartDate: body.StartDate, EndDate: body.EndDate, Description: body.Description,
		}); err != nil {
			writeProfileItemsError(w, "update experience", err)
			return
		}
		reload(existing.ProfileID)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := profileitems.DeleteExperience(id); err != nil {
			writeProfileItemsError(w, "delete experience", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// personaSkillsView is what the persona's skills page needs: which library skills it uses, the library to choose from, and its own
// custom skills.
func personaSkillsView(w http.ResponseWriter, personaID string) {
	use, err := profileitems.GetPersonaUse(personaID)
	if err != nil {
		writeProfileItemsError(w, "read what the target role uses", err)
		return
	}
	library, err := profileitems.SkillsOfPersonasProfile(personaID)
	if err != nil {
		writeProfileItemsError(w, "read the library", err)
		return
	}
	details, err := persona.FetchPersonaDetails(personaID)
	if err != nil {
		writeProfileItemsError(w, "read the target role's skills", err)
		return
	}
	own := []map[string]string{}
	for _, it := range details.OwnSkillItems() {
		own = append(own, map[string]string{"id": it.ID, "skill": it.Skill})
	}
	db.WriteJSON(w, map[string]any{"used": use.SkillIDs, "library": library, "own": own})
}

func PersonaSkillsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		personaSkillsView(w, personaID)

	case http.MethodPut:
		var body struct {
			PersonaID string    `json:"personaId"`
			SkillIDs  *[]string `json:"skillIds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PersonaID == "" || body.SkillIDs == nil {
			http.Error(w, "personaId and skillIds are required", http.StatusBadRequest)
			return
		}
		if err := profileitems.SetPersonaSkills(body.PersonaID, *body.SkillIDs); err != nil {
			writeProfileItemsError(w, "set the target role's skills", err)
			return
		}
		personaSkillsView(w, body.PersonaID)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func personaExperienceView(w http.ResponseWriter, personaID string) {
	use, err := profileitems.GetPersonaUse(personaID)
	if err != nil {
		writeProfileItemsError(w, "read what the target role uses", err)
		return
	}
	library, err := profileitems.ExperienceOfPersonasProfile(personaID)
	if err != nil {
		writeProfileItemsError(w, "read the library", err)
		return
	}
	details, err := persona.FetchPersonaDetails(personaID)
	if err != nil {
		writeProfileItemsError(w, "read the target role's experience", err)
		return
	}
	db.WriteJSON(w, map[string]any{"used": use.Experience, "library": library, "own": details.OwnExperience()})
}

func PersonaExperienceHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		personaExperienceView(w, personaID)

	case http.MethodPut:
		var body struct {
			PersonaID  string                        `json:"personaId"`
			Experience *[]profileitems.ExperienceUse `json:"experience"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PersonaID == "" || body.Experience == nil {
			http.Error(w, "personaId and experience are required", http.StatusBadRequest)
			return
		}
		if err := profileitems.SetPersonaExperience(body.PersonaID, *body.Experience); err != nil {
			writeProfileItemsError(w, "set the target role's experience", err)
			return
		}
		personaExperienceView(w, body.PersonaID)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
