package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/persona"
)

// What a person reads of a target role (a "persona" in the code) says "target role" (plan/ai/tools/career/step-234-rename-personas-to-target-roles.md).
func TestAnUnknownTargetRoleIsA400WithTheOnScreenWord(t *testing.T) {
	rec := httptest.NewRecorder()
	WritePersonaAwareError(rec, "read target role details", persona.ErrUnknownPersona)
	if rec.Code != http.StatusBadRequest || strings.TrimSpace(rec.Body.String()) != "unknown target role id" {
		t.Errorf("%d %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(persona.ErrUnknownPersona.Error(), "target role") || strings.Contains(strings.ToLower(persona.ErrUnknownPersona.Error()), "persona") {
		t.Errorf("the error itself: %q", persona.ErrUnknownPersona)
	}
}

func TestAnyOtherFailureNamesTheActionAndIsA500(t *testing.T) {
	rec := httptest.NewRecorder()
	WritePersonaAwareError(rec, "update target role", errors.New("boom"))
	if rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != "failed to update target role: boom" {
		t.Errorf("%d %q", rec.Code, rec.Body.String())
	}
}
