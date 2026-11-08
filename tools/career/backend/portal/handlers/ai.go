package portalhandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

// aiOverrideRequest is the body of PUT /portals/ai and PUT /portal-links/ai (step
// 102): the id of the portal or link, and its own AI platform and model. Both empty
// removes the override, so the tool-wide choice (or the portal's) applies again.
type aiOverrideRequest struct {
	ID         string `json:"id"`
	PlatformID string `json:"platformId"`
	Model      string `json:"model"`
}

// writeAIOverrideError maps the setters' errors: a bad body is the caller's (400, with
// the reason), an unknown id the same 400 the other portal routes give, anything else 500.
func writeAIOverrideError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, portal.ErrInvalidAIOverride) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	portal.WritePortalLinkAwareError(w, action, err)
}

// PortalAIHandler handles PUT /portals/ai: set or remove a portal's own AI platform and model.
func PortalAIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var body aiOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, `body must be {"id": "…", "platformId": "…", "model": "…"}`, http.StatusBadRequest)
		return
	}
	if err := portal.SetPortalAI(body.ID, portal.AIOverride{PlatformID: body.PlatformID, Model: body.Model}); err != nil {
		writeAIOverrideError(w, "set the portal's AI choice", err)
		return
	}
	portals, err := portal.ListPortals()
	if err != nil {
		http.Error(w, "AI choice saved but failed to reload: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]any{"portals": portals})
}

// PortalLinkAIHandler handles PUT /portal-links/ai: set or remove a link's own AI platform and model.
func PortalLinkAIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var body aiOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, `body must be {"id": "…", "platformId": "…", "model": "…"}`, http.StatusBadRequest)
		return
	}
	if err := portal.SetLinkAI(body.ID, portal.AIOverride{PlatformID: body.PlatformID, Model: body.Model}); err != nil {
		writeAIOverrideError(w, "set the link's AI choice", err)
		return
	}
	portals, err := portal.ListPortals()
	if err != nil {
		http.Error(w, "AI choice saved but failed to reload: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]any{"portals": portals})
}

// --- /portal-links ---
