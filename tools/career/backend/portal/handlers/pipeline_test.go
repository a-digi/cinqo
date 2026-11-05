package portalhandlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/handler/handlertest"
)

func TestPortalPipelineRouteReadsTheStageOfEveryLink(t *testing.T) {
	handlertest.SetupHandlerDB(t) // one platform with one link l1, no instructions
	if _, err := db.JobsDB.Exec(`INSERT INTO portal_links (id, portal_id, url, crawl_instructions, listing_crawl_requested) VALUES ('l2', 'p1', 'https://y', 'L', 1)`); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	PortalPipelineHandler(rec, httptest.NewRequest(http.MethodGet, "/portal-links/pipeline", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Links []struct {
			PortalLinkID string `json:"portalLinkId"`
			Stage        string `json:"stage"`
			WaitingFor   string `json:"waitingFor"`
			Detail       struct {
				Jobs int `json:"jobs"`
			} `json:"detail"`
		} `json:"links"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	stages := map[string]string{}
	for _, l := range out.Links {
		stages[l.PortalLinkID] = l.Stage + "/" + l.WaitingFor
	}
	if stages["l1"] != "no_listing_instructions/user" || stages["l2"] != "listing_crawl_waiting/page" || len(out.Links) != 2 {
		t.Errorf("%v", stages)
	}
	for _, key := range []string{`"portalLinkId"`, `"stage"`, `"waitingFor"`, `"detail"`, `"jobsWithDescription"`, `"detailFailedRuns"`} {
		if !strings.Contains(rec.Body.String(), key) {
			t.Errorf("the body lacks %s: %s", key, rec.Body.String())
		}
	}
}

// Only a read is allowed, and the call changes nothing.
func TestPortalPipelineRouteIsReadOnly(t *testing.T) {
	handlertest.SetupHandlerDB(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := httptest.NewRecorder()
		PortalPipelineHandler(rec, httptest.NewRequest(method, "/portal-links/pipeline", strings.NewReader("{}")))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", method, rec.Code)
		}
	}
	var flags int
	before := func() int {
		if err := db.JobsDB.QueryRow(`SELECT COALESCE(SUM(listing_crawl_requested + job_detail_instructions_ai_requested + job_detail_crawl_requested), 0) FROM portal_links`).Scan(&flags); err != nil {
			t.Fatal(err)
		}
		return flags
	}
	b := before()
	PortalPipelineHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/portal-links/pipeline", nil))
	if before() != b {
		t.Error("reading the pipeline changed a flag")
	}
}

func TestTheManifestDeclaresThePipelineRouteWithThePortalsScope(t *testing.T) {
	raw, err := os.ReadFile("../../../manifest.json")
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
	have := map[string]string{}
	for _, r := range m.Routes {
		if r.PathSuffix == "portal-links/pipeline" {
			have[r.Method] = r.RequiredScope
		}
	}
	if len(have) != 1 || have["GET"] != "tool:career:portals" {
		t.Errorf("the manifest declares %v for portal-links/pipeline, want exactly GET with tool:career:portals", have)
	}
	src, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^\s*http\.HandleFunc\("/portal-links/pipeline", portalhandlers\.PortalPipelineHandler\)`).Match(src) {
		t.Error("main.go does not serve /portal-links/pipeline (a commented-out line does not count)")
	}
}
