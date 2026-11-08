package companieshandlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"career-tool-backend/companies"
	"career-tool-backend/db"
)

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
		// No parameters: every company, as always. query, limit and offset (step 137) give the Companies page one searched
		// page of them; total and summary are in every answer.
		q := r.URL.Query()
		limit, offset := 0, 0
		if raw := q.Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				http.Error(w, "limit must be a whole number", http.StatusBadRequest)
				return
			}
			limit = companies.ClampCompaniesLimit(n)
		}
		if raw := q.Get("offset"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				http.Error(w, "offset must be a whole number", http.StatusBadRequest)
				return
			}
			offset = n
		}
		page, err := companies.ListCompaniesPage(q.Get("query"), limit, offset)
		if err != nil {
			http.Error(w, "failed to list companies: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, page)

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
