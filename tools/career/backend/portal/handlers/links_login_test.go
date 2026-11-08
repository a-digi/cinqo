package portalhandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
	"career-tool-backend/handler/handlertest"
	"career-tool-backend/portal"
)

type loginLinkJSON struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	Title         string `json:"title"`
	RequiresLogin bool   `json:"requiresLogin"`
	LoginURL      string `json:"loginUrl"`
	CrawlEngine   string `json:"crawlEngine"`
	JobHost       string `json:"jobHost"`
}

func linkCall(method, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	PortalLinksHandler(rec, httptest.NewRequest(method, "/portal-links", strings.NewReader(body)))
	return rec
}

func linksOf(t *testing.T, portalID string) []loginLinkJSON {
	t.Helper()
	portals, err := portal.ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(portals)
	var out []struct {
		ID    string          `json:"id"`
		Links []loginLinkJSON `json:"links"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	for _, p := range out {
		if p.ID == portalID {
			return p.Links
		}
	}
	t.Fatalf("portal %s not listed", portalID)
	return nil
}

func linkByID(t *testing.T, portalID, id string) loginLinkJSON {
	t.Helper()
	for _, l := range linksOf(t, portalID) {
		if l.ID == id {
			return l
		}
	}
	t.Fatalf("link %s not listed", id)
	return loginLinkJSON{}
}

func loginSeed(t *testing.T) (portalID, linkID string) {
	t.Helper()
	handlertest.SetupHandlerDB(t)
	portalID, err := portal.CreatePortal("Board")
	if err != nil {
		t.Fatal(err)
	}
	linkID, err = portal.AddPortalLink(portalID, "https://board.example/jobs", "Jobs")
	if err != nil {
		t.Fatal(err)
	}
	return portalID, linkID
}

func TestCreatingALinkWithLoginSettings(t *testing.T) {
	pid, _ := loginSeed(t)
	rec := linkCall(http.MethodPost, `{"portalId":"`+pid+`","url":"https://board.example/members","title":"Members","requiresLogin":true,"loginUrl":"https://accounts.board.example/login"}`)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var resp struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	l := linkByID(t, pid, resp.ID)
	if !l.RequiresLogin || l.LoginURL != "https://accounts.board.example/login" {
		t.Errorf("%+v", l)
	}
	// and the answer's own list carries the settings
	if !strings.Contains(rec.Body.String(), `"requiresLogin":true`) || !strings.Contains(rec.Body.String(), `"loginUrl":"https://accounts.board.example/login"`) {
		t.Errorf("the answer does not show the settings: %s", rec.Body.String())
	}
}

func TestCreatingALinkWithoutLoginSettingsNeedsNoLogin(t *testing.T) {
	pid, _ := loginSeed(t)
	rec := linkCall(http.MethodPost, `{"portalId":"`+pid+`","url":"https://board.example/open","title":"Open"}`)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	for _, l := range linksOf(t, pid) {
		if l.RequiresLogin || l.LoginURL != "" {
			t.Errorf("%+v", l)
		}
	}
	// the keys are always in the JSON, also for a link that needs no login
	if !strings.Contains(rec.Body.String(), `"requiresLogin":false`) || !strings.Contains(rec.Body.String(), `"loginUrl":""`) {
		t.Errorf("the keys are missing: %s", rec.Body.String())
	}
}

func TestACreateWithABadLoginAddressIsRefusedAndAddsNoLink(t *testing.T) {
	pid, _ := loginSeed(t)
	before := len(linksOf(t, pid))
	for _, bad := range []string{"ftp://x.example/login", "javascript:alert(1)", "https://u:p@x.example/login", "not a url"} {
		rec := linkCall(http.MethodPost, `{"portalId":"`+pid+`","url":"https://board.example/new","title":"New","requiresLogin":true,"loginUrl":"`+bad+`"}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "loginUrl") {
			t.Errorf("%q: %d %s", bad, rec.Code, rec.Body.String())
		}
	}
	if after := len(linksOf(t, pid)); after != before {
		t.Errorf("a refused create left %d link(s)", after-before)
	}
}

func TestUpdatingTheLoginSettingsOfALinkThroughTheRoute(t *testing.T) {
	pid, id := loginSeed(t)
	// A new link has auto-crawl on, and a link cannot need a login while it is on (manual_only.go): switch it off first.
	if rec := linkCall(http.MethodPut, `{"id":"`+id+`","autoDiscoveryEnabled":false}`); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	rec := linkCall(http.MethodPut, `{"id":"`+id+`","requiresLogin":true}`)
	if rec.Code != 200 {
		t.Fatalf("only the flag must be a valid update: %d %s", rec.Code, rec.Body.String())
	}
	if l := linkByID(t, pid, id); !l.RequiresLogin || l.LoginURL != "" {
		t.Errorf("%+v", l)
	}
	if rec := linkCall(http.MethodPut, `{"id":"`+id+`","loginUrl":"https://accounts.board.example/login"}`); rec.Code != 200 {
		t.Fatalf("only the address must be a valid update: %d %s", rec.Code, rec.Body.String())
	}
	if l := linkByID(t, pid, id); !l.RequiresLogin || l.LoginURL != "https://accounts.board.example/login" {
		t.Errorf("%+v", l)
	}
	if rec := linkCall(http.MethodPut, `{"id":"`+id+`","requiresLogin":false,"loginUrl":""}`); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if l := linkByID(t, pid, id); l.RequiresLogin || l.LoginURL != "" {
		t.Errorf("cleared: %+v", l)
	}
}

func TestABadLoginAddressInAnUpdateChangesNothingEvenTheOtherFieldsOfTheSameRequest(t *testing.T) {
	pid, id := loginSeed(t)
	rec := linkCall(http.MethodPut, `{"id":"`+id+`","title":"Renamed","requiresLogin":true,"loginUrl":"ftp://bad.example/login"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "loginUrl") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if l := linkByID(t, pid, id); l.Title != "Jobs" || l.RequiresLogin || l.LoginURL != "" {
		t.Errorf("a refused update changed the link: %+v", l)
	}
}

func TestAnUpdateOfAnUnknownLinkWithLoginSettingsIsRefused(t *testing.T) {
	loginSeed(t)
	rec := linkCall(http.MethodPut, `{"id":"nobody","requiresLogin":true}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown portal link id") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestAnUpdateWithNothingInItIsStillRefusedAndNamesTheLoginFields(t *testing.T) {
	_, id := loginSeed(t)
	rec := linkCall(http.MethodPut, `{"id":"`+id+`"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "requiresLogin") || !strings.Contains(rec.Body.String(), "loginUrl") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestTheRouteDeclaresNothingNewInTheManifest(t *testing.T) {
	// The login settings ride on the existing POST and PUT portal-links routes; no new route or scope is needed.
	raw := mustRead(t, "../../../manifest.json")
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
		if r.PathSuffix == "portal-links" {
			have[r.Method] = r.RequiredScope
		}
	}
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		if have[method] != "tool:career:portals" {
			t.Errorf("portal-links %s is declared with scope %q", method, have[method])
		}
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// --- the AI tools ---

func toolClient(t *testing.T) func(name string, args map[string]any) (string, bool) {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	portal.RegisterAddPortalLink(server)
	portal.RegisterUpdatePortalLink(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return func(name string, args map[string]any) (string, bool) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text += tc.Text
			}
		}
		return text, res.IsError
	}
}

func TestTheAILinkToolsAcceptTheLoginSettingsAndRefuseABadAddress(t *testing.T) {
	pid, id := loginSeed(t)
	call := toolClient(t)

	text, isErr := call("add_portal_link", map[string]any{"portalId": pid, "url": "https://board.example/members", "title": "Members", "requiresLogin": true, "loginUrl": "https://accounts.board.example/login"})
	if isErr {
		t.Fatalf("add: %s", text)
	}
	var added struct{ ID string }
	_ = json.Unmarshal([]byte(text), &added)
	if l := linkByID(t, pid, added.ID); !l.RequiresLogin || l.LoginURL != "https://accounts.board.example/login" {
		t.Errorf("added through the tool: %+v", l)
	}

	// auto-crawl is on for a new link: the tool refuses the login until it is off, and writes nothing
	if text, isErr := call("update_portal_link", map[string]any{"id": id, "title": "Renamed by tool", "requiresLogin": true}); !isErr || !strings.Contains(text, "Auto-discovery") {
		t.Fatalf("update with auto-crawl on must be refused with the reason: %v %s", isErr, text)
	}
	if l := linkByID(t, pid, id); l.RequiresLogin || l.Title == "Renamed by tool" {
		t.Errorf("a refused tool update changed the link: %+v", l)
	}
	if err := portal.UpdatePortalLinkAutoDiscoveryEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	if text, isErr := call("update_portal_link", map[string]any{"id": id, "requiresLogin": true}); isErr {
		t.Fatalf("update: %s", text)
	}
	if l := linkByID(t, pid, id); !l.RequiresLogin {
		t.Errorf("updated through the tool: %+v", l)
	}

	before := len(linksOf(t, pid))
	if text, isErr := call("add_portal_link", map[string]any{"portalId": pid, "url": "https://board.example/x", "title": "X", "requiresLogin": true, "loginUrl": "ftp://bad"}); !isErr || !strings.Contains(text, "loginUrl") {
		t.Errorf("a bad address through add: %v %s", isErr, text)
	}
	if after := len(linksOf(t, pid)); after != before {
		t.Errorf("a refused add left a link")
	}
	if text, isErr := call("update_portal_link", map[string]any{"id": id, "title": "Changed", "loginUrl": "javascript:alert(1)"}); !isErr || !strings.Contains(text, "loginUrl") {
		t.Errorf("a bad address through update: %v %s", isErr, text)
	}
	if l := linkByID(t, pid, id); l.Title != "Jobs" {
		t.Errorf("a refused update changed the title: %+v", l)
	}
	if text, isErr := call("update_portal_link", map[string]any{"id": id}); !isErr || !strings.Contains(text, "requiresLogin") {
		t.Errorf("an update with nothing: %v %s", isErr, text)
	}
	if text, isErr := call("update_portal_link", map[string]any{"id": "nobody", "requiresLogin": true}); !isErr || !strings.Contains(text, "unknown portal link id") {
		t.Errorf("an unknown id: %v %s", isErr, text)
	}
}

// ---- a link cannot need a login while any automatic behaviour is on (manual_only.go): the backend refuses, whatever the page does ----

func autoDiscoveryOf(t *testing.T, id string) (enabled bool, interval *int) {
	t.Helper()
	portals, err := portal.ListPortals()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range portals {
		for _, l := range p.Links {
			if l.ID == id {
				return l.AutoDiscoveryEnabled, l.AutoDiscoveryIntervalMinutes
			}
		}
	}
	t.Fatalf("link %s not listed", id)
	return false, nil
}

func TestOneRequestCannotTurnOnTheLoginAndAutoCrawlTogether(t *testing.T) {
	pid, id := loginSeed(t)
	// the link starts with auto-crawl on; the request asks for the login AND auto-crawl, and also renames the link
	rec := linkCall(http.MethodPut, `{"id":"`+id+`","title":"Renamed","requiresLogin":true,"autoDiscoveryEnabled":true}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Auto-discovery") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if l := linkByID(t, pid, id); l.Title != "Jobs" || l.RequiresLogin {
		t.Errorf("a refused request wrote something: %+v", l)
	}
	if on, _ := autoDiscoveryOf(t, id); !on {
		t.Error("auto-crawl must be unchanged")
	}
}

func TestTheLoginCannotBeTurnedOnWhileAutoCrawlIsOnButCanTogetherWithTurningItOff(t *testing.T) {
	pid, id := loginSeed(t)
	// auto-crawl is on (the default) and the request only asks for the login
	rec := linkCall(http.MethodPut, `{"id":"`+id+`","requiresLogin":true}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Auto-discovery") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if l := linkByID(t, pid, id); l.RequiresLogin {
		t.Errorf("%+v", l)
	}
	// turning auto-crawl off in the same request is fine
	rec = linkCall(http.MethodPut, `{"id":"`+id+`","requiresLogin":true,"autoDiscoveryEnabled":false}`)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if l := linkByID(t, pid, id); !l.RequiresLogin {
		t.Errorf("%+v", l)
	}
	if on, iv := autoDiscoveryOf(t, id); on || iv != nil {
		t.Errorf("auto-crawl must be off: %v %v", on, iv)
	}
}

func TestAutoCrawlCannotBeTurnedOnForALinkThatNeedsALogin(t *testing.T) {
	_, id := loginSeed(t)
	if rec := linkCall(http.MethodPut, `{"id":"`+id+`","requiresLogin":true,"autoDiscoveryEnabled":false}`); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	for name, body := range map[string]string{
		"auto-crawl on":          `{"id":"` + id + `","autoDiscoveryEnabled":true}`,
		"an interval":            `{"id":"` + id + `","autoDiscoveryIntervalMinutes":60}`,
		"listing crawl flag":     `{"id":"` + id + `","listingCrawlRequested":true}`,
		"detail crawl flag":      `{"id":"` + id + `","jobDetailCrawlRequested":true}`,
		"detail instructions AI": `{"id":"` + id + `","jobDetailInstructionsAiRequested":true}`,
	} {
		rec := linkCall(http.MethodPut, body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "login") {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if on, iv := autoDiscoveryOf(t, id); on || iv != nil {
		t.Errorf("a refused request changed something: %v %v", on, iv)
	}
	// taking the login away and turning auto-crawl on in ONE request is allowed (the link no longer needs a login)
	if rec := linkCall(http.MethodPut, `{"id":"`+id+`","requiresLogin":false,"autoDiscoveryEnabled":true}`); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if on, _ := autoDiscoveryOf(t, id); !on {
		t.Error("auto-crawl must be on now")
	}
	// switching off, and clearing flags, is always allowed
	if rec := linkCall(http.MethodPut, `{"id":"`+id+`","autoDiscoveryEnabled":false,"listingCrawlRequested":false}`); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestACreateWithTheLoginStartsWithAutoCrawlOffAndAPlainCreateWithItOn(t *testing.T) {
	pid, _ := loginSeed(t)
	rec := linkCall(http.MethodPost, `{"portalId":"`+pid+`","url":"https://board.example/members","title":"Members","requiresLogin":true}`)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var gated struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &gated)
	if on, iv := autoDiscoveryOf(t, gated.ID); on || iv != nil {
		t.Errorf("a new link that needs a login starts with auto-crawl off: %v %v", on, iv)
	}
	rec = linkCall(http.MethodPost, `{"portalId":"`+pid+`","url":"https://board.example/open","title":"Open"}`)
	var plain struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &plain)
	if on, _ := autoDiscoveryOf(t, plain.ID); !on {
		t.Error("a plain new link keeps auto-crawl on")
	}
}

// A refused request changes NOTHING, also the other fields in it: the check is made before any write.
func TestARefusedAutomaticSwitchOnALoginLinkWritesNoOtherFieldOfTheSameRequest(t *testing.T) {
	pid, id := loginSeed(t)
	if rec := linkCall(http.MethodPut, `{"id":"`+id+`","requiresLogin":true,"autoDiscoveryEnabled":false}`); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	for name, extra := range map[string]string{
		"interval":               `"autoDiscoveryIntervalMinutes":60`,
		"listing crawl flag":     `"listingCrawlRequested":true`,
		"detail crawl flag":      `"jobDetailCrawlRequested":true`,
		"detail instructions AI": `"jobDetailInstructionsAiRequested":true`,
	} {
		rec := linkCall(http.MethodPut, `{"id":"`+id+`","title":"Renamed",`+extra+`}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
		if l := linkByID(t, pid, id); l.Title != "Jobs" {
			t.Errorf("%s: the refused request renamed the link to %q", name, l.Title)
		}
	}
}

// An old row that has both (an older build could leave it; the start-up repair removes it) can still be edited for anything else.
func TestAnUnrelatedEditOfALinkThatSomehowHasBothIsNotBlocked(t *testing.T) {
	pid, id := loginSeed(t)
	if _, err := db.JobsDB.Exec(`UPDATE portal_links SET requires_login = 1, auto_discovery_enabled = 1 WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if rec := linkCall(http.MethodPut, `{"id":"`+id+`","title":"Renamed"}`); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if l := linkByID(t, pid, id); l.Title != "Renamed" {
		t.Errorf("%+v", l)
	}
}
