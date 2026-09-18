// paginate.go implements pagination-aware crawling — the AI describes
// which fields to extract (extract.go's own field shape) plus a
// "next page" control and how deep to paginate; the tool extracts,
// clicks next, extracts again, and repeats until told to stop.
// Deliberately not a general link-follower: it only ever clicks the
// one pagination control the instruction names, never an arbitrary
// link found on the page. The AI's own instruction is a YAML
// document (matching login's own step-8 wire shape), not typed JSON
// fields like extract_page_data — this tool's whole reason to exist
// is the pagination loop, so its args reflect that directly. See
// plan/ai/tools/browser/step-16-paginated-crawl-instructions.md.
package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"

	"browser-tool-backend/shared"
)

// paginatedCrawlTimeout bounds the whole multi-page loop — deliberately
// well under the host's own invokeTimeout (45s, api/src/tool/mcp/invoke.go,
// which wraps this call's entire spawn+handshake+logic and can't be
// changed from inside this tool), leaving real headroom so this tool's
// own clean "time_budget_reached" partial result fires before the host
// kills the call outright.
const paginatedCrawlTimeout = 35 * time.Second

// normalSessionPaginatedCrawlTimeout bounds one headed-Chrome
// paginated-crawl fallback end to end. Raised from step 26's original
// 150s specifically so the SAME headed window stays open for the whole
// human-wait instead of closing and reopening mid-attempt — a real bug
// that shorter budget caused when paired with Career's own now-removed
// outer retry loop. See
// plan/ai/tools/browser/step-27-long-lived-headed-fallback-session.md.
//
// Deliberately derived from maxHumanWaitRounds × maxHumanSolveDuration
// rather than a hand-picked number — a real, reproduced bug: this used
// to be a flat 32 minutes (sized for exactly ONE human-wait round, per
// this const's own original doc comment), but performPaginatedCrawlWithNormalSession
// can now retry the wait up to maxHumanWaitRounds times (see that
// function's own doc comment for why a "solved" declaration can turn
// out to be premature). A flat ceiling sized for one round silently
// cut a legitimate second/third round short — closing the visible
// Chrome window on a person actively still solving the challenge. The
// 90s/round overhead covers process launch/navigate/settle plus one
// full pagination loop through effectiveMaxPages pages each round; the
// trailing +90s covers the final successful pagination loop once
// actually cleared.
//
// A var, not a const, for the same reason maxHumanSolveDuration
// (crawl.go) itself is one: it's derived from that var, and a const
// can't reference one.
var normalSessionPaginatedCrawlTimeout = time.Duration(maxHumanWaitRounds)*(maxHumanSolveDuration+90*time.Second) + 90*time.Second

// maxAllowedPaginationPages is the real, non-negotiable ceiling on how
// many pages a single call ever visits, regardless of what the
// instruction requests — sized against paginatedCrawlTimeout: a
// conservative ~3s per page/transition (extraction itself is fast;
// SettleDelay alone is 1.5s, plus real navigation time) puts 10 pages
// at roughly 30s in the worst realistic case, inside budget with
// margin. An over-ceiling request is clamped, not rejected — the
// effective value used is reported back in the response.
const maxAllowedPaginationPages = 10

type paginationSpec struct {
	NextSelector string `yaml:"nextSelector"`
	MaxPages     int    `yaml:"maxPages"`
}

// paginatedCrawlInstructions is the YAML shape the AI writes as this
// tool's own single argument — see this file's own top comment for
// why this tool's args are YAML while extract_page_data's stay typed
// JSON fields. Container (step 18) is optional — see extractRequest's
// own doc comment (extract.go) for what it does; unset, every page's
// own extraction stays in today's flat mode.
type paginatedCrawlInstructions struct {
	Container string         `yaml:"container,omitempty"`
	Fields    []extractField `yaml:"fields"`
	// Mapping (step 20) — optional {sourceLabel: targetKey}, renames
	// extracted fields to specific output keys before each page's own
	// result is returned. See extractRequest's own doc comment
	// (extract.go) and
	// plan/ai/tools/browser/step-20-output-field-mapping.md.
	Mapping    map[string]string `yaml:"mapping,omitempty"`
	Pagination paginationSpec    `yaml:"pagination"`
}

type pageExtractResult struct {
	URL     string         `json:"url"`
	Results map[string]any `json:"results,omitempty"`
	// Items (step 18) — one object per matched container on this page,
	// when Container was set. Exactly one of Results/Items is non-nil,
	// same contract as extractResponse.
	Items    []map[string]any `json:"items,omitempty"`
	NotFound []string         `json:"notFound"`
	// CloudflareDetected/CloudflareReason (step 21) — a single,
	// immediate check (detectCloudflareChallenge, cloudflare.go), not
	// the full wait-then-poll crawlPage's own initial navigation gets
	// (waitForCloudflareClearance) — this tool's own per-page time
	// budget (paginatedCrawlTimeout) has far less slack for an extra
	// multi-second wait per page across up to maxAllowedPaginationPages
	// pages. See this step's own open question 1,
	// plan/ai/tools/browser/step-21-cloudflare-challenge-detection.md.
	CloudflareDetected bool   `json:"cloudflareDetected,omitempty"`
	CloudflareReason   string `json:"cloudflareReason,omitempty"`
}

type paginatedCrawlResponse struct {
	Pages             []pageExtractResult `json:"pages"`
	StoppedReason     string              `json:"stoppedReason"`
	PagesVisited      int                 `json:"pagesVisited"`
	RequestedMaxPages int                 `json:"requestedMaxPages"`
	EffectiveMaxPages int                 `json:"effectiveMaxPages"`
	// BlockedReason carries the Cloudflare check's own Reason
	// (cloudflare.go) when StoppedReason is "cloudflare_blocked" — set
	// only on that one stop reason, empty otherwise.
	BlockedReason string `json:"blockedReason,omitempty"`
}

// paginatedCrawlRequest is the internal JSON shape the --mcp adapter
// sends to its own HTTP-mode sibling — already-parsed-and-validated
// by the time it crosses this boundary, same convention as every
// other feature's own shared.CallSibling request.
type paginatedCrawlRequest struct {
	// URL (step 37) is optional — when set, the shared session
	// navigates there FIRST, still holding shared.Mu, immediately
	// before extraction begins, making navigate-then-extract one
	// atomic operation instead of two separately-locked HTTP calls.
	// Absent for every AI-driven call (crawl_paginated operates on
	// whatever page is already loaded, unchanged) — a real, confirmed
	// bug fix for Career's own "Crawl now", which used to call POST
	// /crawl then, moments later, this endpoint: shared.Mu was
	// released completely in between, so a DIFFERENT concurrent
	// crawl's own navigate could — and, live-reported, did — sneak in
	// and leave this call extracting the wrong link's own page. See
	// plan/ai/tools/browser/step-37-atomic-navigate-and-extract.md.
	URL               string            `json:"url,omitempty"`
	Container         string            `json:"container,omitempty"`
	Fields            []extractField    `json:"fields"`
	Mapping           map[string]string `json:"mapping,omitempty"`
	NextSelector      string            `json:"nextSelector"`
	RequestedMaxPages int               `json:"requestedMaxPages"`
	EffectiveMaxPages int               `json:"effectiveMaxPages"`
	// RequestID (step 31) — same optional, opt-in phase-tracking field
	// crawlRequest (crawl.go) carries; see that field's own doc comment.
	RequestID string `json:"requestId,omitempty"`
}

// PaginatedCrawlHandler handles POST /crawl-paginated — the --mcp
// adapter's own real target for crawl_paginated.
func PaginatedCrawlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body paginatedCrawlRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// nextSelector is only meaningful once there's a second page to find
	// — runPaginatedCrawlLoop's own maxPages check (below) always breaks
	// the loop before nextSelector is ever evaluated when
	// EffectiveMaxPages is 1, so a single-page call (extract_from_url,
	// this file's own RegisterExtractFromURL) has nothing to supply
	// there. Still required for any real multi-page request. See
	// plan/ai/tools/browser/step-XX-atomic-single-page-extract.md.
	if len(body.Fields) == 0 || body.EffectiveMaxPages < 1 || (body.NextSelector == "" && body.EffectiveMaxPages > 1) {
		http.Error(w, "fields and a positive maxPages are required; nextSelector is required whenever maxPages > 1", http.StatusBadRequest)
		return
	}
	// step 37 — the same SSRF guard CrawlHandler's own url already
	// gets; a URL reaching this new field is no less capable of
	// driving the shared session somewhere it shouldn't than /crawl's
	// own url is.
	if body.URL != "" {
		if err := validateCrawlURL(body.URL); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Step 22 — read once, up front: both whether to log at all
	// (DebugEnabled) and whether to also capture each page's own raw
	// HTML while crawling (DebugLogHTML) are decided from this single
	// read, not two independent ones later, so the two can't observe a
	// setting change mid-request. A read failure is treated the same as
	// "Debug off" — logging is best-effort observability, never allowed
	// to turn a successful crawl into a failed response. See
	// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
	settings, _ := shared.LoadBrowserSettings()
	captureHTML := settings.DebugEnabled && settings.DebugLogHTML

	result, pageHTML, err := performPaginatedCrawl(body.URL, body.Container, body.Fields, body.Mapping, body.NextSelector, body.RequestedMaxPages, body.EffectiveMaxPages, captureHTML, body.RequestID)
	if err != nil {
		setCrawlPhase(body.RequestID, phaseFailed, err.Error())
		var cfErr *crawlError
		if errors.As(err, &cfErr) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(cfErr)
			return
		}
		http.Error(w, fmt.Sprintf("paginated crawl failed: %v", err), http.StatusBadGateway)
		return
	}
	setCrawlPhase(body.RequestID, phaseCompleted, fmt.Sprintf("finished — visited %d page(s) (%s)", result.PagesVisited, result.StoppedReason))

	// Step 17 — diagnostic record of this call, covering both the AI's
	// own crawl_paginated tool calls and career's own deterministic
	// "Crawl now" (both reach this same handler). Logged only on
	// success, after the real result is known — a failed crawl (above)
	// has nothing useful to log beyond the error already returned.
	// Step 22 — and only when Debug is on at all; saveCrawlLog is not
	// even called otherwise, so nothing is written, not merely hidden.
	if settings.DebugEnabled {
		saveCrawlLog(body.Container, body.Fields, body.NextSelector, body.RequestedMaxPages, body.EffectiveMaxPages, result, pageHTML)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// runPaginatedCrawlLoop is the actual multi-page loop — extracted
// (verbatim, no behavior change) so it can run against EITHER the
// shared headless session or the ephemeral headed fallback session
// (step 26), identical logic, different ctx. Calls
// runExtractionOnCurrentPage (extract.go) directly rather than
// performExtraction, precisely because the caller already holds
// whichever session's own lock performExtraction would try to take
// again. See that function's own doc comment.
// captureHTML (step 22) gates one extra chromedp.OuterHTML read per
// page — costs nothing when false (the overwhelming default: Debug or
// "Log HTML" off), only paid when a human has explicitly turned "Log
// HTML" on. pageHTML, the second return value, is parallel to the
// returned response's own Pages (one entry per page, empty string when
// captureHTML is false) and is never embedded in paginatedCrawlResponse
// itself — kept as a separate return value specifically so it cannot
// reach the live AI-facing JSON response; only PaginatedCrawlHandler's
// own saveCrawlLog call (crawl_log.go) ever sees it. See
// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
func runPaginatedCrawlLoop(ctx context.Context, container string, fields []extractField, mapping map[string]string, nextSelector string, requestedMaxPages, effectiveMaxPages int, captureHTML bool, requestID string) (paginatedCrawlResponse, []string, error) {
	pages := make([]pageExtractResult, 0, effectiveMaxPages)
	var pageHTML []string
	if captureHTML {
		pageHTML = make([]string, 0, effectiveMaxPages)
	}
	stoppedReason := ""
	blockedReason := ""

	for page := 1; ; page++ {
		setCrawlPhase(requestID, phaseExtracting, fmt.Sprintf("extracting page %d", page))
		result, err := runExtractionOnCurrentPage(ctx, container, fields, mapping)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				stoppedReason = "time_budget_reached"
				break
			}
			return paginatedCrawlResponse{}, nil, err
		}

		var currentURL string
		if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				stoppedReason = "time_budget_reached"
				break
			}
			return paginatedCrawlResponse{}, nil, err
		}

		// step 36 — derived locally from this loop's own already-in-scope
		// container/fields rather than threaded as a separate parameter
		// through every caller: this is the one place per page that
		// actually needs it, and container/fields are already exactly
		// what expectedSelectorsFromFields wants.
		cf, err := detectCloudflareChallenge(ctx, expectedSelectorsFromFields(container, fields))
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				stoppedReason = "time_budget_reached"
				break
			}
			return paginatedCrawlResponse{}, nil, err
		}

		pages = append(pages, pageExtractResult{
			URL:                currentURL,
			Results:            result.Results,
			Items:              result.Items,
			NotFound:           result.NotFound,
			CloudflareDetected: cf.Detected,
			CloudflareReason:   cf.Reason,
		})

		if captureHTML {
			var html string
			if err := chromedp.Run(ctx, chromedp.OuterHTML("html", &html)); err != nil {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					stoppedReason = "time_budget_reached"
					break
				}
				return paginatedCrawlResponse{}, nil, err
			}
			// Step 40 — deliberately NOT truncated to MaxHTMLBytes (crawl.go):
			// that limit exists to protect an LLM's context budget on the
			// live /crawl response; this HTML never reaches one (see this
			// function's own top comment) and, since step 39, is written
			// straight to its own file on disk, not held in a response
			// payload. Clipping it here was carried over unexamined from
			// crawl.go's own constant and was clipping most real pages'
			// captured HTML in the log to well under half its real size.
			// See plan/ai/tools/browser/step-40-log-html-truncation-fix.md.
			pageHTML = append(pageHTML, html)
		}

		// A Cloudflare challenge/block on this page normally means the
		// extraction above ran against interstitial DOM, not real
		// content — but not always: some sites embed a permanently-
		// present, non-blocking Cloudflare/Turnstile trust badge
		// (e.g. a small "protected by Cloudflare" widget) even on
		// their real, fully-loaded page, which detectCloudflareChallenge's
		// own heuristics can't distinguish from an active block —
		// confirmed directly against a real site that never once
		// reported "clear" even while genuinely showing real content.
		// foundRealContent corroborates cf.Detected against the
		// extraction that just ran on THIS SAME page: if it actually
		// matched real data, trust that over the heuristic rather than
		// discarding an already-correct result. Only when extraction
		// ALSO found nothing is this treated as a genuine block — on
		// the very first page that's a hard error (same contract as
		// crawlPage's own unresolved-challenge case); on a later page,
		// real items were already collected on earlier pages, so stop
		// pagination but keep and return what's already there,
		// distinctly flagged, rather than silently falling through to
		// "no_next_link" (today's misleading bug: the interstitial page
		// has no next-page selector either, so the loop used to just
		// end as if pagination were naturally exhausted).
		//
		// In grouped mode (container set), NotFound is NOT a valid
		// signal on its own — a real, reproduced bug: extract.js's own
		// container loop (`for (var c = 0; c < containers.length...)`)
		// never runs at all when the container selector matches ZERO
		// elements (exactly what happens on a Cloudflare interstitial,
		// which has no `.job-listing`-style divs), so notFoundSeen stays
		// completely empty and NotFound comes back as `[]` — which
		// len(result.NotFound) < len(fields) then misread as "0 missing
		// fields out of 5, so content was found," exactly backwards.
		// Only len(result.Items) can tell "found vs. blocked" apart in
		// grouped mode; NotFound only means something in flat mode,
		// where extractFieldsFrom(document, ...) always runs regardless
		// of what matched.
		if cf.Detected {
			foundRealContent := len(result.Items) > 0
			if container == "" {
				foundRealContent = foundRealContent || len(result.NotFound) < len(fields)
			}
			if len(pages) == 1 {
				if !foundRealContent {
					return paginatedCrawlResponse{}, nil, newCloudflareUnresolvedError(cf.Reason)
				}
			} else if !foundRealContent {
				stoppedReason = "cloudflare_blocked"
				blockedReason = cf.Reason
				break
			}
			// else: real data was found on this page despite cf.Detected
			// — already recorded via this page's own CloudflareDetected/
			// CloudflareReason fields above; keep going rather than
			// stopping or failing.
		}

		if page >= effectiveMaxPages {
			stoppedReason = "max_pages_reached"
			break
		}

		var nextExists bool
		existsJS := fmt.Sprintf("!!document.querySelector(%s)", jsStringLiteral(nextSelector))
		if err := chromedp.Run(ctx, chromedp.Evaluate(existsJS, &nextExists)); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				stoppedReason = "time_budget_reached"
				break
			}
			return paginatedCrawlResponse{}, nil, err
		}
		if !nextExists {
			stoppedReason = "no_next_link"
			break
		}

		if err := chromedp.Run(ctx, chromedp.Click(nextSelector), chromedp.Sleep(SettleDelay)); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				stoppedReason = "time_budget_reached"
				break
			}
			stoppedReason = "click_failed"
			break
		}
		// step 45 — this click just navigated whichever session ctx
		// belongs to (the shared headless one, when called from
		// performPaginatedCrawl; the ephemeral headed fallback,
		// harmless-but-unnecessary to clear, when called from
		// performPaginatedCrawlWithNormalSession) somewhere fetch_page_html's
		// own cache has no idea about. Unconditionally invalidating
		// here — rather than only when this is provably the shared
		// session — is the safe default: a real, previously-missed gap
		// in crawlPage's own "crawlPage is the only thing that
		// navigates the shared session" assumption, caught in this
		// step's own final review. See
		// plan/ai/tools/browser/step-45-fetch-html-caching-plan.md.
		shared.LastHeadlessFetchURL = ""

		var urlAfterClick string
		if err := chromedp.Run(ctx, chromedp.Location(&urlAfterClick)); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				stoppedReason = "time_budget_reached"
				break
			}
			return paginatedCrawlResponse{}, nil, err
		}
		if urlAfterClick == currentURL {
			stoppedReason = "url_unchanged"
			break
		}
	}

	return paginatedCrawlResponse{
		Pages:             pages,
		StoppedReason:     stoppedReason,
		PagesVisited:      len(pages),
		RequestedMaxPages: requestedMaxPages,
		EffectiveMaxPages: effectiveMaxPages,
		BlockedReason:     blockedReason,
	}, pageHTML, nil
}

// performPaginatedCrawl runs runPaginatedCrawlLoop against the shared
// headless session. The headless attempt itself is scoped to an inner
// function so shared.Mu is released the moment it concludes — BEFORE
// performPaginatedCrawlWithNormalSession (a wholly separate browser,
// guarded by its own shared.NormalSessionMu) ever starts. Without this,
// shared.Mu would stay held for the fallback's own up-to-150s retry
// budget (step 26) too, serializing every other crawl request behind
// one slow, unrelated Cloudflare fallback — same fix step 25 already
// applied to crawlPage, for the same reason. On a page-1-blocked
// result, captures the shared session's own current URL (still
// holding shared.Mu at that point) before releasing it — the
// fallback, a fresh separate browser, has no other way to know what
// page it should have been looking at, since crawl_paginated itself
// never takes a URL. See
// plan/ai/tools/browser/step-26-headed-fallback-for-paginated-crawl.md.
//
// url (step 37) is optional — when set, navigates the shared session
// there FIRST, still inside this same shared.Mu acquisition, before
// ever running the cache check or the extraction loop. This is what
// makes navigate-then-extract one atomic operation instead of two
// separately-locked HTTP calls: verified directly that Career's own
// prior two-call approach (a separate POST /crawl, then this endpoint)
// released shared.Mu completely in between, letting a DIFFERENT
// concurrent crawl's own navigate land in the gap and silently redirect
// this call's own extraction to the wrong page. See
// plan/ai/tools/browser/step-37-atomic-navigate-and-extract.md.
func performPaginatedCrawl(url, container string, fields []extractField, mapping map[string]string, nextSelector string, requestedMaxPages, effectiveMaxPages int, captureHTML bool, requestID string) (paginatedCrawlResponse, []string, error) {
	// step 63.2 — registered before anything else in this call, same
	// reasoning as crawlPage's own identical registration (crawl.go):
	// cancellable via cancelCrawl(requestID) from the instant this call
	// arrives, even before shared.Mu is ever touched. A no-op when
	// requestID is "" (the AI's own crawl_paginated calls never set
	// one). See plan/ai/tools/career/step-63-stop-crawling-now.md.
	reqCtx, reqCancel := context.WithCancel(context.Background())
	registerCrawlCancel(requestID, reqCancel)
	defer func() {
		unregisterCrawlCancel(requestID)
		reqCancel()
	}()

	result, pageHTML, blockedURL, err := func() (paginatedCrawlResponse, []string, string, error) {
		if err := shared.EnsureSharedSession(); err != nil {
			return paginatedCrawlResponse{}, nil, "", err
		}
		shared.Mu.Lock()
		defer shared.Mu.Unlock()

		// step 63.2 — this request may have been canceled while it sat
		// queued waiting for shared.Mu — bail out immediately after
		// acquiring the lock, before any real chromedp work, same
		// reasoning as crawlPage's own identical check (crawl.go).
		if reqCtx.Err() != nil {
			return paginatedCrawlResponse{}, nil, "", reqCtx.Err()
		}

		// step 47.2/47.3 — fail fast on a tab left wedged by a previous,
		// unrelated caller instead of discovering it only after burning
		// paginatedCrawlTimeout on a navigate that was never going to
		// complete, and replace the wedged tab immediately (still
		// holding shared.Mu) so the NEXT caller gets a fresh, healthy
		// session instead of inheriting the same wedge. blockedURL is
		// deliberately left "" here — the dispatch below
		// (errors.As(err, &cfErr) && blockedURL != "") only routes to
		// the headed Cloudflare fallback when both are true, so this
		// wedge error (a *crawlError, same as the Cloudflare one) can
		// never accidentally trigger it.
		if err := shared.ProbeSessionLiveness(shared.Ctx); err != nil {
			recreateErr := shared.RecreateSharedSessionLocked()
			return paginatedCrawlResponse{}, nil, "", NewSessionWedgedError(err, recreateErr)
		}

		ctx, cancel := context.WithTimeout(shared.Ctx, paginatedCrawlTimeout)
		defer cancel()
		// step 63.2 — links this call's own ctx to reqCtx, same pattern
		// as crawlPage's own identical goroutine (crawl.go) — every
		// chromedp.Run call below (the navigate here, and everything
		// inside runPaginatedCrawlLoop) already respects ctx.Done()
		// internally, so no changes are needed inside that loop itself.
		go func() {
			select {
			case <-reqCtx.Done():
				cancel()
				// step 63.2 — see crawl.go's identical comment: without
				// this, Chrome itself keeps loading after this call has
				// already given up waiting.
				stopBrowserLoad(shared.Ctx)
			case <-ctx.Done():
			}
		}()

		if url != "" {
			setCrawlPhase(requestID, phaseNavigating, "navigating to "+url)
			if err := chromedp.Run(ctx, chromedp.Navigate(url), chromedp.Sleep(SettleDelay)); err != nil {
				return paginatedCrawlResponse{}, nil, "", classifyCancellation(err, reqCtx, shared.Ctx)
			}
			// step 45 — this is the shared session (performPaginatedCrawl
			// only ever runs against shared.Ctx), navigated outside
			// crawlPage entirely; fetch_page_html's own cache check needs
			// to know where the session actually is now. Set precisely
			// (not just cleared) since the destination is known exactly,
			// unlike runPaginatedCrawlLoop's own pagination-click
			// invalidation below, which doesn't know what URL a click
			// landed on until after this point in the surrounding call
			// graph.
			shared.LastHeadlessFetchURL = url
		}

		// step 29 — a known-Cloudflare domain skips straight to the
		// headed fallback. Step 37 — when url was provided, use it
		// directly for this check (we just navigated there, under this
		// same lock, so it's authoritative); only fall back to reading
		// the session's own current location when no url was given at
		// all (the AI's own crawl_paginated calls, which still operate
		// on whatever page is already loaded). A known-Cloudflare
		// domain skips straight to the headed fallback via the same
		// errors.As dispatch below, reusing a synthetic crawlError
		// rather than duplicating the fallback-invocation logic —
		// "known-cloudflare-cache" is deliberately distinct from
		// cloudflareCheck's own detection reasons, so a human reading a
		// recorded reason can tell "we actually saw it this time" apart
		// from "we skipped straight to headed because of a past
		// detection." See
		// plan/ai/tools/browser/step-29-skip-headless-for-known-cloudflare-domains.md.
		checkURL := url
		if checkURL == "" {
			_ = chromedp.Run(ctx, chromedp.Location(&checkURL))
		}
		if checkURL != "" {
			if domain, hostErr := hostnameOf(checkURL); hostErr == nil {
				if known, cacheErr := isDomainKnownCloudflare(domain); cacheErr == nil && known {
					return paginatedCrawlResponse{}, nil, checkURL, newCloudflareUnresolvedError("known-cloudflare-cache")
				}
			}
		}

		result, pageHTML, err := runPaginatedCrawlLoop(ctx, container, fields, mapping, nextSelector, requestedMaxPages, effectiveMaxPages, captureHTML, requestID)

		var cfErr *crawlError
		if !errors.As(err, &cfErr) {
			return result, pageHTML, "", classifyCancellation(err, reqCtx, shared.Ctx)
		}

		var currentURL string
		if locErr := chromedp.Run(ctx, chromedp.Location(&currentURL)); locErr != nil {
			// Can't recover a URL to fall back with — surface the
			// original Cloudflare error rather than the location
			// lookup's own failure.
			return paginatedCrawlResponse{}, nil, "", err
		}
		// step 28 — best-effort: a failed cache write must never turn
		// an otherwise-working fallback into a failure.
		if domain, hostErr := hostnameOf(currentURL); hostErr == nil {
			_ = recordCloudflareDomain(domain, cfErr.Reason)
		}
		return paginatedCrawlResponse{}, nil, currentURL, err
	}()

	var cfErr *crawlError
	if errors.As(err, &cfErr) && blockedURL != "" {
		return performPaginatedCrawlWithNormalSession(reqCtx, blockedURL, container, fields, mapping, nextSelector, requestedMaxPages, effectiveMaxPages, captureHTML, requestID)
	}
	return result, pageHTML, err
}

// maxHumanWaitRounds bounds how many times this function will re-enter
// waitForHumanToClearCloudflare after a "solved" declaration turns out
// to be premature — see this function's own doc comment for why that
// can happen. Belt-and-suspenders only: normalSessionPaginatedCrawlTimeout's
// own ctx deadline already bounds total elapsed time regardless of how
// many rounds run, so this just prevents pathologically looping
// through many fast, cheap disagreements instead of stopping at a
// clear, small number.
const maxHumanWaitRounds = 3

// performPaginatedCrawlWithNormalSession retries the whole paginated
// crawl in a freshly launched headed Chrome instance, navigated to
// blockedURL (the shared headless session's own last-known location)
// — called only after the headless attempt already failed to clear
// the same challenge on page 1. If the headed session's own automated
// wait (inside runPaginatedCrawlLoop's own per-page Cloudflare check)
// also fails, waits for a human via waitForHumanToClearCloudflare,
// then runs the full loop again from page 1 — letting pagination
// proceed normally through page 2..N in the same now-cleared headed
// session, not just page 1. Always tears the headed instance down
// before returning. See
// plan/ai/tools/browser/step-24-headed-chrome-cloudflare-fallback.md,
// step-25-human-assisted-cloudflare-retry.md, and
// step-26-headed-fallback-for-paginated-crawl.md.
//
// waitForHumanToClearCloudflare can declare "solved" without
// detectCloudflareChallenge's own agreement (its own HTML-size rule —
// see that function's own doc comment) — deliberately, since
// detectCloudflareChallenge itself can stay wrongly convinced a
// challenge is still active on a genuinely already-solved page. That
// means the very next line below,
// runPaginatedCrawlLoop's own independent per-page Cloudflare check,
// can occasionally disagree with a "solved" verdict that was in fact
// premature (a residual widget still settling, a transitional
// re-render). A real, reproduced case: waitForHumanToClearCloudflare
// declared solved, and the immediate re-extraction attempt hit the
// same challenge again seconds later — with nothing here to retry, that
// used to surface as an immediate, unrecoverable hard failure straight
// to the caller, discarding the entire up-to-32-minute fallback budget
// over what was really just one bad tick. Bounded at maxHumanWaitRounds
// total rounds — not required to agree with detectCloudflareChallenge
// on every occasion (that would undo the fix this function's own
// waitForHumanToClearCloudflare change exists for), just given another
// chance to actually clear when the first "solved" call turns out to
// have been wrong.
func performPaginatedCrawlWithNormalSession(reqCtx context.Context, blockedURL, container string, fields []extractField, mapping map[string]string, nextSelector string, requestedMaxPages, effectiveMaxPages int, captureHTML bool, requestID string) (paginatedCrawlResponse, []string, error) {
	shared.NormalSessionMu.Lock()
	defer shared.NormalSessionMu.Unlock()

	// step 63.2 — bail out before ever launching a headed Chrome
	// process if this request was already canceled while queued
	// waiting for shared.NormalSessionMu, same reasoning as
	// crawlWithNormalSession's own identical check (crawl.go).
	if reqCtx.Err() != nil {
		return paginatedCrawlResponse{}, nil, reqCtx.Err()
	}

	ctx, cancels, err := shared.StartSharedNormalSession()
	if err != nil {
		return paginatedCrawlResponse{}, nil, fmt.Errorf("normal-session fallback: failed to start: %w", err)
	}
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
	}()

	baseCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, normalSessionPaginatedCrawlTimeout)
	defer cancel()
	// step 63.2 — same "cancel A when B cancels" link as
	// performPaginatedCrawl's own headless attempt, above: a Stop click
	// now reaches this up-to-32-minute headed/human-solve wait too.
	go func() {
		select {
		case <-reqCtx.Done():
			cancel()
			stopBrowserLoad(baseCtx)
		case <-ctx.Done():
		}
	}()

	setCrawlPhase(requestID, phaseNavigating, "navigating to "+blockedURL)
	if err := chromedp.Run(ctx, chromedp.Navigate(blockedURL), chromedp.Sleep(SettleDelay)); err != nil {
		return paginatedCrawlResponse{}, nil, classifyCancellation(err, reqCtx, shared.Ctx)
	}

	result, pageHTML, err := runPaginatedCrawlLoop(ctx, container, fields, mapping, nextSelector, requestedMaxPages, effectiveMaxPages, captureHTML, requestID)

	var cfErr *crawlError
	if !errors.As(err, &cfErr) {
		// step 63.3 — this function's own ctx isn't shared.Ctx, but
		// classifyCancellation's shared.Ctx check still correctly
		// covers the (rare) case where the shared headless session
		// ALSO died at the same moment.
		return result, pageHTML, classifyCancellation(err, reqCtx, shared.Ctx)
	}

	for round := 1; round <= maxHumanWaitRounds; round++ {
		cleared, waitErr := waitForHumanToClearCloudflare(ctx, requestID, cfErr.Reason)
		if waitErr != nil {
			return paginatedCrawlResponse{}, nil, classifyCancellation(waitErr, reqCtx, shared.Ctx)
		}
		if !cleared {
			return paginatedCrawlResponse{}, nil, cfErr
		}

		result, pageHTML, err = runPaginatedCrawlLoop(ctx, container, fields, mapping, nextSelector, requestedMaxPages, effectiveMaxPages, captureHTML, requestID)
		if !errors.As(err, &cfErr) {
			return result, pageHTML, classifyCancellation(err, reqCtx, shared.Ctx)
		}
		// "Solved" was premature — cfErr is now this round's own fresh
		// re-detection. Report it distinctly before looping back, so a
		// human reading crawl_runs.log can see this happened rather
		// than silently repeating "Cloudflare challenge detected"
		// text that looks unchanged from the round before.
		setCrawlPhase(requestID, phaseAwaitingHumanChallenge, fmt.Sprintf(
			"page reported solved but Cloudflare challenge reappeared (%s) — waiting again (round %d/%d)",
			cfErr.Reason, round, maxHumanWaitRounds,
		))
	}
	return paginatedCrawlResponse{}, nil, cfErr
}

// jsStringLiteral marshals a Go string into a JSON string literal for
// safe inline embedding in a chromedp.Evaluate expression — the same
// technique extract.go's own payload marshaling already relies on
// (JSON string escaping is valid JS string escaping), applied here to
// a single value rather than a whole payload object.
func jsStringLiteral(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

type crawlPaginatedArgs struct {
	Instructions string `json:"instructions" jsonschema:"a YAML document describing fields to extract (same shape as extract_page_data) plus a pagination block, and — for a LISTING page — a top-level container. BEFORE writing this, check whether the page lists multiple similar items at once (a search-results/job-listing page, the common case) or describes a single item. Single-item example:\nfields:\n  - label: title\n    selector: h1\npagination:\n  nextSelector: a.next-page\n  maxPages: 5\nmaxPages counts the currently loaded page as page 1. Never navigates to a starting URL itself — call fetch_page_html first. LISTING-page example (use this whenever more than one field describes the same repeating item, e.g. a job listing's own title, company, and link — this is the default correct approach for a listing page, not only a fix for when something looks wrong):\ncontainer: \".job-result\"\nfields:\n  - label: title\n    selector: h2\n  - label: url\n    selector: a\n    attribute: href\npagination:\n  nextSelector: a.next-page\n  maxPages: 5\nWithout container on a listing page, fields describing multiple items are returned as separate arrays (in results) that may NOT actually correspond position-for-position to the same real item — with it, the result is one correctly-grouped object per item (in items). Optionally add a top-level mapping ({sourceLabel: targetKey}) to rename fields to specific output keys before they're returned — e.g. a consuming tool expects title/url/company but this page's own natural fields are better labeled job_title/link/employer:\ncontainer: \".job-result\"\nfields:\n  - label: job_title\n    selector: h2\n  - label: employer\n    selector: .company\nmapping:\n  job_title: title\n  employer: company\npagination:\n  nextSelector: a.next-page\n  maxPages: 5"`
}

// RegisterCrawlPaginated adds the crawl_paginated MCP tool — thin,
// like the other MCP-facing registrations: parses the YAML
// instructions, validates, builds the internal JSON request, and
// calls shared.CallSibling.
func RegisterCrawlPaginated(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "crawl_paginated",
		Description: "Extract fields from the currently loaded page, then follow a pagination control and repeat, up to a maximum number of pages — instructed via a YAML document (fields + pagination). Read-only; submits nothing. " +
			"Before writing the instructions, check whether the page LISTS MULTIPLE similar items at once or describes just one. For a listing page, always add a top-level container selector for one item's own repeating wrapping element — the default correct approach for a listing page, not a fallback — so the result is one correctly-grouped object per item (in `items`) instead of separate same-length arrays (in `results`) that may NOT actually line up. " +
			"Optionally add a top-level mapping to rename extracted fields to specific output keys — e.g. a consuming tool expects title/url/company but this page's own natural fields are better labeled job_title/link/employer. " +
			"If a Cloudflare challenge or block is hit on the very first page, this automatically retries in a normal (non-headless) browser window and can wait up to about 30 minutes for a person to notice and solve the challenge there before giving up — meaning this call can take up to roughly 30 minutes in that case, almost certainly longer than this AI tool-calling session's own timeout, so a Cloudflare block on page 1 is effectively only recoverable through this path by a human watching for the window, not by an AI call waiting on the result — and only then fails with a cloudflare_challenge_unresolved error if it's still blocked. If it's hit on a later page (no such retry there), pagination stops and the response's stoppedReason is \"cloudflare_blocked\" (with blockedReason explaining why) — pages already collected before the block are still returned.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args crawlPaginatedArgs) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(args.Instructions) == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "instructions is required — a YAML document with fields and a pagination block"}},
				IsError: true,
			}, nil, nil
		}

		var parsed paginatedCrawlInstructions
		if err := yaml.Unmarshal([]byte(args.Instructions), &parsed); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid YAML instructions: %v", err)}},
				IsError: true,
			}, nil, nil
		}
		if len(parsed.Fields) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "instructions must include at least one field"}},
				IsError: true,
			}, nil, nil
		}
		for _, f := range parsed.Fields {
			if f.Label == "" || f.Selector == "" {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "every field requires both a label and a selector"}},
					IsError: true,
				}, nil, nil
			}
		}
		if parsed.Pagination.NextSelector == "" || parsed.Pagination.MaxPages < 1 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "instructions must include a pagination block with nextSelector and a positive maxPages"}},
				IsError: true,
			}, nil, nil
		}
		if err := validateMapping(parsed.Fields, parsed.Mapping); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		effectiveMaxPages := parsed.Pagination.MaxPages
		if effectiveMaxPages > maxAllowedPaginationPages {
			effectiveMaxPages = maxAllowedPaginationPages
		}

		reqBody, err := json.Marshal(paginatedCrawlRequest{
			Container:         parsed.Container,
			Fields:            parsed.Fields,
			Mapping:           parsed.Mapping,
			NextSelector:      parsed.Pagination.NextSelector,
			RequestedMaxPages: parsed.Pagination.MaxPages,
			EffectiveMaxPages: effectiveMaxPages,
		})
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to build request: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		respBody, err := shared.CallSibling("crawl-paginated", reqBody)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		var result paginatedCrawlResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to parse crawl_paginated response: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		text, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to format result: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}}, nil, nil
	})
}
