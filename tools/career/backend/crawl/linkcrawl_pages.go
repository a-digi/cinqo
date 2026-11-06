// linkcrawl_pages.go (plan 185, step 5) — what the "instruct the link crawler"
// screen needs from the backend:
//
//	POST /portal-links/link-crawl/candidates  load the links of the link's page (Browser /fetch-links, Katana)
//	POST /portal-links/link-crawl/learn       the picked links -> a proposed partial URL (pure)
//	POST /portal-links/link-crawl/preview     what a rule would find among given links (pure)
//	POST /portal-links/link-crawl/learn-pagination  the picked next-page link(s) -> how the pages follow each other (pure)
//	POST /portal-links/link-crawl/test        load the page (and, with pagination, the next one) now and run a rule on it (nothing is saved)
//
// Anything that LOADS a page (candidates, test) is enforced here, with the
// person's own session: Katana must be the selected engine, the link must not
// need a login, and the page must be on the link's own site (Career never
// asks the Browser for an arbitrary address). learn and preview only compute
// on data the screen sends; they fetch and store nothing.
package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"career-tool-backend/portal"
	"career-tool-backend/portal/linkrule"
)

// browserFetchLinksPath is the Browser tool's link-list route behind core's proxy.
const browserFetchLinksPath = "/api/v1/tools/browser/proxy/fetch-links"

// Bounds of what the screen may send back.
const (
	maxLearnPicks      = 200
	maxLearnCandidates = 2000
	maxLinkCrawlBody   = 4 << 20
)

// pageLinks is the Browser's /fetch-links answer.
type pageLinks struct {
	FinalURL       string          `json:"finalUrl"`
	Title          string          `json:"title"`
	Links          []linkrule.Link `json:"links"`
	Truncated      bool            `json:"truncated"`
	DroppedPrivate int             `json:"droppedPrivate"`
}

// linkFetchError is a verdict or refusal of the Browser tool (a check page, a rate limit, a refused redirect, a bad address).
type linkFetchError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Advice  string `json:"advice,omitempty"`
}

func (e *linkFetchError) Error() string { return e.Message }

// fetchPageLinks loads a page's links through core's proxy to the Browser. A var so tests can script the Browser.
var fetchPageLinks = fetchPageLinksViaCore

func fetchPageLinksViaCore(ctx context.Context, coreURL, accessToken, pageURL, portalID, requestID string) (pageLinks, error) {
	fields := map[string]string{"url": pageURL, "rateLimitKey": portalID}
	if id := keptLoginFrom(ctx); id != "" {
		fields["loginSessionId"] = id // plan 195: the kept login of a link that needs a login
	}
	if requestID != "" {
		fields["requestId"] = requestID // a crawl run: its phases are readable and crawl-cancel reaches the page load
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return pageLinks{}, err
	}
	respBody, status, err := postWithBearer(ctx, coreURL+browserFetchLinksPath, body, accessToken)
	if err != nil {
		return pageLinks{}, err
	}
	switch status {
	case http.StatusOK:
		var out pageLinks
		if err := json.Unmarshal(respBody, &out); err != nil {
			return pageLinks{}, fmt.Errorf("the Browser tool's link list could not be read: %w", err)
		}
		return out, nil
	case http.StatusUnauthorized:
		return pageLinks{}, errors.New("session expired — please sign in again")
	case http.StatusConflict:
		var e struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(respBody, &e) == nil && e.Code == "engine_not_katana" {
			return pageLinks{}, errKatanaNotSelected
		}
	case http.StatusBadGateway:
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Reason  string `json:"reason"`
			Advice  string `json:"advice"`
		}
		if json.Unmarshal(respBody, &e) == nil && e.Code != "" {
			msg := e.Message
			if e.Reason != "" {
				msg += " (" + e.Reason + ")"
			}
			return pageLinks{}, &linkFetchError{Code: e.Code, Message: msg, Advice: e.Advice}
		}
	case http.StatusBadRequest:
		return pageLinks{}, &linkFetchError{Code: "bad_request", Message: strings.TrimSpace(string(respBody))}
	}
	return pageLinks{}, fmt.Errorf("the Browser tool answered %d: %s", status, truncateText(strings.TrimSpace(string(respBody)), 200))
}

func truncateText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// linkCrawlRequest decodes a POST body strictly into v, reads the session and the core address, and answers the error itself.
func decodeLinkCrawlBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLinkCrawlBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// loadingContext is what a handler that LOADS a page needs, checked in one place.
type loadingContext struct {
	coreURL, token, linkURL, portalID string
	// sessionID is the kept login of a link that needs a login ("" for any other link): its pages are loaded with it (plan 195, step 4).
	sessionID string
	linkID    string
}

// checkLoading enforces, for a request that loads a page: a session, the core address, a known link, no login link, and
// Katana selected. It answers the error itself and returns false.
func checkLoading(w http.ResponseWriter, r *http.Request, portalLinkID string) (loadingContext, bool) {
	var lc loadingContext
	portalLinkID = strings.TrimSpace(portalLinkID)
	if portalLinkID == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return lc, false
	}
	cookie, err := r.Cookie("access_token")
	if err != nil {
		http.Error(w, "no active session cookie found", http.StatusUnauthorized)
		return lc, false
	}
	lc.token = cookie.Value
	if lc.coreURL, err = coreAPIURL(); err != nil {
		http.Error(w, "core API address is not configured", http.StatusInternalServerError)
		return lc, false
	}
	if lc.linkURL, err = portal.GetPortalLinkURL(portalLinkID); err != nil {
		portal.WritePortalLinkAwareError(w, "read the link", err)
		return lc, false
	}
	requiresLogin, _, err := portal.GetPortalLinkLogin(portalLinkID)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "read the link", err)
		return lc, false
	}
	lc.linkID = portalLinkID
	if requiresLogin {
		// A login link may use the link crawler only with a login kept in the Browser tool (plan 195, step 4).
		if lc.sessionID = portal.KeptLoginFor(portalLinkID); lc.sessionID == "" {
			http.Error(w, portal.ErrLinkRulesLogin.Error(), http.StatusBadRequest)
			return lc, false
		}
	}
	if err := requireKatanaSelected(r.Context(), lc.coreURL, lc.token); err != nil {
		code := codeEngineUnknown
		if errors.Is(err, errKatanaNotSelected) {
			code = codeKatanaNotSelected
		}
		writeLinkRulesJSON(w, http.StatusConflict, map[string]string{"code": code, "message": err.Error()})
		return lc, false
	}
	if lc.portalID, err = portal.GetPortalIDForLink(portalLinkID); err != nil {
		portal.WritePortalLinkAwareError(w, "read the link", err)
		return lc, false
	}
	return lc, true
}

// ownSitePage returns the page to load: the link's own address when none is given, else the given address if it is an http(s)
// address on the link's own site.
func ownSitePage(linkURL, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return linkURL, nil
	}
	u, err := url.Parse(requested)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", errors.New("the page must be an http or https address")
	}
	linkHost := ""
	if lu, err := url.Parse(linkURL); err == nil {
		linkHost = strings.ToLower(lu.Host)
	}
	if strings.ToLower(u.Host) != linkHost {
		return "", errors.New("the page must be on the link's own site")
	}
	return requested, nil
}

// writeFetchFailure answers a failed page load: 409 when the Browser says Katana is not selected, 502 with the Browser's verdict (code,
// message, advice) when the site stopped it, 502 otherwise.
func writeFetchFailure(w http.ResponseWriter, err error) {
	var fe *linkFetchError
	switch {
	case errors.Is(err, errKatanaNotSelected):
		writeLinkRulesJSON(w, http.StatusConflict, map[string]string{"code": codeKatanaNotSelected, "message": err.Error()})
	case errors.As(err, &fe):
		status := http.StatusBadGateway
		if fe.Code == "bad_request" {
			status = http.StatusBadRequest
		}
		writeLinkRulesJSON(w, status, fe)
	default:
		writeLinkRulesJSON(w, http.StatusBadGateway, &linkFetchError{Code: "fetch_failed", Message: err.Error()})
	}
}

// --- candidates ---

type candidatesBody struct {
	PortalLinkID string `json:"portalLinkId"`
	URL          string `json:"url,omitempty"`
}

type candidatesResponse struct {
	pageLinks
	// URL is the page that was loaded.
	URL string `json:"url"`
}

// LinkCrawlCandidatesHandler handles POST /portal-links/link-crawl/candidates.
func LinkCrawlCandidatesHandler(w http.ResponseWriter, r *http.Request) {
	var body candidatesBody
	if !decodeLinkCrawlBody(w, r, &body) {
		return
	}
	lc, ok := checkLoading(w, r, body.PortalLinkID)
	if !ok {
		return
	}
	page, err := ownSitePage(lc.linkURL, body.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := fetchPageLinks(withKeptLogin(r.Context(), lc.sessionID), lc.coreURL, lc.token, page, lc.portalID, "")
	if err != nil {
		noteLoginVerdict(lc.linkID, err)
		writeFetchFailure(w, err)
		return
	}
	writeLinkRulesJSON(w, http.StatusOK, candidatesResponse{pageLinks: res, URL: page})
}

// --- learn ---

type learnBody struct {
	PortalLinkID string          `json:"portalLinkId"`
	Picked       []string        `json:"picked"`
	NotJob       []string        `json:"notJob"`
	Candidates   []linkrule.Link `json:"candidates"`
}

type learnResponse struct {
	linkrule.Proposal
	// Rule is the proposal as a rule the screen can preview and save.
	Rule linkrule.Rule `json:"rule"`
}

func (b learnBody) tooBig() bool {
	return len(b.Picked) > maxLearnPicks || len(b.NotJob) > maxLearnCandidates || len(b.Candidates) > maxLearnCandidates
}

// LinkCrawlLearnHandler handles POST /portal-links/link-crawl/learn: pure, it loads and stores nothing.
func LinkCrawlLearnHandler(w http.ResponseWriter, r *http.Request) {
	var body learnBody
	if !decodeLinkCrawlBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.PortalLinkID) == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return
	}
	if body.tooBig() {
		http.Error(w, fmt.Sprintf("too many links (at most %d picked, %d others)", maxLearnPicks, maxLearnCandidates), http.StatusBadRequest)
		return
	}
	siteHost, err := portal.LinkSiteHost(body.PortalLinkID)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "read the link", err)
		return
	}
	prop, err := linkrule.LearnJobPattern(body.Picked, body.NotJob, body.Candidates)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if prop.Host != siteHost {
		http.Error(w, "the picked links must be on this link's own site ("+siteHost+")", http.StatusBadRequest)
		return
	}
	writeLinkRulesJSON(w, http.StatusOK, learnResponse{Proposal: prop, Rule: linkrule.Rule{Host: prop.Host, Include: []string{prop.Pattern}, Exclude: prop.Exclude}})
}

// --- preview ---

type previewBody struct {
	PortalLinkID string          `json:"portalLinkId"`
	Rules        linkrule.Rule   `json:"rules"`
	Candidates   []linkrule.Link `json:"candidates"`
}

// LinkCrawlPreviewHandler handles POST /portal-links/link-crawl/preview: what a rule finds among the given links. Pure.
func LinkCrawlPreviewHandler(w http.ResponseWriter, r *http.Request) {
	var body previewBody
	if !decodeLinkCrawlBody(w, r, &body) {
		return
	}
	if len(body.Candidates) > maxLearnCandidates {
		http.Error(w, fmt.Sprintf("too many links (at most %d)", maxLearnCandidates), http.StatusBadRequest)
		return
	}
	rule, err := portal.ValidateRuleForLink(strings.TrimSpace(body.PortalLinkID), body.Rules)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "check the rule", err)
		return
	}
	pv, err := linkrule.PreviewRule(rule, body.Candidates)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeLinkRulesJSON(w, http.StatusOK, pv)
}

// --- learn pagination ---

type learnPaginationBody struct {
	PortalLinkID string          `json:"portalLinkId"`
	CurrentURL   string          `json:"currentUrl,omitempty"`
	Picked       []string        `json:"picked"`
	Candidates   []linkrule.Link `json:"candidates"`
}

// LinkCrawlLearnPaginationHandler handles POST /portal-links/link-crawl/learn-pagination: from the first page's address and the picked
// next-page link(s) it works out how the pages follow each other. Pure: it loads and stores nothing.
func LinkCrawlLearnPaginationHandler(w http.ResponseWriter, r *http.Request) {
	var body learnPaginationBody
	if !decodeLinkCrawlBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.PortalLinkID) == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return
	}
	if len(body.Picked) > maxLearnPicks || len(body.Candidates) > maxLearnCandidates {
		http.Error(w, fmt.Sprintf("too many links (at most %d picked, %d others)", maxLearnPicks, maxLearnCandidates), http.StatusBadRequest)
		return
	}
	linkURL, err := portal.GetPortalLinkURL(strings.TrimSpace(body.PortalLinkID))
	if err != nil {
		portal.WritePortalLinkAwareError(w, "read the link", err)
		return
	}
	current, err := ownSitePage(linkURL, body.CurrentURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	prop, err := linkrule.LearnPagination(current, body.Picked, body.Candidates)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeLinkRulesJSON(w, http.StatusOK, prop)
}

// --- test ---

// testPagesLimit is the most pages a test run loads: enough to see that the pagination works, few enough to stay polite.
const testPagesLimit = 2

type testBody struct {
	PortalLinkID string        `json:"portalLinkId"`
	Rules        linkrule.Rule `json:"rules"`
	URL          string        `json:"url,omitempty"`
}

// testResponse is what a test run found over the pages it loaded (see linkrule.PagesResult); Notes carry the cautions.
type testResponse struct {
	URL string `json:"url"`
	linkrule.PagesResult
	Notes []string `json:"notes,omitempty"`
}

// LinkCrawlTestHandler handles POST /portal-links/link-crawl/test: it loads the page now (and, when the rule has pagination, the next
// one) and runs the rule on it. Nothing is saved. Every page it loads passes the same checks as the first: Katana, no login link,
// the link's own site.
func LinkCrawlTestHandler(w http.ResponseWriter, r *http.Request) {
	var body testBody
	if !decodeLinkCrawlBody(w, r, &body) {
		return
	}
	lc, ok := checkLoading(w, r, body.PortalLinkID)
	if !ok {
		return
	}
	rule, err := portal.ValidateRuleForLink(strings.TrimSpace(body.PortalLinkID), body.Rules)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "check the rule", err)
		return
	}
	first, err := ownSitePage(lc.linkURL, body.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fetch := func(ctx context.Context, pageURL string) (linkrule.Page, error) {
		if _, err := ownSitePage(lc.linkURL, pageURL); err != nil {
			return linkrule.Page{}, err
		}
		res, err := fetchPageLinks(withKeptLogin(ctx, lc.sessionID), lc.coreURL, lc.token, pageURL, lc.portalID, "")
		if err != nil {
			noteLoginVerdict(lc.linkID, err)
			return linkrule.Page{}, err
		}
		return linkrule.Page{FinalURL: res.FinalURL, Title: res.Title, Links: res.Links}, nil
	}
	res, err := linkrule.RunPages(r.Context(), rule, first, fetch, testPagesLimit)
	if err != nil {
		// the first page's own refusals (a bad address, another site) are the caller's mistake; anything else is the page failing
		if errors.Is(err, linkrule.ErrBadURL) || errors.Is(err, linkrule.ErrHostMismatch) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeFetchFailure(w, err)
		return
	}
	out := testResponse{URL: first, PagesResult: res}
	if res.SkippedNoTitle > 0 {
		out.Notes = append(out.Notes, fmt.Sprintf("%d matching link(s) have no usable text, so no job is created for them.", res.SkippedNoTitle))
	}
	writeLinkRulesJSON(w, http.StatusOK, out)
}
