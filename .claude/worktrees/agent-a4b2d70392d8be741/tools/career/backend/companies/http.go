// http.go is the plain HTTP mirror of this package's own MCP tools
// (companies.go/recruiters.go) — a human's own browser session and the
// AI's own MCP tool-calling loop are two different callers needing two
// different transports to reach the same underlying data. Every
// handler here calls the exact same functions those files already
// built — never independently reimplemented. Deliberately not
// MCP-exposed themselves; scope enforcement happens at the host's own
// reverse proxy via manifest.json's routes[], same as every other
// tool's own HTTP routes.
//
// Originally these handlers shared one flat http.go with every other
// domain's own; each now lives beside the business logic it calls, in
// that domain's own package.
package companies

import (
	"encoding/json"
	"net/http"

	"career-tool-backend/shared"
)

// WriteCompanyAwareError maps ErrUnknownCompany to a 400 (a caller
// mistake) and everything else to a 500. Exported because the jobs
// package's own PUT /jobs (link_job_to_company) reports the exact same
// failure — linkJobToCompany calls RequireCompanyExists, so it can
// return this package's own sentinel.
func WriteCompanyAwareError(w http.ResponseWriter, action string, err error) {
	shared.WriteKnownError(w, err, ErrUnknownCompany, "unknown company id", action)
}

// writeRecruiterAwareError is the same idea for errUnknownRecruiter,
// which never leaves this package.
func writeRecruiterAwareError(w http.ResponseWriter, action string, err error) {
	shared.WriteKnownError(w, err, errUnknownRecruiter, "unknown recruiter id", action)
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

func CompaniesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		companies, err := listCompanies()
		if err != nil {
			http.Error(w, "failed to list companies: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"companies": companies})

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
		shared.WriteJSON(w, map[string]any{"id": id, "companies": companies})

	case http.MethodPut:
		var body companyUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if err := updateCompany(body.ID, body.Name, body.Description); err != nil {
			WriteCompanyAwareError(w, "update company", err)
			return
		}
		companies, err := listCompanies()
		if err != nil {
			http.Error(w, "company updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"companies": companies})

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

func RecruitersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		recruiters, err := listRecruiters(r.URL.Query().Get("companyId"))
		if err != nil {
			http.Error(w, "failed to list recruiters: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"recruiters": recruiters})

	case http.MethodPost:
		var body recruiterRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CompanyID == "" {
			http.Error(w, "companyId is required", http.StatusBadRequest)
			return
		}
		id, err := createRecruiter(body.CompanyID, body.FirstName, body.LastName, body.Email)
		if err != nil {
			WriteCompanyAwareError(w, "create recruiter", err)
			return
		}
		recruiters, err := listRecruiters("")
		if err != nil {
			http.Error(w, "recruiter created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"id": id, "recruiters": recruiters})

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
		shared.WriteJSON(w, map[string]any{"recruiters": recruiters})

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
