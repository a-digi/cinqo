package crawler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"browser-tool-backend/shared"
)

func TestBrowserStatus(t *testing.T) {
	ready := browserStatus(shared.ChromeStatus{Installed: true, Version: "Google Chrome 130.0", Detail: "found"})
	if ready.Value != "Ready" || ready.Caption != "Google Chrome 130.0" || ready.Link != "" {
		t.Errorf("ready = %+v", ready)
	}
	if noVersion := browserStatus(shared.ChromeStatus{Installed: true}); noVersion.Caption != "Chrome found" {
		t.Errorf("no version = %+v", noVersion)
	}

	missing := browserStatus(shared.ChromeStatus{Detail: "Chrome or Chromium was not found"})
	if missing.Value != "Needs setup" || missing.Caption != "Chrome or Chromium was not found" || missing.Link != "/tools/browser/chromium" {
		t.Errorf("missing = %+v", missing)
	}
	long := browserStatus(shared.ChromeStatus{Detail: strings.Repeat("d", 500)})
	if n := len([]rune(long.Caption)); n != maxCaptionRunes {
		t.Errorf("caption = %d characters, want %d", n, maxCaptionRunes)
	}
}

func TestDashboardStatusHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	DashboardStatusHandler(rec, httptest.NewRequest(http.MethodGet, "/dashboard/status", nil))
	var got statPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 {
		t.Fatalf("%d %q: %v", rec.Code, rec.Body.String(), err)
	}
	if got.Label != "Browser" || (got.Value != "Ready" && got.Value != "Needs setup") {
		t.Errorf("payload = %+v", got)
	}
	rec = httptest.NewRecorder()
	DashboardStatusHandler(rec, httptest.NewRequest(http.MethodPost, "/dashboard/status", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}
