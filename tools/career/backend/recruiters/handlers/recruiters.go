package recruitershandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/companies"
	"career-tool-backend/db"
	"career-tool-backend/recruiters"
)

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

// A recruiter works for zero or more companies (plan step-169): POST takes the old single companyId, a list companyIds, both
// (merged) or neither; PUT with companyIds replaces the set (an empty list clears it, an absent or null field leaves it).
type recruiterRequest struct {
	CompanyID  string   `json:"companyId"`
	CompanyIDs []string `json:"companyIds"`
	Freelancer bool     `json:"freelancer"`
	FirstName  string   `json:"firstName"`
	LastName   string   `json:"lastName"`
	Email      string   `json:"email"`
}

type recruiterUpdateRequest struct {
	ID         string    `json:"id"`
	FirstName  *string   `json:"firstName,omitempty"`
	LastName   *string   `json:"lastName,omitempty"`
	Email      *string   `json:"email,omitempty"`
	CompanyIDs *[]string `json:"companyIds,omitempty"`
	Freelancer *bool     `json:"freelancer,omitempty"`
}

// writeRecruiterWriteError: an unknown recruiter or company is a caller mistake (400), anything else a 500.
func writeRecruiterWriteError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, recruiters.ErrFreelancerWithCompanies) {
		http.Error(w, "a freelancer has no companies", http.StatusBadRequest)
		return
	}
	if errors.Is(err, companies.ErrUnknownCompany) {
		http.Error(w, "unknown company id", http.StatusBadRequest)
		return
	}
	writeRecruiterAwareError(w, action, err)
}

func RecruitersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		recruiterList, err := recruiters.ListRecruitersOfKind(r.URL.Query().Get("companyId"), r.URL.Query().Get("freelancer") == "true")
		if err != nil {
			http.Error(w, "failed to list recruiters: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"recruiters": recruiterList})

	case http.MethodPost:
		var body recruiterRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		id, err := recruiters.CreateRecruiterOfKind(append([]string{body.CompanyID}, body.CompanyIDs...), body.Freelancer, body.FirstName, body.LastName, body.Email)
		if err != nil {
			if errors.Is(err, recruiters.ErrFreelancerWithCompanies) {
				writeRecruiterWriteError(w, "create recruiter", err)
				return
			}
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
		// The kind and the companies first: they check everything before writing, so a bad id (or a freelancer with companies) changes
		// nothing, not even the name.
		if body.CompanyIDs != nil || body.Freelancer != nil {
			if err := recruiters.ApplyKind(body.ID, body.Freelancer, body.CompanyIDs); err != nil {
				writeRecruiterWriteError(w, "update recruiter", err)
				return
			}
		}
		if err := recruiters.UpdateRecruiter(body.ID, body.FirstName, body.LastName, body.Email); err != nil {
			writeRecruiterWriteError(w, "update recruiter", err)
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
