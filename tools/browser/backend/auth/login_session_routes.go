package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"browser-tool-backend/shared"
)

// login_session_routes.go (plan 195, step 1): the two routes of a saved login session. Both answer only what shared/loginsession.go
// lets out (id, host, state, times), never a cookie. Both need the scope tool:browser:login, and neither is an MCP tool: the AI cannot ask
// about, create or forget a session. Creating one is the person's opt-in at the login confirmation (step 2).

var loginSessionIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// LoginSessionStateHandler handles GET /login-session/state?id=: the session's info; 404 when it does not exist (forgotten or deleted).
func LoginSessionStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if !loginSessionIDPattern.MatchString(id) {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	info, err := shared.GetLoginSession(id)
	if errors.Is(err, shared.ErrLoginSessionNotFound) {
		http.Error(w, "no such login session", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not read the login session", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

// LoginSessionForgetHandler handles DELETE /login-session?id=: forgets the session. Idempotent: 204 whether it existed or not.
func LoginSessionForgetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if !loginSessionIDPattern.MatchString(id) {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := shared.DeleteLoginSession(id); err != nil {
		http.Error(w, "could not forget the login session", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
