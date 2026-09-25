// crawl.go implements the HTML crawler feature — the AI navigates the
// shared browser session to a URL and gets back the page's rendered
// HTML. See plan/ai/tools/browser/step-03-html-crawler-feature.md.
package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"browser-tool-backend/shared"
)

// MaxHTMLBytes bounds how much of a crawled page's HTML is ever
// returned — an arbitrary page's rendered HTML can be megabytes, far
// more than worth spending an LLM's own context budget on in one tool
// result. A fixed, non-configurable cap for this first pass, matching
// this codebase's own established precedent (defaultMaxTokens in
// api/src/platform/anthropic) of plain constants over
// premature configurability.
const MaxHTMLBytes = 200_000

// crawlTimeout bounds one headless navigation end to end — raised from
// 20s so a detected challenge gets its full automatic wait
// (cloudflareAutoWait, 25s) plus navigate/settle/read overhead before
// escalating to the headed fallback. Only a page that actually shows a
// challenge ever uses the extra time; a normal page is unaffected.
const crawlTimeout = 30 * time.Second

// crawlReadReserve is how much of ctx's own deadline the automatic
// Cloudflare wait leaves for reading the page afterwards
// (readCrawlResponse).
const crawlReadReserve = 3 * time.Second

const SettleDelay = 1500 * time.Millisecond

// normalSessionCrawlTimeout bounds one headed-Chrome fallback attempt
// end to end — deliberately its OWN budget, not nested inside or
// derived from the primary attempt's own ctx (already bounded by
// crawlTimeout, and likely close to exhausted by the time a fallback
// is even considered, having just spent up to cloudflareAutoWait
// waiting inside the headless attempt). Raised again in step 27 to
// comfortably contain maxHumanSolveDuration (30 min) on top of
// process launch/navigate/settle/read overhead (~10s) — 31 minutes
// leaves margin. See
// plan/ai/tools/browser/step-27-long-lived-headed-fallback-session.md,
// which replaced step 25's original fixed-count (4*15s=60s) budget
// with this much longer wall-clock one specifically so the window
// never has to close and reopen mid-attempt — a real bug that fixed-
// count budget caused when paired with Career's own now-removed outer
// retry loop (plan/ai/tools/career/step-35-crawl-now-single-long-lived-attempt.md).
const normalSessionCrawlTimeout = 31 * time.Minute

// maxHumanSolveDuration bounds the total time waitForHumanToClearCloudflare
// spends waiting — a wall-clock cap, not a fixed retry count (step
// 25's original maxHumanSolveRetries=4, a 60s budget). Long enough
// that a human realistically never needs the SAME window to close and
// reopen mid-attempt — the window stays open the whole time. See
// plan/ai/tools/browser/step-27-long-lived-headed-fallback-session.md.
// An explicit starting estimate, not verified against a real person's
// own reaction time. var, not const, so a temporary test can shorten
// it.
var maxHumanSolveDuration = 30 * time.Minute

type crawlRequest struct {
	URL string `json:"url"`
	// RequestID (step 31) is optional — when set, this call's own
	// live phase (navigating/checking_cloudflare/awaiting_human_challenge/
	// completed/failed) is tracked under this id and readable via
	// GET /crawl-status?requestId=... while this call is still in
	// flight. Absent for every AI-driven call (shared.CallSibling never sets
	// it) — a no-op in that case, see crawl_status.go's own doc
	// comment. See plan/ai/tools/browser/step-31-crawl-phase-status-endpoint.md.
	RequestID string `json:"requestId,omitempty"`
	// ExpectedSelectors (step 36) is optional — CSS selectors the
	// caller already knows it wants to find on this page (Career's own
	// deterministic "Crawl now" derives these from the link's own
	// stored crawl instructions). When set, corroborates the Cloudflare
	// check both ways: overrides an otherwise-"detected" result the
	// moment this content is visibly present, and — inside the
	// human-wait loop only — treats an otherwise-"clear" result as not
	// yet trustworthy while this content still isn't found. Absent for
	// every AI-driven call, identical to today's behavior. See
	// plan/ai/tools/browser/step-36-content-based-cloudflare-override.md.
	ExpectedSelectors []string `json:"expectedSelectors,omitempty"`
	// RemoveSelectors (step 41) — optional CSS selectors for elements
	// to strip from the returned HTML before it's read (e.g. "header",
	// "script", "style", ".cookie-banner") — removes boilerplate/noise
	// the caller doesn't need, never touching what's actually returned
	// beyond that. Absent means "return the page exactly as rendered,"
	// today's existing behavior. See
	// plan/ai/tools/browser/step-41-remove-html-elements.md.
	RemoveSelectors []string `json:"removeSelectors,omitempty"`
	// RemoveAttributes (step 46) — optional attribute names to strip
	// from every element that has them, regardless of value (e.g.
	// "style", "onclick", "data-testid"). Absent means no
	// named-attribute stripping.
	RemoveAttributes []string `json:"removeAttributes,omitempty"`
	// MaxAttributeLength (step 46) — optional; when > 0, any attribute
	// (any name, on any element) whose own value is longer than this
	// many characters is removed. <= 0 (including absent) disables
	// this — there's no way to ask for "strip every non-empty
	// attribute" via 0 specifically; pass 1 for that instead. Checking
	// every element's every attribute like this can be noticeably
	// slower on a large page — a deliberate, caller-accepted trade-off
	// for the token savings it can produce, not something this tool
	// tries to make cheap.
	MaxAttributeLength int `json:"maxAttributeLength,omitempty"`
	// IgnoreAttributesForMaxLength (step 46) — optional attribute names
	// exempt from MaxAttributeLength's own length check, in addition to
	// the built-in default (href, src — a link/image/script/iframe's
	// own resource locator, where a long value is legitimate, not
	// noise). Has no effect at all when MaxAttributeLength isn't set.
	// See defaultMaxLengthIgnoredAttributes's own doc comment.
	IgnoreAttributesForMaxLength []string `json:"ignoreAttributesForMaxLength,omitempty"`
}

type crawlResponse struct {
	HTML      string `json:"html"`
	Title     string `json:"title"`
	FinalURL  string `json:"finalUrl"`
	Truncated bool   `json:"truncated"`
}

// CrawlHandler handles POST /crawl — the --mcp adapter's own real
// target for fetch_page_html, reached only via shared.CallSibling.
func CrawlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body crawlRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	if err := validateCrawlURL(body.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// r.Context() — ends when the caller disconnects (e.g. the agent
	// host abandoning a timed-out tool call), which stops this crawl and
	// frees its tab instead of leaving it working for nobody.
	result, err := crawlPage(r.Context(), body.URL, body.RequestID, body.ExpectedSelectors, body.RemoveSelectors, body.RemoveAttributes, body.MaxAttributeLength, body.IgnoreAttributesForMaxLength)
	if err != nil {
		setCrawlPhase(body.RequestID, phaseFailed, err.Error())
		var cfErr *crawlError
		if errors.As(err, &cfErr) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(cfErr)
			return
		}
		http.Error(w, fmt.Sprintf("crawl failed: %v", err), http.StatusBadGateway)
		return
	}

	setCrawlPhase(body.RequestID, phaseCompleted, "HTML crawled")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// readCrawlResponse reads back the currently loaded page's rendered
// HTML/title/final URL — assumes the caller has already confirmed (or
// decided not to check) that no Cloudflare challenge is blocking it.
// Extracted so both navigateAndReadWithCloudflareCheck's own automated
// path and waitForHumanToSolveCloudflare's own retry loop (step 25)
// can read the page once cleared, without either needing to
// re-navigate — which would discard whatever a human just did in a
// visible window to clear the challenge.
func readCrawlResponse(ctx context.Context, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	allRemoveSelectors := append(append([]string{}, DefaultRemoveSelectors...), removeSelectors...)
	script, err := RemoveElementsJS(allRemoveSelectors)
	if err != nil {
		return crawlResponse{}, err
	}
	// The title is read BEFORE the removal script runs: <title> lives in
	// <head>, which DefaultRemoveSelectors strips — reading it afterwards
	// (as this function used to) returned "" on every crawl.
	var html, title, finalURL string
	actions := []chromedp.Action{
		chromedp.Title(&title),
		chromedp.Evaluate(script, nil),
	}

	// step 46 — run after element removal (above), never before: an
	// element RemoveElementsJS already deleted doesn't need its own
	// attributes checked, so this necessarily-expensive (checks every
	// element's every attribute when maxAttributeLength > 0) walk does
	// less work against an already-shrunk DOM. A no-op when neither
	// field is set (empty names + maxAttributeLength 0 just walks the
	// DOM removing nothing) — cheap enough not to bother special-casing
	// away entirely.
	if len(removeAttributes) > 0 || maxAttributeLength > 0 {
		allIgnoreForMaxLength := append(append([]string{}, defaultMaxLengthIgnoredAttributes...), ignoreAttributesForMaxLength...)
		attrScript, err := removeAttributesJS(removeAttributes, maxAttributeLength, allIgnoreForMaxLength)
		if err != nil {
			return crawlResponse{}, err
		}
		actions = append(actions, chromedp.Evaluate(attrScript, nil))
	}

	actions = append(actions,
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &html),
	)
	if err := chromedp.Run(ctx, actions...); err != nil {
		return crawlResponse{}, err
	}

	truncated := false
	if len(html) > MaxHTMLBytes {
		html = html[:MaxHTMLBytes]
		truncated = true
	}

	return crawlResponse{
		HTML:      html,
		Title:     title,
		FinalURL:  finalURL,
		Truncated: truncated,
	}, nil
}

// DefaultRemoveSelectors (step 44, svg added step 62) are always
// stripped from crawled HTML before it ever reaches an AI model's own
// context — <head>, <script>, <style>, and <svg> content is never
// useful to a model reading a page's own visible/structural content,
// and on a real page these can easily account for the majority of a
// crawl's own token cost. svg specifically: inline icon
// sprites/illustrations are pure path/coordinate data, arguably even
// less useful to a model than <style> is. Merged with (not replaced
// by) any caller-supplied removeSelectors in readCrawlResponse, below
// — a caller can still ask for MORE removed, never less. This is a
// global default — every fetch_page_html caller gets it, not just
// Career's own conversations, since the Browser tool has no concept
// of which portal/tool prompted a given AI conversation. See
// plan/ai/tools/browser/step-44-default-html-element-removal.md and
// plan/ai/tools/career/step-62-svg-removal-and-attribute-length-enforcement.md.
var DefaultRemoveSelectors = []string{"head", "script", "style", "svg"}

// defaultMaxLengthIgnoredAttributes (step 46) are always exempt from
// maxAttributeLength's own length-based stripping, regardless of
// whether the caller sets ignoreAttributesForMaxLength at all — href
// and src are how a link/image/script/iframe element actually points
// at its own resource; a long URL there is a real, legitimate value
// (query strings, tracking params, signed/expiring links routinely run
// well past an arbitrary character count), not noise the way a long
// inline style or data-* blob usually is, so removing it wholesale on
// length alone would silently break the element's own function
// instead of just shrinking harmless bulk. Merged with (not replaced
// by) any caller-supplied ignoreAttributesForMaxLength in
// readCrawlResponse, below — same "caller can only ask for MORE
// ignored, never less" shape DefaultRemoveSelectors above already
// established. See
// plan/ai/tools/browser/step-46-remove-attributes-plan.md.
var defaultMaxLengthIgnoredAttributes = []string{"href", "src"}

// RemoveElementsJS returns a JS snippet that removes every element
// matching any of selectors from the current document — run
// immediately before OuterHTML captures it, so the returned HTML never
// includes them. Selectors are embedded via json.Marshal (a valid JS
// array literal), the same safe-embedding technique
// buildCloudflareDetectJS (cloudflare.go) already uses for
// expectedSelectors — never string-concatenated raw. An individual
// selector that fails to parse (querySelectorAll throws) is skipped,
// not fatal to the others or to the crawl itself. See
// plan/ai/tools/browser/step-41-remove-html-elements.md.
func RemoveElementsJS(selectors []string) (string, error) {
	selectorsJSON, err := json.Marshal(selectors)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`(function() {
		var selectors = %s;
		selectors.forEach(function(sel) {
			try {
				document.querySelectorAll(sel).forEach(function(el) { el.remove(); });
			} catch (e) {}
		});
	})()`, selectorsJSON), nil
}

// removeAttributesJS returns a JS snippet that, for every element
// still in the document, removes any attribute named in names and —
// when maxLength > 0 — any attribute (any name) whose own value is
// longer than maxLength characters. Both checks run in one DOM walk,
// not two, since a caller opting into maxLength has already accepted
// the cost of checking every element's every attribute (its own doc
// comment on the caller-facing field says so) — there's no reason to
// pay for a second full traversal on top of that.
//
// Run this AFTER RemoveElementsJS in the same caller (readCrawlResponse),
// never before: elements already removed by that pass don't need their
// own attributes checked at all, so ordering it second measurably
// shrinks the work this necessarily-"excessive" (per this feature's
// own design doc) walk has to do.
//
// names/maxLength are embedded via json.Marshal, the same
// safe-embedding technique RemoveElementsJS itself already uses —
// never string-concatenated raw. Each element's own attributes are
// snapshotted into a plain array before any removal: attempting to
// remove while iterating the live attributes NamedNodeMap directly
// skips entries out from under the loop, a real DOM footgun, not
// hypothetical. See
// plan/ai/tools/browser/step-46-remove-attributes-plan.md.
func removeAttributesJS(names []string, maxLength int, ignoreForMaxLength []string) (string, error) {
	// A nil names/ignoreForMaxLength (the common case: a caller who
	// only sets maxAttributeLength never populates RemoveAttributes at
	// all, and a caller who never overrides the default ignore list
	// passes nil for the "additional" half of it) marshals to the JSON
	// literal null, not [] — caught directly by a real test, not
	// assumed: the generated script would then do `var names = null;
	// names.forEach(...)`, throwing before ever reaching the maxLength
	// check. Normalizing here means every call site stays simple;
	// RemoveElementsJS never hits this because its own caller always
	// builds its selectors via append(append([]string{}, ...), ...),
	// which is never nil regardless of what's appended — this function
	// has no equivalent guarantee from any of its own callers.
	if names == nil {
		names = []string{}
	}
	if ignoreForMaxLength == nil {
		ignoreForMaxLength = []string{}
	}
	namesJSON, err := json.Marshal(names)
	if err != nil {
		return "", err
	}
	ignoreJSON, err := json.Marshal(ignoreForMaxLength)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`(function() {
		var names = %s;
		var maxLength = %d;
		var ignoreForMaxLength = %s;
		document.querySelectorAll('*').forEach(function(el) {
			names.forEach(function(name) {
				try {
					if (el.hasAttribute(name)) el.removeAttribute(name);
				} catch (e) {}
			});
			if (maxLength > 0) {
				var attrs = Array.prototype.slice.call(el.attributes);
				attrs.forEach(function(attr) {
					try {
						if (ignoreForMaxLength.indexOf(attr.name) !== -1) return;
						if (attr.value && attr.value.length > maxLength) el.removeAttribute(attr.name);
					} catch (e) {}
				});
			}
		});
	})()`, namesJSON, maxLength, ignoreJSON), nil
}

// navigateWithHangGuard runs actions (a Navigate plus a trailing settle
// Sleep, at every call site in this package) against ctx, without
// trusting chromedp's own context handling alone to guarantee prompt
// return. Real, observed bug: a page whose own background activity
// (e.g. a Cloudflare Turnstile widget's own polling, still active even
// though the rest of the page has visibly finished rendering) can keep
// Chrome's navigation lifecycle from ever reaching "load complete,"
// leaving chromedp.Run blocked well past ctx's own deadline — a real
// Cloudflare Turnstile challenge left one navigate blocked for 370+
// seconds with no sign of ever returning on its own. On the primary tab
// that stuck request held shared.Mu, wedging the tab for every other
// caller tool-wide — including the existing wedge-recovery mechanism
// (shared.ProbeSessionLiveness/RecreateSharedSessionLocked), which
// needs the very lock the wedge is holding; on a worker or headed tab
// it would hold that tab's pool slot forever. Only a full process
// restart could recover from this before this fix existed.
//
// recreate is called, and this function returns ctx.Err() immediately,
// the moment ctx's own deadline passes — regardless of whether the
// underlying chromedp.Run call has returned yet. It must forcibly close
// the tab ctx belongs to (not just abandon the Go-side wait), so the CDP
// session breaks and the orphaned goroutine below unblocks — with a
// now-meaningless result, discarded into its own buffered channel —
// instead of continuing to touch a tab this call's own caller has
// already moved on from: shared.RecreateSharedSessionLocked for the
// primary tab (called with shared.Mu already held, its own
// precondition), or the worker/headed tab's own idempotent release.
func navigateWithHangGuard(ctx context.Context, recreate func(), actions ...chromedp.Action) error {
	done := make(chan error, 1)
	go func() {
		done <- chromedp.Run(ctx, actions...)
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if recreate != nil {
			recreate()
		}
		return ctx.Err()
	}
}

// navigateAndReadWithCloudflareCheck runs the shared navigate → settle
// → Cloudflare-wait → read-HTML sequence against ctx — one
// implementation for every tab a single-page crawl can run in (the
// primary tab, a worker tab, a headed tab). maxAutoWait bounds the
// automatic Cloudflare wait: short for the primary tab (primaryAutoWait
// — it must not hold shared.Mu while a challenge clears), the full
// cloudflareAutoWait everywhere else. Returns
// cloudflare_challenge_unresolved when the challenge is still present
// after that wait, cloudflare_blocked on a block page. See
// plan/ai/tools/browser/step-24-headed-chrome-cloudflare-fallback.md.
// recreate (see navigateWithHangGuard's own doc comment) is the
// caller's own tab-specific recovery action — shared.RecreateSharedSessionLocked
// for the primary tab, or closing the caller's own worker/headed tab.
func navigateAndReadWithCloudflareCheck(ctx context.Context, rawURL, requestID string, recreate func(), maxAutoWait time.Duration, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	// Registered before navigating — the challenge response's own
	// cf-mitigated header is only observable while it arrives.
	signals := watchChallengeSignals(ctx)

	setCrawlPhase(requestID, phaseNavigating, "navigating to "+rawURL)
	if err := navigateWithHangGuard(ctx, recreate, chromedp.Navigate(rawURL), chromedp.Sleep(SettleDelay)); err != nil {
		return crawlResponse{}, err
	}

	setCrawlPhase(requestID, phaseCheckingCloudflare, "checking for a Cloudflare challenge")
	tracker, err := newChallengeTracker(ctx, signals, expectedSelectors)
	if err != nil {
		return crawlResponse{}, err
	}
	outcome, err := tracker.checkAndAwaitAutoClearance(ctx, requestID, maxAutoWait, crawlReadReserve)
	if err != nil {
		return crawlResponse{}, err
	}
	if outcome.HardBlocked {
		return crawlResponse{}, newCloudflareBlockedError(outcome.Reason)
	}
	if !outcome.Cleared {
		return crawlResponse{}, newCloudflareUnresolvedError(outcome.Reason, tracker)
	}

	return readCrawlResponse(ctx, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
}

// crawlPage navigates the primary tab to rawURL and reads back its
// rendered HTML/title/final URL — the primary tab, not a worker, because
// fetch_page_html's own AI callers routinely follow up with calls that
// operate on "the current page" (extract_page_data, find_login_elements,
// login).
//
// The primary attempt is scoped to an inner function so shared.Mu is
// released the moment it concludes. When it finds a Cloudflare challenge
// that doesn't clear within primaryAutoWait, it leaves the challenge page
// (about:blank) and releases shared.Mu at once; the challenge is then
// resolved elsewhere (escalateChallenge, challenge_resolver.go: a
// headless worker tab, then a headed tab) while every other operation
// keeps using the primary tab. For AI callers (no requestID), the
// primary tab is afterwards pointed back at the page
// (syncPrimaryTabAsync). Career's own callers (requestID set) never
// operate on the current page afterwards, so they skip that.
//
// AI calls (no requestID) are bounded by aiCallBudget end to end and
// never go headed — see challenge_resolver.go's own top comment. parent
// is the HTTP request's own context: when the caller disconnects, the
// crawl stops.
func crawlPage(parent context.Context, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	// step 63.2 — registered before anything else in this call,
	// including the known-Cloudflare shortcut below, so this exact
	// call is cancellable (via cancelCrawl(requestID), crawl_cancel.go)
	// from the very instant it arrives — even before shared.Mu is ever
	// touched. A no-op registration when requestID is "" (the AI's own
	// MCP-driven calls never set one — registerCrawlCancel's own doc
	// comment). See plan/ai/tools/career/step-63-stop-crawling-now.md.
	reqCtx, reqCancel := context.WithCancel(parent)
	registerCrawlCancel(requestID, reqCancel)
	defer func() {
		unregisterCrawlCancel(requestID)
		reqCancel()
	}()
	deadline := aiDeadline(requestID, time.Now())

	// step 46 — a caller-supplied strip setting changes the output, so it
	// must never be served from (or written to) a cache keyed only by URL.
	skipCache := len(removeSelectors) > 0 || len(removeAttributes) > 0 || maxAttributeLength > 0

	// The cloudflare-web-scraper port (library_fallback.go) runs when this
	// tool's own headless attempt couldn't clear a challenge: for an AI
	// call right after it, within aiCallBudget (the headless attempt
	// leaves libraryFallbackReserve for it); for Career's own calls right
	// before the headed tab — so it also runs for a known-Cloudflare
	// domain, which skips the headless attempt.
	fallback := libraryFallbackEnabled()
	workerDeadline := deadline
	if fallback && !deadline.IsZero() {
		workerDeadline = deadline.Add(-libraryFallbackReserve)
	}
	worker := func() (crawlResponse, error) {
		return crawlOnWorkerTab(reqCtx, workerDeadline, rawURL, requestID, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
	}
	library := func() (crawlResponse, error) {
		return libraryFallbackFetch(reqCtx, deadline, rawURL, requestID, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
	}
	headed := func() (crawlResponse, error) {
		if fallback {
			resp, err := library()
			var cfErr *crawlError
			if !errors.As(err, &cfErr) || cfErr.Code != codeCloudflareUnresolved {
				return resp, err
			}
		}
		return crawlWithNormalSession(reqCtx, rawURL, requestID, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
	}
	resolve := func(tryWorkerFirst bool) (crawlResponse, error) {
		if requestID != "" {
			return escalateChallenge(reqCtx, rawURL, requestID, tryWorkerFirst, worker, headed)
		}
		// AI call: one headless worker attempt, then the library fallback,
		// within aiCallBudget; no domain lock (nothing headed to
		// serialize), no headed tab.
		resp, err := worker()
		var cfErr *crawlError
		if fallback && errors.As(err, &cfErr) && cfErr.Code == codeCloudflareUnresolved {
			resp, err = library()
		}
		recordIfUnresolved(rawURL, err)
		if err == nil {
			syncPrimaryTabAsync(rawURL, resp, skipCache)
		}
		return resp, err
	}

	// step 29 — a known-Cloudflare domain skips the primary tab entirely:
	// the only signal available before ever navigating anywhere is the
	// cache. It still tries a headless worker first when the headless
	// browser already holds a cf_clearance for this URL (earned earlier,
	// or imported from the headed browser) — otherwise straight to
	// headed, as before. (An AI call always goes to the worker — see
	// resolve.) A lookup failure (e.g. a transient DB error) is
	// treated the same as "not known". See
	// plan/ai/tools/browser/step-29-skip-headless-for-known-cloudflare-domains.md.
	if domain, err := hostnameOf(rawURL); err == nil {
		if known, err := isDomainKnownCloudflare(domain); err == nil && known {
			return resolve(shared.HeadlessHasCookie(rawURL, "cf_clearance"))
		}
	}

	result, err := func() (crawlResponse, error) {
		// step 45 — served for free only when the primary tab is already
		// showing this exact URL (shared.LastHeadlessFetchURL, set below
		// on every real navigation): the only case where a
		// stale-relative-to-the-live-DOM result can't happen, since
		// nothing has navigated the tab away since the cached fetch. Any
		// other case falls straight through to a real navigation. Never
		// consulted when the caller supplied its own strip settings — see
		// fetch_cache.go's own top comment. See
		// plan/ai/tools/browser/step-45-fetch-html-caching-plan.md and
		// plan/ai/tools/browser/step-46-remove-attributes-plan.md.
		if !skipCache {
			if cached, ok := fetchCacheLookup(rawURL); ok {
				shared.Mu.Lock()
				alreadyThere := shared.LastHeadlessFetchURL == rawURL
				shared.Mu.Unlock()
				if alreadyThere {
					return cached, nil
				}
			}
		}

		if err := shared.EnsureSharedSession(); err != nil {
			return crawlResponse{}, err
		}
		shared.Mu.Lock()
		defer shared.Mu.Unlock()

		// step 63.2 — this request may have been canceled while it sat
		// queued waiting for shared.Mu (a plain sync.Mutex isn't
		// cancellable) — bail out before doing any real chromedp work.
		// Checked before the liveness probe below: no point recreating a
		// perfectly healthy tab for a request nobody wants anymore.
		if reqCtx.Err() != nil {
			return crawlResponse{}, reqCtx.Err()
		}

		// step 47.2/47.3 — fail fast on a tab left wedged by a previous,
		// unrelated caller, and replace it immediately (still holding
		// shared.Mu) so the NEXT caller gets a healthy one.
		if err := shared.ProbeSessionLiveness(shared.Ctx); err != nil {
			recreateErr := shared.RecreateSharedSessionLocked()
			return crawlResponse{}, NewSessionWedgedError(err, recreateErr)
		}

		ctx, cancel := context.WithTimeout(shared.Ctx, capTimeout(crawlTimeout, deadline))
		defer cancel()
		// step 63.2 — cancelCrawl(requestID) reaches this attempt too.
		linkRequestCancel(reqCtx, ctx, cancel, shared.Ctx)

		recreate := func() { _ = shared.RecreateSharedSessionLocked() }
		resp, err := navigateAndReadWithCloudflareCheck(ctx, rawURL, requestID, recreate, primaryAutoWait, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
		if err != nil {
			var cfErr *crawlError
			if errors.As(err, &cfErr) && cfErr.Code == codeCloudflareUnresolved {
				leavePrimaryTabLocked(shared.Ctx, recreate)
			}
			return crawlResponse{}, err
		}
		shared.LastHeadlessFetchURL = rawURL
		if !skipCache {
			// Best-effort — a failed cache write must never turn an
			// otherwise-successful crawl into a failure.
			_ = fetchCacheStore(rawURL, resp)
		}
		return resp, nil
	}()

	// step 47.3 — checking the Code, not just the *crawlError type: a
	// wedged-tab or block-page error must never trigger the challenge
	// fallback.
	var cfErr *crawlError
	if errors.As(err, &cfErr) && cfErr.Code == codeCloudflareUnresolved {
		return resolve(true)
	}
	// step 38/63.3 — classified only here, after the Cloudflare dispatch
	// above: neither a session-interrupted NOR a user-cancelled error may
	// ever trigger the fallback.
	return result, classifyCancellation(err, reqCtx, shared.Ctx)
}

// crawlWithNormalSession retries rawURL in a tab of the headed browser
// (shared.AcquireHeadedTab) — only ever reached through
// escalateChallenge, after the headless attempts already failed to
// clear the same challenge. Always closes its tab before returning, on
// every exit path. If the automatic wait also fails there, hands off to
// waitForHumanToSolveCloudflare (step 25). On success, copies the headed
// tab's cookies for rawURL into the headless browser
// (importHeadedCookies), so the primary tab and later headless attempts
// can reach the site without a headed tab. See
// plan/ai/tools/browser/step-24-headed-chrome-cloudflare-fallback.md
// and plan/ai/tools/browser/step-25-human-assisted-cloudflare-retry.md.
func crawlWithNormalSession(reqCtx context.Context, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	setCrawlPhase(requestID, phaseQueuedForTab, "waiting for a free headed browser tab")
	tabCtx, release, err := shared.AcquireHeadedTab(reqCtx)
	if err != nil {
		if reqCtx.Err() != nil {
			return crawlResponse{}, newCrawlCancelledError()
		}
		return crawlResponse{}, err
	}
	defer release()

	ctx, cancel := context.WithTimeout(tabCtx, normalSessionCrawlTimeout)
	defer cancel()
	// step 63.2 — cancelCrawl(requestID) reaches this up-to-31-minute
	// headed/human-solve wait too.
	linkRequestCancel(reqCtx, ctx, cancel, tabCtx)

	result, err := navigateAndReadWithCloudflareCheck(ctx, rawURL, requestID, release, cloudflareAutoWait, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)

	var cfErr *crawlError
	if errors.As(err, &cfErr) && cfErr.Code == codeCloudflareUnresolved {
		result, err = waitForHumanToSolveCloudflare(ctx, cfErr, requestID, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
	}
	if err == nil {
		importHeadedCookies(ctx, rawURL)
	}
	return result, classifyCancellation(err, reqCtx, tabCtx)
}

// waitForHumanToClearCloudflare gives a person at the now-visible
// headed window real time to solve a challenge the automatic wait
// couldn't clear — the SAME window stays open for the whole wait, never
// closing and reopening mid-attempt (step 27). Shared by the single-page
// (/crawl, this file) and paginated (/crawl-paginated, paginate.go)
// fallbacks. See
// plan/ai/tools/browser/step-25-human-assisted-cloudflare-retry.md,
// step-26-headed-fallback-for-paginated-crawl.md, and
// step-27-long-lived-headed-fallback-session.md.
//
// A thin wrapper since the clearance rules moved to
// challenge_resolution.go (challengeTracker.await, human mode): polls
// every humanPollInterval for up to maxHumanSolveDuration, using every
// clearance signal the automatic wait uses — including the Turnstile
// token and cf_clearance cookie, which catch the silent "Success!" pass
// the former HTML-growth/widget-gone rules alone could never see — and
// returns a HardBlocked outcome immediately on a block page instead of
// waiting out the budget. cfErr.tracker carries the automatic wait's own
// detection-time baseline for this same tab; nil (the known-Cloudflare-
// domain shortcut) starts a fresh one here.
func waitForHumanToClearCloudflare(ctx context.Context, requestID string, cfErr *crawlError) (challengeOutcome, error) {
	setCrawlPhase(requestID, phaseAwaitingHumanChallenge, fmt.Sprintf(
		"Cloudflare challenge detected (%s) — waiting for a person to solve it in the open browser window", cfErr.Reason,
	))
	// Several headed tabs can share one window — show this one to the
	// person. Best-effort.
	_ = chromedp.Run(ctx, page.BringToFront())

	tracker := cfErr.tracker
	if tracker == nil {
		var err error
		if tracker, err = newChallengeTracker(ctx, nil, nil); err != nil {
			return challengeOutcome{}, err
		}
		tracker.captureBaseline()
	}
	return tracker.await(ctx, challengeModeHuman, maxHumanSolveDuration, requestID, cfErr.Reason)
}

// waitForHumanToSolveCloudflare gives a person sitting at the now-
// visible headed Chrome window real time to notice a still-present
// Cloudflare challenge and solve it themselves, then reads the page
// once cleared. See
// plan/ai/tools/browser/step-25-human-assisted-cloudflare-retry.md.
func waitForHumanToSolveCloudflare(ctx context.Context, fallback *crawlError, requestID string, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	outcome, err := waitForHumanToClearCloudflare(ctx, requestID, fallback)
	if err != nil {
		return crawlResponse{}, err
	}
	if outcome.HardBlocked {
		return crawlResponse{}, newCloudflareBlockedError(outcome.Reason)
	}
	if !outcome.Cleared {
		return crawlResponse{}, fallback
	}
	return readCrawlResponse(ctx, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
}

// validateCrawlURL is this tool's own SSRF guard — a page-fetching
// tool driven by an AI model is a textbook "agent as SSRF proxy"
// vector (cloud metadata endpoints, internal-network services, local
// files) if left unchecked. Rejects anything but a real http(s) URL
// whose hostname resolves to a public IP address before ever handing
// it to chromedp. See plan/ai/tools/browser/step-03-html-crawler-feature.md's
// own "Security considerations".
func validateCrawlURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme must be http or https, got %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("url must include a hostname")
	}

	ips, err := net.LookupIP(u.Hostname())
	if err != nil {
		return fmt.Errorf("could not resolve host %q: %w", u.Hostname(), err)
	}
	for _, ip := range ips {
		if isDisallowedIP(ip) {
			return fmt.Errorf("refusing to crawl %q: resolves to a private/internal address", u.Hostname())
		}
	}
	return nil
}

// isDisallowedIP rejects loopback, private (RFC 1918), and link-local
// (including the cloud-metadata range, 169.254.0.0/16) addresses —
// every real destination a public http(s) crawl should ever need is
// none of these.
func isDisallowedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

type fetchPageHTMLArgs struct {
	URL                          string   `json:"url" jsonschema:"the absolute http(s) URL to navigate to and read"`
	RemoveSelectors              []string `json:"removeSelectors,omitempty" jsonschema:"optional CSS selectors for ANY elements to strip from the returned HTML before it's read — e.g. [\"header\", \"footer\", \"nav\", \"script\", \"style\", \".cookie-banner\", \"#ads\"]. Not limited to these examples: any valid CSS selector is matched and removed. Use this to cut boilerplate/noise you don't need out of the result; omit to get the page exactly as rendered."`
	RemoveAttributes             []string `json:"removeAttributes,omitempty" jsonschema:"optional attribute names to strip from EVERY element that has them, regardless of the attribute's own value — e.g. [\"style\", \"onclick\", \"data-testid\"]. Use this to cut attribute noise you know the name of; omit to leave those attributes as rendered."`
	MaxAttributeLength           int      `json:"maxAttributeLength,omitempty" jsonschema:"optional — when set above 0, ANY attribute (any name, on any element) whose own value is longer than this many characters is removed — useful for stripping long inline noise (base64 data URIs, huge inline style/class blobs) you don't know the name of in advance. href and src are always exempt (see ignoreAttributesForMaxLength to exempt more) since a long URL there is often legitimate, not noise, and removing it would break the element. This checks every element's every attribute, which can take noticeably longer on a large page; omit or leave at 0 to disable."`
	IgnoreAttributesForMaxLength []string `json:"ignoreAttributesForMaxLength,omitempty" jsonschema:"optional attribute names to additionally exempt from maxAttributeLength's own check, on top of the built-in href/src exemption — e.g. [\"poster\", \"action\"] if some other URL-valued attribute on this particular page also needs protecting from removal. Has no effect unless maxAttributeLength is also set."`
}

// RegisterFetchPageHTML adds the fetch_page_html MCP tool — thin: it
// only ever calls shared.CallSibling and formats the result, never touches
// shared.Ctx directly (this --mcp subprocess never holds it — see
// main.go's own runMCPServer doc comment).
func RegisterFetchPageHTML(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "fetch_page_html",
		Description: "Navigate the shared browser session to a URL and return the page's rendered HTML, title, and final URL (after any redirect). " +
			"Calling this again for the exact same URL within about 5 minutes, without calling it for any other URL in between, returns a short-lived cached copy of what was fetched last time instead of navigating again — useful to know if you call this tool repeatedly for the same page, since you won't see the page's own content change again until either 5 minutes pass or you fetch a different URL first. Passing removeSelectors, removeAttributes, or maxAttributeLength always fetches a fresh copy — caching only ever applies to a plain re-fetch with none of those set. " +
			"Pass removeSelectors to strip elements you don't want out of the HTML before it's returned to you — any valid CSS selector works (tag names like \"header\", \"script\", \"style\", \"nav\", \"footer\"; classes like \".cookie-banner\"; ids like \"#ads\"; or anything else CSS can target). Use this whenever you only need part of the page, to cut boilerplate/noise out of your own result instead of reading past it. " +
			"Pass removeAttributes (a list of attribute names, e.g. [\"style\", \"onclick\", \"data-testid\"]) to strip those attributes from EVERY element that has them, keeping the element itself. Pass maxAttributeLength (a number of characters) to strip ANY attribute on ANY element whose own value is longer than that — useful for cutting long inline noise (base64 data URIs, huge inline style/class blobs) you don't know the name of ahead of time; this checks every element's every attribute, so it can take noticeably longer on a large page. href and src are always left alone by maxAttributeLength regardless of their own length, since a link/image/script's own URL is often legitimately long and removing it would break the element, not just shrink it — pass ignoreAttributesForMaxLength (more attribute names) if some other URL-like attribute on this page needs the same protection. Both removeAttributes and maxAttributeLength can be used together, and neither touches the elements themselves, only their attributes. " +
			"If the page is behind a Cloudflare challenge, this waits briefly for it to clear before reading the page; if it's still blocking once the wait runs out, this automatically retries in a normal (non-headless) browser window and can wait up to about 30 minutes for a person to notice and solve the challenge there before giving up — meaning this call can take up to roughly 30 minutes in that case, almost certainly longer than this AI tool-calling session's own timeout, so a Cloudflare-blocked page is effectively only recoverable through this path by a human watching for the window, not by an AI call waiting on the result. If it's still blocked after that, this call fails with a cloudflare_challenge_unresolved error instead of returning the interstitial as if it were the real page. " +
			"For crawling a BATCH of many individual pages (e.g. job detail pages), don't call this yourself in a loop — use crawl_urls_with_subagents instead, which crawls each one as an isolated, concurrency-safe sub-agent.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args fetchPageHTMLArgs) (*mcp.CallToolResult, any, error) {
		if args.URL == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "url is required"}},
				IsError: true,
			}, nil, nil
		}

		reqBody, err := json.Marshal(crawlRequest{
			URL:                          args.URL,
			RemoveSelectors:              args.RemoveSelectors,
			RemoveAttributes:             args.RemoveAttributes,
			MaxAttributeLength:           args.MaxAttributeLength,
			IgnoreAttributesForMaxLength: args.IgnoreAttributesForMaxLength,
		})
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to build request: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		respBody, err := shared.CallSibling("crawl", reqBody)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		var result crawlResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to parse crawl response: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		text := fmt.Sprintf("Title: %s\nURL: %s\n", result.Title, result.FinalURL)
		if result.Truncated {
			text += "(HTML truncated to fit the response size limit)\n"
		}
		text += "\n" + result.HTML

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	})
}
