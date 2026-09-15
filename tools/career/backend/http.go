// http.go is the plain HTTP mirror of profile.go/persona.go/
// persona_details.go/jobs.go's own MCP tools — needed because a
// human's own browser session and the AI's own MCP tool-calling loop
// are two different callers needing two different transports to reach
// the same underlying data (the same reason browser's own
// /login-credentials route exists alongside its MCP tools). Every
// handler here calls the exact same functions those files already
// built — never independently reimplemented. Deliberately not
// MCP-exposed themselves (the AI already has the real MCP tools) —
// scope enforcement happens at the host's own reverse proxy via
// manifest.json's routes[], same as every other tool's own HTTP
// routes; nothing here re-checks it. See
// plan/ai/tools/career/step-05-react-tailwind-frontend.md,
// plan/ai/tools/career/step-08-persona.md (personaId threading), and
// plan/ai/tools/career/step-10-job-seeker-profile.md (/profiles,
// /profile-links, /profile renamed to /persona-details).
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
// mistake) and everything else to a 500, matching
// persona_details.go's own personaAwareErrResult reasoning on the MCP
// side.
func writePersonaAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, errUnknownPersona) {
		http.Error(w, "unknown persona id", http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

// writeProfileAwareError is the same idea for errUnknownProfile.
func writeProfileAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, errUnknownProfile) {
		http.Error(w, "unknown profile id", http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

// writeCompanyAwareError is the same idea for errUnknownCompany.
func writeCompanyAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, errUnknownCompany) {
		http.Error(w, "unknown company id", http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

// writeRecruiterAwareError is the same idea for errUnknownRecruiter.
func writeRecruiterAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, errUnknownRecruiter) {
		http.Error(w, "unknown recruiter id", http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

// writePortalAwareError is the same idea for errUnknownPortal.
func writePortalAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, errUnknownPortal) {
		http.Error(w, "unknown portal id", http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

// writePortalLinkAwareError additionally maps errDuplicatePortalLink
// (the same url added twice to one portal) and errInvalidCrawlInstructions
// (step 19's own shape check) to a 400 — the latter's own error text
// already carries the specific, actionable reason (e.g. "pagination.
// maxPages is required"), so it's passed straight through rather than
// replaced with a generic message.
func writePortalLinkAwareError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, errUnknownPortal):
		http.Error(w, "unknown portal id", http.StatusBadRequest)
	case errors.Is(err, errUnknownPortalLink):
		http.Error(w, "unknown portal link id", http.StatusBadRequest)
	case errors.Is(err, errDuplicatePortalLink):
		http.Error(w, "this url is already a link on this portal", http.StatusBadRequest)
	case errors.Is(err, errInvalidCrawlInstructions):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, errNoCrawlInstructions):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
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

func profilesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profiles, err := listProfiles()
		if err != nil {
			http.Error(w, "failed to list profiles: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"profiles": profiles})

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
		writeJSON(w, map[string]any{"id": id, "profiles": profiles})

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
		writeJSON(w, map[string]any{"profiles": profiles})

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

func profileLinksHandler(w http.ResponseWriter, r *http.Request) {
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
				writeJSON(w, map[string]any{"externalLinks": p.ExternalLinks})
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

func personasHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personas, err := listPersonas(r.URL.Query().Get("profileId"))
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
		personas, err := listPersonas("")
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

// --- /persona-details ---

func personaDetailsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		result, err := fetchPersonaDetails(personaID)
		if err != nil {
			writePersonaAwareError(w, "read persona details", err)
			return
		}
		writeJSON(w, result)

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
		result, err := fetchPersonaDetails(body.PersonaID)
		if err != nil {
			http.Error(w, "persona details updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
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
		result, err := fetchPersonaDetails(personaID)
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
		result, err := fetchPersonaDetails(body.PersonaID)
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
		result, err := fetchPersonaDetails(personaID)
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
		result, err := fetchPersonaDetails(body.PersonaID)
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
		result, err := fetchPersonaDetails(body.PersonaID)
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

type linkJobToCompanyRequest struct {
	ID        string `json:"id"`
	CompanyID string `json:"companyId"`
}

// jobsHandler's own GET always calls searchJobs — deliberately not a
// separate list-vs-search branch: searchJobs already degrades to
// "return everything" when query/location/companyId are all empty,
// the same overlap this step's own design doc calls out on the MCP
// side (list_jobs/search_jobs). PUT is narrow — it only ever sets or
// clears company_id (step 14's own link_job_to_company), never any
// other job field: jobs are still populated only by the AI's own
// crawling workflow (step 4), never hand-entered through this UI. See
// plan/ai/tools/career/step-14-companies.md.
func jobsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		limit := atoiOrZero(q.Get("limit"))
		offset := atoiOrZero(q.Get("offset"))
		result, err := searchJobs(q.Get("query"), q.Get("location"), q.Get("companyId"), q.Get("portalLinkId"), clampLimit(limit), offset)
		if err != nil {
			http.Error(w, "failed to list jobs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, result)

	case http.MethodPut:
		var body linkJobToCompanyRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := linkJobToCompany(body.ID, body.CompanyID); err != nil {
			writeCompanyAwareError(w, "link job to company", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

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

// --- /companies ---

type companyRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type companyUpdateRequest struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func companiesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		companies, err := listCompanies()
		if err != nil {
			http.Error(w, "failed to list companies: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"companies": companies})

	case http.MethodPost:
		var body companyRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		id, err := createCompany(body.Name, body.Description)
		if err != nil {
			http.Error(w, "failed to create company: "+err.Error(), http.StatusInternalServerError)
			return
		}
		companies, err := listCompanies()
		if err != nil {
			http.Error(w, "company created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"id": id, "companies": companies})

	case http.MethodPut:
		var body companyUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := updateCompany(body.ID, body.Name, body.Description); err != nil {
			writeCompanyAwareError(w, "update company", err)
			return
		}
		companies, err := listCompanies()
		if err != nil {
			http.Error(w, "company updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"companies": companies})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := deleteCompany(id); err != nil {
			http.Error(w, "failed to delete company: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /recruiters ---

type recruiterRequest struct {
	CompanyID string `json:"companyId"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
}

type recruiterUpdateRequest struct {
	ID        string  `json:"id"`
	FirstName *string `json:"firstName,omitempty"`
	LastName  *string `json:"lastName,omitempty"`
	Email     *string `json:"email,omitempty"`
}

func recruitersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		recruiters, err := listRecruiters(r.URL.Query().Get("companyId"))
		if err != nil {
			http.Error(w, "failed to list recruiters: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"recruiters": recruiters})

	case http.MethodPost:
		var body recruiterRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CompanyID == "" {
			http.Error(w, "companyId is required", http.StatusBadRequest)
			return
		}
		id, err := createRecruiter(body.CompanyID, body.FirstName, body.LastName, body.Email)
		if err != nil {
			writeCompanyAwareError(w, "create recruiter", err)
			return
		}
		recruiters, err := listRecruiters("")
		if err != nil {
			http.Error(w, "recruiter created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"id": id, "recruiters": recruiters})

	case http.MethodPut:
		var body recruiterUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := updateRecruiter(body.ID, body.FirstName, body.LastName, body.Email); err != nil {
			writeRecruiterAwareError(w, "update recruiter", err)
			return
		}
		recruiters, err := listRecruiters("")
		if err != nil {
			http.Error(w, "recruiter updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"recruiters": recruiters})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := deleteRecruiter(id); err != nil {
			http.Error(w, "failed to delete recruiter: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /portals ---

type portalRequest struct {
	Name string `json:"name"`
}

type portalUpdateRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func portalsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		portals, err := listPortals()
		if err != nil {
			http.Error(w, "failed to list portals: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"portals": portals})

	case http.MethodPost:
		var body portalRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		id, err := createPortal(body.Name)
		if err != nil {
			http.Error(w, "failed to create portal: "+err.Error(), http.StatusInternalServerError)
			return
		}
		portals, err := listPortals()
		if err != nil {
			http.Error(w, "portal created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"id": id, "portals": portals})

	case http.MethodPut:
		var body portalUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" || body.Name == "" {
			http.Error(w, "id and name are both required", http.StatusBadRequest)
			return
		}
		if err := updatePortal(body.ID, body.Name); err != nil {
			writePortalAwareError(w, "update portal", err)
			return
		}
		portals, err := listPortals()
		if err != nil {
			http.Error(w, "portal updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"portals": portals})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := deletePortal(id); err != nil {
			http.Error(w, "failed to delete portal: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /portal-links ---

type portalLinkRequest struct {
	PortalID string `json:"portalId"`
	URL      string `json:"url"`
	Title    string `json:"title"`
}

type portalLinkUpdateRequest struct {
	ID                string  `json:"id"`
	URL               *string `json:"url,omitempty"`
	Title             *string `json:"title,omitempty"`
	CrawlInstructions *string `json:"crawlInstructions,omitempty"`
}

func portalLinksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var body portalLinkRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
			http.Error(w, "url is required", http.StatusBadRequest)
			return
		}
		if body.PortalID == "" {
			http.Error(w, "portalId is required", http.StatusBadRequest)
			return
		}
		if body.Title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}
		id, err := addPortalLink(body.PortalID, body.URL, body.Title)
		if err != nil {
			writePortalLinkAwareError(w, "add portal link", err)
			return
		}
		portals, err := listPortals()
		if err != nil {
			http.Error(w, "portal link added but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"id": id, "portals": portals})

	case http.MethodPut:
		var body portalLinkUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if body.URL == nil && body.Title == nil && body.CrawlInstructions == nil {
			http.Error(w, "url, title, or crawlInstructions is required", http.StatusBadRequest)
			return
		}
		if body.URL != nil && *body.URL == "" {
			http.Error(w, "url cannot be empty", http.StatusBadRequest)
			return
		}
		if body.Title != nil && *body.Title == "" {
			http.Error(w, "title cannot be empty", http.StatusBadRequest)
			return
		}
		if body.URL != nil || body.Title != nil {
			if err := updatePortalLink(body.ID, body.URL, body.Title); err != nil {
				writePortalLinkAwareError(w, "update portal link", err)
				return
			}
		}
		if body.CrawlInstructions != nil {
			if err := updatePortalLinkCrawlInstructions(body.ID, *body.CrawlInstructions); err != nil {
				writePortalLinkAwareError(w, "update portal link crawl instructions", err)
				return
			}
		}
		portals, err := listPortals()
		if err != nil {
			http.Error(w, "portal link updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"portals": portals})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := removePortalLink(id); err != nil {
			http.Error(w, "failed to remove portal link: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- deterministic ("Crawl now") crawl support (step 27) ---
//
// Two new, separately registered routes rather than sub-paths of
// /portal-links — this tool's own router (main.go's plain
// http.HandleFunc calls) has no path-parameter matching, only exact
// registered paths, the same reason every other by-id operation in
// this file takes its id via ?id= or a JSON body field instead of a
// URL segment. See plan/ai/tools/career/step-27-ai-free-manual-crawl.md.

// crawlRequestHandler handles GET /portal-links/crawl-request?id=... —
// the frontend's first step in the deterministic "Crawl now" flow:
// turn the link's own stored crawl_instructions into the JSON shape
// browser's own POST /crawl-paginated (called directly by the
// frontend, via that tool's own proxy route) expects.
func crawlRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id query parameter is required", http.StatusBadRequest)
		return
	}
	req, err := buildCrawlRequest(id)
	if err != nil {
		writePortalLinkAwareError(w, "build crawl request", err)
		return
	}
	writeJSON(w, req)
}

type ingestCrawlResultsRequest struct {
	PortalLinkID string            `json:"portalLinkId"`
	Pages        []crawlResultPage `json:"pages"`
}

// ingestCrawlResultsHandler handles POST /portal-links/ingest-crawl-
// results — the deterministic flow's last step: the frontend forwards
// browser's own /crawl-paginated response body (plus which portal
// link it was crawling) here unmodified, and this tool maps it onto
// job rows via ingestCrawlResults' own fixed label vocabulary.
func ingestCrawlResultsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body ingestCrawlResultsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PortalLinkID == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return
	}
	result, err := ingestCrawlResults(body.PortalLinkID, body.Pages)
	if err != nil {
		writePortalLinkAwareError(w, "ingest crawl results", err)
		return
	}
	writeJSON(w, result)
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
