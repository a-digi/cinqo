package crawl

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/portal"
)

func TestTheClearedAnswerIsRelayedForTheLinksOwnSiteAndItsConfirmedJobSiteOnly(t *testing.T) {
	id := duapuneLink(t)
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b map[string]string
		_ = json.Unmarshal(raw, &b)
		seen = append(seen, r.URL.Path+"|"+b["url"]+"|"+r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	t.Setenv("CORE_API_URL", srv.URL)

	code, resp := post(t, LinkCrawlWindowClearedHandler, "/x", `{"portalLinkId":"`+id+`"}`, true)
	if code != 200 || len(seen) != 1 || !strings.HasPrefix(seen[0], "/api/v1/tools/browser/proxy/katana-window-cleared|https://duapune.com/search/advanced/filter") || !strings.HasSuffix(seen[0], "|Bearer tok-5") {
		t.Fatalf("the link's own address: %d %s %v", code, resp, seen)
	}
	// another site, a private address and no session are refused and nothing is relayed
	for name, c := range map[string]struct {
		body   string
		cookie bool
	}{
		"other site": {`{"portalLinkId":"` + id + `","url":"https://evil.example/x"}`, true},
		"unknown":    {`{"portalLinkId":"` + id + `","x":1}`, true},
		"no link":    {`{}`, true},
		"no cookie":  {`{"portalLinkId":"` + id + `"}`, false},
	} {
		if code, _ := post(t, LinkCrawlWindowClearedHandler, "/x", c.body, c.cookie); code < 400 {
			t.Errorf("%s: status %d", name, code)
		}
	}
	if len(seen) != 1 {
		t.Errorf("a refused request was relayed: %v", seen)
	}
	// the confirmed job site is allowed
	if err := portal.SetLinkJobHost(id, "jobs.other.example"); err != nil {
		t.Fatal(err)
	}
	if code, _ := post(t, LinkCrawlWindowClearedHandler, "/x", `{"portalLinkId":"`+id+`","url":"https://jobs.other.example/job/1"}`, true); code != 200 || len(seen) != 2 {
		t.Errorf("job site: %d %v", code, seen)
	}
}

func TestTheOpenRequestIsRelayedToTheBrowsersOpenRouteForTheLinksAddress(t *testing.T) {
	id := duapuneLink(t)
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b map[string]string
		_ = json.Unmarshal(raw, &b)
		seen = append(seen, r.URL.Path+"|"+b["url"])
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	t.Setenv("CORE_API_URL", srv.URL)
	if code, resp := post(t, LinkCrawlWindowOpenHandler, "/x", `{"portalLinkId":"`+id+`"}`, true); code != 200 || len(seen) != 1 ||
		!strings.HasPrefix(seen[0], "/api/v1/tools/browser/proxy/katana-window-open|https://duapune.com/search/advanced/filter") {
		t.Fatalf("open: %d %s %v", code, resp, seen)
	}
	if code, _ := post(t, LinkCrawlWindowOpenHandler, "/x", `{"portalLinkId":"`+id+`","url":"https://evil.example/"}`, true); code < 400 || len(seen) != 1 {
		t.Errorf("another site must be refused and not relayed: %d %v", code, seen)
	}
	if code, _ := post(t, LinkCrawlWindowOpenHandler, "/x", `{"portalLinkId":"`+id+`"}`, false); code < 400 || len(seen) != 1 {
		t.Errorf("no session must be refused: %d %v", code, seen)
	}
}
