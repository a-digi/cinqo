package courseshandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/courses"
	"career-tool-backend/db"
)

// The profile's courses (plan 231). Route courses, scope tool:career:profile:
//
//	GET    ?profileId=                                                        { courses: [...] }
//	POST   {profileId, title, provider?, completedDate?, credentialUrl?, description?}   the profile's list
//	PUT    {id, title?, provider?, completedDate?, credentialUrl?, description?}         the profile's list
//	DELETE ?id=                                                               204
//
// Every unknown id and every refusal of a caller (no title, a date that is not YYYY, YYYY-MM or YYYY-MM-DD, a link that is not http or https,
// a text too long) is a 400 with the reason; anything else a 500.
func writeError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, courses.ErrUnknownProfile):
		http.Error(w, "unknown profile id", http.StatusBadRequest)
	case errors.Is(err, courses.ErrUnknownCourse):
		http.Error(w, "unknown course id", http.StatusBadRequest)
	case errors.Is(err, courses.ErrTitleRequired), errors.Is(err, courses.ErrInvalidDate), errors.Is(err, courses.ErrInvalidURL), errors.Is(err, courses.ErrTooLong):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

func CoursesHandler(w http.ResponseWriter, r *http.Request) {
	reload := func(profileID string) {
		list, err := courses.List(profileID)
		if err != nil {
			http.Error(w, "saved but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"courses": list})
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
			ProfileID     string `json:"profileId"`
			Title         string `json:"title"`
			Provider      string `json:"provider"`
			CompletedDate string `json:"completedDate"`
			CredentialURL string `json:"credentialUrl"`
			Description   string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProfileID == "" {
			http.Error(w, "profileId and title are required", http.StatusBadRequest)
			return
		}
		if _, err := courses.Add(body.ProfileID, courses.Input{
			Title: body.Title, Provider: body.Provider, CompletedDate: body.CompletedDate, CredentialURL: body.CredentialURL, Description: body.Description,
		}); err != nil {
			writeError(w, "add course", err)
			return
		}
		reload(body.ProfileID)

	case http.MethodPut:
		var body struct {
			ID            string  `json:"id"`
			Title         *string `json:"title,omitempty"`
			Provider      *string `json:"provider,omitempty"`
			CompletedDate *string `json:"completedDate,omitempty"`
			CredentialURL *string `json:"credentialUrl,omitempty"`
			Description   *string `json:"description,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		updated, err := courses.Update(body.ID, courses.Patch{
			Title: body.Title, Provider: body.Provider, CompletedDate: body.CompletedDate, CredentialURL: body.CredentialURL, Description: body.Description,
		})
		if err != nil {
			writeError(w, "update course", err)
			return
		}
		reload(updated.ProfileID)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := courses.Delete(id); err != nil {
			writeError(w, "delete course", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
