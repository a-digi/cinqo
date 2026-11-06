package portalhandlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

const (
	sessA = "0123456789abcdef0123456789abcdef"
	sessB = "fedcba9876543210fedcba9876543210"
)

// fakeBrowser records which sessions the Browser tool was asked to forget.
type fakeBrowser struct {
	mu        sync.Mutex
	forgotten []string
	srv       *httptest.Server
}

func newFakeBrowser(t *testing.T) *fakeBrowser {
	t.Helper()
	f := &fakeBrowser{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v1/tools/browser/proxy/login-session" && r.Header.Get("Authorization") == "Bearer person-token" {
			f.mu.Lock()
			f.forgotten = append(f.forgotten, r.URL.Query().Get("id"))
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "unexpected", http.StatusTeapot)
	}))
	t.Cleanup(f.srv.Close)
	t.Setenv("CORE_API_URL", f.srv.URL)
	return f
}

func (f *fakeBrowser) forgot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.forgotten...)
}

func sessionCall(method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "person-token"})
	PortalLinkLoginSessionHandler(rec, req)
	return rec
}

var loginLinkCounter int

func loginLink(t *testing.T, pid string) string {
	t.Helper()
	loginLinkCounter++
	rec := linkCall(http.MethodPost, `{"portalId":"`+pid+`","url":"https://board.example/members`+strings.Repeat("x", loginLinkCounter)+`","title":"Members","requiresLogin":true}`)
	var resp struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.ID == "" {
		t.Fatalf("no link: %d %s", rec.Code, rec.Body.String())
	}
	return resp.ID
}

func sessionOf(t *testing.T, link string) string {
	t.Helper()
	rec := sessionCall(http.MethodGet, "/portal-links/login-session?linkId="+link, "")
	var out struct {
		SessionID string `json:"sessionId"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	return out.SessionID
}

func TestALinkThatNeedsALoginHoldsOnlyTheSessionId(t *testing.T) {
	pid, _ := loginSeed(t)
	fb := newFakeBrowser(t)
	link := loginLink(t, pid)

	if got := sessionOf(t, link); got != "" {
		t.Fatalf("a new link has no session: %q", got)
	}
	if rec := sessionCall(http.MethodPut, "/portal-links/login-session", `{"linkId":"`+link+`","sessionId":"`+sessA+`"}`); rec.Code != 200 {
		t.Fatalf("attach: %d %s", rec.Code, rec.Body.String())
	}
	if got := sessionOf(t, link); got != sessA {
		t.Errorf("attached id: %q", got)
	}
	// Replacing: the older session is forgotten in the Browser, not kept next to the new one.
	if rec := sessionCall(http.MethodPut, "/portal-links/login-session", `{"linkId":"`+link+`","sessionId":"`+sessB+`"}`); rec.Code != 200 {
		t.Fatalf("replace: %d", rec.Code)
	}
	if got := fb.forgot(); len(got) != 1 || got[0] != sessA {
		t.Errorf("the replaced session must be forgotten: %v", got)
	}
	// Forget: the id is cleared and the Browser forgets it; twice is fine.
	if rec := sessionCall(http.MethodDelete, "/portal-links/login-session?linkId="+link, ""); rec.Code != http.StatusNoContent {
		t.Errorf("forget: %d", rec.Code)
	}
	if rec := sessionCall(http.MethodDelete, "/portal-links/login-session?linkId="+link, ""); rec.Code != http.StatusNoContent {
		t.Errorf("forget twice: %d", rec.Code)
	}
	if got := sessionOf(t, link); got != "" {
		t.Errorf("cleared: %q", got)
	}
	if got := fb.forgot(); len(got) != 2 || got[1] != sessB {
		t.Errorf("forgotten: %v", got)
	}
}

func TestASessionIsRefusedForABadIdAnUnknownLinkOrALinkWithoutLogin(t *testing.T) {
	pid, plain := loginSeed(t)
	newFakeBrowser(t)
	link := loginLink(t, pid)
	for _, bad := range []string{"short", strings.ToUpper(sessA), "../../etc/passwd", sessA + "00"} {
		if rec := sessionCall(http.MethodPut, "/portal-links/login-session", `{"linkId":"`+link+`","sessionId":"`+bad+`"}`); rec.Code != http.StatusBadRequest {
			t.Errorf("id %q: %d", bad, rec.Code)
		}
	}
	if rec := sessionCall(http.MethodPut, "/portal-links/login-session", `{"linkId":"nope","sessionId":"`+sessA+`"}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown link: %d", rec.Code)
	}
	if rec := sessionCall(http.MethodPut, "/portal-links/login-session", `{"linkId":"`+plain+`","sessionId":"`+sessA+`"}`); rec.Code != http.StatusConflict {
		t.Errorf("a link that needs no login: %d", rec.Code)
	}
	if got := sessionOf(t, link); got != "" {
		t.Errorf("nothing may be attached after refusals: %q", got)
	}
}

func TestRemovingALinkOrDroppingItsLoginForgetsTheSession(t *testing.T) {
	pid, _ := loginSeed(t)
	fb := newFakeBrowser(t)

	link := loginLink(t, pid)
	sessionCall(http.MethodPut, "/portal-links/login-session", `{"linkId":"`+link+`","sessionId":"`+sessA+`"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/portal-links", strings.NewReader(`{"id":"`+link+`","requiresLogin":false}`))
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "person-token"})
	PortalLinksHandler(rec, req)
	if rec.Code != 200 {
		t.Fatalf("drop login: %d %s", rec.Code, rec.Body.String())
	}
	if got := fb.forgot(); len(got) != 1 || got[0] != sessA {
		t.Errorf("dropping the login must forget the session: %v", got)
	}
	if got := sessionOf(t, link); got != "" {
		t.Errorf("id after dropping the login: %q", got)
	}

	link2 := loginLink(t, pid)
	sessionCall(http.MethodPut, "/portal-links/login-session", `{"linkId":"`+link2+`","sessionId":"`+sessB+`"}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/portal-links?id="+link2, nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "person-token"})
	PortalLinksHandler(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("remove link: %d", rec.Code)
	}
	if got := fb.forgot(); len(got) != 2 || got[1] != sessB {
		t.Errorf("removing the link must forget its session: %v", got)
	}
}

func TestABrowserThatIsDownNeverBlocksRemovingALink(t *testing.T) {
	pid, _ := loginSeed(t)
	t.Setenv("CORE_API_URL", "http://127.0.0.1:1")
	link := loginLink(t, pid)
	sessionCall(http.MethodPut, "/portal-links/login-session", `{"linkId":"`+link+`","sessionId":"`+sessA+`"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/portal-links?id="+link, nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "person-token"})
	PortalLinksHandler(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("remove link with the Browser down: %d", rec.Code)
	}
	if _, err := portal.GetLinkLoginSession(link); err == nil {
		t.Error("the link must be gone")
	}
}

// The link's JSON and every list stay free of the session: only the dedicated route answers an id.
func TestTheLinkListNeverCarriesTheSessionId(t *testing.T) {
	pid, _ := loginSeed(t)
	newFakeBrowser(t)
	link := loginLink(t, pid)
	sessionCall(http.MethodPut, "/portal-links/login-session", `{"linkId":"`+link+`","sessionId":"`+sessA+`"}`)
	portals, _ := portal.ListPortals()
	raw, _ := json.Marshal(portals)
	// Only the state label ("loginSession": none/valid/expired) is shown, never the id (plan 195, step 4).
	if strings.Contains(string(raw), sessA) || strings.Contains(string(raw), "sessionId") || !strings.Contains(string(raw), `"loginSession":"valid"`) {
		t.Errorf("the portal list must show the state of the kept login and never its id: %s", raw)
	}
	_ = db.JobsDB
}
