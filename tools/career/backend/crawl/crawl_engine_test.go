package crawl

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func getCrawlEngine(t *testing.T, method string, cookie bool) (int, string, http.Header) {
	t.Helper()
	t.Setenv("CORE_API_URL", "http://core.invalid")
	req := httptest.NewRequest(method, "/portal-links/crawl-engine", nil)
	if cookie {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok"})
	}
	rec := httptest.NewRecorder()
	CrawlEngineHandler(rec, req)
	var body map[string]string
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, body["engine"], rec.Header()
}

func TestCrawlEngineRouteNamesTheSelectedEngine(t *testing.T) {
	for _, c := range []struct {
		engine string
		err    error
		want   string
	}{
		{"katana", nil, "katana"},
		{"rod", nil, "rod"},
		{"", nil, "rod"},                       // an older Browser sends none: its default was Rod
		{"selenium", nil, "unknown"},           // a name this screen does not know
		{"Katana", nil, "unknown"},             // exact
		{"katana", errors.New("x"), "unknown"}, // the Browser could not be read
	} {
		scriptedEngine(t, c.engine, c.err)
		code, got, h := getCrawlEngine(t, http.MethodGet, true)
		if code != http.StatusOK || got != c.want {
			t.Errorf("engine %q err %v: %d %q, want %q", c.engine, c.err, code, got, c.want)
		}
		if !strings.Contains(h.Get("Cache-Control"), "no-store") || !strings.HasPrefix(h.Get("Content-Type"), "application/json") {
			t.Errorf("headers: %v", h)
		}
	}
}

func TestCrawlEngineRouteNeedsASessionAndOnlyAnswersGet(t *testing.T) {
	scriptedEngine(t, "katana", nil)
	if code, _, _ := getCrawlEngine(t, http.MethodGet, false); code != http.StatusUnauthorized {
		t.Errorf("without the session cookie: %d, want 401", code)
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if code, _, _ := getCrawlEngine(t, m, true); code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d, want 405", m, code)
		}
	}
}

func TestCrawlEngineRouteIsDeclaredInTheManifestWithTheSameScopeAsItsNeighbour(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []map[string]string `json:"routes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	scope := func(suffix string) string {
		for _, r := range m.Routes {
			if r["method"] == "GET" && r["path_suffix"] == suffix {
				return r["required_scope"]
			}
		}
		return ""
	}
	if s := scope("portal-links/crawl-engine"); s == "" || s != scope("portal-links/link-crawl/availability") {
		t.Errorf("crawl-engine scope %q, want the availability route's %q", s, scope("portal-links/link-crawl/availability"))
	}
}

// The auto-discovery sweep lists links by the method the engine means; no usable answer lists both kinds (each crawl decides again).
func TestListingMethodNowFollowsTheEngineAndSaysNothingWhenItCannotRead(t *testing.T) {
	t.Setenv("CORE_API_URL", "http://core.invalid")
	for _, c := range []struct {
		engine string
		err    error
		token  string
		want   string
	}{
		{"katana", nil, "tok", "links"},
		{"rod", nil, "tok", "instructions"},
		{"", nil, "tok", "instructions"},
		{"katana", errors.New("down"), "tok", ""},
		{"katana", nil, "", ""}, // no live token: nothing to ask with
	} {
		scriptedEngine(t, c.engine, c.err)
		if got := ListingMethodNow(c.token); got != c.want {
			t.Errorf("engine %q err %v token %q: %q, want %q", c.engine, c.err, c.token, got, c.want)
		}
	}
}
