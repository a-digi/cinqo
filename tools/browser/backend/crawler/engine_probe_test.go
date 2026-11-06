package crawler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postEngineTest(body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	EngineTestHandler(rec, httptest.NewRequest(http.MethodPost, "/engine-test", strings.NewReader(body)))
	return rec
}

func TestEngineTestHandlerValidatesItsRequest(t *testing.T) {
	for _, body := range []string{`not json`, `{}`, `{"engine":""}`, `{"engine":"rod"}`, `{"engine":"Katana"}`, `{"engine":"playwright"}`, `{"engine":"Playwright"}`, `{"engine":"playwright; rm -rf /"}`} {
		if rec := postEngineTest(body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", body, rec.Code)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		EngineTestHandler(rec, httptest.NewRequest(method, "/engine-test", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", method, rec.Code)
		}
	}
}

func TestEngineTestHandlerAdmitsOneTestAtATime(t *testing.T) {
	katanaProbeMu.Lock() // a test is "running"
	defer katanaProbeMu.Unlock()
	if rec := postEngineTest(`{"engine":"katana"}`); rec.Code != http.StatusConflict {
		t.Fatalf("a second concurrent test must be a 409, got %d", rec.Code)
	}
}

func TestManifestDeclaresTheEngineTestRouteWithItsScope(t *testing.T) {
	raw := readManifest(t)
	for _, r := range raw {
		if r["path_suffix"] == "engine-test" {
			if r["method"] != "POST" || r["required_scope"] != "tool:browser:crawl" {
				t.Errorf("route: %+v", r)
			}
			return
		}
	}
	t.Error("manifest.json does not declare the engine-test route")
}
