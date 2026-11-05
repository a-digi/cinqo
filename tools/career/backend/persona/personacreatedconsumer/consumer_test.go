package personacreatedconsumer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/rematchworker"
)

func post(h http.HandlerFunc, method, body string) int {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, "/events/x", strings.NewReader(body)))
	return rec.Code
}

// A persona event notes the persona's profile (not the persona) into the shared worker.
func TestAPersonaEventNotesItsProfile(t *testing.T) {
	before := rematchworker.Default.Waiting()
	body := `{"topic":"career.persona.created","payload":{"persona_id":"per-created-test-1","profile_id":"prof-for-persona-test-1"}}`
	if code := post(EventHandler, http.MethodPost, body); code != http.StatusAccepted {
		t.Fatalf("status %d", code)
	}
	if rematchworker.Default.Waiting() != before+1 {
		t.Fatal("the profile was not noted")
	}
	// a second persona of the same profile is the same entry: one re-match for the profile
	post(EventHandler, http.MethodPost, `{"payload":{"persona_id":"per-created-test-2","profile_id":"prof-for-persona-test-1"}}`)
	if rematchworker.Default.Waiting() != before+1 {
		t.Error("two personas of one profile made two entries")
	}
}

func TestThePersonaRouteRefusesWhatItCannotUse(t *testing.T) {
	before := rematchworker.Default.Waiting()
	for name, tc := range map[string]struct {
		method, body string
		want         int
	}{
		"GET":            {http.MethodGet, "", http.StatusMethodNotAllowed},
		"not json":       {http.MethodPost, "nope", http.StatusBadRequest},
		"no profile id":  {http.MethodPost, `{"payload":{"persona_id":"p"}}`, http.StatusBadRequest},
		"empty profile":  {http.MethodPost, `{"payload":{"persona_id":"p","profile_id":""}}`, http.StatusBadRequest},
		"only a profile": {http.MethodPost, `{"payload":{"profile_id":"x"}}`, http.StatusAccepted},
	} {
		if code := post(EventHandler, tc.method, tc.body); code != tc.want {
			t.Errorf("%s: %d, want %d", name, code, tc.want)
		}
	}
	// "only a profile" was accepted and noted; nothing else was
	if rematchworker.Default.Waiting() != before+1 {
		t.Errorf("waiting %d, want %d", rematchworker.Default.Waiting(), before+1)
	}
}

// "career.persona.changed" never starts a wait by itself: for a profile that is not pending it changes nothing.
func TestAPersonaChangeDoesNotStartAWait(t *testing.T) {
	before := rematchworker.Default.Waiting()
	body := `{"topic":"career.persona.changed","payload":{"persona_id":"p","profile_id":"never-pending","change":"skill_added"}}`
	if code := post(ChangedHandler, http.MethodPost, body); code != http.StatusAccepted {
		t.Fatalf("status %d", code)
	}
	if rematchworker.Default.Waiting() != before {
		t.Error("a change started a wait")
	}
	if code := post(ChangedHandler, http.MethodGet, ""); code != http.StatusMethodNotAllowed {
		t.Errorf("GET %d", code)
	}
	if code := post(ChangedHandler, http.MethodPost, `{"payload":{"persona_id":"p"}}`); code != http.StatusBadRequest {
		t.Errorf("no profile id %d", code)
	}
}
