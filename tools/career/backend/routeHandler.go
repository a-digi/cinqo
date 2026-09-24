// http.go is the plain HTTP mirror of the profile/persona/persona-
// details/jobs domains' own MCP tools — needed because a human's own
// browser session and the AI's own MCP tool-calling loop are two
// different callers needing two different transports to reach the same
// underlying data (the same reason browser's own /login-credentials
// route exists alongside its MCP tools). Every handler here calls the
// exact same functions those domains already built — never
// independently reimplemented. Deliberately not MCP-exposed themselves
// (the AI already has the real MCP tools) — scope enforcement happens
// at the host's own reverse proxy via manifest.json's routes[], same as
// every other tool's own HTTP routes; nothing here re-checks it. See
// plan/ai/tools/career/step-05-react-tailwind-frontend.md,
// plan/ai/tools/career/step-08-persona.md (personaId threading), and
// plan/ai/tools/career/step-10-job-seeker-profile.md (/profiles,
// /profile-links, /profile renamed to /persona-details).
//
// As of the package-split refactor (step XX), profile/persona/persona-
// details/recruiters/job_match's own business logic still lives
// alongside their own handlers here in package main — only companies/
// jobs/portals/the crawl-orchestration files moved into their own
// subpackages (companies/jobs/portal/crawl), mirroring tools/browser/
// backend's own auth/crawler/shared split. This file's own calls into
// those four are qualified accordingly; everything else below is
// unchanged. See plan/ai/tools/career/step-XX-package-split.md.
package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/companies"
	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/persona"
	"career-tool-backend/portal"
	"career-tool-backend/profile"
	"career-tool-backend/profileimage"
	"career-tool-backend/recruiters"
)

// writePersonaAwareError maps persona.ErrUnknownPersona to a 400 (a
// caller mistake) and everything else to a 500, matching the persona
// package's own personaAwareErrResult reasoning on the MCP side.
func writePersonaAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, persona.ErrUnknownPersona) {
		http.Error(w, "unknown persona id", http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

// writeProfileAwareError is the same idea for
// profile.ErrUnknownProfile.
func writeProfileAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, profile.ErrUnknownProfile) {
		http.Error(w, "unknown profile id", http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

// writeRecruiterAwareError is the same idea for
// recruiters.ErrUnknownRecruiter.
func writeRecruiterAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, recruiters.ErrUnknownRecruiter) {
		http.Error(w, "unknown recruiter id", http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
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
		// Best-effort, before the profile row (and its own
		// image_media_file_id column) is gone — same "capture then
		// forward-delete, never block the parent delete" reasoning
		// cvbuilder's own cv_documents DELETE case already established.
		profileimage.DeleteBestEffort(r, id)
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
		profiles, err := profile.ListProfiles()
		if err != nil {
			http.Error(w, "failed to load external links: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for _, p := range profiles {
			if p.ID == profileID {
				db.WriteJSON(w, map[string]any{"externalLinks": p.ExternalLinks})
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
		if err := profile.UpsertProfileExternalLink(body.ProfileID, body.Platform, body.URL); err != nil {
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
		if err := profile.RemoveProfileExternalLink(profileID, platform); err != nil {
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
		personas, err := persona.ListPersonas(r.URL.Query().Get("profileId"))
		if err != nil {
			http.Error(w, "failed to list personas: "+err.Error(), http.StatusInternalServerError)
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
			http.Error(w, "failed to create persona: "+err.Error(), http.StatusInternalServerError)
			return
		}
		personas, err := persona.ListPersonas("")
		if err != nil {
			http.Error(w, "persona created but failed to reload: "+err.Error(), http.StatusInternalServerError)
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
			writePersonaAwareError(w, "update persona", err)
			return
		}
		personas, err := persona.ListPersonas("")
		if err != nil {
			http.Error(w, "persona updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
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
		result, err := persona.FetchPersonaDetails(personaID)
		if err != nil {
			writePersonaAwareError(w, "read persona details", err)
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
			writePersonaAwareError(w, "update persona details", err)
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
		result, err := persona.FetchPersonaDetails(personaID)
		if err != nil {
			writePersonaAwareError(w, "read skills", err)
			return
		}
		db.WriteJSON(w, map[string]any{"skills": result.Skills})

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
			writePersonaAwareError(w, "add skill", err)
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
		result, err := persona.FetchPersonaDetails(personaID)
		if err != nil {
			writePersonaAwareError(w, "read experience", err)
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
			writePersonaAwareError(w, "add experience", err)
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
			writePersonaAwareError(w, "update experience", err)
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

// jobsHandler's own GET always calls jobs.SearchJobs — deliberately not
// a separate list-vs-search branch: SearchJobs already degrades to
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
		// id (Jobs page's own Eye icon -> job details page) short-circuits
		// to a single-job read instead of the paged list below — same
		// "?id= means one item" convention this handler's own DELETE
		// branch already uses.
		if id := q.Get("id"); id != "" {
			j, err := jobs.GetJobByID(id)
			if err != nil {
				if errors.Is(err, jobs.ErrUnknownJob) {
					http.Error(w, "unknown job id", http.StatusNotFound)
					return
				}
				http.Error(w, "failed to load job: "+err.Error(), http.StatusInternalServerError)
				return
			}
			db.WriteJSON(w, map[string]any{"job": j})
			return
		}
		limit := atoiOrZero(q.Get("limit"))
		offset := atoiOrZero(q.Get("offset"))
		result, err := jobs.SearchJobs(q.Get("query"), q.Get("location"), q.Get("companyId"), q.Get("portalLinkId"), q.Get("portalId"), jobs.ClampLimit(limit), offset)
		if err != nil {
			http.Error(w, "failed to list jobs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, result)

	case http.MethodPut:
		var body linkJobToCompanyRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := jobs.LinkJobToCompany(body.ID, body.CompanyID); err != nil {
			companies.WriteCompanyAwareError(w, "link job to company", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := jobs.DeleteJob(id); err != nil {
			http.Error(w, "failed to delete job: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// jobMatchUpdateRequest is the human/frontend-facing shape PUT
// /jobs/match accepts — deliberately has NO score or personaId field
// at all: those are written exclusively by the AI's own save_job_match
// MCP tool (job_match.go), never by this endpoint, so a compromised or
// buggy frontend call can never fabricate a match result. Status is
// always required (the only two real calls are "start tracking a
// fresh attempt" [status: matching, profileId + conversationId set]
// and "record a client-observed failure" [status: failed, error set])
// — ErrorText a plain *string (not the elsewhere-established
// presence-gated-clear pattern) since this endpoint never needs to
// CLEAR a previously recorded error independently of also changing
// status; every call sets both together.
type jobMatchUpdateRequest struct {
	JobID          string  `json:"jobId"`
	ProfileID      string  `json:"profileId,omitempty"`
	Status         string  `json:"status"`
	ConversationID string  `json:"conversationId,omitempty"`
	ErrorText      *string `json:"error,omitempty"`
}

// jobMatchHandler handles PUT /jobs/match — see jobMatchUpdateRequest's
// own doc comment for the security reasoning behind its narrow shape.
func jobMatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body jobMatchUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.JobID == "" {
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}
	switch body.Status {
	case "matching":
		if body.ProfileID == "" {
			http.Error(w, "profileId is required when starting a match", http.StatusBadRequest)
			return
		}
		if err := jobs.StartJobMatch(body.JobID, body.ProfileID, body.ConversationID); err != nil {
			if errors.Is(err, jobs.ErrUnknownJob) {
				http.Error(w, "unknown job id", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to start job match: "+err.Error(), http.StatusInternalServerError)
			return
		}
	case "failed":
		if err := jobs.UpdateJobMatchStatus(body.JobID, body.Status, body.ErrorText); err != nil {
			if errors.Is(err, jobs.ErrUnknownJob) {
				http.Error(w, "unknown job id", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to update job match: "+err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, `status must be "matching" or "failed"`, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- /jobs/cv — "Generate CV PDF" ---

type cvPdfUpdateRequest struct {
	JobID          string  `json:"jobId"`
	ProfileID      string  `json:"profileId,omitempty"`
	Status         string  `json:"status"`
	ConversationID string  `json:"conversationId,omitempty"`
	ErrorText      *string `json:"error,omitempty"`
}

// cvPdfHandler handles PUT /jobs/cv — starts tracking a new CV
// generation attempt (status: "generating", profileId + conversationId
// set) or records a client-observed failure (status: "failed", error
// set). Mirrors jobMatchHandler exactly. A successful render is never
// recorded here — that's the AI-facing save_cv_document MCP tool's own job
// (cv_pdf.go's own top doc comment explains how it ends up with an
// already-permanent Media file id by the time it runs).
func cvPdfHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body cvPdfUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.JobID == "" {
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}
	switch body.Status {
	case "generating":
		if body.ProfileID == "" {
			http.Error(w, "profileId is required when starting a CV generation", http.StatusBadRequest)
			return
		}
		if err := jobs.StartCvGeneration(body.JobID, body.ProfileID, body.ConversationID); err != nil {
			if errors.Is(err, jobs.ErrUnknownJob) {
				http.Error(w, "unknown job id", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to start CV generation: "+err.Error(), http.StatusInternalServerError)
			return
		}
	case "failed":
		if err := jobs.UpdateCvGenerationStatus(body.JobID, body.Status, body.ErrorText); err != nil {
			if errors.Is(err, jobs.ErrUnknownJob) {
				http.Error(w, "unknown job id", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to update CV generation: "+err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, `status must be "generating" or "failed"`, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// cvGenerationsHandler handles GET /jobs/cv-generations?jobId= — the
// full audit trail of every CV generation attempt ever made for a job
// (job_cv_generations, an append-only table distinct from job_cv_pdfs'
// own single "current state" row — see db.go's own doc comment). See
// plan/ai/career/cv-generation-audit/step-03-backend-wiring.md.
func cvGenerationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId query parameter is required", http.StatusBadRequest)
		return
	}
	generations, err := jobs.ListCvGenerationsForJob(jobID)
	if err != nil {
		if errors.Is(err, jobs.ErrUnknownJob) {
			http.Error(w, "unknown job id", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to list cv generations: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]any{"generations": generations})
}

// jobLocationsHandler handles GET /job-locations — the Jobs page's own
// Location filter dropdown's data source (step — see
// jobs.ListDistinctJobLocations's own doc comment). Read-only, no
// query params: every distinct location is always returned, the same
// "fetch everything, no pagination" shape fetchCompanies/fetchPortals
// already use for their own filter dropdowns.
func jobLocationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	locations, err := jobs.ListDistinctJobLocations()
	if err != nil {
		http.Error(w, "failed to list job locations: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]any{"locations": locations})
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
		list, err := companies.ListCompanies()
		if err != nil {
			http.Error(w, "failed to list companies: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"companies": list})

	case http.MethodPost:
		var body companyRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		id, err := companies.CreateCompany(body.Name, body.Description)
		if err != nil {
			http.Error(w, "failed to create company: "+err.Error(), http.StatusInternalServerError)
			return
		}
		list, err := companies.ListCompanies()
		if err != nil {
			http.Error(w, "company created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"id": id, "companies": list})

	case http.MethodPut:
		var body companyUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := companies.UpdateCompany(body.ID, body.Name, body.Description); err != nil {
			companies.WriteCompanyAwareError(w, "update company", err)
			return
		}
		list, err := companies.ListCompanies()
		if err != nil {
			http.Error(w, "company updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"companies": list})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := companies.DeleteCompany(id); err != nil {
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
		recruiterList, err := recruiters.ListRecruiters(r.URL.Query().Get("companyId"))
		if err != nil {
			http.Error(w, "failed to list recruiters: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"recruiters": recruiterList})

	case http.MethodPost:
		var body recruiterRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CompanyID == "" {
			http.Error(w, "companyId is required", http.StatusBadRequest)
			return
		}
		id, err := recruiters.CreateRecruiter(body.CompanyID, body.FirstName, body.LastName, body.Email)
		if err != nil {
			companies.WriteCompanyAwareError(w, "create recruiter", err)
			return
		}
		recruiterList, err := recruiters.ListRecruiters("")
		if err != nil {
			http.Error(w, "recruiter created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"id": id, "recruiters": recruiterList})

	case http.MethodPut:
		var body recruiterUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := recruiters.UpdateRecruiter(body.ID, body.FirstName, body.LastName, body.Email); err != nil {
			writeRecruiterAwareError(w, "update recruiter", err)
			return
		}
		recruiterList, err := recruiters.ListRecruiters("")
		if err != nil {
			http.Error(w, "recruiter updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"recruiters": recruiterList})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := recruiters.DeleteRecruiter(id); err != nil {
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
		portals, err := portal.ListPortals()
		if err != nil {
			http.Error(w, "failed to list portals: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"portals": portals})

	case http.MethodPost:
		var body portalRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		id, err := portal.CreatePortal(body.Name)
		if err != nil {
			http.Error(w, "failed to create portal: "+err.Error(), http.StatusInternalServerError)
			return
		}
		portals, err := portal.ListPortals()
		if err != nil {
			http.Error(w, "portal created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"id": id, "portals": portals})

	case http.MethodPut:
		var body portalUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" || body.Name == "" {
			http.Error(w, "id and name are both required", http.StatusBadRequest)
			return
		}
		if err := portal.UpdatePortal(body.ID, body.Name); err != nil {
			portal.WritePortalAwareError(w, "update portal", err)
			return
		}
		portals, err := portal.ListPortals()
		if err != nil {
			http.Error(w, "portal updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"portals": portals})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := portal.DeletePortal(id); err != nil {
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
	// JobDetailCrawlInstructions (step XX) — same presence-gated shape
	// as CrawlInstructions above, for the separate job-detail-page
	// instruction document. See
	// plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
	JobDetailCrawlInstructions *string `json:"jobDetailCrawlInstructions,omitempty"`
	// InstructionsAIError (step 33) — nil: don't touch. Present with
	// "": clear any previously recorded failure (a fresh attempt
	// succeeded). Present non-empty: record this as the most recent
	// failure. Same presence-gated shape CrawlInstructions above uses.
	InstructionsAIError *string `json:"instructionsAiError,omitempty"`
	// InstructionsAIConversationID (step 60) — nil: don't touch.
	// Present with "": clear (the run just finished, one way or
	// another). Present non-empty: record the hidden conversation that
	// just started generating/updating this link's own crawl
	// instructions. Same presence-gated shape InstructionsAIError above
	// uses.
	InstructionsAIConversationID *string `json:"instructionsAiConversationId,omitempty"`
	// JobDetailInstructionsAIError/JobDetailInstructionsAIConversationID
	// (step XX) — same presence-gated shape as InstructionsAIError/
	// InstructionsAIConversationID above, for the SEPARATE job-detail
	// instructions document's own AI generation tracking instead.
	JobDetailInstructionsAIError          *string `json:"jobDetailInstructionsAiError,omitempty"`
	JobDetailInstructionsAIConversationID *string `json:"jobDetailInstructionsAiConversationId,omitempty"`
	// ListingCrawlRequested/JobDetailInstructionsAIRequested (step 67)
	// — nil: don't touch. Present: set to that exact value. The
	// frontend only ever sends these as false, to clear a flag a
	// Domain Events listener (events.go) set to true — see
	// plan/ai/tools/career/step-67-frontend-flag-consumption.md.
	ListingCrawlRequested            *bool `json:"listingCrawlRequested,omitempty"`
	JobDetailInstructionsAIRequested *bool `json:"jobDetailInstructionsAiRequested,omitempty"`
	// InstructionsAIErrorDismissedAt/JobDetailInstructionsAIErrorDismissedAt
	// (step 71) — nil: don't touch. Present with "": clear. Present
	// non-empty: record when that document's own error banner was
	// dismissed — the frontend always sends the dismissed error's own
	// errorAt value here, never "now". See
	// plan/ai/tools/career/step-71-dismiss-tracking-data-model.md.
	InstructionsAIErrorDismissedAt          *string `json:"instructionsAiErrorDismissedAt,omitempty"`
	JobDetailInstructionsAIErrorDismissedAt *string `json:"jobDetailInstructionsAiErrorDismissedAt,omitempty"`
	// JobDetailCrawlRequested (step 76) — nil: don't touch. Present: set
	// to that exact value. Same shape as ListingCrawlRequested/
	// JobDetailInstructionsAIRequested above — the frontend only ever
	// sends this as false, to clear a flag events.go's own
	// JobsCrawledEventHandler set to true. See
	// plan/ai/tools/career/step-76-job-detail-crawl-requested-backend.md.
	JobDetailCrawlRequested *bool `json:"jobDetailCrawlRequested,omitempty"`
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
		id, err := portal.AddPortalLink(body.PortalID, body.URL, body.Title)
		if err != nil {
			portal.WritePortalLinkAwareError(w, "add portal link", err)
			return
		}
		portals, err := portal.ListPortals()
		if err != nil {
			http.Error(w, "portal link added but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"id": id, "portals": portals})

	case http.MethodPut:
		var body portalLinkUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if body.URL == nil && body.Title == nil && body.CrawlInstructions == nil && body.JobDetailCrawlInstructions == nil &&
			body.InstructionsAIError == nil && body.InstructionsAIConversationID == nil &&
			body.JobDetailInstructionsAIError == nil && body.JobDetailInstructionsAIConversationID == nil &&
			body.ListingCrawlRequested == nil && body.JobDetailInstructionsAIRequested == nil &&
			body.InstructionsAIErrorDismissedAt == nil && body.JobDetailInstructionsAIErrorDismissedAt == nil &&
			body.JobDetailCrawlRequested == nil {
			http.Error(w, "url, title, crawlInstructions, jobDetailCrawlInstructions, instructionsAiError, instructionsAiConversationId, jobDetailInstructionsAiError, jobDetailInstructionsAiConversationId, listingCrawlRequested, jobDetailInstructionsAiRequested, instructionsAiErrorDismissedAt, jobDetailInstructionsAiErrorDismissedAt, or jobDetailCrawlRequested is required", http.StatusBadRequest)
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
			if err := portal.UpdatePortalLink(body.ID, body.URL, body.Title); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link", err)
				return
			}
		}
		if body.CrawlInstructions != nil {
			if err := portal.UpdatePortalLinkCrawlInstructions(body.ID, *body.CrawlInstructions); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link crawl instructions", err)
				return
			}
		}
		if body.JobDetailCrawlInstructions != nil {
			if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(body.ID, *body.JobDetailCrawlInstructions); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail crawl instructions", err)
				return
			}
		}
		if body.InstructionsAIError != nil {
			if err := portal.UpdatePortalLinkInstructionsAIStatus(body.ID, body.InstructionsAIError); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link instructions AI status", err)
				return
			}
		}
		if body.InstructionsAIConversationID != nil {
			if err := portal.UpdatePortalLinkInstructionsAIConversationID(body.ID, body.InstructionsAIConversationID); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link instructions AI conversation id", err)
				return
			}
		}
		if body.JobDetailInstructionsAIError != nil {
			if err := portal.UpdatePortalLinkJobDetailInstructionsAIStatus(body.ID, body.JobDetailInstructionsAIError); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail instructions AI status", err)
				return
			}
		}
		if body.JobDetailInstructionsAIConversationID != nil {
			if err := portal.UpdatePortalLinkJobDetailInstructionsAIConversationID(body.ID, body.JobDetailInstructionsAIConversationID); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail instructions AI conversation id", err)
				return
			}
		}
		if body.ListingCrawlRequested != nil {
			if err := portal.UpdatePortalLinkListingCrawlRequested(body.ID, *body.ListingCrawlRequested); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link listing crawl requested", err)
				return
			}
		}
		if body.JobDetailInstructionsAIRequested != nil {
			if err := portal.UpdatePortalLinkJobDetailInstructionsAIRequested(body.ID, *body.JobDetailInstructionsAIRequested); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail instructions AI requested", err)
				return
			}
		}
		if body.InstructionsAIErrorDismissedAt != nil {
			if err := portal.UpdatePortalLinkInstructionsAIErrorDismissedAt(body.ID, body.InstructionsAIErrorDismissedAt); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link instructions AI error dismissed at", err)
				return
			}
		}
		if body.JobDetailInstructionsAIErrorDismissedAt != nil {
			if err := portal.UpdatePortalLinkJobDetailInstructionsAIErrorDismissedAt(body.ID, body.JobDetailInstructionsAIErrorDismissedAt); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail instructions AI error dismissed at", err)
				return
			}
		}
		if body.JobDetailCrawlRequested != nil {
			if err := portal.UpdatePortalLinkJobDetailCrawlRequested(body.ID, *body.JobDetailCrawlRequested); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail crawl requested", err)
				return
			}
		}
		portals, err := portal.ListPortals()
		if err != nil {
			http.Error(w, "portal link updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"portals": portals})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := portal.RemovePortalLink(id); err != nil {
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
	req, err := portal.BuildCrawlRequest(id)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "build crawl request", err)
		return
	}
	db.WriteJSON(w, req)
}

type ingestCrawlResultsRequest struct {
	PortalLinkID string                   `json:"portalLinkId"`
	Pages        []portal.CrawlResultPage `json:"pages"`
}

// ingestCrawlResultsHandler handles POST /portal-links/ingest-crawl-
// results — the deterministic flow's last step: the frontend forwards
// browser's own /crawl-paginated response body (plus which portal
// link it was crawling) here unmodified, and this tool maps it onto
// job rows via portal.IngestCrawlResults' own fixed label vocabulary.
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
	result, err := portal.IngestCrawlResults(body.PortalLinkID, body.Pages)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "ingest crawl results", err)
		return
	}
	db.WriteJSON(w, result)
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
