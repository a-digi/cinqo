package educationhandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/education"
)

// The profile's education (plan 173, step 6). Route education, scope tool:career:profile:
//
//	GET    ?profileId=                              { education: [...] }
//	POST   {profileId, institution, degree?, fieldOfStudy?, startDate?, endDate?, description?}   the profile's list
//	PUT    {id, institution?, degree?, fieldOfStudy?, startDate?, endDate?, description?}         the profile's list
//	DELETE ?id=                                     204
//
// Every unknown id and every refusal of a caller (no institution, a date that is not YYYY, YYYY-MM or YYYY-MM-DD, an end before the
// start) is a 400 with the reason; anything else a 500.
func writeEducationError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, education.ErrUnknownProfile):
		http.Error(w, "unknown profile id", http.StatusBadRequest)
	case errors.Is(err, education.ErrUnknownEducation):
		http.Error(w, "unknown education id", http.StatusBadRequest)
	case errors.Is(err, education.ErrInstitutionRequired), errors.Is(err, education.ErrInvalidDate), errors.Is(err, education.ErrEndBeforeStart):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

func EducationHandler(w http.ResponseWriter, r *http.Request) {
	reload := func(profileID string) {
		list, err := education.List(profileID)
		if err != nil {
			http.Error(w, "saved but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"education": list})
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
			ProfileID    string `json:"profileId"`
			Institution  string `json:"institution"`
			Degree       string `json:"degree"`
			FieldOfStudy string `json:"fieldOfStudy"`
			StartDate    string `json:"startDate"`
			EndDate      string `json:"endDate"`
			Description  string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProfileID == "" {
			http.Error(w, "profileId and institution are required", http.StatusBadRequest)
			return
		}
		if _, err := education.Add(body.ProfileID, education.Input{
			Institution: body.Institution, Degree: body.Degree, FieldOfStudy: body.FieldOfStudy,
			StartDate: body.StartDate, EndDate: body.EndDate, Description: body.Description,
		}); err != nil {
			writeEducationError(w, "add education", err)
			return
		}
		reload(body.ProfileID)

	case http.MethodPut:
		var body struct {
			ID           string  `json:"id"`
			Institution  *string `json:"institution,omitempty"`
			Degree       *string `json:"degree,omitempty"`
			FieldOfStudy *string `json:"fieldOfStudy,omitempty"`
			StartDate    *string `json:"startDate,omitempty"`
			EndDate      *string `json:"endDate,omitempty"`
			Description  *string `json:"description,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		updated, err := education.Update(body.ID, education.Patch{
			Institution: body.Institution, Degree: body.Degree, FieldOfStudy: body.FieldOfStudy,
			StartDate: body.StartDate, EndDate: body.EndDate, Description: body.Description,
		})
		if err != nil {
			writeEducationError(w, "update education", err)
			return
		}
		reload(updated.ProfileID)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := education.Delete(id); err != nil {
			writeEducationError(w, "delete education", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
