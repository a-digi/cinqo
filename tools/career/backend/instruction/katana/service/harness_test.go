package service_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/autodiscovery"
	"career-tool-backend/db"
	"career-tool-backend/instruction"
	"career-tool-backend/instruction/katana"
	"career-tool-backend/instruction/katana/service"
	"career-tool-backend/portal"
)

// The harness is the plan's "MCP stdio harness with a scripted Browser, no AI, no tokens": the real Katana MCP tools (through an MCP
// client), the real loopback service and the real validated functions, a fake core standing in for the Browser.

const (
	listURL  = "https://board.example/jobs"
	pageTwo  = "https://board.example/jobs?page=2"
	svcToken = "service-token"
)

type fakeCore struct {
	mu           sync.Mutex
	engine       string
	pages        map[string]string // page URL -> fetch-links answer body
	fetches      []string
	status       map[string]int // page URL -> forced status
	events       []string       // published domain events (raw bodies)
	detail       string         // the fetch-detail-candidates answer
	detailStatus int            // forced status of that answer (0: ok)
	extract      func(pageURL string) string
	// plan 195, step 5: the kept login each fetch-links call carried, and whether the site now refuses it.
	sessions     []string
	loginExpired bool
	// blockCode is the code a forced-status page answers with (default "challenge")
	blockCode string
	// the kept login the job-page analysis and the selector crawls carried
	detailSessions, extractSessions []string
}

func (f *fakeCore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case strings.HasSuffix(r.URL.Path, "/browser-settings"):
		fmt.Fprintf(w, `{"fetchEngine":%q}`, f.engine)
	case strings.HasSuffix(r.URL.Path, "/fetch-links"):
		var in struct {
			URL            string
			LoginSessionID string `json:"loginSessionId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.fetches = append(f.fetches, in.URL)
		f.sessions = append(f.sessions, in.LoginSessionID)
		if f.loginExpired {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, `{"code":"login_expired","message":"The kept login no longer works","reason":"the page shows a login form"}`)
			return
		}
		if st := f.status[in.URL]; st != 0 {
			w.WriteHeader(st)
			code := f.blockCode
			if code == "" {
				code = "challenge"
			}
			fmt.Fprintf(w, `{"code":%q,"message":"a check page stopped the load","advice":"try later"}`, code)
			return
		}
		body, ok := f.pages[in.URL]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "no such page")
			return
		}
		fmt.Fprint(w, body)
	case strings.HasSuffix(r.URL.Path, "/fetch-detail-candidates"):
		var din struct {
			LoginSessionID string `json:"loginSessionId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&din)
		f.detailSessions = append(f.detailSessions, din.LoginSessionID)
		if f.detailStatus != 0 {
			w.WriteHeader(f.detailStatus)
			fmt.Fprint(w, `{"code":"challenge","message":"a check page stopped the load","reason":"cf-mitigated: challenge","advice":"clear it once"}`)
			return
		}
		fmt.Fprint(w, f.detail)
	case strings.HasSuffix(r.URL.Path, "/crawl-paginated"):
		var in struct {
			URL            string
			LoginSessionID string `json:"loginSessionId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.extractSessions = append(f.extractSessions, in.LoginSessionID)
		fmt.Fprint(w, f.extract(in.URL))
	case strings.HasSuffix(r.URL.Path, "/capabilities"):
		http.NotFound(w, r)
	case strings.HasSuffix(r.URL.Path, "/events/publish"):
		var raw json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&raw)
		f.events = append(f.events, string(raw))
		w.WriteHeader(http.StatusAccepted)
	default:
		http.NotFound(w, r)
	}
}

func jobsPage(ids ...string) string {
	var parts []string
	parts = append(parts, `{"url":"https://board.example/about","texts":["About us"]}`)
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf(`{"url":"https://board.example/j/%s","texts":["Posting %s"]}`, id, strings.ToUpper(id)))
	}
	parts = append(parts, `{"url":"`+pageTwo+`","texts":["Next"]}`)
	return `{"finalUrl":"","title":"Board jobs","links":[` + strings.Join(parts, ",") + `]}`
}

type rig struct {
	t      *testing.T
	core   *fakeCore
	client *mcp.ClientSession
	linkID string
}

func jwt(exp time.Time) string {
	p, _ := json.Marshal(map[string]int64{"exp": exp.Unix()})
	return "h." + base64.RawURLEncoding.EncodeToString(p) + ".s"
}

func newRig(t *testing.T) *rig {
	t.Helper()
	service.Reset()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })

	core := &fakeCore{engine: "katana", pages: map[string]string{
		listURL: jobsPage("a1", "b2", "c3"),
		pageTwo: jobsPage("d4", "e5"),
	}, status: map[string]int{}}
	coreSrv := httptest.NewServer(core)
	t.Cleanup(coreSrv.Close)
	t.Setenv("CORE_API_URL", coreSrv.URL)
	autodiscovery.RecordSeenToken(jwt(time.Now().Add(time.Hour)))

	mux := http.NewServeMux()
	mux.HandleFunc(service.Prefix, service.Handler)
	svc := httptest.NewServer(mux)
	t.Cleanup(svc.Close)
	t.Setenv("TOOL_OWN_PORT", svc.URL[strings.LastIndex(svc.URL, ":")+1:])
	t.Setenv("TOOL_SERVICE_TOKEN", svcToken)

	pid, err := portal.CreatePortal("Board")
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := portal.AddPortalLink(pid, listURL, "Jobs")
	if err != nil {
		t.Fatal(err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "career", Version: "0"}, nil)
	katana.New().RegisterTools(server)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return &rig{t: t, core: core, client: cs, linkID: linkID}
}

// call returns the tool's text and whether it was an error.
func (r *rig) call(tool string, args map[string]any) (string, bool) {
	r.t.Helper()
	if _, ok := args["portalLinkId"]; !ok {
		args["portalLinkId"] = r.linkID
	}
	res, err := r.client.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		r.t.Fatalf("%s: %v", tool, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), res.IsError
}

func TestTheWholeJobListRecipe(t *testing.T) {
	r := newRig(t)

	if _, _, err := portal.StartInstructionAttempt(r.linkID, portal.InstructionKindListing, portal.InstructionTriggerManual, "conv"); err != nil {
		t.Fatal(err)
	}
	// 1. the digest: grouped, small, the job group first
	text, isErr := r.call("katana_get_page_links", map[string]any{})
	if isErr {
		t.Fatal(text)
	}
	var d struct {
		Links  int `json:"links"`
		Groups []struct {
			Shape   string `json:"shape"`
			Count   int    `json:"count"`
			Samples []struct{ URL string }
		} `json:"groups"`
		Next []string `json:"nextPageCandidates"`
	}
	if err := json.Unmarshal([]byte(text), &d); err != nil {
		t.Fatal(err, text)
	}
	if d.Links != 5 || d.Groups[0].Shape != "board.example/j/*" && d.Groups[0].Shape != "board.example/j/a1" && d.Groups[0].Count != 3 {
		t.Fatalf("digest: %s", text)
	}
	if len(d.Next) != 1 || len(text) > 1500 {
		t.Errorf("next %v, %d bytes", d.Next, len(text))
	}

	// the save is refused before any test
	if text, isErr := r.call("katana_save_link_rule", map[string]any{}); !isErr || !strings.Contains(text, "no rule yet") {
		t.Errorf("save without a proposal: %v %q", isErr, text)
	}

	// 2. propose from the digest's own (short) sample addresses, with the next page
	var picks []string
	for _, g := range d.Groups {
		if g.Shape != "board.example/about" && strings.HasPrefix(g.Shape, "board.example/j/") {
			for _, s := range g.Samples {
				picks = append(picks, s.URL)
			}
		}
	}
	if len(picks) < 2 {
		t.Fatalf("no job samples in %s", text)
	}
	text, isErr = r.call("katana_propose_link_rule", map[string]any{"jobUrls": picks, "nextPageUrls": d.Next})
	if isErr {
		t.Fatal(text)
	}
	var prop struct {
		Rule          map[string]any `json:"rule"`
		MatchesOnPage int            `json:"matchesOnPage"`
		Pagination    string         `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(text), &prop); err != nil || prop.MatchesOnPage != 3 || prop.Rule["host"] != "board.example" || prop.Pagination == "" {
		t.Fatalf("proposal: %v %s", err, text)
	}

	// still refused: tested nothing yet
	if text, isErr := r.call("katana_save_link_rule", map[string]any{}); !isErr || !strings.Contains(text, "not_tested") {
		t.Errorf("save before the test: %v %q", isErr, text)
	}
	if s, _ := portal.GetLinkCrawlSettings(r.linkID); s.Rules != nil {
		t.Fatal("a refused save wrote a rule")
	}
	r.core.mu.Lock()
	if len(r.core.events) != 0 {
		t.Errorf("a refused save published %v", r.core.events)
	}
	r.core.mu.Unlock()

	// 3. test: two pages at most, nothing saved
	text, isErr = r.call("katana_test_link_rule", map[string]any{})
	if isErr {
		t.Fatal(text)
	}
	var tr struct {
		Jobs    int    `json:"jobs"`
		Verdict string `json:"verdict"`
		Pages   []any  `json:"pages"`
	}
	if err := json.Unmarshal([]byte(text), &tr); err != nil || tr.Jobs != 5 || len(tr.Pages) != 2 || !strings.HasPrefix(tr.Verdict, "ok") {
		t.Fatalf("test: %v %s", err, text)
	}
	if s, _ := portal.GetLinkCrawlSettings(r.linkID); s.Rules != nil {
		t.Fatal("a test wrote a rule")
	}

	// 4. save: the same validated function as the screen
	text, isErr = r.call("katana_save_link_rule", map[string]any{})
	if isErr || !strings.Contains(text, `"saved":true`) {
		t.Fatalf("save: %v %s", isErr, text)
	}
	// the engine's own event announced the saved rule, and nothing else was published
	r.core.mu.Lock()
	events := append([]string(nil), r.core.events...)
	r.core.mu.Unlock()
	if len(events) != 1 || !strings.Contains(events[0], "career.portal_link.katana_rule_saved") || !strings.Contains(events[0], r.linkID) {
		t.Errorf("published events: %v", events)
	}
	// the attempt the screen opened for this run ends as a success
	if status, err := portal.FinishInstructionAttempt(r.linkID, portal.InstructionKindListing, ""); err != nil || status != portal.InstructionAttemptSucceeded {
		t.Errorf("the attempt did not record the save: %q %v", status, err)
	}
	s, err := portal.GetLinkCrawlSettings(r.linkID)
	if err != nil || s.Rules == nil || s.Rules.Host != "board.example" || s.Rules.Pagination == nil {
		t.Fatalf("stored: %+v %v", s, err)
	}
}

func TestARuleThatFindsNothingIsNeverSaved(t *testing.T) {
	r := newRig(t)
	r.call("katana_get_page_links", map[string]any{})
	rule := map[string]any{"host": "board.example", "include": []string{"board.example/nothing/*"}}
	text, _ := r.call("katana_test_link_rule", map[string]any{"rule": rule})
	if !strings.Contains(text, "no jobs found") {
		t.Fatalf("test: %s", text)
	}
	if text, isErr := r.call("katana_save_link_rule", map[string]any{"rule": rule}); !isErr || !strings.Contains(text, "not_tested") {
		t.Errorf("save: %v %q", isErr, text)
	}
}

func TestRefusals(t *testing.T) {
	r := newRig(t)

	r.core.engine = "rod"
	if text, isErr := r.call("katana_get_page_links", map[string]any{}); !isErr || !strings.Contains(text, "katana") {
		t.Errorf("Rod selected: %v %q", isErr, text)
	}
	r.core.engine = "katana"

	// a page of another site is never loaded
	if text, isErr := r.call("katana_get_page_links", map[string]any{"url": "https://evil.example/x"}); !isErr || !strings.Contains(text, "own site") {
		t.Errorf("other site: %v %q", isErr, text)
	}
	for _, f := range r.core.fetches {
		if strings.Contains(f, "evil") {
			t.Fatal("the Browser was asked for another site")
		}
	}

	// the site stops the page: a message that says STOP
	r.core.status[listURL] = http.StatusBadGateway
	if text, isErr := r.call("katana_get_page_links", map[string]any{}); !isErr || !strings.Contains(text, "STOP") {
		t.Errorf("blocked page: %v %q", isErr, text)
	}
	delete(r.core.status, listURL)

	// a login link is never instructed
	yes := true
	if err := portal.UpdatePortalLinkAutoDiscoveryEnabled(r.linkID, false); err != nil {
		t.Fatal(err)
	}
	if err := portal.UpdatePortalLinkLogin(r.linkID, &yes, nil); err != nil {
		t.Fatal(err)
	}
	if text, isErr := r.call("katana_get_page_links", map[string]any{}); !isErr || !strings.Contains(text, "login") {
		t.Errorf("login link: %v %q", isErr, text)
	}
	if text, isErr := r.call("katana_get_page_links", map[string]any{"portalLinkId": "nope"}); !isErr || !strings.Contains(text, "does not exist") {
		t.Errorf("unknown link: %v %q", isErr, text)
	}
}

func TestNoSessionAndNoService(t *testing.T) {
	r := newRig(t)
	autodiscovery.RecordSeenToken(jwt(time.Now().Add(-time.Hour)))
	if text, isErr := r.call("katana_get_page_links", map[string]any{}); !isErr || !strings.Contains(text, "open a Career page") {
		t.Errorf("expired session: %v %q", isErr, text)
	}
	t.Setenv("TOOL_SERVICE_TOKEN", "wrong")
	if text, isErr := r.call("katana_get_page_links", map[string]any{}); !isErr {
		t.Errorf("a wrong service token was accepted: %q", text)
	}
	t.Setenv("TOOL_OWN_PORT", "")
	if text, isErr := r.call("katana_get_page_links", map[string]any{}); !isErr || !strings.Contains(text, "not running") {
		t.Errorf("no port: %v %q", isErr, text)
	}
}

func TestTheServiceWantsTheServiceToken(t *testing.T) {
	newRig(t)
	for _, auth := range []string{"", "Bearer wrong"} {
		req := httptest.NewRequest(http.MethodPost, service.Prefix+"page-links", strings.NewReader(`{}`))
		req.RemoteAddr = "127.0.0.1:5555"
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		service.Handler(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("auth %q: %d", auth, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, service.Prefix+"page-links", strings.NewReader(`{}`))
	req.RemoteAddr = "10.0.0.5:5555" // not this machine
	req.Header.Set("Authorization", "Bearer "+svcToken)
	rec := httptest.NewRecorder()
	service.Handler(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a non-loopback caller: %d", rec.Code)
	}
}

func dbQuery(query, arg string, dest *string) error {
	return db.JobsDB.QueryRow(query, arg).Scan(dest)
}

// A real run (plan 192, step 10) sent the addresses as one string: the tool accepts it, in the shapes a model uses.
func TestAddressListsMayBeSentAsAString(t *testing.T) {
	for name, jobUrls := range map[string]any{
		"spaces":    "board.example/j/a1 board.example/j/b2",
		"commas":    "board.example/j/a1, board.example/j/b2",
		"json text": `["board.example/j/a1", "board.example/j/b2"]`,
		"array":     []string{"board.example/j/a1", "board.example/j/b2"},
	} {
		r := newRig(t)
		r.call("katana_get_page_links", map[string]any{})
		text, isErr := r.call("katana_propose_link_rule", map[string]any{"jobUrls": jobUrls})
		if isErr || !strings.Contains(text, `"matchesOnPage":3`) {
			t.Errorf("%s: %v %s", name, isErr, text)
		}
	}
}

func TestATooNarrowRuleIsSaidInTheTest(t *testing.T) {
	r := newRig(t)
	r.call("katana_get_page_links", map[string]any{})
	rule := map[string]any{"host": "board.example", "include": []string{"board.example/j/a*"}}
	text, isErr := r.call("katana_test_link_rule", map[string]any{"rule": rule})
	if isErr || !strings.Contains(text, "too narrow") || !strings.Contains(text, "2 other links") {
		t.Errorf("%v %s", isErr, text)
	}
	// the right rule says nothing of the kind
	good := map[string]any{"host": "board.example", "include": []string{"board.example/j/*"}}
	if text, _ := r.call("katana_test_link_rule", map[string]any{"rule": good}); strings.Contains(text, "too narrow") {
		t.Errorf("a complete rule was called narrow: %s", text)
	}
	// the save tells the AI it is finished
	if text, isErr := r.call("katana_save_link_rule", map[string]any{"rule": good}); isErr || !strings.Contains(text, "no more tool calls") {
		t.Errorf("save: %v %s", isErr, text)
	}
}

func dbQueryInt(query, arg string, dest *int) error {
	return db.JobsDB.QueryRow(query, arg).Scan(dest)
}

// The crawl the AI starts decides how a link is crawled by asking the registered engines, as the production binary does (engines.go).
func init() { instruction.Register(katana.New()) }

// plan 203: a page whose postings are on another host than the link's own cannot get a rule: the AI is told where they are, and stops.
func TestPostingsOnAnotherHostAreSaidAndNeverProposed(t *testing.T) {
	r := newRig(t)
	var parts []string
	parts = append(parts, `{"url":"https://board.example/about","texts":["About"]}`, `{"url":"https://board.example/jobs?page=2","texts":["Next"]}`)
	for _, id := range []string{"a1", "b2", "c3", "d4"} {
		parts = append(parts, fmt.Sprintf(`{"url":"https://jobs.other.example/j/%s","texts":["Posting %s"]}`, id, id))
	}
	r.core.mu.Lock()
	r.core.pages[listURL] = `{"finalUrl":"","title":"Board jobs","links":[` + strings.Join(parts, ",") + `]}`
	r.core.mu.Unlock()

	text, isErr := r.call("katana_get_page_links", map[string]any{})
	if isErr {
		t.Fatalf("%s", text)
	}
	for _, want := range []string{`"otherHosts"`, "jobs.other.example", `"site"`, "STOP", "another site than this link's own", "do not guess a rule"} {
		if !strings.Contains(text, want) {
			t.Errorf("the page answer lacks %q: %s", want, text)
		}
	}
	text, isErr = r.call("katana_propose_link_rule", map[string]any{"jobUrls": []string{"jobs.other.example/j/a1", "jobs.other.example/j/b2"}})
	if !isErr || !strings.Contains(text, "jobs.other.example") || !strings.Contains(text, "board.example") || !strings.Contains(text, "STOP") {
		t.Errorf("proposing links of another site: %v %s", isErr, text)
	}
}

func TestAPageWithItsPostingsOnTheOwnSiteHasNoNote(t *testing.T) {
	r := newRig(t)
	text, isErr := r.call("katana_get_page_links", map[string]any{})
	if isErr || strings.Contains(text, `"note"`) || strings.Contains(text, "another site than") {
		t.Errorf("%v %s", isErr, text)
	}
	if !strings.Contains(text, `"site"`) {
		t.Errorf("the own site is always named: %s", text)
	}
}

// plan 203, step 2: once a PERSON confirmed the job host on the link, the AI may use it; it can never confirm one itself.
func confirmedRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	var parts []string
	parts = append(parts, `{"url":"https://board.example/about","texts":["About"]}`, `{"url":"`+pageTwo+`","texts":["Next"]}`)
	for _, id := range []string{"a1", "b2", "c3", "d4"} {
		parts = append(parts, fmt.Sprintf(`{"url":"https://jobs.other.example/j/%s","texts":["Posting %s"]}`, id, id))
	}
	r.core.mu.Lock()
	r.core.pages[listURL] = `{"finalUrl":"","title":"Board jobs","links":[` + strings.Join(parts, ",") + `]}`
	r.core.pages[pageTwo] = `{"finalUrl":"","title":"Board jobs 2","links":[]}`
	r.core.mu.Unlock()
	return r
}

func TestTheAIUsesTheJobHostAPersonConfirmedAndNothingWiderIsPossible(t *testing.T) {
	r := confirmedRig(t)
	if err := portal.SetLinkJobHost(r.linkID, "jobs.other.example"); err != nil {
		t.Fatal(err)
	}
	// the page answer no longer calls the postings "another site", and marks the confirmed host
	text, isErr := r.call("katana_get_page_links", map[string]any{})
	if isErr || strings.Contains(text, `"note"`) || strings.Contains(text, "another site than") || !strings.Contains(text, `"allowed":true`) {
		t.Fatalf("page answer: %v %s", isErr, text)
	}
	// the AI may propose, test and save a rule for the confirmed host's links
	text, isErr = r.call("katana_propose_link_rule", map[string]any{"jobUrls": []string{"jobs.other.example/j/a1", "jobs.other.example/j/b2"}})
	if isErr {
		t.Fatalf("propose: %s", text)
	}
	var prop struct {
		Rule map[string]any `json:"rule"`
	}
	if err := json.Unmarshal([]byte(text), &prop); err != nil || prop.Rule["host"] != "board.example" || prop.Rule["jobHost"] != "jobs.other.example" {
		t.Fatalf("the proposed rule is for the link's host and names the job host: %v %s", err, text)
	}
	if text, isErr = r.call("katana_test_link_rule", map[string]any{}); isErr || !strings.Contains(text, `"jobs":4`) {
		t.Fatalf("test: %v %s", isErr, text)
	}
	if text, isErr = r.call("katana_save_link_rule", map[string]any{}); isErr || !strings.Contains(text, `"saved":true`) {
		t.Fatalf("save: %v %s", isErr, text)
	}
	if s, _ := portal.GetLinkCrawlSettings(r.linkID); s.Rules == nil || s.Rules.Host != "board.example" || s.Rules.JobHost != "jobs.other.example" {
		t.Errorf("stored: %+v", s.Rules)
	}
	// a third host is still refused, with the confirmed one named
	text, isErr = r.call("katana_propose_link_rule", map[string]any{"jobUrls": []string{"third.example/j/a1", "third.example/j/b2"}})
	if !isErr || !strings.Contains(text, "STOP") {
		t.Errorf("a third host: %v %s", isErr, text)
	}
}

func TestTheAICannotWidenTheSiteWithARuleThatNamesAJobHost(t *testing.T) {
	r := confirmedRig(t) // nothing confirmed on the link
	r.call("katana_get_page_links", map[string]any{})
	rule := map[string]any{"host": "board.example", "jobHost": "jobs.other.example", "include": []string{"jobs.other.example/j/*"}}
	for _, tool := range []string{"katana_test_link_rule", "katana_save_link_rule"} {
		text, isErr := r.call(tool, map[string]any{"rule": rule})
		if !isErr {
			t.Errorf("%s accepted a rule with an unconfirmed job host: %s", tool, text)
		}
	}
	if s, _ := portal.GetLinkCrawlSettings(r.linkID); s.Rules != nil {
		t.Fatalf("a rule with an unconfirmed job host was stored: %+v", s.Rules)
	}
	if h, _ := portal.LinkJobHost(r.linkID); h != "" {
		t.Errorf("the AI set a job host: %q", h)
	}
	// and no tool sets one: a job host appears in the tools' schemas only inside the rule that is passed back (never as an argument of its
	// own), whose description says it is never to be invented, and the server refused it above
	tools, err := r.client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range tools.Tools {
		if strings.Contains(strings.ToLower(tl.Name), "job_host") || strings.Contains(strings.ToLower(tl.Name), "jobhost") {
			t.Errorf("a tool named for the job host: %s", tl.Name)
		}
		raw, _ := json.Marshal(tl.InputSchema)
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		if _, top := schema.Properties["jobHost"]; top {
			t.Errorf("%s takes a jobHost argument of its own", tl.Name)
		}
		if strings.Contains(string(raw), "jobHost") {
			rule, ok := schema.Properties["rule"]
			if !ok || !strings.Contains(string(rule), "Never invent a jobHost") {
				t.Errorf("%s names jobHost outside a rule that warns against inventing one: %s", tl.Name, raw)
			}
		}
	}
}
