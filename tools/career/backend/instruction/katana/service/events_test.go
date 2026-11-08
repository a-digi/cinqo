package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/instruction/katana"
	"career-tool-backend/portal"
)

func eventLink(t *testing.T) string {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	pid, err := portal.CreatePortal("Board")
	if err != nil {
		t.Fatal(err)
	}
	id, err := portal.AddPortalLink(pid, "https://board.example/jobs", "Jobs")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func postEvent(body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/"+RuleSavedPath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	RuleSavedEventHandler(rec, req)
	return rec
}

func flag(t *testing.T, id string) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT listing_crawl_requested FROM portal_links WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTheRuleSavedEventRaisesTheCrawlRequest(t *testing.T) {
	id := eventLink(t)
	if rec := postEvent(`{"topic":"` + TopicRuleSaved + `","payload":{"portal_link_id":"` + id + `"}}`); rec.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if flag(t, id) != 1 {
		t.Error("the crawl request was not raised")
	}
}

func TestTheRuleSavedEventNeverRaisesAnythingForALoginLink(t *testing.T) {
	id := eventLink(t)
	if err := portal.UpdatePortalLinkAutoDiscoveryEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	yes := true
	if err := portal.UpdatePortalLinkLogin(id, &yes, nil); err != nil {
		t.Fatal(err)
	}
	postEvent(`{"payload":{"portal_link_id":"` + id + `"}}`)
	if flag(t, id) != 0 {
		t.Error("a login link was flagged for an automatic crawl")
	}
}

func TestTheRuleSavedEventRefusesBadInput(t *testing.T) {
	eventLink(t)
	for _, body := range []string{``, `{`, `{"payload":{}}`, `{"payload":{"portal_link_id":""}}`} {
		if rec := postEvent(body); rec.Code != http.StatusBadRequest {
			t.Errorf("%q: %d", body, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/"+RuleSavedPath, nil)
	rec := httptest.NewRecorder()
	RuleSavedEventHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
	// an unknown link is not an error (nothing to flag)
	if rec := postEvent(`{"payload":{"portal_link_id":"nope"}}`); rec.Code != http.StatusAccepted {
		t.Errorf("unknown link: %d", rec.Code)
	}
}

// The engine declares the topic and the path the service uses; the manifest check (engine package) compares them with manifest.json.
func TestTheEngineDeclaresTheServicesTopicAndPath(t *testing.T) {
	var published, consumed bool
	for _, e := range katana.New().Events() {
		if e.Topic != TopicRuleSaved {
			t.Errorf("unexpected topic %q", e.Topic)
		}
		switch e.Role {
		case "publishes":
			published = true
		case "consumes":
			consumed = e.PathSuffix == RuleSavedPath
		}
	}
	if !published || !consumed {
		t.Errorf("published %v consumed-with-the-service's-path %v", published, consumed)
	}
}
