// katana_links.go (plan 185, step 1) — POST /fetch-links: the links of ONE
// page, for Career's "link crawler" (jobs found by URL pattern, without AI).
//
// Only while Katana is the selected engine (409 engine_not_katana
// otherwise — the gate is enforced here, not only by the pages that hide
// the feature). The page is loaded by the Katana engine (runKatanaPage: our
// own Chrome, locked-down options, stealth, the same verdicts and the
// redirect-to-private refusal as fetch_page_html) and its links are read on
// the live tab in the same hook. Step 0 of plan 185 measured that Katana's
// own URL parser adds only resource URLs (css/js/images/feeds) and no link
// text, so the list is the page's anchors: <a href> (also inside OPEN shadow
// roots) with their text, plus <link rel="next|prev|canonical">.
//
// Read-only and one page: nothing in the list is requested, no login window,
// no AI context, no cookies of any login (the body takes only url, requestId
// and rateLimitKey; any other field is a 400). Never an MCP tool.
package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/go-rod/rod"
)

const (
	// maxPageLinks bounds the list one page returns (distinct URLs).
	maxPageLinks = 2000
	// maxLinkTextRunes / maxTextsPerLink bound what one link carries.
	maxLinkTextRunes = 200
	maxTextsPerLink  = 5
	// codeEngineNotKatana is the 409 of this route while another engine is selected.
	codeEngineNotKatana = "engine_not_katana"
)

// pageLink is one distinct URL of the page with everything the page says
// about it. A URL that is linked several times (a title link and a "More"
// link) is ONE entry with all its distinct texts, in page order, so the
// caller can choose the right title.
type pageLink struct {
	URL       string   `json:"url"`
	Texts     []string `json:"texts,omitempty"`
	TitleAttr string   `json:"titleAttr,omitempty"`
	Rel       string   `json:"rel,omitempty"`
	// Anchors (plan 247): the numbers of the <a> elements of the snapshot that carry this URL (data-cinqo-a), only when a snapshot was asked for.
	Anchors []int `json:"anchors,omitempty"`
}

// fetchLinksResponse is the body of a successful POST /fetch-links.
type fetchLinksResponse struct {
	FinalURL string     `json:"finalUrl"`
	Title    string     `json:"title"`
	Links    []pageLink `json:"links"`
	// Truncated: the page had more than maxPageLinks distinct links.
	Truncated bool `json:"truncated"`
	// DroppedPrivate counts links to loopback/private/link-local addresses
	// (literal IPs and localhost names), which are never offered.
	DroppedPrivate int `json:"droppedPrivate"`
	// Snapshot (plan 247): the sanitised copy of the page the picker shows, when asked for and made; SnapshotError says why not (the list is still good).
	Snapshot      *snapshotInfo `json:"snapshot,omitempty"`
	SnapshotError string        `json:"snapshotError,omitempty"`
}

// rawPageLink is what the page script reports for one element.
type rawPageLink struct {
	Href  string `json:"h"`
	Text  string `json:"t"`
	Title string `json:"ti"`
	Rel   string `json:"r"`
	Kind  string `json:"k"` // "a" or "link"
	// N is the number the page script gave an <a> (1, 2, ...) when a snapshot was asked for; 0 otherwise.
	N int `json:"n"`
}

// pageLinksJS collects the links of the live document: anchors (open shadow
// roots included) and <link rel=next|prev|canonical>. The href is resolved
// against the document's base URL by the browser itself.
const pageLinksJS = `(function () {
	var out = [], num = 0, number = __NUMBER__;
	function abs(v) { try { return new URL(v, document.baseURI).href; } catch (e) { return ""; } }
	function walk(root) {
		var anchors = root.querySelectorAll("a[href]");
		for (var i = 0; i < anchors.length; i++) {
			var a = anchors[i], h = a.getAttribute("href");
			if (h === null) continue;
			var n = 0;
			if (number) { n = ++num; a.setAttribute("data-cinqo-a", String(n)); }
			out.push({h: abs(h), t: ((a.innerText || a.textContent || "") + "").replace(/\s+/g, " ").trim().slice(0, 400),
				ti: (a.getAttribute("title") || a.getAttribute("aria-label") || "").trim().slice(0, 400), r: a.getAttribute("rel") || "", k: "a", n: n});
		}
		var all = root.querySelectorAll("*");
		for (var j = 0; j < all.length; j++) { if (all[j].shadowRoot) walk(all[j].shadowRoot); }
	}
	walk(document);
	var links = document.querySelectorAll("link[href][rel]");
	for (var k = 0; k < links.length; k++) {
		var rel = (links[k].getAttribute("rel") || "").toLowerCase();
		if (/(^|\s)(next|prev|canonical)(\s|$)/.test(rel)) out.push({h: abs(links[k].getAttribute("href")), t: "", ti: "", r: rel, k: "link"});
	}
	return JSON.stringify(out);
})()`

// pageLinksScript is pageLinksJS; with number true it also numbers the page's anchors (data-cinqo-a) for the snapshot.
func pageLinksScript(number bool) string {
	return strings.Replace(pageLinksJS, "__NUMBER__", fmt.Sprintf("%t", number), 1)
}

// normalizePageURL makes the URLs of one page comparable: http(s) only, no
// credentials, lower-case scheme and host, no fragment, a bare host gets
// "/". ok is false for anything else (javascript:, mailto:, tel:, data:, an
// empty or unparsable href, a URL with user info).
func normalizePageURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return "", false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment, u.RawFragment = "", ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), true
}

// isPrivateLinkHost reports whether a link's host is a literal private
// address or a local name. No DNS lookup (a page has up to thousands of
// links); a name that merely RESOLVES to a private address is caught when a
// page is actually fetched (validateCrawlURL), and a link is never fetched
// by this route.
func isPrivateLinkHost(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return isDisallowedIP(ip)
	}
	return false
}

func cleanLinkText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxLinkTextRunes {
		s = string([]rune(s)[:maxLinkTextRunes])
	}
	return s
}

// buildLinkList turns what the page reported into the response list:
// normalised, deduplicated by URL (first position wins, texts merged),
// links to private hosts dropped and counted (a link on the page's OWN host
// is kept: the page itself was allowed to load), capped at maxPageLinks
// distinct URLs.
func buildLinkList(raw []rawPageLink, pageHost string) (links []pageLink, droppedPrivate int, truncated bool) {
	pageHost = strings.ToLower(pageHost)
	index := map[string]int{}
	for _, r := range raw {
		norm, ok := normalizePageURL(r.Href)
		if !ok {
			continue
		}
		if u, err := url.Parse(norm); err != nil || (isPrivateLinkHost(u.Hostname()) && !strings.EqualFold(u.Hostname(), pageHost)) {
			droppedPrivate++
			continue
		}
		i, seen := index[norm]
		if !seen {
			if len(links) >= maxPageLinks {
				truncated = true
				continue
			}
			index[norm] = len(links)
			links = append(links, pageLink{URL: norm})
			i = len(links) - 1
		}
		l := &links[i]
		if r.N > 0 && r.Kind == "a" {
			l.Anchors = append(l.Anchors, r.N)
		}
		if t := cleanLinkText(r.Text); t != "" && len(l.Texts) < maxTextsPerLink && !containsString(l.Texts, t) {
			l.Texts = append(l.Texts, t)
		}
		if l.TitleAttr == "" {
			l.TitleAttr = cleanLinkText(r.Title)
		}
		if rel := strings.TrimSpace(strings.ToLower(r.Rel)); rel != "" {
			for _, token := range strings.Fields(rel) {
				if !containsToken(l.Rel, token) {
					l.Rel = strings.TrimSpace(l.Rel + " " + token)
				}
			}
		}
	}
	return links, droppedPrivate, truncated
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func containsToken(space string, token string) bool {
	return containsString(strings.Fields(space), token)
}

// fetchLinksWithKatana loads rawURL with the Katana engine and returns its
// links. A *crawlError is a verdict (Cloudflare, rate limit, redirect to a
// private address); any other error means the engine failed — there is no
// other engine to fall through to here.
func fetchLinksWithKatana(ctx context.Context, rawURL, requestID string) (fetchLinksResponse, error) {
	return runKatanaPage(ctx, rawURL, requestID, func(fctx context.Context, page *rod.Page, main *mainDocumentResponse) (fetchLinksResponse, error) {
		p := page.Context(fctx)
		if err := katanaVerdict(fctx, p, rawURL, requestID, main, nil); err != nil {
			return fetchLinksResponse{}, err
		}
		if err := waitForDOMToSettle(fctx, p); err != nil {
			return fetchLinksResponse{}, err
		}
		title, err := rodEvalString(p, `document.title`)
		if err != nil {
			return fetchLinksResponse{}, fmt.Errorf("katana: failed to read the title: %w", err)
		}
		finalURL, err := rodEvalString(p, `document.location.toString()`)
		if err != nil {
			return fetchLinksResponse{}, fmt.Errorf("katana: failed to read the final URL: %w", err)
		}
		if rerr := refuseRedirectToPrivate(rawURL, finalURL); rerr != nil {
			return fetchLinksResponse{}, rerr
		}
		snapshotWanted := wantsSnapshot(ctx)
		listJSON, err := rodEvalString(p, pageLinksScript(snapshotWanted))
		if err != nil {
			return fetchLinksResponse{}, fmt.Errorf("katana: failed to read the links: %w", err)
		}
		var raw []rawPageLink
		if err := json.Unmarshal([]byte(listJSON), &raw); err != nil {
			return fetchLinksResponse{}, fmt.Errorf("katana: links could not be decoded: %w", err)
		}
		pageHost := ""
		if fu, err := url.Parse(finalURL); err == nil {
			pageHost = fu.Hostname()
		}
		links, dropped, truncated := buildLinkList(raw, pageHost)
		resp := fetchLinksResponse{FinalURL: finalURL, Title: title, Links: links, Truncated: truncated, DroppedPrivate: dropped}
		if snapshotWanted {
			// Never fails the read: the list is good without the picture.
			if info, serr := snapshotLivePage(p, ctx, rawURL, finalURL, title); serr != nil {
				resp.SnapshotError = serr.Error()
			} else {
				resp.Snapshot = &info
			}
		}
		return resp, nil
	})
}

// fetchLinksFn is fetchLinksWithKatana behind a variable so tests can script
// a fetch without a browser.
var fetchLinksFn = fetchLinksWithKatana

// fetchLinksRequest is the body of POST /fetch-links. Unknown fields are
// refused (see the file comment: no login window, no AI context).
type fetchLinksRequest struct {
	URL       string `json:"url"`
	RequestID string `json:"requestId,omitempty"`
	// RateLimitKey (the portal id): the same per-portal pacing paginated crawls use.
	RateLimitKey string `json:"rateLimitKey,omitempty"`
	// LoginSessionID (plan 195): the kept login to fetch with; see login_session_fetch.go. Never set by an AI tool.
	LoginSessionID string `json:"loginSessionId,omitempty"`
	// Snapshot (plan 247): also keep a sanitised copy of the page for the picker to show.
	Snapshot bool `json:"snapshot,omitempty"`
	// Attended (plan 198): a person started this and is watching, so a check only they can clear may open the Katana window. Never set by an AI tool on its own.
	Attended bool `json:"attended,omitempty"`
}

func writeFetchLinksJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// FetchLinksHandler handles POST /fetch-links.
func FetchLinksHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body fetchLinksRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || strings.TrimSpace(body.URL) == "" {
		http.Error(w, "url is required (and only url, requestId, rateLimitKey, loginSessionId, snapshot and attended are accepted)", http.StatusBadRequest)
		return
	}
	if err := checkLoginSessionID(body.LoginSessionID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// The gate first: while another engine is selected nothing else is looked at.
	if selectedFetchEngine() != fetchEngineKatana {
		writeFetchLinksJSON(w, http.StatusConflict, map[string]string{
			"code":    codeEngineNotKatana,
			"message": "listing the links of a page needs the Katana engine; select Katana on the Browser & Engine page",
		})
		return
	}
	if err := validateCrawlURL(body.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Cancellable by requestId like every crawl (crawl-cancel), and ends when the caller disconnects.
	ctx, cancel := context.WithCancel(withSnapshot(withAttended(withLoginSession(r.Context(), body.LoginSessionID), body.Attended), body.Snapshot))
	registerCrawlCancel(body.RequestID, cancel)
	defer func() {
		unregisterCrawlCancel(body.RequestID)
		cancel()
	}()
	defer beginEngineFetch(fetchEngineKatana)() // keeps the engine's switch bookkeeping right
	awaitPortalCrawlPacing(ctx, body.RateLimitKey)

	result, err := fetchLinksFn(ctx, body.URL, body.RequestID)
	notePacingOutcome(body.RateLimitKey, err)
	withPacingRetry(body.RateLimitKey, err)
	if err != nil {
		setCrawlPhase(body.RequestID, phaseFailed, err.Error())
		var cfErr *crawlError
		if errors.As(err, &cfErr) {
			writeFetchLinksJSON(w, http.StatusBadGateway, cfErr)
			return
		}
		http.Error(w, fmt.Sprintf("fetch links failed: %v", err), http.StatusBadGateway)
		return
	}
	setCrawlPhase(body.RequestID, phaseCompleted, "links read")
	writeFetchLinksJSON(w, http.StatusOK, result)
}

// snapshotLivePage captures the live tab, makes the safe copy and stores it (plan 247).
func snapshotLivePage(p *rod.Page, ctx context.Context, rawURL, finalURL, title string) (snapshotInfo, error) {
	raw, err := rodEvalString(p, snapshotCaptureJS)
	if err != nil {
		return snapshotInfo{}, fmt.Errorf("the page could not be copied: %w", err)
	}
	capture, err := parseSnapshotCapture(raw)
	if err != nil {
		return snapshotInfo{}, err
	}
	htmlText, truncated, err := buildSnapshotHTML(capture, finalURL)
	if err != nil {
		return snapshotInfo{}, err
	}
	return writeSnapshot(htmlText, snapshotMeta{URL: rawURL, FinalURL: finalURL, Title: title, Login: loginSessionFrom(ctx) != "", Truncated: truncated})
}
