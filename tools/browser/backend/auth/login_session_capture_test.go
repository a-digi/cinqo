package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"browser-tool-backend/shared"
)

func confirmKeep(id string, keep bool) (int, loginWindowStateResponse, string) {
	b, _ := json.Marshal(map[string]any{"requestId": id, "keepSession": keep})
	rec := call(LoginWindowConfirmHandler, http.MethodPost, "/login-window/confirm", string(b))
	var out loginWindowStateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

func TestAConfirmedLoginIsKeptOnlyWhenThePersonAsked(t *testing.T) {
	initSessionDB(t)
	f := useFakes(t)
	openRun("r", "https://www.example.com/jobs", "")
	w := f.all()[0]
	w.cookies = []shared.SessionCookie{
		{Name: "sid", Value: "SECRET-SID", Domain: ".example.com", Path: "/", HTTPOnly: true, Secure: true},
		{Name: "foreign", Value: "SECRET-FOREIGN", Domain: ".tracker.test", Path: "/"},
	}

	// No opt-in: confirmed, nothing read from the window, nothing stored.
	code, resp, _ := confirmKeep("r", false)
	if code != 200 || resp.State != "confirmed" || resp.Session != nil || len(w.cookieURLs) != 0 {
		t.Fatalf("without the opt-in: %d %+v cookieReads=%d", code, resp, len(w.cookieURLs))
	}
	if list, _ := shared.ListLoginSessions(); len(list) != 0 {
		t.Fatalf("nothing may be stored without the opt-in: %v", list)
	}

	// With the opt-in on the already confirmed run: the session is saved, for the site's own address only.
	code, resp, body := confirmKeep("r", true)
	if code != 200 || resp.Session == nil || resp.Session.Host != "www.example.com" || resp.Session.State != "valid" {
		t.Fatalf("with the opt-in: %d %s", code, body)
	}
	if len(w.cookieURLs) != 1 || w.cookieURLs[0] != "https://www.example.com/" {
		t.Errorf("cookies must be read for the run's own site only: %v", w.cookieURLs)
	}
	for _, banned := range []string{"SECRET", "sid", "tracker"} {
		if strings.Contains(body, banned) {
			t.Errorf("the answer mentions %q: %s", banned, body)
		}
	}
	got, err := shared.OpenLoginSession(resp.Session.ID, "www.example.com")
	if err != nil || len(got) != 1 || got[0].Name != "sid" {
		t.Errorf("only the site's own cookie may be kept: %+v %v", got, err)
	}

	// Asking again for the same run returns the same session, it does not make another.
	_, again, _ := confirmKeep("r", true)
	if again.Session == nil || again.Session.ID != resp.Session.ID || len(w.cookieURLs) != 1 {
		t.Errorf("a second request must reuse the session: %+v reads=%d", again.Session, len(w.cookieURLs))
	}
	if list, _ := shared.ListLoginSessions(); len(list) != 1 {
		t.Errorf("exactly one session expected: %d", len(list))
	}
}

func TestAFirstConfirmationWithTheOptInKeepsTheSession(t *testing.T) {
	initSessionDB(t)
	f := useFakes(t)
	openRun("r", "https://example.com/jobs", "")
	f.all()[0].cookies = []shared.SessionCookie{{Name: "sid", Value: "v", Domain: "example.com", Path: "/", Expires: float64(time.Now().Add(time.Hour).Unix())}}
	code, resp, body := confirmKeep("r", true)
	if code != 200 || resp.State != "confirmed" || resp.Session == nil {
		t.Fatalf("%d %s", code, body)
	}
}

func TestAKeepThatFailsNeverFailsTheLogin(t *testing.T) {
	initSessionDB(t)
	f := useFakes(t)
	openRun("r", "https://example.com/jobs", "")
	w := f.all()[0]

	w.cookiesErr = errors.New("chrome said SECRET-DETAIL")
	code, resp, body := confirmKeep("r", true)
	if code != 200 || resp.State != "confirmed" || resp.Session != nil || resp.SessionError == "" || strings.Contains(body, "SECRET") {
		t.Errorf("a window that cannot be read: %d %s", code, body)
	}
	w.cookiesErr = nil
	w.cookies = []shared.SessionCookie{{Name: "x", Value: "y", Domain: ".other.test", Path: "/"}}
	code, resp, _ = confirmKeep("r", true)
	if code != 200 || resp.Session != nil || resp.SessionError == "" {
		t.Errorf("no cookie of the site: %d %+v", code, resp)
	}
	wantState(t, "r", "confirmed", "")
	if list, _ := shared.ListLoginSessions(); len(list) != 0 {
		t.Errorf("nothing may be stored: %v", list)
	}
}

func TestARefusedConfirmationKeepsNothing(t *testing.T) {
	initSessionDB(t)
	f := useFakes(t)
	openRun("r", "https://example.com/jobs", "")
	w := f.all()[0]
	w.cookies = []shared.SessionCookie{{Name: "sid", Value: "v", Domain: "example.com", Path: "/"}}
	w.setPage(pageInspection{PasswordField: true, TextLength: 900})
	code, _, _ := confirmKeep("r", true)
	if code != http.StatusConflict || len(w.cookieURLs) != 0 {
		t.Errorf("still logged out: %d cookieReads=%d", code, len(w.cookieURLs))
	}
	if list, _ := shared.ListLoginSessions(); len(list) != 0 {
		t.Errorf("nothing may be stored: %v", list)
	}
}
