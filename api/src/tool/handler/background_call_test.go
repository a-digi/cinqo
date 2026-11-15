package handler

import (
	"database/sql"
	lift_security "github.com/a-digi/coco-lift/security"
	server_security "github.com/a-digi/coco-server/server/security"
	"gopkg.in/yaml.v3"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/organisation"
	organisation_persistent "github.com/a-digi/cinqo/src/organisation/repository/persistent"
	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

type bgEnv struct {
	di                              *uploadDI
	db                              *sql.DB
	acme, off, susp                 string
	target                          *tool_entity.Tool
	listLinks, loginWindow, capture *tool_entity.ToolRoute
}

// bgSetup: a browser that allows list-links and capture in the background (not login-window), a namako that is approved for list-links and login-window (not
// capture), a career with no approvals, a disabled tool, and four organisations: default (unattended off), acme (on), off (on, then off) and susp (on, suspended).
func bgSetup(t *testing.T) *bgEnv {
	t.Helper()
	di, _ := newUploadDI(t)
	db := di.db
	for _, q := range []string{
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, service_token, created_at) VALUES ('tb', 'browser', 'Browser', '1.0.0', 'backend_only', 1, 'running', '/x', 'tok-browser', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, service_token, created_at) VALUES ('tn', 'namako', 'Namako', '1.0.0', 'backend_only', 1, 'running', '/x', 'tok-namako', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, service_token, created_at) VALUES ('tc', 'career', 'Career', '1.0.0', 'backend_only', 1, 'running', '/x', 'tok-career', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, service_token, created_at) VALUES ('td', 'dormant', 'Dormant', '1.0.0', 'backend_only', 0, 'stopped', '/x', 'tok-dormant', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, service_token, created_at) VALUES ('ts', 'stalled', 'Stalled', '1.0.0', 'backend_only', 1, 'stopped', '/x', 'tok-stalled', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, service_token, created_at) VALUES ('tx', 'nosecret', 'No secret', '1.0.0', 'backend_only', 1, 'running', '/x', '', '2026-11-15T00:00:00Z')`,
		`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope, allow_background) VALUES ('tb', 'POST', 'list-links', 's', 1)`,
		`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope, allow_background) VALUES ('tb', 'POST', 'capture', 's', 1)`,
		`INSERT INTO tool_routes (tool_id, method, path_suffix, required_scope, allow_background) VALUES ('tb', 'POST', 'login-window', 's', 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	bp := tool_persistent.NewToolBackgroundPersistentRepo(db)
	for _, p := range []string{"list-links", "login-window", "capture"} {
		_ = bp.ReplaceRequests("tn", append(mustRequests(t, db, "tn"), tool_entity.ToolBackgroundRequest{TargetSlug: "browser", Method: "POST", PathSuffix: p}))
	}
	for _, p := range []string{"list-links", "login-window"} { // login-window: approved earlier, the browser does not allow it
		if err := bp.Approve("tn", tool_entity.ToolBackgroundRequest{TargetSlug: "browser", Method: "POST", PathSuffix: p}, "andet"); err != nil {
			t.Fatal(err)
		}
	}
	op := organisation_persistent.NewOrganisationPersistentRepo(db)
	e := &bgEnv{di: di, db: db}
	mk := func(name string, on bool, status string) string {
		o, _ := op.Create(name)
		if on {
			_ = op.SetBackground(o.ID, true, "andet")
		}
		if status != "active" {
			_ = op.Update(o.ID, name, status)
		}
		return o.ID
	}
	e.acme, e.off, e.susp = mk("Acme", true, "active"), mk("Off", false, "active"), mk("Susp", true, "suspended")
	q := tool_query.NewToolQueryRepo(db)
	e.target, _ = q.FindBySlug("browser")
	e.listLinks, _ = q.FindRoute("tb", "POST", "list-links")
	e.loginWindow, _ = q.FindRoute("tb", "POST", "login-window")
	e.capture, _ = q.FindRoute("tb", "POST", "capture")
	backgroundLimiter = newLimiter()
	t.Cleanup(func() { backgroundLimiter = newLimiter() })
	return e
}

func mustRequests(t *testing.T, db *sql.DB, tool string) []tool_entity.ToolBackgroundRequest {
	t.Helper()
	r, err := tool_query.NewToolBackgroundQueryRepo(db).Requests(tool)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func svcReq(token string) *http.Request {
	r := httptest.NewRequest("POST", "/x", nil)
	if token != "" {
		r.Header.Set("Authorization", serviceAuthScheme+token)
	}
	r.Header.Set("Cookie", "access_token=a-persons-token")
	return r
}

func (e *bgEnv) call(r *http.Request, acting string, route *tool_entity.ToolRoute, remainder string) serviceResult {
	return serviceCall(e.db, r, acting, e.target, route, remainder)
}

func (e *bgEnv) usage(t *testing.T, tool string) (calls, denied int) {
	t.Helper()
	u, err := tool_query.NewToolBackgroundQueryRepo(e.db).Usage(tool)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range u {
		calls += x.Calls
		denied += x.Denied
	}
	return
}

func TestAnApprovedToolCallsAnAllowedRouteForAnOrganisationThatSwitchedItOn(t *testing.T) {
	e := bgSetup(t)
	r := svcReq("tok-namako")
	r.Header.Set(organisation.Header, "someone-elses")              // whatever was put there is replaced
	r.Header.Set(organisation.CallerToolHeader, "pretending-to-be") // so is this
	res := e.call(r, e.acme, e.listLinks, "list-links")
	if res.code != 0 || res.release == nil {
		t.Fatalf("%+v", res)
	}
	defer res.release()
	if r.Header.Get(organisation.Header) != e.acme || r.Header.Get(organisation.CallerToolHeader) != "namako" {
		t.Errorf("the target is told the checked organisation and the real caller: %v", r.Header)
	}
	if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
		t.Errorf("the service token (and any cookie) must never reach the target: %v", r.Header)
	}
	if calls, denied := e.usage(t, "tn"); calls != 1 || denied != 0 {
		t.Errorf("%d calls, %d denied", calls, denied)
	}
}

func TestEveryRefusalWithItsReasonAndNothingGetsThrough(t *testing.T) {
	e := bgSetup(t)
	cases := map[string]struct {
		token, acting, remainder string
		route                    *tool_entity.ToolRoute
		want                     int
		contains                 string
	}{
		"no token at all":                             {"", e.acme, "list-links", e.listLinks, 401, "service token"},
		"an unknown token":                            {"tok-nobody", e.acme, "list-links", e.listLinks, 401, "invalid"},
		"a person's session token is not one":         {"a-persons-token", e.acme, "list-links", e.listLinks, 401, "invalid"},
		"a blank token (a tool with no secret)":       {"x", e.acme, "list-links", e.listLinks, 401, "service token"},
		"the calling tool is switched off":            {"tok-dormant", e.acme, "list-links", e.listLinks, 403, "not running"},
		"a route the target never allowed":            {"tok-namako", e.acme, "login-window", e.loginWindow, 403, "cannot be used without a signed-in person"},
		"allowed, but not approved":                   {"tok-namako", e.acme, "capture", e.capture, 403, "no approval"},
		"nothing approved for this tool":              {"tok-career", e.acme, "list-links", e.listLinks, 403, "no approval"},
		"no organisation named":                       {"tok-namako", "", "list-links", e.listLinks, 403, "no organisation"},
		"a path in place of an organisation":          {"tok-namako", "../acme", "list-links", e.listLinks, 403, "no organisation"},
		"an unknown organisation":                     {"tok-namako", "11111111-2222-3333-4444-555555555555", "list-links", e.listLinks, 403, "unknown organisation"},
		"a suspended organisation":                    {"tok-namako", e.susp, "list-links", e.listLinks, 403, "suspended"},
		"unattended use is off":                       {"tok-namako", e.off, "list-links", e.listLinks, 403, "not switched on"},
		"the default organisation, never switched on": {"tok-namako", "default", "list-links", e.listLinks, 403, "not switched on"},
		"approved for another route's path":           {"tok-namako", e.acme, "something-else", e.listLinks, 403, "no approval"},
	}
	for name, c := range cases {
		token := c.token
		if strings.HasPrefix(name, "a blank token") {
			token = " " // an empty service token must never match a tool that has none
		}
		r := svcReq(token)
		res := e.call(r, c.acting, c.route, c.remainder)
		if res.code != c.want || !strings.Contains(res.message, c.contains) || res.release != nil {
			t.Errorf("%s: %d %q", name, res.code, res.message)
		}
		if r.Header.Get(organisation.Header) != "" || r.Header.Get(organisation.CallerToolHeader) != "" {
			t.Errorf("%s: a refused call must not be marked for forwarding: %v", name, r.Header)
		}
	}
	if calls, _ := e.usage(t, "tn"); calls != 0 {
		t.Errorf("a refused call is not a call: %d", calls)
	}
}

func TestRefusalsAfterIdentifyingAreCountedAsDeniedAndAnUnknownTokenIsNot(t *testing.T) {
	e := bgSetup(t)
	e.call(svcReq("tok-namako"), e.off, e.listLinks, "list-links")  // refused: off
	e.call(svcReq("tok-namako"), e.susp, e.listLinks, "list-links") // refused: suspended
	e.call(svcReq("tok-namako"), e.acme, e.capture, "capture")      // refused: not approved
	e.call(svcReq("tok-nobody"), e.acme, e.listLinks, "list-links") // not a tool: counted for nobody
	if calls, denied := e.usage(t, "tn"); calls != 0 || denied != 3 {
		t.Errorf("%d calls, %d denied", calls, denied)
	}
	res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links")
	res.release()
	res = e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links")
	res.release()
	if calls, denied := e.usage(t, "tn"); calls != 2 || denied != 3 {
		t.Errorf("%d calls, %d denied", calls, denied)
	}
	u, _ := tool_query.NewToolBackgroundQueryRepo(e.db).Usage("tn")
	if len(u) < 2 || u[0].Day != time.Now().UTC().Format("2006-01-02") {
		t.Errorf("one row per organisation and day: %+v", u)
	}
	if n, _ := e.usage(t, "tc"); n != 0 {
		t.Error("another tool's counters are its own")
	}
}

func TestTheKillSwitchStopsEveryUnattendedCallAndCountsNothing(t *testing.T) {
	e := bgSetup(t)
	t.Setenv(killSwitchEnv, "off")
	res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links")
	if res.code != http.StatusServiceUnavailable || !strings.Contains(res.message, "switched off") || res.release != nil {
		t.Errorf("%+v", res)
	}
	if calls, denied := e.usage(t, "tn"); calls != 0 || denied != 0 {
		t.Errorf("%d %d", calls, denied)
	}
	t.Setenv(killSwitchEnv, "on")
	if res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links"); res.code != 0 {
		t.Errorf("only the value off stops it: %+v", res)
	} else {
		res.release()
	}
}

func TestRevokingOrUnmarkingStopsTheNextCallAtOnce(t *testing.T) {
	e := bgSetup(t)
	if res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links"); res.code != 0 {
		t.Fatalf("%+v", res)
	} else {
		res.release()
	}
	if _, err := tool_persistent.NewToolBackgroundPersistentRepo(e.db).Revoke("tn", tool_entity.ToolBackgroundRequest{TargetSlug: "browser", Method: "POST", PathSuffix: "list-links"}); err != nil {
		t.Fatal(err)
	}
	if res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links"); res.code != http.StatusForbidden {
		t.Errorf("after the approval was taken back: %+v", res)
	}
	_ = tool_persistent.NewToolBackgroundPersistentRepo(e.db).Approve("tn", tool_entity.ToolBackgroundRequest{TargetSlug: "browser", Method: "POST", PathSuffix: "list-links"}, "andet")
	if res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links"); res.code != 0 {
		t.Fatalf("approved again: %+v", res)
	} else {
		res.release()
	}
	unmarked := *e.listLinks
	unmarked.AllowBackground = false // the browser stops allowing the route after the approval
	if res := e.call(svcReq("tok-namako"), e.acme, &unmarked, "list-links"); res.code != http.StatusForbidden {
		t.Errorf("the approval stays on record but the route flag is checked on every call: %+v", res)
	}
	op := organisation_persistent.NewOrganisationPersistentRepo(e.db)
	_ = op.SetBackground(e.acme, false, "andet")
	if res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links"); res.code != http.StatusForbidden {
		t.Errorf("switched off: %+v", res)
	}
	_ = op.SetBackground(e.acme, true, "andet")
	_ = op.Update(e.acme, "Acme", "suspended")
	if res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links"); res.code != http.StatusForbidden {
		t.Errorf("suspended: %+v", res)
	}
}

func TestTheRateLimitIsPerToolAndOrganisationAndTheWindowSlides(t *testing.T) {
	e := bgSetup(t)
	oldMin, oldConc := backgroundPerMinute, backgroundConcurrent
	backgroundPerMinute, backgroundConcurrent = 3, 10
	t.Cleanup(func() { backgroundPerMinute, backgroundConcurrent = oldMin, oldConc })
	clock := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	backgroundLimiter.now = func() time.Time { return clock }
	other := func() string {
		o, _ := organisation_persistent.NewOrganisationPersistentRepo(e.db).Create("Other")
		_ = organisation_persistent.NewOrganisationPersistentRepo(e.db).SetBackground(o.ID, true, "andet")
		return o.ID
	}()
	for i := 0; i < 3; i++ {
		res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links")
		if res.code != 0 {
			t.Fatalf("call %d: %+v", i+1, res)
		}
		res.release()
		clock = clock.Add(10 * time.Second)
	}
	// another tool, approved for the same route, in the same organisation, has an allowance of its own
	if err := tool_persistent.NewToolBackgroundPersistentRepo(e.db).ReplaceRequests("tc", []tool_entity.ToolBackgroundRequest{{TargetSlug: "browser", Method: "POST", PathSuffix: "list-links"}}); err != nil {
		t.Fatal(err)
	}
	_ = tool_persistent.NewToolBackgroundPersistentRepo(e.db).Approve("tc", tool_entity.ToolBackgroundRequest{TargetSlug: "browser", Method: "POST", PathSuffix: "list-links"}, "andet")
	if res := e.call(svcReq("tok-career"), e.acme, e.listLinks, "list-links"); res.code != 0 {
		t.Errorf("a tool does not use up another tool's allowance: %+v", res)
	} else {
		res.release()
	}
	res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links") // the 4th in 30 seconds
	if res.code != http.StatusTooManyRequests || res.retryAfter != 30*time.Second || res.release != nil || !strings.Contains(res.message, "slow down") {
		t.Fatalf("%+v", res)
	}
	if res := e.call(svcReq("tok-namako"), other, e.listLinks, "list-links"); res.code != 0 {
		t.Errorf("another organisation has its own allowance: %+v", res)
	} else {
		res.release()
	}
	clock = clock.Add(31 * time.Second) // the first call of the window is now 61 seconds old
	if res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links"); res.code != 0 {
		t.Errorf("the window slides: %+v", res)
	} else {
		res.release()
	}
	if _, denied := e.usage(t, "tn"); denied != 1 {
		t.Errorf("the refused call is counted as denied: %d", denied)
	}
	clock = clock.Add(time.Hour)
	for i := 0; i < 3; i++ {
		res := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links")
		if res.code != 0 {
			t.Fatalf("after an hour call %d: %+v", i+1, res)
		}
		res.release()
	}
	key := "tn\x00" + e.acme
	if got := len(backgroundLimiter.hits[key]); got != 3 {
		t.Errorf("a key that is used again keeps only its current window, not the hour-old calls: %d", got)
	}
	if len(backgroundLimiter.hits) > 3 {
		t.Errorf("keys are bounded by the tools and organisations that call: %d", len(backgroundLimiter.hits))
	}
}

func TestTheConcurrencyLimitIsReleasedWhenACallEndsAndOnlyOnce(t *testing.T) {
	e := bgSetup(t)
	oldMin, oldConc := backgroundPerMinute, backgroundConcurrent
	backgroundPerMinute, backgroundConcurrent = 1000, 2
	t.Cleanup(func() { backgroundPerMinute, backgroundConcurrent = oldMin, oldConc })
	a := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links")
	b := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links")
	if a.code != 0 || b.code != 0 {
		t.Fatal("two in flight are fine")
	}
	c := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links")
	if c.code != http.StatusTooManyRequests || c.retryAfter != time.Second {
		t.Fatalf("a third at the same time: %+v", c)
	}
	a.release()
	a.release() // twice must not free a second slot
	d := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links")
	if d.code != 0 {
		t.Fatalf("one slot is free: %+v", d)
	}
	if again := e.call(svcReq("tok-namako"), e.acme, e.listLinks, "list-links"); again.code != http.StatusTooManyRequests {
		t.Errorf("a double release freed two slots: %+v", again)
	}
	b.release()
	d.release()
	if len(backgroundLimiter.active) != 0 {
		t.Errorf("nothing in flight, nothing remembered: %v", backgroundLimiter.active)
	}
}

func TestInternalHeadersAreScrubbedInEverySpellingAndTheActingOrganisationIsReturned(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set(organisation.Header, "a")
	r.Header["x-cinqo-org"] = []string{"b"}
	r.Header.Set(organisation.CallerToolHeader, "c")
	r.Header["X-CINQO-CALLER-TOOL"] = []string{"d"}
	r.Header.Set(organisation.ActingOrgHeader, "  acme ")
	r.Header.Set("X-Other", "kept")
	if got := scrubInternalHeaders(r); got != "acme" {
		t.Errorf("%q", got)
	}
	for k := range r.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-cinqo-") {
			t.Errorf("%s survived", k)
		}
	}
	if r.Header.Get("X-Other") != "kept" {
		t.Error("other headers stay")
	}
	if scrubInternalHeaders(httptest.NewRequest("GET", "/x", nil)) != "" {
		t.Error("no header, no acting organisation")
	}
	if !isServiceCall(svcReq("t")) || isServiceCall(httptest.NewRequest("GET", "/x", nil)) {
		t.Error("only the ToolService scheme announces a tool")
	}
	bearer := httptest.NewRequest("GET", "/x", nil)
	bearer.Header.Set("Authorization", "Bearer abc")
	if isServiceCall(bearer) {
		t.Error("a person's bearer token is not a tool")
	}
	for _, v := range []string{"Bearer ToolService x", "Bearer Tool-abc", "toolservice x", "ToolService", " ToolService x"} {
		r := httptest.NewRequest("GET", "/x", nil)
		r.Header.Set("Authorization", v)
		if isServiceCall(r) {
			t.Errorf("%q must not announce a tool", v)
		}
	}
}

// Through the real BackgroundProxyHandler: every refusal answers before the target's port is even looked up, and a call that passes every check is the one that reaches
// "tool is not currently running" (no tool runs in the test). The limiter slot of that call is given back.
func TestThroughTheBackgroundProxyTheRefusalsHappenFirstAndAnAllowedCallReachesThePort(t *testing.T) {
	e := bgSetup(t)
	call := func(path, token, acting string) (int, string, http.Header) {
		r := httptest.NewRequest("POST", "/api/v1/tools/browser/background-proxy/"+path, nil)
		if token != "" {
			r.Header.Set("Authorization", serviceAuthScheme+token)
		}
		if acting != "" {
			r.Header.Set(organisation.ActingOrgHeader, acting)
		}
		rec := httptest.NewRecorder()
		ctx := request.NewContext(rec, r, e.di)
		ctx.GetURI().SetPathVariable("slug", "browser")
		ctx.GetURI().SetPathRemainder(path)
		BackgroundProxyHandler(ctx)
		return rec.Code, rec.Body.String(), rec.Header()
	}
	if code, _, _ := call("list-links", "", e.acme); code != http.StatusUnauthorized {
		t.Errorf("a person's acting-organisation header alone gets nowhere: %d", code)
	}
	if code, _, _ := call("list-links", "tok-nobody", e.acme); code != http.StatusUnauthorized {
		t.Errorf("%d", code)
	}
	if code, body, _ := call("login-window", "tok-namako", e.acme); code != http.StatusForbidden || !strings.Contains(body, "signed-in person") {
		t.Errorf("%d %s", code, body)
	}
	if code, _, _ := call("list-links", "tok-career", e.acme); code != http.StatusForbidden {
		t.Errorf("%d", code)
	}
	if code, _, _ := call("list-links", "tok-namako", e.off); code != http.StatusForbidden {
		t.Errorf("%d", code)
	}
	code, body, _ := call("list-links", "tok-namako", e.acme)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "not currently running") {
		t.Errorf("a call that passed every check reaches the port lookup: %d %s", code, body)
	}
	if len(backgroundLimiter.active) != 0 {
		t.Errorf("the slot of a finished call is given back: %v", backgroundLimiter.active)
	}
	// the limit answer carries Retry-After
	oldMin := backgroundPerMinute
	backgroundPerMinute = 1
	t.Cleanup(func() { backgroundPerMinute = oldMin })
	code, _, h := call("list-links", "tok-namako", e.acme)
	if code != http.StatusTooManyRequests || h.Get("Retry-After") == "" {
		t.Errorf("%d %v", code, h)
	}
	if n, _ := strconv_atoi(h.Get("Retry-After")); n < 1 || n > 61 {
		t.Errorf("Retry-After %q", h.Get("Retry-After"))
	}
}

func strconv_atoi(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, os.ErrInvalid
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func TestAPersonsRouteNeverTakesAServiceTokenAndTheUnattendedRouteNeverTakesAPerson(t *testing.T) {
	e := bgSetup(t)
	// a person's route: the service token is refused outright, even a valid one for an approved route
	r := httptest.NewRequest("POST", "/api/v1/tools/browser/proxy/list-links", nil)
	r.Header.Set("Authorization", serviceAuthScheme+"tok-namako")
	r.Header.Set(organisation.ActingOrgHeader, e.acme)
	rec := httptest.NewRecorder()
	ctx := request.NewContext(rec, r, e.di)
	ctx.GetURI().SetPathVariable("slug", "browser")
	ctx.GetURI().SetPathRemainder("list-links")
	ProxyHandler(ctx)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "not accepted on this route") {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if calls, denied := e.usage(t, "tn"); calls != 0 || denied != 0 {
		t.Errorf("it never reached the service checks: %d %d", calls, denied)
	}
	// the unattended route: a person's bearer token is not a service token
	r = httptest.NewRequest("POST", "/api/v1/tools/browser/background-proxy/list-links", nil)
	r.Header.Set("Authorization", "Bearer a-persons-jwt")
	rec = httptest.NewRecorder()
	ctx = request.NewContext(rec, r, e.di)
	ctx.GetURI().SetPathVariable("slug", "browser")
	ctx.GetURI().SetPathRemainder("list-links")
	BackgroundProxyHandler(ctx)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("%d", rec.Code)
	}
}

func TestTheProxyHandlerKeepsTheTwoBranchesApartAndNeverForwardsAnUncheckedRequest(t *testing.T) {
	raw, err := os.ReadFile("proxy_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	scrub, find, svc, ident, gate, fwd := strings.Index(src, "scrubInternalHeaders(r)"), strings.Index(src, "FindBySlug(slug)"), strings.Index(src, "if service {"),
		strings.Index(src, "callerIdentity(reqCtx)"), strings.Index(src, "organisationGate(db, r, sub)"), strings.Index(src, "proxy.ServeHTTP(w, r)")
	if !(scrub >= 0 && scrub < find && find < svc && svc < ident && ident < gate && gate < fwd) {
		t.Errorf("order: scrub %d, find %d, service %d, identity %d, gate %d, forward %d", scrub, find, svc, ident, gate, fwd)
	}
	if !strings.Contains(src, "defer res.release()") || !strings.Contains(src, `w.Header().Set("Retry-After"`) {
		t.Error("the slot must be given back and the wait told")
	}
	if strings.Count(src, "proxy.ServeHTTP(") != 1 {
		t.Error("one place forwards, behind both branches")
	}
	svcBranch := src[svc:strings.Index(src, "} else {")]
	if strings.Contains(svcBranch, "organisationGate") || strings.Contains(svcBranch, "callerIdentity") {
		t.Error("the service branch never trusts a person's identity")
	}
	if !strings.Contains(src, "func ProxyHandler(reqCtx request.RequestContext) { serveProxy(reqCtx, false) }") ||
		!strings.Contains(src, "func BackgroundProxyHandler(reqCtx request.RequestContext) { serveProxy(reqCtx, true) }") {
		t.Error("the person's route must be the non-service one and the unattended route the service one")
	}
}

// The router's security layer decides before any handler runs: the unattended route must be public (the handler does the whole check), and the person's route
// must stay authenticated. Declared with every method, with the executors registered. Without this a service token would be refused with 401 by the layer, which
// no handler test can see.
func TestTheRouterDeclaresTheUnattendedRoutePublicAndThePersonsRouteAuthenticated(t *testing.T) {
	yaml, err := os.ReadFile("../../../config/routes/route-tool.yaml")
	if err != nil {
		t.Fatal(err)
	}
	routesGo, _ := os.ReadFile("../../../config/routes/routes.go")
	blocks := strings.Split(string(yaml), "\n  - path: ")
	count := map[string]map[string]string{"/api/v1/tools/{slug}/proxy/**": {}, "/api/v1/tools/{slug}/background-proxy/**": {}}
	for _, b := range blocks {
		path, rest, _ := strings.Cut(b, "\n")
		path = strings.TrimPrefix(strings.TrimSpace(path), "- path: ")
		if _, ok := count[path]; !ok {
			continue
		}
		method, security, executor := "", "", ""
		for _, line := range strings.Split(rest, "\n") {
			line = strings.TrimSpace(line)
			if v, ok := strings.CutPrefix(line, "method: "); ok {
				method = v
			}
			if v, ok := strings.CutPrefix(line, "security: "); ok {
				security = v
			}
			if v, ok := strings.CutPrefix(line, "executor: "); ok {
				executor = v
			}
		}
		count[path][method] = security + "/" + executor
	}
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		if got := count["/api/v1/tools/{slug}/proxy/**"][m]; got != "authenticated/ToolProxy" {
			t.Errorf("person's route %s: %q", m, got)
		}
		if got := count["/api/v1/tools/{slug}/background-proxy/**"][m]; got != "public/ToolBackgroundProxy" {
			t.Errorf("unattended route %s: %q", m, got)
		}
	}
	if !strings.Contains(string(routesGo), `"ToolBackgroundProxy":`) || !strings.Contains(string(routesGo), "tool_handler.BackgroundProxyHandler") {
		t.Error("the executor is not registered")
	}
}

type fakeValidator struct{}

func (fakeValidator) Validate(token string) (string, []string, time.Time, error) {
	if token == "good-persons-jwt" {
		return "user-1", []string{"cinqo:super:admin"}, time.Now().Add(time.Hour), nil
	}
	return "", nil, time.Time{}, os.ErrInvalid
}

// The real security layer of the router, fed with the real route file: a request with a tool's service token must get past it on the unattended route (public) and
// be refused on the person's route (authenticated, bearer only), and a person's valid token must pass the person's route. The handler tests cannot see this layer.
func TestTheRealRouterLayerLetsAServiceTokenThroughOnlyOnTheUnattendedRoute(t *testing.T) {
	raw, err := os.ReadFile("../../../config/routes/route-tool.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Routes []struct {
			Path     string   `yaml:"path"`
			Method   string   `yaml:"method"`
			Security string   `yaml:"security"`
			Scopes   []string `yaml:"scopes"`
		} `yaml:"routes"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	layer := &lift_security.ScopeSecurityLayer{Validator: fakeValidator{}}
	authorize := func(path, method, authHeader string) int {
		for _, rt := range file.Routes {
			if rt.Path != path || rt.Method != method {
				continue
			}
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(method, "/x", nil)
			if authHeader != "" {
				r.Header.Set("Authorization", authHeader)
			}
			err := layer.Authorize(rec, r, nil, &server_security.Route{Path: rt.Path, Method: rt.Method, Security: rt.Security, Scopes: rt.Scopes})
			if err != nil {
				return rec.Code
			}
			return 0 // let through to the handler
		}
		t.Fatalf("%s %s is not declared", method, path)
		return -1
	}
	svc := serviceAuthScheme + "tok-namako"
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		if code := authorize("/api/v1/tools/{slug}/background-proxy/**", m, svc); code != 0 {
			t.Errorf("%s: the unattended route must let a service token through to its handler, got %d", m, code)
		}
		if code := authorize("/api/v1/tools/{slug}/proxy/**", m, svc); code != http.StatusUnauthorized {
			t.Errorf("%s: the person's route must refuse a service token, got %d", m, code)
		}
		if code := authorize("/api/v1/tools/{slug}/proxy/**", m, ""); code != http.StatusUnauthorized {
			t.Errorf("%s: no credentials at all: %d", m, code)
		}
		if code := authorize("/api/v1/tools/{slug}/proxy/**", m, "Bearer good-persons-jwt"); code != 0 {
			t.Errorf("%s: a person's valid token still works: %d", m, code)
		}
		if code := authorize("/api/v1/tools/{slug}/proxy/**", m, "Bearer forged"); code != http.StatusUnauthorized {
			t.Errorf("%s: %d", m, code)
		}
	}
}
