package profilehandlers

import (
	"encoding/json"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/profile"
)

type profileLinkRequest struct {
	ProfileID string `json:"profileId"`
	Platform  string `json:"platform"`
	URL       string `json:"url"`
}

func ProfileLinksHandler(w http.ResponseWriter, r *http.Request) {
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
