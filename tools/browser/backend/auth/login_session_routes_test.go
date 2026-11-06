package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"browser-tool-backend/shared"
)

func initSessionDB(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("TOOL_DB_DIR", filepath.Join(root, "db"))
	t.Setenv("TOOL_LOGS_DIR", filepath.Join(root, "logs"))
	t.Setenv("TOOL_CACHE_DIR", filepath.Join(root, "cache"))
	if err := shared.InitDB(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shared.DB.Close() })
}

func TestLoginSessionRoutesAnswerStateNeverCookiesAndForgetIsIdempotent(t *testing.T) {
	initSessionDB(t)
	info, err := shared.SaveLoginSession("www.example.com", []shared.SessionCookie{{Name: "sid", Value: "SECRET-VALUE", Domain: ".example.com", Path: "/", HTTPOnly: true}})
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	LoginSessionStateHandler(rec, httptest.NewRequest(http.MethodGet, "/login-session/state?id="+info.ID, nil))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `"state":"valid"`) || !strings.Contains(body, `"host":"www.example.com"`) {
		t.Fatalf("state: %d %s", rec.Code, body)
	}
	for _, banned := range []string{"SECRET", "sid", "cookie", "sealed"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(banned)) {
			t.Errorf("the state answer mentions %q: %s", banned, body)
		}
	}

	for _, bad := range []string{"", "../x", "ABC", strings.Repeat("g", 32)} {
		rec = httptest.NewRecorder()
		LoginSessionStateHandler(rec, httptest.NewRequest(http.MethodGet, "/login-session/state?id="+bad, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("id %q must be 400, got %d", bad, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	LoginSessionStateHandler(rec, httptest.NewRequest(http.MethodPost, "/login-session/state?id="+info.ID, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST must be 405, got %d", rec.Code)
	}

	for i := 0; i < 2; i++ { // twice: idempotent
		rec = httptest.NewRecorder()
		LoginSessionForgetHandler(rec, httptest.NewRequest(http.MethodDelete, "/login-session?id="+info.ID, nil))
		if rec.Code != http.StatusNoContent {
			t.Errorf("forget #%d: %d", i, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	LoginSessionStateHandler(rec, httptest.NewRequest(http.MethodGet, "/login-session/state?id="+info.ID, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("after forget: %d", rec.Code)
	}
}
