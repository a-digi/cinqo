package crawl

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"career-tool-backend/portal"
	"career-tool-backend/portal/linkrule"
)

// fakeCore stands in for core's proxy to the Browser tool: the engine setting and the link list.
type fakeCore struct {
	mu             sync.Mutex
	engine         string
	settingsStatus int
	settingsCalls  int
	fetchStatus    int
	fetchBody      string
	fetches        []map[string]string
	auths          []string
	// fetchBodyFor answers a given page address with its own body (else fetchBody); failFor makes that address answer 502.
	fetchBodyFor map[string]string
	failFor      string
}

func newFakeCore(t *testing.T, engine string) *fakeCore {
	t.Helper()
	f := &fakeCore{engine: engine, settingsStatus: 200, fetchStatus: 200, fetchBody: `{"finalUrl":"","title":"","links":[],"truncated":false,"droppedPrivate":0}`}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/v1/tools/browser/proxy/browser-settings":
			f.settingsCalls++
			w.WriteHeader(f.settingsStatus)
			_, _ = w.Write([]byte(`{"fetchEngine":"` + f.engine + `"}`))
		case "/api/v1/tools/browser/proxy/fetch-links":
			raw, _ := io.ReadAll(r.Body)
			var b map[string]string
			_ = json.Unmarshal(raw, &b)
			f.fetches = append(f.fetches, b)
			if f.failFor != "" && b["url"] == f.failFor {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"code":"cloudflare_blocked","message":"blocked"}`))
				return
			}
			if body, ok := f.fetchBodyFor[b["url"]]; ok {
				w.WriteHeader(f.fetchStatus)
				_, _ = w.Write([]byte(body))
				return
			}
			w.WriteHeader(f.fetchStatus)
			_, _ = w.Write([]byte(f.fetchBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	return f
}

func (f *fakeCore) fetchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.fetches)
}

func post(t *testing.T, h http.HandlerFunc, path, body string, cookie bool) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if cookie {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok-5"})
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec.Code, rec.Body.String()
}

const listingJSON = `{"finalUrl":"https://duapune.com/search","title":"Listing","truncated":false,"droppedPrivate":1,"links":[
 {"url":"https://duapune.com/","texts":["duapune"]},
 {"url":"https://duapune.com/jobs/121888-backend-developer","texts":["Backend Developer","Më shumë"]},
 {"url":"https://duapune.com/jobs/121726-devops-engineer","texts":["DevOps Engineer","Më shumë"]},
 {"url":"https://duapune.com/jobs/3-qa","texts":["QA Engineer","Më shumë"]},
 {"url":"https://duapune.com/employers/2rm-lab","texts":["2RM Lab"]}]}`

func duapuneLink(t *testing.T) string {
	t.Helper()
	return linkWithHost(t, "https://duapune.com/search/advanced/filter?keyword=&category=91")
}

func TestCandidatesLoadsTheLinksOfThePageThroughTheBrowser(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "katana")
	f.fetchBody = listingJSON
	code, resp := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true)
	if code != 200 {
		t.Fatalf("%d %s", code, resp)
	}
	var got struct {
		URL            string          `json:"url"`
		Title          string          `json:"title"`
		Links          []linkrule.Link `json:"links"`
		DroppedPrivate int             `json:"droppedPrivate"`
	}
	if err := json.Unmarshal([]byte(resp), &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://duapune.com/search/advanced/filter?keyword=&category=91" || got.Title != "Listing" || len(got.Links) != 5 || got.DroppedPrivate != 1 {
		t.Errorf("%+v", got)
	}
	if len(f.fetches) != 1 || f.fetches[0]["url"] != got.URL || f.fetches[0]["rateLimitKey"] == "" {
		t.Errorf("the Browser was asked %v (want the link's address and the portal id as the pacing key)", f.fetches)
	}
	pid, _ := portal.GetPortalIDForLink(id)
	if f.fetches[0]["rateLimitKey"] != pid {
		t.Errorf("pacing key %q, want the portal id %q", f.fetches[0]["rateLimitKey"], pid)
	}
	for _, a := range f.auths {
		if a != "Bearer tok-5" {
			t.Errorf("the person's own token must be used, got %q", a)
		}
	}
}

func TestCandidatesOnlyLoadsPagesOfTheLinksOwnSite(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "katana")
	f.fetchBody = listingJSON
	ok := func(page string) int {
		code, _ := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+id+`","url":"`+page+`"}`, true)
		return code
	}
	if c := ok("https://duapune.com/search/advanced/filter?page=2"); c != 200 {
		t.Errorf("a page of the same site: %d", c)
	}
	if c := ok("HTTPS://DUAPUNE.com/x"); c != 200 {
		t.Errorf("the same site in capitals: %d", c)
	}
	before := f.fetchCount()
	for _, bad := range []string{"https://evil.example/x", "https://www.duapune.com/x", "https://duapune.com.evil.example/", "http://127.0.0.1/", "http://169.254.169.254/latest", "ftp://duapune.com/x", "javascript:alert(1)", "https://u:p@duapune.com/x", "//duapune.com/x", "/relative"} {
		if c := ok(bad); c != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", bad, c)
		}
	}
	if f.fetchCount() != before {
		t.Errorf("a refused page reached the Browser")
	}
}

func TestCandidatesNeedsKatanaAKnownLinkNoLoginAndASession(t *testing.T) {
	id := duapuneLink(t)
	body := `{"portalLinkId":"` + id + `"}`

	f := newFakeCore(t, "rod")
	code, resp := post(t, LinkCrawlCandidatesHandler, "/x", body, true)
	var e map[string]string
	_ = json.Unmarshal([]byte(resp), &e)
	if code != http.StatusConflict || e["code"] != codeKatanaNotSelected || f.fetchCount() != 0 {
		t.Errorf("rod: %d %s fetches=%d", code, resp, f.fetchCount())
	}
	f.settingsStatus = 404 // unreadable engine
	code, resp = post(t, LinkCrawlCandidatesHandler, "/x", body, true)
	_ = json.Unmarshal([]byte(resp), &e)
	if code != http.StatusConflict || e["code"] != codeEngineUnknown || f.fetchCount() != 0 {
		t.Errorf("unreadable: %d %s", code, resp)
	}
	f.settingsStatus = 200
	f.engine = "katana"
	if code, _ := post(t, LinkCrawlCandidatesHandler, "/x", body, false); code != http.StatusUnauthorized {
		t.Errorf("no session: %d", code)
	}
	if code, _ := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"nobody"}`, true); code != http.StatusBadRequest {
		t.Errorf("unknown link: %d", code)
	}
	for _, b := range []string{``, `nope`, `{}`, `{"portalLinkId":" "}`, `{"portalLinkId":"` + id + `","cookies":"x"}`} {
		if code, _ := post(t, LinkCrawlCandidatesHandler, "/x", b, true); code != http.StatusBadRequest {
			t.Errorf("%q: %d", b, code)
		}
	}
	rec := httptest.NewRecorder()
	LinkCrawlCandidatesHandler(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
	if f.fetchCount() != 0 {
		t.Errorf("the Browser was asked %d times for requests that must be refused", f.fetchCount())
	}
}

func TestCandidatesForALoginLinkAsksTheBrowserNothing(t *testing.T) {
	id, _ := loginLinkSetup(t, true, "https://accounts.example/login")
	f := newFakeCore(t, "katana")
	code, resp := post(t, LinkCrawlCandidatesHandler, "/x", `{"portalLinkId":"`+id+`"}`, true)
	if code != http.StatusBadRequest || !strings.Contains(resp, "login") {
		t.Errorf("%d %s", code, resp)
	}
	if f.fetchCount() != 0 || f.settingsCalls != 0 {
		t.Errorf("fetches=%d settingsCalls=%d: a login link must not reach the Browser at all", f.fetchCount(), f.settingsCalls)
	}
	// the same for the test route
	code, _ = post(t, LinkCrawlTestHandler, "/x", `{"portalLinkId":"`+id+`","rules":{"host":"x","include":["x/*"]}}`, true)
	if code != http.StatusBadRequest || f.fetchCount() != 0 {
		t.Errorf("test route on a login link: %d", code)
	}
}

func TestBrowserAnswersAreTranslated(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "katana")
	body := `{"portalLinkId":"` + id + `"}`
	for name, c := range map[string]struct {
		status   int
		body     string
		wantCode int
		wantHas  string
	}{
		"check page":          {502, `{"code":"cloudflare_challenge_unresolved","message":"Cloudflare challenge could not be cleared","reason":"cf-mitigated: challenge","advice":"Do not retry."}`, 502, `"code":"cloudflare_challenge_unresolved"`},
		"rate limit":          {502, `{"code":"rate_limited","message":"slow down"}`, 502, `"code":"rate_limited"`},
		"redirect refused":    {502, `{"code":"redirect_refused","message":"the page redirected to localhost"}`, 502, `redirect_refused`},
		"engine not katana":   {409, `{"code":"engine_not_katana","message":"x"}`, 409, `katana_not_selected`},
		"bad address":         {400, `refusing to crawl "x": resolves to a private/internal address`, 400, `private/internal`},
		"session expired":     {401, `no`, 502, `session expired`},
		"browser crashed":     {500, `boom`, 502, `fetch_failed`},
		"plain 502":           {502, `upstream down`, 502, `fetch_failed`},
		"garbage with 200":    {200, `<html>`, 502, `could not be read`},
		"unknown 409":         {409, `{"code":"something_else"}`, 502, `fetch_failed`},
		"not found (no tool)": {404, `not found`, 502, `fetch_failed`},
	} {
		f.fetchStatus, f.fetchBody = c.status, c.body
		code, resp := post(t, LinkCrawlCandidatesHandler, "/x", body, true)
		if code != c.wantCode || !strings.Contains(resp, c.wantHas) {
			t.Errorf("%s: %d %s, want %d containing %q", name, code, resp, c.wantCode, c.wantHas)
		}
	}
	// the advice of a verdict reaches the screen
	f.fetchStatus, f.fetchBody = 502, `{"code":"cloudflare_challenge_unresolved","message":"m","advice":"Do not retry; tell the user."}`
	if _, resp := post(t, LinkCrawlCandidatesHandler, "/x", body, true); !strings.Contains(resp, "Do not retry; tell the user.") {
		t.Errorf("advice missing: %s", resp)
	}
}

// --- learn ---

func learnReq(id string, picked, notJob []string, candidates string) string {
	pj, _ := json.Marshal(picked)
	nj, _ := json.Marshal(notJob)
	if notJob == nil {
		nj = []byte("[]")
	}
	return `{"portalLinkId":"` + id + `","picked":` + string(pj) + `,"notJob":` + string(nj) + `,"candidates":` + candidates + `}`
}

func TestLearnProposesThePartialURLWithoutTheBrowser(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "rod") // learn is pure: it needs neither Katana nor the Browser
	var listing struct {
		Links json.RawMessage `json:"links"`
	}
	_ = json.Unmarshal([]byte(listingJSON), &listing)
	code, resp := post(t, LinkCrawlLearnHandler, "/x", learnReq(id, []string{"https://duapune.com/jobs/121888-backend-developer", "https://duapune.com/jobs/121726-devops-engineer"}, nil, string(listing.Links)), false)
	if code != 200 {
		t.Fatalf("%d %s", code, resp)
	}
	var got learnResponse
	if err := json.Unmarshal([]byte(resp), &got); err != nil {
		t.Fatal(err)
	}
	if got.Pattern != "duapune.com/jobs/*" || got.MatchCount != 3 || got.NeedsConfirm || got.Rule.Host != "duapune.com" || len(got.Rule.Include) != 1 || got.Rule.Include[0] != "duapune.com/jobs/*" {
		t.Errorf("%+v", got)
	}
	if f.fetchCount() != 0 || f.settingsCalls != 0 {
		t.Errorf("learn reached the Browser (fetches=%d settings=%d)", f.fetchCount(), f.settingsCalls)
	}
}

func TestLearnRefusesWhatIsNotOnTheLinksSiteOrNotUsable(t *testing.T) {
	id := duapuneLink(t)
	newFakeCore(t, "katana")
	pick := func(u ...string) string { return learnReq(id, u, nil, "[]") }
	for name, body := range map[string]string{
		"another site":       pick("https://other.example/jobs/1", "https://other.example/jobs/2"),
		"mixed sites":        pick("https://duapune.com/jobs/1", "https://other.example/jobs/2"),
		"www is another":     pick("https://www.duapune.com/jobs/1"),
		"no picks":           pick(),
		"bad address":        pick("not a url"),
		"start page and job": pick("https://duapune.com/", "https://duapune.com/jobs/1"),
		"unknown link":       learnReq("nobody", []string{"https://duapune.com/jobs/1"}, nil, "[]"),
		"no link id":         learnReq("", []string{"https://duapune.com/jobs/1"}, nil, "[]"),
		"unknown field":      `{"portalLinkId":"` + id + `","picked":["https://duapune.com/jobs/1"],"notJob":[],"candidates":[],"regex":"x"}`,
		"not json":           `x`,
	} {
		if code, resp := post(t, LinkCrawlLearnHandler, "/x", body, true); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, code, resp)
		}
	}
	var many []string
	for i := 0; i < maxLearnPicks+1; i++ {
		many = append(many, "https://duapune.com/jobs/"+strings.Repeat("1", i%5+1))
	}
	if code, _ := post(t, LinkCrawlLearnHandler, "/x", learnReq(id, many, nil, "[]"), true); code != http.StatusBadRequest {
		t.Errorf("too many picks: %d", code)
	}
	var cands []string
	for i := 0; i < maxLearnCandidates+1; i++ {
		cands = append(cands, `{"url":"https://duapune.com/jobs/1"}`)
	}
	if code, _ := post(t, LinkCrawlLearnHandler, "/x", learnReq(id, []string{"https://duapune.com/jobs/1"}, nil, "["+strings.Join(cands, ",")+"]"), true); code != http.StatusBadRequest {
		t.Errorf("too many candidates: %d", code)
	}
}

func TestLearnKeepsNotJobLinksOutWhereItCan(t *testing.T) {
	id := duapuneLink(t)
	newFakeCore(t, "katana")
	body := learnReq(id, []string{"https://duapune.com/careers/eng/1", "https://duapune.com/careers/eng/team/2"}, []string{"https://duapune.com/careers/eng/blog/hello"},
		`[{"url":"https://duapune.com/careers/eng/1"},{"url":"https://duapune.com/careers/eng/team/2"},{"url":"https://duapune.com/careers/eng/blog/hello"}]`)
	code, resp := post(t, LinkCrawlLearnHandler, "/x", body, true)
	if code != 200 {
		t.Fatalf("%d %s", code, resp)
	}
	var got learnResponse
	_ = json.Unmarshal([]byte(resp), &got)
	if got.Pattern != "duapune.com/careers/eng/**" || len(got.Rule.Exclude) != len(got.Exclude) {
		t.Errorf("%+v", got)
	}
}

// --- preview ---

func TestPreviewShowsWhatARuleFindsAndIsPure(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "rod")
	var listing struct {
		Links json.RawMessage `json:"links"`
	}
	_ = json.Unmarshal([]byte(listingJSON), &listing)
	body := `{"portalLinkId":"` + id + `","rules":{"host":"DUAPUNE.com","include":["duapune.com/jobs/*"]},"candidates":` + string(listing.Links) + `}`
	code, resp := post(t, LinkCrawlPreviewHandler, "/x", body, false)
	if code != 200 {
		t.Fatalf("%d %s", code, resp)
	}
	var pv linkrule.Preview
	if err := json.Unmarshal([]byte(resp), &pv); err != nil {
		t.Fatal(err)
	}
	if len(pv.Items) != 3 || pv.Items[0].Title != "Backend Developer" || pv.Matched != 3 || pv.NeedsConfirm {
		t.Errorf("%+v", pv)
	}
	if f.fetchCount() != 0 || f.settingsCalls != 0 {
		t.Error("preview reached the Browser")
	}
	// a broad rule and a rule that matches too much need a confirm
	broad := `{"portalLinkId":"` + id + `","rules":{"host":"duapune.com","include":["duapune.com/**"]},"candidates":` + string(listing.Links) + `}`
	_, resp = post(t, LinkCrawlPreviewHandler, "/x", broad, false)
	_ = json.Unmarshal([]byte(resp), &pv)
	if !pv.NeedsConfirm {
		t.Errorf("a broad rule needs a confirm: %s", resp)
	}
}

func TestPreviewRefusesBadRules(t *testing.T) {
	id := duapuneLink(t)
	newFakeCore(t, "katana")
	for name, rules := range map[string]string{
		"another host":      `{"host":"other.example","include":["other.example/*"]}`,
		"no include":        `{"host":"duapune.com","include":[]}`,
		"bad pattern":       `{"host":"duapune.com","include":["duapune.com/a b"]}`,
		"host in a pattern": `{"host":"duapune.com","include":["other.example/jobs/*"]}`,
		"unknown field":     `{"host":"duapune.com","include":["duapune.com/*"],"regex":"x"}`,
	} {
		code, _ := post(t, LinkCrawlPreviewHandler, "/x", `{"portalLinkId":"`+id+`","rules":`+rules+`,"candidates":[]}`, true)
		if code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := post(t, LinkCrawlPreviewHandler, "/x", `{"portalLinkId":"nobody","rules":{"host":"a","include":["a/*"]},"candidates":[]}`, true); code != http.StatusBadRequest {
		t.Errorf("unknown link: %d", code)
	}
}

func TestPreviewMoreThan200MatchesNeedsAConfirm(t *testing.T) {
	id := duapuneLink(t)
	newFakeCore(t, "katana")
	var parts []string
	for i := 0; i < linkrule.BroadMatchLimit+1; i++ {
		parts = append(parts, `{"url":"https://duapune.com/jobs/`+strings.Repeat("a", 1)+itoa(i)+`","texts":["Job `+itoa(i)+`"]}`)
	}
	body := `{"portalLinkId":"` + id + `","rules":{"host":"duapune.com","include":["duapune.com/jobs/*"]},"candidates":[` + strings.Join(parts, ",") + `]}`
	_, resp := post(t, LinkCrawlPreviewHandler, "/x", body, true)
	var pv linkrule.Preview
	_ = json.Unmarshal([]byte(resp), &pv)
	if !pv.NeedsConfirm || pv.Matched != linkrule.BroadMatchLimit+1 {
		t.Errorf("matched %d needsConfirm %v", pv.Matched, pv.NeedsConfirm)
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

// --- test run ---

func TestTheTestRunLoadsThePageAppliesTheRuleAndSavesNothing(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "katana")
	f.fetchBody = listingJSON
	body := `{"portalLinkId":"` + id + `","rules":{"host":"duapune.com","include":["duapune.com/jobs/*"]}}`
	code, resp := post(t, LinkCrawlTestHandler, "/x", body, true)
	if code != 200 {
		t.Fatalf("%d %s", code, resp)
	}
	var got struct {
		URL string
		linkrule.PagesResult
	}
	if err := json.Unmarshal([]byte(resp), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 1 || got.Pages[0].FinalURL != "https://duapune.com/search" || got.Pages[0].Title != "Listing" || len(got.Items) != 3 || got.Matched != 3 || got.StopReason != linkrule.StopMaxPages {
		t.Errorf("%+v", got)
	}
	if f.fetchCount() != 1 {
		t.Errorf("fetches = %d", f.fetchCount())
	}
	if s, _ := portal.GetLinkCrawlSettings(id); s.Rules != nil || s.CrawlMethod != portal.CrawlMethodInstructions {
		t.Errorf("a test run saved something: %+v", s)
	}
}

func TestTheTestRunEnforcesTheSameRulesAsSaving(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "rod")
	f.fetchBody = listingJSON
	good := `"rules":{"host":"duapune.com","include":["duapune.com/jobs/*"]}`
	code, resp := post(t, LinkCrawlTestHandler, "/x", `{"portalLinkId":"`+id+`",`+good+`}`, true)
	if code != http.StatusConflict || f.fetchCount() != 0 {
		t.Errorf("rod: %d %s", code, resp)
	}
	f.engine = "katana"
	for name, b := range map[string]string{
		"rule of another host": `{"portalLinkId":"` + id + `","rules":{"host":"other.example","include":["other.example/*"]}}`,
		"bad rule":             `{"portalLinkId":"` + id + `","rules":{"host":"duapune.com","include":[]}}`,
		"other site page":      `{"portalLinkId":"` + id + `",` + good + `,"url":"https://evil.example/x"}`,
		"unknown field":        `{"portalLinkId":"` + id + `",` + good + `,"save":true}`,
	} {
		if code, _ := post(t, LinkCrawlTestHandler, "/x", b, true); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	if f.fetchCount() != 0 {
		t.Errorf("a refused test run reached the Browser %d times", f.fetchCount())
	}
	if code, _ := post(t, LinkCrawlTestHandler, "/x", `{"portalLinkId":"`+id+`",`+good+`}`, false); code != http.StatusUnauthorized {
		t.Errorf("no session: %d", code)
	}
	f.fetchStatus, f.fetchBody = 502, `{"code":"cloudflare_blocked","message":"blocked"}`
	if code, resp := post(t, LinkCrawlTestHandler, "/x", `{"portalLinkId":"`+id+`",`+good+`}`, true); code != http.StatusBadGateway || !strings.Contains(resp, "cloudflare_blocked") {
		t.Errorf("verdict: %d %s", code, resp)
	}
}

func TestLinkCrawlPageRoutesAreListedAndRegistered(t *testing.T) {
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
	mainSrc, _ := os.ReadFile("../main.go")
	for name, handler := range map[string]string{"candidates": "LinkCrawlCandidatesHandler", "learn": "LinkCrawlLearnHandler", "learn-pagination": "LinkCrawlLearnPaginationHandler", "preview": "LinkCrawlPreviewHandler", "test": "LinkCrawlTestHandler"} {
		found := false
		for _, r := range m.Routes {
			if r["path_suffix"] == "portal-links/link-crawl/"+name {
				found = true
				if r["method"] != "POST" || r["required_scope"] != "tool:career:portals" {
					t.Errorf("%s: %v", name, r)
				}
			}
		}
		if !found {
			t.Errorf("%s is not in manifest.json", name)
		}
		if !strings.Contains(string(mainSrc), `"/portal-links/link-crawl/`+name+`", crawl.`+handler) {
			t.Errorf("main.go does not register %s", name)
		}
	}
}

// --- pagination ---

func TestLearnPaginationRouteIsPureAndStaysOnTheSite(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "rod") // pure: no Katana, no Browser
	body := func(extra string) string {
		return `{"portalLinkId":"` + id + `",` + extra + `}`
	}
	code, resp := post(t, LinkCrawlLearnPaginationHandler, "/x", body(`"picked":["https://duapune.com/search/advanced/filter?keyword=&category=91&page=2"],"candidates":[]`), false)
	if code != 200 {
		t.Fatalf("%d %s", code, resp)
	}
	var got linkrule.PaginationProposal
	if err := json.Unmarshal([]byte(resp), &got); err != nil {
		t.Fatal(err)
	}
	if got.Pagination.Kind != linkrule.PagesTemplate || got.Pagination.Template != "https://duapune.com/search/advanced/filter?keyword=&category=91&page={n}" || got.Pagination.Second != 2 || got.NeedsConfirm {
		t.Errorf("%+v", got)
	}
	// another first page of the site is allowed, another site is not
	if code, _ := post(t, LinkCrawlLearnPaginationHandler, "/x", body(`"currentUrl":"https://duapune.com/search/filter/latest/1","picked":["https://duapune.com/search/filter/latest/2"],"candidates":[]`), false); code != 200 {
		t.Errorf("another page of the site: %d", code)
	}
	for name, b := range map[string]string{
		"other site current":   body(`"currentUrl":"https://evil.example/x","picked":["https://evil.example/x?page=2"],"candidates":[]`),
		"pick on another site": body(`"picked":["https://evil.example/x?page=2"],"candidates":[]`),
		"no picks":             body(`"picked":[],"candidates":[]`),
		"bad pick":             body(`"picked":["javascript:alert(1)"],"candidates":[]`),
		"unknown field":        body(`"picked":["https://duapune.com/a"],"candidates":[],"save":true`),
		"unknown link":         `{"portalLinkId":"nobody","picked":["https://duapune.com/a"],"candidates":[]}`,
		"no link":              `{"picked":["https://duapune.com/a"],"candidates":[]}`,
	} {
		if code, resp := post(t, LinkCrawlLearnPaginationHandler, "/x", b, false); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, code, resp)
		}
	}
	var many []string
	for i := 0; i < maxLearnPicks+1; i++ {
		many = append(many, `"https://duapune.com/a?page=`+itoa(i+2)+`"`)
	}
	if code, _ := post(t, LinkCrawlLearnPaginationHandler, "/x", body(`"picked":[`+strings.Join(many, ",")+`],"candidates":[]`), false); code != http.StatusBadRequest {
		t.Errorf("too many picks: %d", code)
	}
	if f.fetchCount() != 0 || f.settingsCalls != 0 {
		t.Error("learning the pagination reached the Browser")
	}
}

// scriptedPages answers fetch-links per requested page address.
func scriptedPages(f *fakeCore, pages map[string]string) {
	f.fetchBodyFor = pages
}

func TestTheTestRunFollowsTheRulesPaginationOverTwoPagesAtMost(t *testing.T) {
	id := duapuneLink(t)
	f := newFakeCore(t, "katana")
	page := func(title string, jobs ...string) string {
		var parts []string
		for i, j := range jobs {
			parts = append(parts, `{"url":"https://duapune.com/jobs/`+j+`","texts":["Job `+j+` `+itoa(i)+`"]}`)
		}
		return `{"finalUrl":"","title":"` + title + `","links":[` + strings.Join(parts, ",") + `]}`
	}
	scriptedPages(f, map[string]string{
		"https://duapune.com/search/advanced/filter?keyword=&category=91":        page("Page one", "1", "2", "3"),
		"https://duapune.com/search/advanced/filter?keyword=&category=91&page=2": page("Page two", "4", "5"),
		"https://duapune.com/search/advanced/filter?keyword=&category=91&page=3": page("Page three", "6", "7"),
	})
	rules := `{"host":"duapune.com","include":["duapune.com/jobs/*"],"pagination":{"kind":"template","template":"https://duapune.com/search/advanced/filter?keyword=&category=91&page={n}","second":2,"maxPages":9}}`
	code, resp := post(t, LinkCrawlTestHandler, "/x", `{"portalLinkId":"`+id+`","rules":`+rules+`}`, true)
	if code != 200 {
		t.Fatalf("%d %s", code, resp)
	}
	var got struct {
		linkrule.PagesResult
	}
	if err := json.Unmarshal([]byte(resp), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 2 || len(got.Items) != 5 || got.StopReason != linkrule.StopMaxPages || f.fetchCount() != 2 {
		t.Errorf("a test run loads two pages at most: pages=%d items=%d stop=%s fetches=%d", len(got.Pages), len(got.Items), got.StopReason, f.fetchCount())
	}
	for _, fe := range f.fetches {
		if fe["rateLimitKey"] == "" {
			t.Errorf("every page needs the pacing key: %v", fe)
		}
	}
	// a later page that fails keeps the first page's jobs
	f.mu.Lock()
	f.fetches = nil
	f.failFor = "https://duapune.com/search/advanced/filter?keyword=&category=91&page=2"
	f.mu.Unlock()
	code, resp = post(t, LinkCrawlTestHandler, "/x", `{"portalLinkId":"`+id+`","rules":`+rules+`}`, true)
	if code != 200 {
		t.Fatalf("%d %s", code, resp)
	}
	got.PagesResult = linkrule.PagesResult{}
	_ = json.Unmarshal([]byte(resp), &got)
	if got.StopReason != linkrule.StopFetchFailed || len(got.Items) != 3 {
		t.Errorf("%+v", got.PagesResult)
	}
	if s, _ := portal.GetLinkCrawlSettings(id); s.Rules != nil {
		t.Errorf("a test run saved a rule: %+v", s)
	}
}

func TestARuleWithABadPaginationIsRefusedEverywhere(t *testing.T) {
	id := duapuneLink(t)
	newFakeCore(t, "katana")
	bad := `{"host":"duapune.com","include":["duapune.com/jobs/*"],"pagination":{"kind":"template","template":"https://evil.example/x?page={n}","second":2}}`
	for name, h := range map[string]http.HandlerFunc{"preview": LinkCrawlPreviewHandler, "test": LinkCrawlTestHandler} {
		body := `{"portalLinkId":"` + id + `","rules":` + bad + `,"candidates":[]}`
		if name == "test" {
			body = `{"portalLinkId":"` + id + `","rules":` + bad + `}`
		}
		if code, resp := post(t, h, "/x", body, true); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, code, resp)
		}
	}
	scriptedEngine(t, "katana", nil) // putRules points the core address at a dead host: the engine is scripted from here on
	code, resp := putRules(t, `{"portalLinkId":"`+id+`","crawlMethod":"instructions","rules":`+bad+`}`, true)
	if code != http.StatusBadRequest {
		t.Errorf("save: %d %s", code, resp)
	}
	good := `{"host":"duapune.com","include":["duapune.com/jobs/*"],"pagination":{"kind":"link","pattern":"duapune.com/more/*","maxPages":3}}`
	if code, resp := putRules(t, `{"portalLinkId":"`+id+`","crawlMethod":"instructions","rules":`+good+`}`, true); code != 200 {
		t.Fatalf("save with a good pagination: %d %s", code, resp)
	}
	if s, _ := portal.GetLinkCrawlSettings(id); s.Rules == nil || s.Rules.Pagination == nil || s.Rules.Pagination.Kind != linkrule.PagesLink || s.Rules.Pagination.MaxPages != 3 {
		t.Errorf("the pagination was not stored: %+v", s.Rules)
	}
}
