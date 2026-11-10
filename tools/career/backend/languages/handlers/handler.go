package languageshandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/languages"
)

// The profile's languages (plan 231). Route languages, scope tool:career:profile:
//
//	GET    ?profileId=                                  { languages: [...] }
//	POST   {profileId, language, level?, canSpeak, canWrite}   the profile's list
//	PUT    {id, language?, level?, canSpeak?, canWrite?}       the profile's list
//	DELETE ?id=                                         204
//
// Every unknown id and every refusal of a caller (no name, a level outside the list, neither spoken nor written, a language already listed)
// is a 400 with the reason; anything else a 500.
func writeError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, languages.ErrUnknownProfile):
		http.Error(w, "unknown profile id", http.StatusBadRequest)
	case errors.Is(err, languages.ErrUnknownLanguage):
		http.Error(w, "unknown language id", http.StatusBadRequest)
	case errors.Is(err, languages.ErrNameRequired), errors.Is(err, languages.ErrTooLong), errors.Is(err, languages.ErrInvalidLevel),
		errors.Is(err, languages.ErrSpokenOrWritten), errors.Is(err, languages.ErrDuplicate):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

func LanguagesHandler(w http.ResponseWriter, r *http.Request) {
	reload := func(profileID string) {
		list, err := languages.List(profileID)
		if err != nil {
			http.Error(w, "saved but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"languages": list})
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
			Language  string `json:"language"`
			Level     string `json:"level"`
			CanSpeak  bool   `json:"canSpeak"`
			CanWrite  bool   `json:"canWrite"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProfileID == "" {
			http.Error(w, "profileId and language are required", http.StatusBadRequest)
			return
		}
		if _, err := languages.Add(body.ProfileID, languages.Input{Language: body.Language, Level: body.Level, CanSpeak: body.CanSpeak, CanWrite: body.CanWrite}); err != nil {
			writeError(w, "add language", err)
			return
		}
		reload(body.ProfileID)

	case http.MethodPut:
		var body struct {
			ID       string  `json:"id"`
			Language *string `json:"language,omitempty"`
			Level    *string `json:"level,omitempty"`
			CanSpeak *bool   `json:"canSpeak,omitempty"`
			CanWrite *bool   `json:"canWrite,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		updated, err := languages.Update(body.ID, languages.Patch{Language: body.Language, Level: body.Level, CanSpeak: body.CanSpeak, CanWrite: body.CanWrite})
		if err != nil {
			writeError(w, "update language", err)
			return
		}
		reload(updated.ProfileID)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := languages.Delete(id); err != nil {
			writeError(w, "delete language", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
