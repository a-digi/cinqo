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

// crawlTimeout bounds one navigation — shorter than pdf_generator's
// own 30s render budget, since crawling has no print-to-PDF step of
// its own to also account for.
const crawlTimeout = 20 * time.Second

const SettleDelay = 1500 * time.Millisecond

// normalSessionCrawlTimeout bounds one headed-Chrome fallback attempt
// end to end — deliberately its OWN budget, not nested inside or
// derived from the primary attempt's own ctx (already bounded by
// crawlTimeout, and likely close to exhausted by the time a fallback
// is even considered, having just spent up to cloudflareMaxWait
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

// humanSolveRetryInterval — after the automated wait
// (waitForCloudflareClearance's own cloudflareMaxWait=8s) still finds
// the challenge present in the now-visible headed window, a human
// needs real time to notice it and react — checked again every
// humanSolveRetryInterval (not continuously) rather than one more
// short automated poll.
// var, not const, solely so a temporary test can shorten it — see
// TestWaitForHumanToClearCloudflareReportsEveryTick.
var humanSolveRetryInterval = 15 * time.Second

// maxHumanSolveDuration bounds the total time waitForHumanToClearCloudflare
// spends waiting — a wall-clock cap, not a fixed retry count (step
// 25's original maxHumanSolveRetries=4, a 60s budget). Long enough
// that a human realistically never needs the SAME window to close and
// reopen mid-attempt — the window stays open the whole time. See
// plan/ai/tools/browser/step-27-long-lived-headed-fallback-session.md.
// An explicit starting estimate, not verified against a real person's
// own reaction time. var, not const, for the same testing reason as
// humanSolveRetryInterval above.
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

	result, err := crawlPage(body.URL, body.RequestID, body.ExpectedSelectors, body.RemoveSelectors, body.RemoveAttributes, body.MaxAttributeLength, body.IgnoreAttributesForMaxLength)
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
	actions := []chromedp.Action{chromedp.Evaluate(script, nil)}

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

	var html, title, finalURL string
	actions = append(actions,
		chromedp.Title(&title),
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

// navigateAndReadWithCloudflareCheck runs the shared navigate → settle
// → Cloudflare-wait → read-HTML sequence against ctx — extracted so
// both the primary (shared headless session) and fallback (ephemeral
// headed session, step 24) crawl attempts share one implementation
// instead of two copies that could drift apart. Returns the same
// crawlError (cloudflare_challenge_unresolved) as before when the
// challenge is still present once ctx's own wait budget is spent. See
// plan/ai/tools/browser/step-24-headed-chrome-cloudflare-fallback.md.
func navigateAndReadWithCloudflareCheck(ctx context.Context, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	setCrawlPhase(requestID, phaseNavigating, "navigating to "+rawURL)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(rawURL),
		chromedp.Sleep(SettleDelay),
	); err != nil {
		return crawlResponse{}, err
	}

	setCrawlPhase(requestID, phaseCheckingCloudflare, "checking for a Cloudflare challenge")
	cf, err := waitForCloudflareClearance(ctx, expectedSelectors)
	if err != nil {
		return crawlResponse{}, err
	}
	if cf.Detected {
		return crawlResponse{}, newCloudflareUnresolvedError(cf.Reason)
	}

	return readCrawlResponse(ctx, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
}

// crawlPage navigates the one shared headless session to rawURL and
// reads back its rendered HTML/title/final URL. The headless attempt
// itself is scoped to an inner function so shared.Mu (see that
// variable's own doc comment in main.go) is released the moment that
// attempt concludes — BEFORE crawlWithNormalSession (a wholly separate
// browser, guarded by its own shared.NormalSessionMu) ever starts. Without
// this, shared.Mu would stay held for the fallback's own up-to-90s
// retry budget (step 25) too, serializing every other crawl request
// behind one slow, unrelated Cloudflare fallback. When the headless
// attempt hits an unresolved Cloudflare challenge, retries via
// crawlWithNormalSession (step 24) instead of failing immediately — a
// real, headed browser is often materially harder for Cloudflare to
// flag as automated than headless Chrome, even with this tool's own
// existing stealth patches applied.
func crawlPage(rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	// step 63.2 — registered before anything else in this call,
	// including the known-Cloudflare shortcut below, so this exact
	// call is cancellable (via cancelCrawl(requestID), crawl_cancel.go)
	// from the very instant it arrives — even before shared.Mu is ever
	// touched. A no-op registration when requestID is "" (the AI's own
	// MCP-driven calls never set one — registerCrawlCancel's own doc
	// comment). See plan/ai/tools/career/step-63-stop-crawling-now.md.
	reqCtx, reqCancel := context.WithCancel(context.Background())
	registerCrawlCancel(requestID, reqCancel)
	defer func() {
		unregisterCrawlCancel(requestID)
		reqCancel()
	}()

	// step 29 — a known-Cloudflare domain skips the headless attempt
	// entirely, straight to the headed fallback: the only signal
	// available before ever navigating anywhere is the cache (nothing
	// has been loaded yet to inspect live). A lookup failure (e.g. a
	// transient DB error) is treated the same as "not known" — falls
	// through to the normal headless attempt rather than blocking the
	// whole call on a cache problem. See
	// plan/ai/tools/browser/step-29-skip-headless-for-known-cloudflare-domains.md.
	if domain, err := hostnameOf(rawURL); err == nil {
		if known, err := isDomainKnownCloudflare(domain); err == nil && known {
			return crawlWithNormalSession(reqCtx, rawURL, requestID, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
		}
	}

	// step 46 — same reasoning as removeSelectors below: a caller-
	// supplied attribute-strip setting changes the output, so it must
	// never be served from (or written to) a cache keyed only by URL.
	skipCache := len(removeSelectors) > 0 || len(removeAttributes) > 0 || maxAttributeLength > 0

	result, err := func() (crawlResponse, error) {
		// step 45 — served for free only when the shared session is
		// already showing this exact URL (shared.LastHeadlessFetchURL, set
		// below on every real navigation): the only case where a
		// stale-relative-to-the-live-DOM result can't happen, since
		// nothing has navigated the session away since the cached
		// fetch. Any other case — no cache entry, an expired one, or
		// a fresh one but the session has moved on — falls straight
		// through to the exact same real navigate/settle/Cloudflare-
		// wait/read sequence as before this step, unchanged. Never
		// consulted at all when the caller supplied its own
		// removeSelectors/removeAttributes/maxAttributeLength — see
		// fetch_cache.go's own top comment for why. See
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
		// queued waiting for shared.Mu (a plain sync.Mutex isn't itself
		// cancellable/selectable, so that wait can't be interrupted
		// early — see step 63's own Open Question 1) — bail out here,
		// immediately after acquiring the lock, before doing any real
		// chromedp work, rather than only discovering the cancellation
		// once crawlTimeout's own budget is spent. Checked before the
		// liveness probe below: no point recreating a perfectly healthy
		// session for a request nobody wants an answer for anymore.
		if reqCtx.Err() != nil {
			return crawlResponse{}, reqCtx.Err()
		}

		// step 47.2/47.3 — fail fast on a tab left wedged by a previous,
		// unrelated caller instead of discovering it only after burning
		// crawlTimeout on a navigate that was never going to complete,
		// and replace the wedged tab immediately (still holding
		// shared.Mu) so the NEXT caller gets a fresh, healthy session
		// instead of inheriting the same wedge.
		if err := shared.ProbeSessionLiveness(shared.Ctx); err != nil {
			recreateErr := shared.RecreateSharedSessionLocked()
			return crawlResponse{}, NewSessionWedgedError(err, recreateErr)
		}

		ctx, cancel := context.WithTimeout(shared.Ctx, crawlTimeout)
		defer cancel()
		// step 63.2 — links this call's own ctx to reqCtx: the moment
		// cancelCrawl(requestID) fires reqCancel (crawl_cancel.go), this
		// goroutine cancels ctx too, which every chromedp.Run call below
		// already respects internally — no changes needed inside
		// navigateAndReadWithCloudflareCheck/readCrawlResponse
		// themselves. Exits via whichever side finishes first, leaking
		// nothing. Go's stdlib context package has no built-in "cancel
		// when either of two contexts is done" combinator, so this is
		// the standard idiomatic substitute.
		go func() {
			select {
			case <-reqCtx.Done():
				cancel()
				// step 63.2 — canceling ctx above only stops THIS call
				// from waiting; Chrome itself keeps loading otherwise,
				// leaving the tab busy for a beat afterward (caught
				// directly by a disposable test, not assumed).
				stopBrowserLoad(shared.Ctx)
			case <-ctx.Done():
			}
		}()

		resp, err := navigateAndReadWithCloudflareCheck(ctx, rawURL, requestID, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
		if err != nil {
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

	var cfErr *crawlError
	// step 47.3 — checking cfErr.Code, not just the *crawlError type, is
	// required now that a second, unrelated *crawlError variant exists
	// (browser_session_wedged) that can come out of this same closure —
	// without this, a wedged-tab error would incorrectly trigger the
	// headed Cloudflare fallback below (and wrongly record this domain
	// as "known Cloudflare"). paginate.go's equivalent dispatch is
	// already safe via its own separate blockedURL != "" guard; this
	// file had no second guard, so the Code check is the fix here.
	if errors.As(err, &cfErr) && cfErr.Code == "cloudflare_challenge_unresolved" {
		// step 28 — best-effort: a failed cache write must never turn
		// an otherwise-working fallback into a failure.
		if domain, hostErr := hostnameOf(rawURL); hostErr == nil {
			_ = recordCloudflareDomain(domain, cfErr.Reason)
		}
		return crawlWithNormalSession(reqCtx, rawURL, requestID, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
	}
	// step 38/63.3 — deliberately classified only here, after the
	// Cloudflare dispatch above: neither a session-interrupted NOR a
	// user-cancelled error may ever trigger crawlWithNormalSession's
	// own fresh headed-Chrome fallback (the whole subprocess died in
	// the first case; the user explicitly asked to stop in the
	// second — launching a second, unrelated browser fixes nothing in
	// either case, and would directly defeat the second one).
	return result, classifyCancellation(err, reqCtx, shared.Ctx)
}

// crawlWithNormalSession retries rawURL in a freshly launched, non-
// headless Chrome instance — called only after the shared headless
// session (crawlPage, above) already failed to clear the same
// Cloudflare challenge. Always tears the headed instance down before
// returning, on every exit path (success, still blocked, or any other
// error) — "close after it is finished." If the automated wait inside
// navigateAndReadWithCloudflareCheck also fails to clear it, hands off
// to waitForHumanToSolveCloudflare (step 25) instead of giving up
// immediately. See
// plan/ai/tools/browser/step-24-headed-chrome-cloudflare-fallback.md
// and plan/ai/tools/browser/step-25-human-assisted-cloudflare-retry.md.
func crawlWithNormalSession(reqCtx context.Context, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	shared.NormalSessionMu.Lock()
	defer shared.NormalSessionMu.Unlock()

	// step 63.2 — bail out before ever launching a headed Chrome
	// process at all if this request was already canceled while
	// queued waiting for shared.NormalSessionMu — no point spending several
	// seconds spinning up a whole browser window nobody wants anymore.
	if reqCtx.Err() != nil {
		return crawlResponse{}, reqCtx.Err()
	}

	ctx, cancels, err := shared.StartSharedNormalSession()
	if err != nil {
		return crawlResponse{}, fmt.Errorf("normal-session fallback: failed to start: %w", err)
	}
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
	}()

	baseCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, normalSessionCrawlTimeout)
	defer cancel()
	// step 63.2 — same "cancel A when B cancels" link as crawlPage's
	// own headless attempt, above: cancelCrawl(requestID) now reaches
	// this up-to-31-minute headed/human-solve wait too, so a Stop
	// click during that wait actually interrupts it instead of only
	// ever being able to time out.
	go func() {
		select {
		case <-reqCtx.Done():
			cancel()
			stopBrowserLoad(baseCtx)
		case <-ctx.Done():
		}
	}()

	result, err := navigateAndReadWithCloudflareCheck(ctx, rawURL, requestID, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)

	var cfErr *crawlError
	if !errors.As(err, &cfErr) {
		// step 63.3 — this function's own ctx isn't shared.Ctx (it's
		// rooted in the ephemeral headed session's own baseCtx), but
		// classifyCancellation's shared.Ctx check still correctly
		// covers the (rare) case where the whole shared headless
		// session ALSO died at the same moment — never a false
		// positive, since that check only ever fires when shared.Ctx
		// itself is actually done.
		return result, classifyCancellation(err, reqCtx, shared.Ctx)
	}
	result, err = waitForHumanToSolveCloudflare(ctx, cfErr, requestID, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
	return result, classifyCancellation(err, reqCtx, shared.Ctx)
}

// waitForHumanToClearCloudflare polls detectCloudflareChallenge every
// humanSolveRetryInterval, until either it clears or
// maxHumanSolveDuration is spent, giving a human at a now-visible
// headed window real time to solve a challenge themselves — the SAME
// window stays open for the whole wait, never closing and reopening
// mid-attempt (step 27). Returns (true, nil) the moment it clears,
// (false, nil) if the budget is spent with the challenge still
// present, or a real error only if a chromedp action itself failed.
// Extracted out of this file's own original crawlWithNormalSession-
// specific version so both the single-page (/crawl, this file) and
// paginated (/crawl-paginated, paginate.go) fallbacks share one
// implementation. See
// plan/ai/tools/browser/step-25-human-assisted-cloudflare-retry.md,
// step-26-headed-fallback-for-paginated-crawl.md, and
// step-27-long-lived-headed-fallback-session.md.
// initialReason is the Reason the caller's own initial detection
// already found (crawlError.Reason) — reported immediately so the
// very first status a poller ever sees already says which signal
// triggered this wait, not a generic placeholder.
//
// expectedSelectors (step 36, strengthened step 66) is checked FIRST,
// unconditionally, on every tick — a match ends the wait immediately,
// regardless of what detectCloudflareChallenge's own heuristic still
// thinks is on the page. This is deliberately no longer gated behind
// the heuristic's own "not detected" verdict: a real, reproduced bug
// (a human genuinely solved a challenge, yet this loop kept reporting
// "still detected" — challenge-text, then visible-widget — for
// upwards of a minute) showed that some Cloudflare-looking DOM
// fragment (a lingering Turnstile "verified" checkbox, a cookie/trust
// banner, cached challenge markup Cloudflare doesn't always fully tear
// down) can keep the heuristic itself convinced a challenge is still
// active even after the real, already-loaded target content is
// visible underneath it. detectCloudflareChallenge's own internal
// "expected content found" override (cloudflare.go) only ever runs
// once the heuristic ALREADY concluded detected=true within that same
// evaluate call — it cannot help here, because it's nested inside the
// very verdict this loop needs to be able to override from the
// outside. Checking expectedContentVisible directly, first, makes "the
// crawl's own target content is actually there" the authoritative
// signal, exactly as it already is for a fresh crawl's very first
// navigation (readCrawlResponse is reached the instant the target
// content is confirmed, without waiting on Cloudflare's own DOM to
// admit anything). Deliberately scoped to THIS function alone, never
// waitForCloudflareClearance's own short automated wait — that one
// still leans on detectCloudflareChallenge's own internal override,
// which is sufficient for its far shorter 8s budget. See
// plan/ai/tools/browser/step-36-content-based-cloudflare-override.md.
func waitForHumanToClearCloudflare(ctx context.Context, requestID, initialReason string, expectedSelectors []string) (bool, error) {
	setCrawlPhase(requestID, phaseAwaitingHumanChallenge, fmt.Sprintf(
		"Cloudflare challenge detected (%s) — waiting for a person to solve it in the open browser window", initialReason,
	))

	// Read once, up front — same "read once, not re-checked per tick"
	// convention paginatedCrawlHandler's own settings.DebugEnabled read
	// already establishes. A read failure is treated the same as "off"
	// — this is pure debug observability, never allowed to interrupt
	// the wait itself.
	settings, _ := shared.LoadBrowserSettings()
	logHTML := settings.DebugEnabled && settings.DebugLogHTML

	deadline := time.Now().Add(maxHumanSolveDuration)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		if err := chromedp.Run(ctx, chromedp.Sleep(humanSolveRetryInterval)); err != nil {
			return false, err
		}

		// Captured BEFORE the checks below, so the logged HTML is
		// exactly what expectedContentVisible/detectCloudflareChallenge
		// are about to evaluate on this same tick, not a stale snapshot
		// from a moment earlier. Best-effort and skipped entirely (no
		// extra chromedp round trip at all) unless Debug + Log HTML are
		// both on.
		if logHTML {
			var html string
			if err := chromedp.Run(ctx, chromedp.OuterHTML("html", &html)); err == nil {
				logChallengeDetectorHTML(requestID, attempt, html)
			}
		}

		if len(expectedSelectors) > 0 {
			if found, err := expectedContentVisible(ctx, expectedSelectors); err != nil {
				return false, err
			} else if found {
				return true, nil
			}
		}

		check, err := detectCloudflareChallenge(ctx, expectedSelectors)
		if err != nil {
			return false, err
		}
		if !check.Detected {
			return true, nil
		}
		// Reported every tick, not just once at the top — a real gap
		// fixed here: without this, this function's own live status
		// never changed for the ENTIRE wait (up to 30 minutes), no
		// matter how many times the check underneath actually re-ran,
		// making a perfectly-alive retry loop look frozen from any
		// poller's own point of view. Including check.Reason on every
		// tick (not just the first) also surfaces a reason that
		// CHANGES between ticks — e.g. a page that first shows the
		// resolvable JS challenge (reason "title") and later, after a
		// redirect, shows a harder WAF block (reason
		// "waf-block-params") — as a real, visible signal rather than
		// silently indistinguishable "still detected" text.
		setCrawlPhase(requestID, phaseAwaitingHumanChallenge, fmt.Sprintf(
			"still detected (%s) — check #%d, waiting for a person to solve it in the open browser window", check.Reason, attempt,
		))
	}
	return false, nil
}

// waitForHumanToSolveCloudflare gives a person sitting at the now-
// visible headed Chrome window real time to notice a still-present
// Cloudflare challenge and solve it themselves, then reads the page
// once cleared. See
// plan/ai/tools/browser/step-25-human-assisted-cloudflare-retry.md.
func waitForHumanToSolveCloudflare(ctx context.Context, fallback *crawlError, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	cleared, err := waitForHumanToClearCloudflare(ctx, requestID, fallback.Reason, expectedSelectors)
	if err != nil {
		return crawlResponse{}, err
	}
	if !cleared {
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
			"If the page is behind a Cloudflare challenge, this waits briefly for it to clear before reading the page; if it's still blocking once the wait runs out, this automatically retries in a normal (non-headless) browser window and can wait up to about 30 minutes for a person to notice and solve the challenge there before giving up — meaning this call can take up to roughly 30 minutes in that case, almost certainly longer than this AI tool-calling session's own timeout, so a Cloudflare-blocked page is effectively only recoverable through this path by a human watching for the window, not by an AI call waiting on the result. If it's still blocked after that, this call fails with a cloudflare_challenge_unresolved error instead of returning the interstitial as if it were the real page.",
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
