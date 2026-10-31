package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func personaForHandler(t *testing.T) string {
	t.Helper()
	setupHandlerDB(t)
	profileID, err := profile.CreateProfile("Ada", "Test")
	if err != nil {
		t.Fatal(err)
	}
	id, err := persona.CreatePersona(profileID, "Backend", "")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func detailsCall(t *testing.T, method, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	personaDetailsHandler(rec, httptest.NewRequest(method, url, strings.NewReader(body)))
	return rec
}

func remoteFromBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var res struct {
		PersonaDetails *struct {
			RemotePreference string `json:"remotePreference"`
		} `json:"personaDetails"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.PersonaDetails == nil {
		t.Fatalf("body = %s (%v)", rec.Body.String(), err)
	}
	return res.PersonaDetails.RemotePreference
}

func TestPersonaDetailsRouteSavesAndReturnsTheRemotePreference(t *testing.T) {
	id := personaForHandler(t)
	rec := detailsCall(t, "POST", "/persona-details", `{"personaId":"`+id+`","headline":"Engineer"}`)
	wantStatus(t, rec, 200)
	if got := remoteFromBody(t, rec); got != "" {
		t.Errorf("not set yet, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), `"remotePreference":""`) {
		t.Errorf("the field is always present: %s", rec.Body.String())
	}
	for _, v := range []string{"remote_only", "open", "onsite", ""} {
		rec = detailsCall(t, "POST", "/persona-details", `{"personaId":"`+id+`","remotePreference":"`+v+`"}`)
		wantStatus(t, rec, 200)
		if got := remoteFromBody(t, rec); got != v {
			t.Errorf("saved %q, answered %q", v, got)
		}
		read := detailsCall(t, "GET", "/persona-details?personaId="+id, "")
		wantStatus(t, read, 200)
		if got := remoteFromBody(t, read); got != v {
			t.Errorf("saved %q, read %q", v, got)
		}
	}
}

func TestPersonaDetailsRouteRefusesAnInvalidRemotePreference(t *testing.T) {
	id := personaForHandler(t)
	wantStatus(t, detailsCall(t, "POST", "/persona-details", `{"personaId":"`+id+`","remotePreference":"open","headline":"Keep"}`), 200)
	for _, bad := range []string{"remote", "Remote", "hybrid", "yes", "ONSITE"} {
		rec := detailsCall(t, "POST", "/persona-details", `{"personaId":"`+id+`","remotePreference":"`+bad+`","headline":"Lost"}`)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), "remotePreference") {
			t.Errorf("%q: status %d, body %q", bad, rec.Code, rec.Body.String())
		}
	}
	read := detailsCall(t, "GET", "/persona-details?personaId="+id, "")
	if got := remoteFromBody(t, read); got != "open" || strings.Contains(read.Body.String(), "Lost") {
		t.Errorf("a refused update must change nothing: %s", read.Body.String())
	}
	if rec := detailsCall(t, "POST", "/persona-details", `{"personaId":"nope","remotePreference":"open"}`); rec.Code != 400 {
		t.Errorf("unknown persona: %d", rec.Code)
	}
}
