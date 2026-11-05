package crawl

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"career-tool-backend/portal"
)

// putRules calls the handler like the core's proxy does: with the person's session cookie.
func putRules(t *testing.T, body string, cookie bool) (int, string) {
	t.Helper()
	t.Setenv("CORE_API_URL", "http://core.invalid")
	req := httptest.NewRequest(http.MethodPut, "/portal-links/link-crawl/rules", strings.NewReader(body))
	if cookie {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok"})
	}
	rec := httptest.NewRecorder()
	LinkCrawlRulesHandler(rec, req)
	return rec.Code, rec.Body.String()
}

func getRules(t *testing.T, linkID string) (int, portal.LinkCrawlSettings) {
	t.Helper()
	rec := httptest.NewRecorder()
	LinkCrawlRulesHandler(rec, httptest.NewRequest(http.MethodGet, "/portal-links/link-crawl/rules?portalLinkId="+linkID, nil))
	var out portal.LinkCrawlSettings
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, out
}

func linkWithHost(t *testing.T, url string) string {
	t.Helper()
	setupTrialDB(t)
	pid, err := portal.CreatePortal("Board")
	if err != nil {
		t.Fatal(err)
	}
	id, err := portal.AddPortalLink(pid, url, "Jobs")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

const goodRules = `{"host":"board.example","include":["board.example/jobs/*"],"exclude":["board.example/jobs/featured"]}`

func TestRulesAreSavedOnlyWhileKatanaIsSelected(t *testing.T) {
	id := linkWithHost(t, "https://board.example/jobs")
	body := `{"portalLinkId":"` + id + `","crawlMethod":"links","rules":` + goodRules + `}`

	for _, engine := range []string{"rod", "playwright", ""} {
		scriptedEngine(t, engine, nil)
		code, resp := putRules(t, body, true)
		var e map[string]string
		_ = json.Unmarshal([]byte(resp), &e)
		if code != http.StatusConflict || e["code"] != codeKatanaNotSelected {
			t.Errorf("engine %q: %d %s", engine, code, resp)
		}
	}
	if code, got := getRules(t, id); code != 200 || got.Rules != nil || got.CrawlMethod != portal.CrawlMethodInstructions {
		t.Errorf("a refused save wrote something: %+v", got)
	}

	// an unreadable engine closes the gate too, with its own code
	scriptedEngine(t, "katana", errors.New("the Browser tool answered 404"))
	code, resp := putRules(t, body, true)
	var e map[string]string
	_ = json.Unmarshal([]byte(resp), &e)
	if code != http.StatusConflict || e["code"] != codeEngineUnknown {
		t.Errorf("unreadable: %d %s", code, resp)
	}
	if _, got := getRules(t, id); got.Rules != nil {
		t.Errorf("an unreadable engine must not let a rule through: %+v", got)
	}

	scriptedEngine(t, "katana", nil)
	if code, resp := putRules(t, body, true); code != http.StatusOK {
		t.Fatalf("katana: %d %s", code, resp)
	}
	code2, got := getRules(t, id)
	if code2 != 200 || got.CrawlMethod != portal.CrawlMethodLinks || got.Rules == nil || got.Rules.Host != "board.example" || len(got.Rules.Exclude) != 1 {
		t.Errorf("%+v", got)
	}
}

func TestGoingBackAndClearingNeedNoKatana(t *testing.T) {
	id := linkWithHost(t, "https://board.example/jobs")
	scriptedEngine(t, "katana", nil)
	if code, resp := putRules(t, `{"portalLinkId":"`+id+`","crawlMethod":"links","rules":`+goodRules+`}`, true); code != 200 {
		t.Fatalf("%d %s", code, resp)
	}
	calls := scriptedEngine(t, "rod", nil) // Katana is no longer selected
	// back to instructions (rule stays stored): allowed, and the engine is not even asked
	if code, resp := putRules(t, `{"portalLinkId":"`+id+`","crawlMethod":"instructions"}`, true); code != 200 {
		t.Errorf("back to instructions: %d %s", code, resp)
	}
	if code, got := getRules(t, id); code != 200 || got.CrawlMethod != portal.CrawlMethodInstructions || got.Rules == nil {
		t.Errorf("%+v", got)
	}
	// clearing the rule: allowed
	if code, resp := putRules(t, `{"portalLinkId":"`+id+`","crawlMethod":"instructions","rules":null}`, true); code != 200 {
		t.Errorf("clear: %d %s", code, resp)
	}
	if _, got := getRules(t, id); got.Rules != nil {
		t.Errorf("not cleared: %+v", got)
	}
	if *calls != 0 {
		t.Errorf("leaving the link crawler asked the Browser %d times: it must never need Katana", *calls)
	}
	// but choosing "links" again does
	if code, _ := putRules(t, `{"portalLinkId":"`+id+`","crawlMethod":"links"}`, true); code != http.StatusConflict {
		t.Errorf("choosing links without Katana: %d", code)
	}
}

func TestStoringARuleWhileOnInstructionsStillNeedsKatana(t *testing.T) {
	id := linkWithHost(t, "https://board.example/jobs")
	scriptedEngine(t, "rod", nil)
	if code, _ := putRules(t, `{"portalLinkId":"`+id+`","crawlMethod":"instructions","rules":`+goodRules+`}`, true); code != http.StatusConflict {
		t.Errorf("a rule stored with another engine selected: %d", code)
	}
	if _, got := getRules(t, id); got.Rules != nil {
		t.Errorf("%+v", got)
	}
}

func TestSavingRefusesBadRequests(t *testing.T) {
	id := linkWithHost(t, "https://board.example/jobs")
	scriptedEngine(t, "katana", nil)
	link := `"portalLinkId":"` + id + `"`
	for name, body := range map[string]string{
		"no body":                 ``,
		"not json":                `nope`,
		"no link":                 `{"crawlMethod":"links","rules":` + goodRules + `}`,
		"blank link":              `{"portalLinkId":"  ","crawlMethod":"links","rules":` + goodRules + `}`,
		"no method":               `{` + link + `,"rules":` + goodRules + `}`,
		"unknown method":          `{` + link + `,"crawlMethod":"colly","rules":` + goodRules + `}`,
		"method in capitals":      `{` + link + `,"crawlMethod":"Links","rules":` + goodRules + `}`,
		"unknown body field":      `{` + link + `,"crawlMethod":"links","rules":` + goodRules + `,"useLoginWindow":true}`,
		"unknown rule field":      `{` + link + `,"crawlMethod":"links","rules":{"host":"board.example","include":["board.example/*"],"regex":".*"}}`,
		"rules is a string":       `{` + link + `,"crawlMethod":"links","rules":"board.example/*"}`,
		"rules is an array":       `{` + link + `,"crawlMethod":"links","rules":[]}`,
		"links method no rule":    `{` + link + `,"crawlMethod":"links"}`,
		"rule of another host":    `{` + link + `,"crawlMethod":"links","rules":{"host":"other.example","include":["other.example/*"]}}`,
		"rule without includes":   `{` + link + `,"crawlMethod":"links","rules":{"host":"board.example","include":[]}}`,
		"rule with a bad pattern": `{` + link + `,"crawlMethod":"links","rules":{"host":"board.example","include":["board.example/a b"]}}`,
		"unknown link":            `{"portalLinkId":"nobody","crawlMethod":"instructions"}`,
	} {
		if code, resp := putRules(t, body, true); code != http.StatusBadRequest {
			t.Errorf("%s: got %d %s, want 400", name, code, resp)
		}
	}
	if _, got := getRules(t, id); got.Rules != nil || got.CrawlMethod != portal.CrawlMethodInstructions {
		t.Errorf("a refused request changed the link: %+v", got)
	}
}

func TestSavingNeedsTheSessionOnlyWhereKatanaIsAsked(t *testing.T) {
	id := linkWithHost(t, "https://board.example/jobs")
	scriptedEngine(t, "katana", nil)
	if code, _ := putRules(t, `{"portalLinkId":"`+id+`","crawlMethod":"links","rules":`+goodRules+`}`, false); code != http.StatusUnauthorized {
		t.Errorf("no cookie: %d", code)
	}
	t.Setenv("CORE_API_URL", "")
	req := httptest.NewRequest(http.MethodPut, "/portal-links/link-crawl/rules", strings.NewReader(`{"portalLinkId":"`+id+`","crawlMethod":"links","rules":`+goodRules+`}`))
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "tok"})
	rec := httptest.NewRecorder()
	LinkCrawlRulesHandler(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("no core address: %d", rec.Code)
	}
}

func TestALoginLinkCannotSaveRulesOrChooseLinks(t *testing.T) {
	id, _ := loginLinkSetup(t, true, "https://accounts.example/login")
	scriptedEngine(t, "katana", nil)
	u, err := portal.GetPortalLinkURL(id)
	if err != nil {
		t.Fatal(err)
	}
	host := strings.SplitN(strings.SplitN(u, "://", 2)[1], "/", 2)[0]
	body := `{"portalLinkId":"` + id + `","crawlMethod":"links","rules":{"host":"` + host + `","include":["` + host + `/*"]}}`
	if code, resp := putRules(t, body, true); code != http.StatusBadRequest || !strings.Contains(resp, "login") {
		t.Errorf("%d %s", code, resp)
	}
	if code, _ := putRules(t, `{"portalLinkId":"`+id+`","crawlMethod":"instructions","rules":null}`, true); code != 200 {
		t.Errorf("clearing on a login link: %d", code)
	}
}

func TestGetNeedsNoKatanaAndRefusesBadRequests(t *testing.T) {
	id := linkWithHost(t, "https://board.example/jobs")
	calls := scriptedEngine(t, "rod", nil)
	if code, got := getRules(t, id); code != 200 || got.CrawlMethod != portal.CrawlMethodInstructions {
		t.Errorf("%d %+v", code, got)
	}
	rec := httptest.NewRecorder()
	LinkCrawlRulesHandler(rec, httptest.NewRequest(http.MethodGet, "/portal-links/link-crawl/rules", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("no id: %d", rec.Code)
	}
	if code, _ := getRules(t, "nobody"); code != http.StatusBadRequest {
		t.Errorf("unknown: %d", code)
	}
	for _, m := range []string{http.MethodPost, http.MethodDelete, http.MethodPatch} {
		rec := httptest.NewRecorder()
		LinkCrawlRulesHandler(rec, httptest.NewRequest(m, "/portal-links/link-crawl/rules?portalLinkId="+id, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
	if *calls != 0 {
		t.Errorf("reading asked the Browser %d times", *calls)
	}
	rec = httptest.NewRecorder()
	LinkCrawlRulesHandler(rec, httptest.NewRequest(http.MethodGet, "/portal-links/link-crawl/rules?portalLinkId="+id, nil))
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control %q", rec.Header().Get("Cache-Control"))
	}
}

func TestRulesRoutesAreListedAndRegistered(t *testing.T) {
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
	seen := map[string]bool{}
	for _, r := range m.Routes {
		if r["path_suffix"] == "portal-links/link-crawl/rules" {
			seen[r["method"]] = true
			if r["required_scope"] != "tool:career:portals" {
				t.Errorf("scope: %v", r)
			}
		}
	}
	if !seen["GET"] || !seen["PUT"] {
		t.Errorf("routes in manifest.json: %v", seen)
	}
	mainSrc, _ := os.ReadFile("../main.go")
	if !strings.Contains(string(mainSrc), `"/portal-links/link-crawl/rules", crawl.LinkCrawlRulesHandler`) {
		t.Error("main.go does not register the rules handler")
	}
}
