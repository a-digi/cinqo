package portalhandlers

import (
	"encoding/json"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

type portalRequest struct {
	Name string `json:"name"`
}

type portalUpdateRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func PortalsHandler(w http.ResponseWriter, r *http.Request) {
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
		kept := portal.LoginSessionIDsOfPortal(id) // plan 195: the logins kept for its links go with it
		if err := portal.DeletePortal(id); err != nil {
			http.Error(w, "failed to delete portal: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for _, sessionID := range kept {
			forgetBrowserSession(r, sessionID)
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
