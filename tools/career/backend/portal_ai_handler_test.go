package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"career-tool-backend/portal"
)

type aiPortalJSON struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AIPlatformID string `json:"aiPlatformId"`
	AIModel      string `json:"aiModel"`
	Links        []struct {
		ID           string `json:"id"`
		URL          string `json:"url"`
		AIPlatformID string `json:"aiPlatformId"`
		AIModel      string `json:"aiModel"`
	} `json:"links"`
}

func putAI(h http.HandlerFunc, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPut, target, strings.NewReader(body)))
	return rec
}

func portalsFrom(t *testing.T, rec *httptest.ResponseRecorder) []aiPortalJSON {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Portals []aiPortalJSON `json:"portals"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Portals
}

// byID picks a portal out of the list. setupHandlerDB seeds a portal of its own, so
// the one a test created is not necessarily the first.
func byID(t *testing.T, portals []aiPortalJSON, id string) aiPortalJSON {
	t.Helper()
	for _, p := range portals {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("portal %s not in the list", id)
	return aiPortalJSON{}
}

func seedAIPortal(t *testing.T) (portalID, link1, link2 string) {
	t.Helper()
	setupHandlerDB(t)
	portalID, err := portal.CreatePortal("Stepstone")
	if err != nil {
		t.Fatal(err)
	}
	link1, err = portal.AddPortalLink(portalID, "https://a.example", "A")
	if err != nil {
		t.Fatal(err)
	}
	link2, err = portal.AddPortalLink(portalID, "https://b.example", "B")
	if err != nil {
		t.Fatal(err)
	}
	return portalID, link1, link2
}

func TestPortalAIHandler_SetReplaceClear(t *testing.T) {
	pid, _, _ := seedAIPortal(t)

	got := byID(t, portalsFrom(t, putAI(portalAIHandler, "/portals/ai", `{"id":"`+pid+`","platformId":"openrouter","model":"m1"}`)), pid)
	if got.AIPlatformID != "openrouter" || got.AIModel != "m1" {
		t.Fatalf("set: %+v", got)
	}
	if got.Name != "Stepstone" || len(got.Links) != 2 {
		t.Fatalf("set changed other fields: %+v", got)
	}
	// The links are untouched by a portal-level override.
	for _, l := range got.Links {
		if l.AIPlatformID != "" || l.AIModel != "" {
			t.Fatalf("portal override leaked to link %s", l.ID)
		}
	}

	got = byID(t, portalsFrom(t, putAI(portalAIHandler, "/portals/ai", `{"id":"`+pid+`","platformId":"anthropic","model":""}`)), pid)
	if got.AIPlatformID != "anthropic" || got.AIModel != "" {
		t.Fatalf("replace: %+v", got)
	}

	got = byID(t, portalsFrom(t, putAI(portalAIHandler, "/portals/ai", `{"id":"`+pid+`","platformId":"","model":""}`)), pid)
	if got.AIPlatformID != "" || got.AIModel != "" {
		t.Fatalf("clear: %+v", got)
	}
}

func TestPortalLinkAIHandler_SetsOneLinkOnly(t *testing.T) {
	pid, l1, l2 := seedAIPortal(t)

	got := byID(t, portalsFrom(t, putAI(portalLinkAIHandler, "/portal-links/ai", `{"id":"`+l1+`","platformId":"openrouter","model":"m2"}`)), pid)
	links := map[string][2]string{}
	for _, l := range got.Links {
		links[l.ID] = [2]string{l.AIPlatformID, l.AIModel}
	}
	if links[l1] != [2]string{"openrouter", "m2"} || links[l2] != [2]string{"", ""} {
		t.Fatalf("links: %+v", links)
	}
	if got.AIPlatformID != "" {
		t.Fatalf("link override leaked to the portal: %+v", got)
	}

	got = byID(t, portalsFrom(t, putAI(portalLinkAIHandler, "/portal-links/ai", `{"id":"`+l1+`","platformId":"","model":""}`)), pid)
	for _, l := range got.Links {
		if l.AIPlatformID != "" || l.AIModel != "" {
			t.Fatalf("clear left %+v", l)
		}
	}
}

func TestAIHandlers_RefuseBadRequests(t *testing.T) {
	pid, l1, _ := seedAIPortal(t)
	cases := []struct {
		name   string
		h      http.HandlerFunc
		target string
		method string
		body   string
		want   int
	}{
		{"portal: model without platform", portalAIHandler, "/portals/ai", http.MethodPut, `{"id":"` + pid + `","platformId":"","model":"m"}`, 400},
		{"link: model without platform", portalLinkAIHandler, "/portal-links/ai", http.MethodPut, `{"id":"` + l1 + `","platformId":"","model":"m"}`, 400},
		{"portal: too long", portalAIHandler, "/portals/ai", http.MethodPut, `{"id":"` + pid + `","platformId":"` + strings.Repeat("a", 201) + `"}`, 400},
		{"portal: missing id", portalAIHandler, "/portals/ai", http.MethodPut, `{"platformId":"p"}`, 400},
		{"link: missing id", portalLinkAIHandler, "/portal-links/ai", http.MethodPut, `{"platformId":"p"}`, 400},
		{"portal: not json", portalAIHandler, "/portals/ai", http.MethodPut, `nope`, 400},
		{"portal: unknown id", portalAIHandler, "/portals/ai", http.MethodPut, `{"id":"nope","platformId":"p"}`, 400},
		{"link: unknown id", portalLinkAIHandler, "/portal-links/ai", http.MethodPut, `{"id":"nope","platformId":"p"}`, 400},
		{"portal: GET not allowed", portalAIHandler, "/portals/ai", http.MethodGet, ``, 405},
		{"link: POST not allowed", portalLinkAIHandler, "/portal-links/ai", http.MethodPost, `{}`, 405},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c.h(rec, httptest.NewRequest(c.method, c.target, strings.NewReader(c.body)))
			if rec.Code != c.want {
				t.Fatalf("%d %s, want %d", rec.Code, rec.Body.String(), c.want)
			}
		})
	}
	// None of the refused calls changed anything.
	rec := httptest.NewRecorder()
	portalsHandler(rec, httptest.NewRequest(http.MethodGet, "/portals", nil))
	for _, p := range portalsFrom(t, rec) {
		if p.AIPlatformID != "" || p.AIModel != "" {
			t.Fatalf("a refused call changed the portal: %+v", p)
		}
		for _, l := range p.Links {
			if l.AIPlatformID != "" || l.AIModel != "" {
				t.Fatalf("a refused call changed a link: %+v", l)
			}
		}
	}
}

// The new routes (steps 102 and 103) are declared in the manifest with the portals scope, so the host lets them through.
func TestAIRoutesAreInTheManifest(t *testing.T) {
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []struct {
			Method        string `json:"method"`
			PathSuffix    string `json:"path_suffix"`
			RequiredScope string `json:"required_scope"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ method, path string }{
		{"PUT", "portals/ai"}, {"PUT", "portal-links/ai"}, {"POST", "portal-links/inherit-instructions"},
	} {
		found := false
		for _, r := range m.Routes {
			if r.Method == want.method && r.PathSuffix == want.path && r.RequiredScope == "tool:career:portals" {
				found = true
			}
		}
		if !found {
			t.Errorf("manifest has no %s %s with scope tool:career:portals", want.method, want.path)
		}
	}
}
