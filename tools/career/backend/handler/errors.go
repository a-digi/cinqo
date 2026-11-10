package handler

import (
	"errors"
	"net/http"

	"career-tool-backend/persona"
)

// WritePersonaAwareError maps persona.ErrUnknownPersona to a 400 (a
// caller mistake) and everything else to a 500, matching the persona
// package's own personaAwareErrResult reasoning on the MCP side.
func WritePersonaAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, persona.ErrUnknownPersona) {
		http.Error(w, "unknown target role id", http.StatusBadRequest)
		return
	}
	if errors.Is(err, persona.ErrInvalidRemotePreference) || errors.Is(err, persona.ErrProfileItem) ||
		errors.Is(err, persona.ErrUnknownFollowField) || errors.Is(err, persona.ErrSetAndFollow) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}
