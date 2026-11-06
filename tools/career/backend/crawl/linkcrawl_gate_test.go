package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"career-tool-backend/portal"
)

var gotEngine atomic.Value

// scriptedEngine replaces the read of the Browser tool's engine setting.
func scriptedEngine(t *testing.T, engine string, err error) *int {
	t.Helper()
	calls := 0
	var mu sync.Mutex
	orig := readBrowserEngine
	readBrowserEngine = func(context.Context, string, string) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return engine, err
	}
	t.Cleanup(func() { readBrowserEngine = orig })
	return &calls
}

func availability(t *testing.T, linkID string) (int, linkCrawlAvailabilityResult) {
	t.Helper()
	t.Setenv("CORE_API_URL", "http://core.invalid")
	req := httptest.NewRequest(http.MethodGet, "/portal-links/link-crawl/availability?portalLinkId="+linkID, nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok"})
	rec := httptest.NewRecorder()
	LinkCrawlAvailabilityHandler(rec, req)
	var out linkCrawlAvailabilityResult
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, out
}

func TestLinkCrawlIsAvailableOnlyWhileKatanaIsSelected(t *testing.T) {
	setupTrialDB(t)
	_, linkID, _ := trialPortal(t)

	for _, c := range []struct {
		engine     string
		wantOK     bool
		wantReason string
	}{
		{"katana", true, linkCrawlAvailable},
		{"rod", false, linkCrawlReasonNotKatana},
		{"selenium", false, linkCrawlReasonNotKatana}, // any other name, e.g. an engine this Browser never had
		{"", false, linkCrawlReasonNotKatana},         // an older Browser sends none: that is rod, the default
		{"Katana", false, linkCrawlReasonNotKatana},
		{"katana ", true, linkCrawlAvailable}, // surrounding space is not a different engine
		{"colly", false, linkCrawlReasonNotKatana},
	} {
		scriptedEngine(t, c.engine, nil)
		code, got := availability(t, linkID)
		if code != http.StatusOK || got.Available != c.wantOK || got.Reason != c.wantReason || got.Message == "" {
			t.Errorf("engine %q: %d %+v, want available=%v reason=%s", c.engine, code, got, c.wantOK, c.wantReason)
		}
	}
}

func TestTheAnswerFollowsTheEngineOnTheNextRequest(t *testing.T) {
	setupTrialDB(t)
	_, linkID, _ := trialPortal(t)
	engine := "rod"
	orig := readBrowserEngine
	readBrowserEngine = func(context.Context, string, string) (string, error) { return engine, nil }
	t.Cleanup(func() { readBrowserEngine = orig })

	for _, step := range []struct {
		engine string
		want   bool
	}{{"rod", false}, {"katana", true}, {"selenium", false}, {"katana", true}, {"rod", false}} {
		engine = step.engine
		if _, got := availability(t, linkID); got.Available != step.want {
			t.Errorf("after switching to %s: available=%v, want %v (the setting must be read on every request, never cached)", step.engine, got.Available, step.want)
		}
	}
}

func TestLinkCrawlFailsClosedWhenTheEngineCannotBeRead(t *testing.T) {
	setupTrialDB(t)
	_, linkID, _ := trialPortal(t)
	scriptedEngine(t, "katana", errors.New("the Browser tool answered 404")) // even a "katana" value alongside an error must not open the gate
	code, got := availability(t, linkID)
	if code != http.StatusOK || got.Available || got.Reason != linkCrawlReasonEngineRead {
		t.Errorf("%d %+v", code, got)
	}
}

func TestALinkThatNeedsALoginNeverGetsTheLinkCrawlerAndAsksNoBrowser(t *testing.T) {
	linkID, _ := loginLinkSetup(t, true, "https://accounts.example/login")
	calls := scriptedEngine(t, "katana", nil)
	code, got := availability(t, linkID)
	if code != http.StatusOK || got.Available || got.Reason != linkCrawlReasonLoginLink {
		t.Errorf("%d %+v", code, got)
	}
	if *calls != 0 {
		t.Errorf("a login link must be answered without a Browser call (%d)", *calls)
	}
}

func TestAvailabilityRefusesBadRequests(t *testing.T) {
	setupTrialDB(t)
	scriptedEngine(t, "katana", nil)
	t.Setenv("CORE_API_URL", "http://core.invalid")

	get := func(target string, cookie bool) int {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if cookie {
			req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok"})
		}
		rec := httptest.NewRecorder()
		LinkCrawlAvailabilityHandler(rec, req)
		return rec.Code
	}
	base := "/portal-links/link-crawl/availability"
	if c := get(base, true); c != http.StatusBadRequest {
		t.Errorf("no link id: %d", c)
	}
	if c := get(base+"?portalLinkId=%20", true); c != http.StatusBadRequest {
		t.Errorf("blank link id: %d", c)
	}
	if c := get(base+"?portalLinkId=nobody", true); c != http.StatusBadRequest {
		t.Errorf("unknown link: %d", c)
	}
	_, linkID, _ := trialPortal(t)
	if c := get(base+"?portalLinkId="+linkID, false); c != http.StatusUnauthorized {
		t.Errorf("no session cookie: %d", c)
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		LinkCrawlAvailabilityHandler(rec, httptest.NewRequest(m, base+"?portalLinkId="+linkID, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
	t.Setenv("CORE_API_URL", "")
	if c := get(base+"?portalLinkId="+linkID, true); c != http.StatusInternalServerError {
		t.Errorf("no core address: %d", c)
	}
}

func TestAvailabilityIsNeverCachedAndChangesNothing(t *testing.T) {
	setupTrialDB(t)
	_, linkID, _ := trialPortal(t)
	scriptedEngine(t, "katana", nil)
	before, _ := portal.GetPortalLinkURL(linkID)
	t.Setenv("CORE_API_URL", "http://core.invalid")
	req := httptest.NewRequest(http.MethodGet, "/portal-links/link-crawl/availability?portalLinkId="+linkID, nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok"})
	rec := httptest.NewRecorder()
	LinkCrawlAvailabilityHandler(rec, req)
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	if after, _ := portal.GetPortalLinkURL(linkID); after != before {
		t.Error("the link changed")
	}
}

// requireKatanaSelected is what the later steps (save, test, run) call: it
// must refuse for every other engine and for an unreadable setting.
func TestRequireKatanaSelected(t *testing.T) {
	scriptedEngine(t, "katana", nil)
	if err := requireKatanaSelected(context.Background(), "http://core.invalid", "t"); err != nil {
		t.Errorf("katana: %v", err)
	}
	scriptedEngine(t, "rod", nil)
	if err := requireKatanaSelected(context.Background(), "http://core.invalid", "t"); !errors.Is(err, errKatanaNotSelected) {
		t.Errorf("rod: %v", err)
	}
	scriptedEngine(t, "", errors.New("boom"))
	if err := requireKatanaSelected(context.Background(), "http://core.invalid", "t"); !errors.Is(err, errBrowserEngineUnknown) {
		t.Errorf("unreadable: %v", err)
	}
}

// The real reader against a stand-in for core: it asks the Browser's settings
// route with the person's own token and understands its answer.
func TestReadBrowserEngineViaCore(t *testing.T) {
	var gotAuth, gotPath, gotMethod string
	status := http.StatusOK
	body := `{"debugEnabled":false,"fetchEngine":"katana","humanAssist":true}`
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotMethod = r.Header.Get("Authorization"), r.URL.Path, r.Method
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer core.Close()

	got, err := readBrowserEngineViaCore(context.Background(), core.URL, "tok-42")
	if err != nil || got != "katana" {
		t.Fatalf("got %q %v", got, err)
	}
	if gotAuth != "Bearer tok-42" || gotPath != "/api/v1/tools/browser/proxy/browser-settings" || gotMethod != http.MethodGet {
		t.Errorf("request: %s %s %s", gotMethod, gotPath, gotAuth)
	}

	for name, c := range map[string]struct {
		status int
		body   string
	}{
		// a failing status must be an error even when its body happens to look like a good answer
		"tool not installed": {http.StatusNotFound, `{"fetchEngine":"katana"}`},
		"session expired":    {http.StatusUnauthorized, `{"fetchEngine":"katana"}`},
		"tool crashed":       {http.StatusBadGateway, `{"fetchEngine":"katana"}`},
		"not json":           {http.StatusOK, `<html>`},
	} {
		status, body = c.status, c.body
		if got, err := readBrowserEngineViaCore(context.Background(), core.URL, "t"); err == nil {
			t.Errorf("%s: got %q without an error", name, got)
		}
	}
	core.Close()
	if _, err := readBrowserEngineViaCore(context.Background(), core.URL, "t"); err == nil {
		t.Error("a core that is down must be an error")
	}

	// the whole chain end to end against the stand-in core: the gate follows what the Browser says
	core2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"fetchEngine":"` + strings.TrimSpace(gotEngine.Load().(string)) + `"}`))
	}))
	defer core2.Close()
	for engine, want := range map[string]bool{"katana": true, "rod": false, "selenium": false} {
		gotEngine.Store(engine)
		err := requireKatanaSelected(context.Background(), core2.URL, "t")
		if (err == nil) != want {
			t.Errorf("%s: %v", engine, err)
		}
	}
}

// The route must be listed in the manifest (the core forwards only listed
// routes) with the portals scope, and registered by main.go.
func TestAvailabilityRouteIsListedAndRegistered(t *testing.T) {
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
	found := false
	for _, r := range m.Routes {
		if r["path_suffix"] == "portal-links/link-crawl/availability" {
			found = true
			if r["method"] != "GET" || r["required_scope"] != "tool:career:portals" {
				t.Errorf("route: %v", r)
			}
		}
	}
	if !found {
		t.Error("the availability route is not in manifest.json")
	}
	mainSrc, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), `"/portal-links/link-crawl/availability", crawl.LinkCrawlAvailabilityHandler`) {
		t.Error("main.go does not register the availability handler")
	}
}
