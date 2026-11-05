package profilecreatedconsumer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/match/rematchworker"
)

func post(method, body string) int {
	rec := httptest.NewRecorder()
	EventHandler(rec, httptest.NewRequest(method, "/events/profile-created-rematch", strings.NewReader(body)))
	return rec.Code
}

// A profile event notes the profile into the shared worker (the one the persona consumer uses too) and is answered at once.
func TestAProfileEventIsNotedIntoTheSharedWorker(t *testing.T) {
	before := rematchworker.Default.Waiting()
	if code := post(http.MethodPost, `{"topic":"career.profile.created","payload":{"profile_id":"prof-created-test-1"}}`); code != http.StatusAccepted {
		t.Fatalf("status %d", code)
	}
	if rematchworker.Default.Waiting() != before+1 {
		t.Error("the profile was not noted")
	}
	// the same profile again is still one entry
	post(http.MethodPost, `{"payload":{"profile_id":"prof-created-test-1"}}`)
	if rematchworker.Default.Waiting() != before+1 {
		t.Error("a repeated event made a second entry")
	}
}

func TestTheProfileRouteRefusesWhatItCannotUse(t *testing.T) {
	before := rematchworker.Default.Waiting()
	for name, tc := range map[string]struct {
		method, body string
		want         int
	}{
		"GET":             {http.MethodGet, "", http.StatusMethodNotAllowed},
		"not json":        {http.MethodPost, "nope", http.StatusBadRequest},
		"no profile id":   {http.MethodPost, `{"payload":{}}`, http.StatusBadRequest},
		"a persona event": {http.MethodPost, `{"payload":{"persona_id":"p"}}`, http.StatusBadRequest},
	} {
		if code := post(tc.method, tc.body); code != tc.want {
			t.Errorf("%s: %d, want %d", name, code, tc.want)
		}
	}
	if rematchworker.Default.Waiting() != before {
		t.Error("a refused event was noted")
	}
}
