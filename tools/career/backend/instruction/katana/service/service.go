// Package service is the Katana engine's internal service (plan 192, step 5): the loopback routes /ai/katana/* that the Katana AI tools
// (a short-lived MCP subprocess that cannot reach the Browser) call on Career's long-running process. It does what the screen does with
// the same checks and the same validated functions: Katana selected, no login link, the link's own site, the rule validated and stored by
// portal.SaveLinkCrawlSettings. It is not listed in manifest.json, so core never forwards it; it also needs the tool's own service token.
//
// This package may import the shared crawl and portal code, but no other engine.
package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"career-tool-backend/autodiscovery"
	"career-tool-backend/crawl"
	"career-tool-backend/portal"
	"career-tool-backend/portal/linkrule"
)

// Prefix is where the routes live.
const Prefix = "/ai/katana/"

const (
	maxBody       = 256 << 10
	scratchTTL    = 15 * time.Minute
	scratchMax    = 50
	maxPicks      = 40
	sampleItems   = 5
	reasonNoToken = "no_session"
)

// problem is a refusal the AI can act on: a code and a sentence.
type problem struct {
	Status  int
	Code    string
	Message string
}

func (p *problem) Error() string { return p.Message }

func refuse(status int, code, format string, args ...any) *problem {
	return &problem{Status: status, Code: code, Message: fmt.Sprintf(format, args...)}
}

// scratch is what one link's AI session remembers between calls, so a rule's pieces need not be retyped: the page's links (silently
// reloaded when lost), the last proposed rule and the rules whose test passed.
type scratch struct {
	page     string
	title    string
	links    []linkrule.Link
	trunc    bool
	proposed *linkrule.Rule
	// the job-detail session: the picks of the last job page (the default of a test and a save) and the instructions whose test passed
	detailSuggested *picks
	// a rule was saved in this session and not crawled since: the jobs of an older rule do not count
	needCrawl    bool
	detailTested map[string]bool
	passed       map[string]int // canonical rule JSON -> jobs found by its test
	at           time.Time
}

var (
	scratchMu sync.Mutex
	scratches = map[string]*scratch{}
)

func getScratch(linkID string) *scratch {
	scratchMu.Lock()
	defer scratchMu.Unlock()
	now := time.Now()
	for id, s := range scratches {
		if now.Sub(s.at) > scratchTTL {
			delete(scratches, id)
		}
	}
	s := scratches[linkID]
	if s == nil {
		if len(scratches) >= scratchMax {
			return &scratch{passed: map[string]int{}, detailTested: map[string]bool{}, at: now} // not remembered: a flood of links cannot grow the memory
		}
		s = &scratch{passed: map[string]int{}, detailTested: map[string]bool{}}
		scratches[linkID] = s
	}
	s.at = now
	return s
}

// Reset forgets every scratch (tests).
func Reset() {
	scratchMu.Lock()
	defer scratchMu.Unlock()
	scratches = map[string]*scratch{}
}

// session is what every call needs, checked in one place.
type session struct {
	linkID, linkURL, portalID, coreURL, token string
	// sessionID is the kept login of a link that needs a login ("" for any other link): every Browser call of this session carries it.
	sessionID string
}

// withLogin puts the session's kept login (if any) into the context of a Browser call.
func (s *session) withLogin(ctx context.Context) context.Context {
	return crawl.WithKeptLogin(ctx, s.linkID, s.sessionID)
}

func openSession(ctx context.Context, linkID string) (*session, *problem) {
	linkID = strings.TrimSpace(linkID)
	if linkID == "" {
		return nil, refuse(http.StatusBadRequest, "bad_request", "portalLinkId is required")
	}
	linkURL, err := portal.GetPortalLinkURL(linkID)
	if err != nil {
		return nil, refuse(http.StatusNotFound, "unknown_link", "this portal link does not exist")
	}
	login, _, err := portal.GetPortalLinkLogin(linkID)
	if err != nil {
		return nil, refuse(http.StatusNotFound, "unknown_link", "this portal link does not exist")
	}
	// A link that needs a login is read with the login the person kept in the Browser tool (plan 195, step 5). Without a usable one the AI
	// stops: it can neither log in nor open a login window.
	keptLogin := ""
	if login {
		if keptLogin = portal.KeptLoginFor(linkID); keptLogin == "" {
			if needs, state := portal.LoginSessionOf(linkID); needs && state == portal.LoginSessionExpired {
				return nil, refuse(http.StatusConflict, "login_expired", "the login kept for this link has expired: STOP, tell the person to log in again and keep the login (Crawl now or Generate opens the window); do not write a rule for it")
			}
			return nil, refuse(http.StatusConflict, "login_link", "this link needs a login and none is kept: STOP, tell the person to log in once and tick Keep this login; do not write a rule for it")
		}
	}
	token, fresh := autodiscovery.CachedToken()
	if !fresh {
		return nil, refuse(http.StatusConflict, reasonNoToken, "no signed-in session is known right now: ask the person to open a Career page (Platforms) and try again")
	}
	coreURL, err := crawl.CoreURL()
	if err != nil {
		return nil, refuse(http.StatusInternalServerError, "no_core", "core API address is not configured")
	}
	if code, msg := crawl.KatanaGate(ctx, coreURL, token, linkID); code != crawl.GateOK {
		return nil, refuse(http.StatusConflict, code, "%s", msg)
	}
	portalID, err := portal.GetPortalIDForLink(linkID)
	if err != nil {
		return nil, refuse(http.StatusNotFound, "unknown_link", "this portal link does not exist")
	}
	return &session{linkID: linkID, linkURL: linkURL, portalID: portalID, coreURL: coreURL, token: token, sessionID: keptLogin}, nil
}

// loadLinks loads a page's links (the Browser, with the person's token) and keeps them in the scratch.
func (s *session) loadLinks(ctx context.Context, sc *scratch, requested string) *problem {
	page, err := crawl.OwnSitePage(s.linkURL, requested)
	if err != nil {
		return refuse(http.StatusBadRequest, "bad_page", "%s", err.Error())
	}
	res, err := crawl.LoadPageLinks(s.withLogin(ctx), s.coreURL, s.token, page, s.portalID)
	crawl.NoteFetchOutcome(s.linkID, err) // plan 197: a block is recorded on the link, a read that went through forgets it
	if err != nil {
		return fetchProblem(err)
	}
	sc.page, sc.title, sc.links, sc.trunc = page, res.Title, res.Links, res.Truncated
	return nil
}

func fetchProblem(err error) *problem {
	var fe *crawl.PageFetchError
	if errors.As(err, &fe) {
		if fe.Code == "login_expired" {
			return refuse(http.StatusConflict, "login_expired", "the login kept for this link no longer works: STOP, tell the person to log in again and keep the login; do not guess a rule")
		}
		msg := fe.Message
		if fe.Advice != "" {
			msg += " (" + fe.Advice + ")"
		}
		if portal.IsEngineBlockCode(fe.Code) {
			// plan 197: the person can switch this link to Rod; the page offers it too.
			return refuse(http.StatusBadGateway, "page_"+fe.Code, "the site stopped the page load: %s. STOP and tell the person that Rod can read this link (use Rod for this link on the link's card); do not guess a rule", msg)
		}
		return refuse(http.StatusBadGateway, "page_"+fe.Code, "the site stopped the page load: %s. STOP and tell the person; do not guess a rule", msg)
	}
	return refuse(http.StatusBadGateway, "fetch_failed", "the page could not be loaded: %s", err.Error())
}

// ---- operations ----

type linkBody struct {
	PortalLinkID string `json:"portalLinkId"`
	URL          string `json:"url,omitempty"`
}

// PageLinks is the digest of the link's page (or of another page on its own site).
func PageLinks(ctx context.Context, b linkBody) (any, *problem) {
	s, p := openSession(ctx, b.PortalLinkID)
	if p != nil {
		return nil, p
	}
	sc := getScratch(s.linkID)
	if p := s.loadLinks(ctx, sc, b.URL); p != nil {
		return nil, p
	}
	sc.proposed = nil
	return BuildDigest(sc.page, sc.title, sc.links, sc.trunc), nil
}

type proposeBody struct {
	PortalLinkID string   `json:"portalLinkId"`
	JobURLs      []string `json:"jobUrls"`
	NotJobURLs   []string `json:"notJobUrls,omitempty"`
	NextPageURLs []string `json:"nextPageUrls,omitempty"`
}

type proposal struct {
	Rule          linkrule.Rule `json:"rule"`
	MatchesOnPage int           `json:"matchesOnPage"`
	SampleTitles  []string      `json:"sampleTitles,omitempty"`
	NeedsConfirm  bool          `json:"needsConfirm"`
	Notes         []string      `json:"notes,omitempty"`
	Pagination    string        `json:"pagination,omitempty"`
	Next          string        `json:"next"`
}

// expandPicks turns the addresses the AI copied from the digest back into the full addresses of the page. The digest drops the scheme and
// clips a long address with an ellipsis, so a pick is matched exactly first and, when it ends with the ellipsis, by its beginning (the first
// link of the page that starts with it and was not already taken by an earlier pick).
func expandPicks(picks []string, links []linkrule.Link) []string {
	bare := func(u string) string { return strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://") }
	taken := map[string]bool{}
	out := make([]string, 0, len(picks))
	for _, p := range picks {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		full := p
		if !strings.HasPrefix(p, "http://") && !strings.HasPrefix(p, "https://") {
			full = ""
			clipped := strings.HasSuffix(p, "…")
			prefix := strings.TrimSuffix(p, "…")
			for _, l := range links {
				u := bare(l.URL)
				if u == p || (clipped && strings.HasPrefix(u, prefix) && !taken[l.URL]) {
					full = l.URL
					break
				}
			}
			if full == "" {
				full = "https://" + prefix
			}
		}
		taken[full] = true
		out = append(out, full)
	}
	return out
}

// ProposeRule turns the AI's picked job addresses into a rule (pure, on the scratch's links; nothing is saved).
func ProposeRule(ctx context.Context, b proposeBody) (any, *problem) {
	if len(b.JobURLs) == 0 {
		return nil, refuse(http.StatusBadRequest, "bad_request", "jobUrls needs 2 to 5 addresses of job postings from the page links")
	}
	if len(b.JobURLs) > maxPicks || len(b.NotJobURLs) > maxPicks || len(b.NextPageURLs) > maxPicks {
		return nil, refuse(http.StatusBadRequest, "bad_request", "too many addresses (at most %d of each)", maxPicks)
	}
	s, p := openSession(ctx, b.PortalLinkID)
	if p != nil {
		return nil, p
	}
	sc := getScratch(s.linkID)
	if len(sc.links) == 0 {
		if p := s.loadLinks(ctx, sc, ""); p != nil {
			return nil, p
		}
	}
	siteHost, err := portal.LinkSiteHost(s.linkID)
	if err != nil {
		return nil, refuse(http.StatusNotFound, "unknown_link", "this portal link does not exist")
	}
	prop, err := linkrule.LearnJobPattern(expandPicks(b.JobURLs, sc.links), expandPicks(b.NotJobURLs, sc.links), sc.links)
	if err != nil {
		return nil, refuse(http.StatusBadRequest, "no_pattern", "%s", err.Error())
	}
	if prop.Host != siteHost {
		return nil, refuse(http.StatusBadRequest, "other_site", "the picked links must be on this link's own site (%s)", siteHost)
	}
	rule := linkrule.Rule{Host: prop.Host, Include: []string{prop.Pattern}, Exclude: prop.Exclude}
	out := proposal{MatchesOnPage: prop.MatchCount, NeedsConfirm: prop.NeedsConfirm, Notes: prop.Notes}
	if len(prop.Unmatched) > 0 {
		out.Notes = append(out.Notes, fmt.Sprintf("%d picked link(s) are not matched by the pattern", len(prop.Unmatched)))
	}
	if len(prop.ExcludedStillMatched) > 0 {
		out.Notes = append(out.Notes, fmt.Sprintf("%d not-a-job link(s) are still matched", len(prop.ExcludedStillMatched)))
	}
	if len(b.NextPageURLs) > 0 {
		pp, err := linkrule.LearnPagination(sc.page, expandPicks(b.NextPageURLs, sc.links), sc.links)
		if err != nil {
			out.Notes = append(out.Notes, "next page not understood: "+err.Error())
		} else {
			rule.Pagination = &pp.Pagination
			out.Pagination = pp.Pagination.Kind
			out.NeedsConfirm = out.NeedsConfirm || pp.NeedsConfirm
			out.Notes = append(out.Notes, pp.Notes...)
		}
	}
	valid, err := portal.ValidateRuleForLink(s.linkID, rule)
	if err != nil {
		return nil, refuse(http.StatusBadRequest, "bad_rule", "%s", err.Error())
	}
	if res, err := linkrule.Apply(valid, sc.links); err == nil {
		for i, it := range res.Items {
			if i >= sampleItems {
				break
			}
			out.SampleTitles = append(out.SampleTitles, clip(it.Title, maxSampleText))
		}
	}
	out.Rule = valid
	sc.proposed = &valid
	out.Next = "katana_test_link_rule (omit rule to test this one), then katana_save_link_rule"
	return out, nil
}

type ruleBody struct {
	PortalLinkID string         `json:"portalLinkId"`
	Rule         *linkrule.Rule `json:"rule,omitempty"`
}

func ruleKey(r linkrule.Rule) string {
	b, _ := json.Marshal(r)
	return string(b)
}

// which picks the rule to work on: the one given, else the last proposal.
func (sc *scratch) which(given *linkrule.Rule) (linkrule.Rule, *problem) {
	switch {
	case given != nil:
		return *given, nil
	case sc.proposed != nil:
		return *sc.proposed, nil
	}
	return linkrule.Rule{}, refuse(http.StatusBadRequest, "no_rule", "there is no rule yet: call katana_propose_link_rule first, or pass rule")
}

type testResult struct {
	Pages          []linkrule.PageResult `json:"pages"`
	Jobs           int                   `json:"jobs"`
	SampleJobs     []linkrule.Item       `json:"sampleJobs,omitempty"`
	SkippedNoTitle int                   `json:"skippedNoTitle,omitempty"`
	StopReason     string                `json:"stopReason"`
	StopDetail     string                `json:"stopDetail,omitempty"`
	Notes          []string              `json:"notes,omitempty"`
	Verdict        string                `json:"verdict"`
}

// TestRule runs the rule on up to two pages (nothing is saved).
func TestRule(ctx context.Context, b ruleBody) (any, *problem) {
	s, p := openSession(ctx, b.PortalLinkID)
	if p != nil {
		return nil, p
	}
	sc := getScratch(s.linkID)
	given, p := sc.which(b.Rule)
	if p != nil {
		return nil, p
	}
	rule, err := portal.ValidateRuleForLink(s.linkID, given)
	if err != nil {
		return nil, refuse(http.StatusBadRequest, "bad_rule", "%s", err.Error())
	}
	first, err := crawl.OwnSitePage(s.linkURL, "")
	if err != nil {
		return nil, refuse(http.StatusBadRequest, "bad_page", "%s", err.Error())
	}
	fetch := func(ctx context.Context, pageURL string) (linkrule.Page, error) {
		if _, err := crawl.OwnSitePage(s.linkURL, pageURL); err != nil {
			return linkrule.Page{}, err
		}
		res, err := crawl.LoadPageLinks(s.withLogin(ctx), s.coreURL, s.token, pageURL, s.portalID)
		crawl.NoteFetchOutcome(s.linkID, err)
		if err != nil {
			return linkrule.Page{}, err
		}
		return linkrule.Page{FinalURL: res.FinalURL, Title: res.Title, Links: res.Links}, nil
	}
	res, err := linkrule.RunPages(ctx, rule, first, fetch, crawl.TestPagesLimit)
	if err != nil {
		if errors.Is(err, linkrule.ErrBadURL) || errors.Is(err, linkrule.ErrHostMismatch) {
			return nil, refuse(http.StatusBadRequest, "bad_rule", "%s", err.Error())
		}
		return nil, fetchProblem(err)
	}
	out := testResult{Pages: res.Pages, Jobs: len(res.Items), SkippedNoTitle: res.SkippedNoTitle, StopReason: res.StopReason, StopDetail: res.StopDetail}
	for i, it := range res.Items {
		if i >= sampleItems {
			break
		}
		out.SampleJobs = append(out.SampleJobs, linkrule.Item{Title: clip(it.Title, maxSampleText), URL: shortURL(it.URL)})
	}
	if res.SkippedNoTitle > 0 {
		out.Notes = append(out.Notes, fmt.Sprintf("%d matching link(s) have no usable text, so no job is created for them", res.SkippedNoTitle))
	}
	if n, prefix := unmatchedLookalikes(rule, sc.links); n > 0 {
		out.Notes = append(out.Notes, fmt.Sprintf("%d other links of the first page start like the rule (%s) but are not matched: if they are job postings the rule is too narrow (a fixed value in it?). Propose again with addresses that differ more, from different postings.", n, prefix))
	}
	switch {
	case len(res.Items) == 0:
		out.Verdict = "no jobs found: the rule does not match the job links; fix the picks and propose again"
	case res.SkippedNoTitle > len(res.Items):
		out.Verdict = "most matching links have no text: the rule probably matches something else than job postings"
	default:
		out.Verdict = "ok: save it with katana_save_link_rule if the sample jobs are real postings"
		sc.passed[ruleKey(rule)] = len(res.Items)
	}
	return out, nil
}

// SaveRule stores a rule, only one whose test found jobs in this session (the AI saves after its own test passes).
func SaveRule(ctx context.Context, b ruleBody) (any, *problem) {
	s, p := openSession(ctx, b.PortalLinkID)
	if p != nil {
		return nil, p
	}
	sc := getScratch(s.linkID)
	given, p := sc.which(b.Rule)
	if p != nil {
		return nil, p
	}
	rule, err := portal.ValidateRuleForLink(s.linkID, given)
	if err != nil {
		return nil, refuse(http.StatusBadRequest, "bad_rule", "%s", err.Error())
	}
	if sc.passed[ruleKey(rule)] == 0 {
		return nil, refuse(http.StatusConflict, "not_tested", "this rule has not passed katana_test_link_rule in this session: test it first (it must find jobs)")
	}
	cur, err := portal.GetLinkCrawlSettings(s.linkID)
	if err != nil {
		return nil, refuse(http.StatusInternalServerError, "read_failed", "%s", err.Error())
	}
	if err := portal.SaveLinkCrawlSettings(s.linkID, cur.CrawlMethod, &rule, false); err != nil {
		return nil, refuse(http.StatusBadRequest, "save_failed", "%s", err.Error())
	}
	// The attempt of this AI run (opened by the screen when Generate was pressed) ends as a success because the rule was saved by it.
	if err := portal.MarkInstructionSaved(s.linkID, portal.InstructionKindListing); err != nil {
		log.Printf("katana: could not record the saved rule on the attempt of link %s: %v", s.linkID, err)
	}
	sc.needCrawl = true
	publishRuleSaved(s.linkID)
	return map[string]any{
		"saved": true, "jobsInTest": sc.passed[ruleKey(rule)], "host": rule.Host, "include": rule.Include,
		"next": "done: the job list is saved. Reply with one short line and make no more tool calls.",
	}, nil
}

// ---- HTTP ----

type opFunc func(ctx context.Context, body []byte) (any, *problem)

func decode[T any](fn func(context.Context, T) (any, *problem)) opFunc {
	return func(ctx context.Context, body []byte) (any, *problem) {
		var v T
		dec := json.NewDecoder(strings.NewReader(string(body)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&v); err != nil {
			return nil, refuse(http.StatusBadRequest, "bad_request", "invalid request: %s", err.Error())
		}
		return fn(ctx, v)
	}
}

var ops = map[string]opFunc{
	"page-links":   decode(PageLinks),
	"propose-rule": decode(ProposeRule),
	"test-rule":    decode(TestRule),
	"save-rule":    decode(SaveRule),
	"state":        decode(LinkState),
	"crawl-jobs":   decode(CrawlJobs),
	"job-page":     decode(JobPageCandidates),
	"test-picks":   decode(TestJobDetailPicks),
	"save-picks":   decode(SaveJobDetailPicks),
}

// authorized: the request comes from this machine and carries the tool's own service token.
func authorized(r *http.Request) bool {
	want := os.Getenv("TOOL_SERVICE_TOKEN")
	if want == "" {
		return false
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err != nil || !net.ParseIP(host).IsLoopback() {
		return false
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Handler serves POST /ai/katana/<operation>.
func Handler(w http.ResponseWriter, r *http.Request) {
	if !authorized(r) {
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	op, ok := ops[strings.TrimPrefix(r.URL.Path, Prefix)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	res, prob := op(ctx, body)
	if prob != nil {
		writeJSON(w, prob.Status, map[string]string{"code": prob.Code, "message": prob.Message})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// selfAttempts is how many 'running' attempts belong to the AI run that is asking: its own, which the app opened before the run started.
// A run of the AI that is calling the state tool is itself one running attempt, so only a second one means "another run".
func selfAttempts() int { return 1 }

// unmatchedLookalikes counts the links of the page that start like the rule's first pattern (its address up to the query, or up to the first
// wildcard cut back to a whole path part) but that the rule does not match: a sign of a pattern that holds a fixed value and is too narrow.
func unmatchedLookalikes(rule linkrule.Rule, links []linkrule.Link) (int, string) {
	if len(rule.Include) == 0 || len(links) == 0 {
		return 0, ""
	}
	prefix := rule.Include[0]
	if i := strings.Index(prefix, "?"); i >= 0 {
		prefix = prefix[:i]
	} else if i := strings.Index(prefix, "*"); i >= 0 {
		prefix = prefix[:i]
		if j := strings.LastIndex(prefix, "/"); j >= 0 {
			prefix = prefix[:j]
		}
	}
	if prefix == "" || !strings.Contains(prefix, "/") {
		return 0, ""
	}
	res, err := linkrule.Apply(rule, links)
	if err != nil {
		return 0, ""
	}
	matched := map[string]bool{}
	for _, it := range res.Items {
		matched[strings.TrimPrefix(strings.TrimPrefix(it.URL, "https://"), "http://")] = true
	}
	seen := map[string]bool{}
	n := 0
	for _, l := range links {
		u := strings.TrimPrefix(strings.TrimPrefix(l.URL, "https://"), "http://")
		if (u == prefix || strings.HasPrefix(u, prefix+"/") || strings.HasPrefix(u, prefix+"?")) && !matched[u] && !seen[u] {
			seen[u] = true
			n++
		}
	}
	return n, prefix
}
