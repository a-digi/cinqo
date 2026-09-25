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

// paginatedAutoWaitReserve is how much of ctx's own deadline page 1's
// automatic Cloudflare wait (runPaginatedCrawlLoop) leaves for the
// extraction/pagination that follows once it clears.
const paginatedAutoWaitReserve = 10 * time.Second

// paginatedCrawlTimeoutWithPacing replaces paginatedCrawlTimeout
// whenever rateLimitKey is set (portal_pacing.go) — i.e. only ever for
// Career's own deterministic calls, never the AI-driven path (which
// never sets rateLimitKey and is completely unaffected by this
// constant). paginatedCrawlTimeout's own 35s budget is sized for the
// AI path's hard external ceiling (the MCP host's own 45s invoke
// timeout) and has no slack at all for maxAllowedPaginationPages pages
// each also waiting up to maxPortalCrawlDelay between them — a real
// conflict, not a hypothetical: without this, pacing would make a
// multi-page listing crawl reliably stop after 1 page with
// "time_budget_reached" the moment the first pacing wait alone
// approached this budget. Career's own outbound HTTP call has no such
// external ceiling (crawlNowHTTPClient's own 100-minute timeout,
// crawl_now.go), so this can be generous: sized for
// maxAllowedPaginationPages pages at up to maxPortalCrawlDelay pacing
// plus real navigate/extract time each, with margin.
const paginatedCrawlTimeoutWithPacing = 10 * time.Minute

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
	// immediate check (challengeTracker.state, challenge_resolution.go) per
	// page. Only page 1 additionally gets the automatic wait-then-poll
	// (challengeTracker, challenge_resolution.go) when it's blocked —
	// this tool's own per-page time budget (paginatedCrawlTimeout) has
	// far less slack for an extra multi-second wait on every page
	// across up to maxAllowedPaginationPages pages. See this step's own open question 1,
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
	// RateLimitKey (step XX) is optional — when set, a randomized
	// 10-30s pacing delay (portal_pacing.go) is enforced before every
	// page fetch this call performs, shared across every OTHER call
	// using the same key (Career sets this to the owning portal's own
	// id, across every link belonging to it). Absent for every
	// AI-driven call — crawl_paginated's own MCP-facing args never set
	// this, so that path is completely unaffected. See
	// plan/ai/tools/career/step-XX-portal-crawl-pacing.md.
	RateLimitKey string `json:"rateLimitKey,omitempty"`
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

	// r.Context() — see CrawlHandler's identical comment (crawl.go).
	result, pageHTML, err := performPaginatedCrawl(r.Context(), body.URL, body.Container, body.Fields, body.Mapping, body.NextSelector, body.RequestedMaxPages, body.EffectiveMaxPages, captureHTML, body.RequestID, body.RateLimitKey)
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
// loopTab is what runPaginatedCrawlLoop needs to know about the tab it
// runs in: its challenge signals (watched from before navigation),
// what to invalidate when a pagination click navigates it (onNavigate —
// only the primary tab has fetch-cache state to clear; nil elsewhere),
// and how long page 1 may wait for a challenge to clear automatically
// (primaryAutoWait on the primary tab, cloudflareAutoWait elsewhere).
type loopTab struct {
	signals    *challengeSignals
	onNavigate func()
	autoWait   time.Duration
}

func runPaginatedCrawlLoop(ctx context.Context, tab loopTab, args paginatedArgs) (paginatedCrawlResponse, []string, error) {
	signals := tab.signals
	container, fields, mapping, nextSelector := args.container, args.fields, args.mapping, args.nextSelector
	requestedMaxPages, effectiveMaxPages, captureHTML := args.requestedMaxPages, args.effectiveMaxPages, args.captureHTML
	requestID, rateLimitKey := args.requestID, args.rateLimitKey
	pages := make([]pageExtractResult, 0, effectiveMaxPages)
	var pageHTML []string
	if captureHTML {
		pageHTML = make([]string, 0, effectiveMaxPages)
	}
	stoppedReason := ""
	blockedReason := ""
	// tracker checks every page against what Cloudflare sent for it
	// (challenge_resolution.go); page 1's automatic wait runs at most
	// once per call (autoWaited) — the same wait crawlPage's own
	// navigation gets.
	tracker, err := newChallengeTracker(ctx, signals, expectedSelectorsFromFields(container, fields))
	if err != nil {
		return paginatedCrawlResponse{}, nil, err
	}
	autoWaited := false

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
		cf, err := tracker.state(ctx)
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
			CloudflareDetected: cf.challenge || cf.blocked,
			CloudflareReason:   cf.reason,
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

		// A Cloudflare challenge/block on this page means the extraction
		// above ran against challenge DOM, not real content. foundRealContent
		// corroborates that against the extraction itself: if it matched
		// real data, keep it. Only when extraction ALSO found nothing is
		// this treated as a genuine block — on the very first page that's
		// the automatic wait (then a hard error, same contract as
		// crawlPage's own unresolved-challenge case); on a later page,
		// real items were already collected on earlier pages, so stop
		// pagination but keep and return what's already there, distinctly
		// flagged, rather than silently falling through to "no_next_link".
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
		if cf.blocked && len(pages) == 1 {
			return paginatedCrawlResponse{}, nil, newCloudflareBlockedError(cf.reason)
		}
		if cf.challenge || cf.blocked {
			foundRealContent := len(result.Items) > 0
			if container == "" {
				foundRealContent = foundRealContent || len(result.NotFound) < len(fields)
			}
			if len(pages) == 1 {
				if !foundRealContent {
					if autoWaited {
						// Already waited once on this call — a challenge
						// that reappeared after clearing is left to the
						// caller (headed fallback / human wait).
						return paginatedCrawlResponse{}, nil, newCloudflareUnresolvedError(cf.reason, tracker)
					}
					autoWaited = true
					outcome, err := tracker.await(ctx, challengeModeAuto, autoWaitBudget(ctx, tab.autoWait, paginatedAutoWaitReserve), requestID, cf.reason)
					if err != nil {
						return paginatedCrawlResponse{}, nil, err
					}
					if outcome.HardBlocked {
						return paginatedCrawlResponse{}, nil, newCloudflareBlockedError(outcome.Reason)
					}
					if !outcome.Cleared {
						return paginatedCrawlResponse{}, nil, newCloudflareUnresolvedError(outcome.Reason, tracker)
					}
					// Cleared — discard page 1's interstitial result and
					// extract it again from the real page.
					pages = pages[:0]
					if captureHTML {
						pageHTML = pageHTML[:0]
					}
					page = 0
					continue
				}
			} else if !foundRealContent {
				stoppedReason = "cloudflare_blocked"
				blockedReason = cf.reason
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

		// portal_pacing.go — a no-op when rateLimitKey is "" (every
		// AI-driven call). Checked here, right before clicking through
		// to the NEXT page, not at the top of the loop — page 1 was
		// already paced by performPaginatedCrawl's own pre-navigate
		// call, and there's nothing to pace an ALREADY-stopped loop
		// for (the break above already handles "no more pages").
		awaitPortalCrawlPacing(ctx, rateLimitKey)
		if ctx.Err() != nil {
			stoppedReason = "time_budget_reached"
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
		// step 45 — this click just navigated the tab somewhere
		// fetch_page_html's own cache has no idea about; only the primary
		// tab has that state (see loopTab). See
		// plan/ai/tools/browser/step-45-fetch-html-caching-plan.md.
		if tab.onNavigate != nil {
			tab.onNavigate()
		}

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

// paginatedResult bundles runPaginatedCrawlLoop's own two results so a
// paginated attempt fits escalateChallenge's single result type.
type paginatedResult struct {
	resp     paginatedCrawlResponse
	pageHTML []string
}

// performPaginatedCrawl dispatches one /crawl-paginated call.
//
// url (step 37) set — a self-contained navigate-then-extract (Career's
// own crawls, extract_from_url): runs entirely in its own headless
// worker tab (performPaginatedCrawlForURL), never touching shared.Mu or
// the primary tab. One tab from navigation to the last page is what
// keeps it atomic: no other caller can navigate that tab in between
// (the real, confirmed step-37 bug of Career's former two-call
// approach), and a paced multi-page crawl (up to
// paginatedCrawlTimeoutWithPacing) no longer blocks every other
// operation for its whole duration.
//
// url empty — the AI's own crawl_paginated, which extracts from
// whatever page the primary tab already shows, so it runs there under
// shared.Mu. A page-1 challenge that doesn't clear within
// primaryAutoWait is handed to escalateChallenge (worker, then headed)
// at the primary tab's current URL, and shared.Mu is released first.
//
// AI calls (no requestID: crawl_paginated, extract_from_url) are
// bounded by aiCallBudget and never go headed — see
// challenge_resolver.go's own top comment. parent is the HTTP request's
// own context: when the caller disconnects, the crawl stops.
func performPaginatedCrawl(parent context.Context, url, container string, fields []extractField, mapping map[string]string, nextSelector string, requestedMaxPages, effectiveMaxPages int, captureHTML bool, requestID, rateLimitKey string) (paginatedCrawlResponse, []string, error) {
	// step 63.2 — registered before anything else in this call, same
	// reasoning as crawlPage's own identical registration (crawl.go):
	// cancellable via cancelCrawl(requestID) from the instant this call
	// arrives. A no-op when requestID is "" (the AI's own
	// crawl_paginated calls never set one). See
	// plan/ai/tools/career/step-63-stop-crawling-now.md.
	reqCtx, reqCancel := context.WithCancel(parent)
	registerCrawlCancel(requestID, reqCancel)
	defer func() {
		unregisterCrawlCancel(requestID)
		reqCancel()
	}()

	args := paginatedArgs{container, fields, mapping, nextSelector, requestedMaxPages, effectiveMaxPages, captureHTML, requestID, rateLimitKey, aiDeadline(requestID, time.Now())}
	if url != "" {
		return performPaginatedCrawlForURL(reqCtx, url, args)
	}

	result, pageHTML, blockedURL, err := func() (paginatedCrawlResponse, []string, string, error) {
		if err := shared.EnsureSharedSession(); err != nil {
			return paginatedCrawlResponse{}, nil, "", err
		}
		shared.Mu.Lock()
		defer shared.Mu.Unlock()

		// step 63.2 — this request may have been canceled while it sat
		// queued waiting for shared.Mu — bail out before any real
		// chromedp work, same as crawlPage's own identical check.
		if reqCtx.Err() != nil {
			return paginatedCrawlResponse{}, nil, "", reqCtx.Err()
		}

		// step 47.2/47.3 — fail fast on a tab left wedged by a previous,
		// unrelated caller, and replace it immediately (still holding
		// shared.Mu). blockedURL stays "" so this wedge error (a
		// *crawlError too) can never reach the Cloudflare fallback.
		if err := shared.ProbeSessionLiveness(shared.Ctx); err != nil {
			recreateErr := shared.RecreateSharedSessionLocked()
			return paginatedCrawlResponse{}, nil, "", NewSessionWedgedError(err, recreateErr)
		}

		timeout := paginatedCrawlTimeout
		if rateLimitKey != "" {
			timeout = paginatedCrawlTimeoutWithPacing
		}
		ctx, cancel := context.WithTimeout(shared.Ctx, capTimeout(timeout, args.deadline))
		defer cancel()
		linkRequestCancel(reqCtx, ctx, cancel, shared.Ctx)

		// step 45 — a pagination click navigates the primary tab
		// somewhere fetch_page_html's own cache has no idea about.
		tab := loopTab{
			signals:    watchChallengeSignals(ctx),
			onNavigate: func() { shared.LastHeadlessFetchURL = "" },
			autoWait:   primaryAutoWait,
		}
		result, pageHTML, err := runPaginatedCrawlLoop(ctx, tab, args)

		// Only an unresolved challenge goes to the fallback — a block
		// page (cloudflare_blocked) or a wedged session is returned as-is.
		var cfErr *crawlError
		if !errors.As(err, &cfErr) || cfErr.Code != codeCloudflareUnresolved {
			return result, pageHTML, "", classifyCancellation(err, reqCtx, shared.Ctx)
		}

		var currentURL string
		if locErr := chromedp.Run(ctx, chromedp.Location(&currentURL)); locErr != nil {
			// Can't recover a URL to fall back with — surface the
			// original Cloudflare error rather than the location
			// lookup's own failure.
			return paginatedCrawlResponse{}, nil, "", err
		}
		leavePrimaryTabLocked(shared.Ctx, func() { _ = shared.RecreateSharedSessionLocked() })
		return paginatedCrawlResponse{}, nil, currentURL, err
	}()

	var cfErr *crawlError
	if !errors.As(err, &cfErr) || cfErr.Code != codeCloudflareUnresolved || blockedURL == "" {
		return result, pageHTML, err
	}
	if requestID == "" {
		// AI call: one headless worker attempt within aiCallBudget, then
		// — crawl_paginated operates on "the current page" — put the
		// primary tab back on it for the AI's next call.
		r, err := paginatedCrawlOnWorkerTab(reqCtx, blockedURL, args)
		recordIfUnresolved(blockedURL, err)
		if err == nil {
			syncPrimaryTabAsync(blockedURL, crawlResponse{}, true)
		}
		return r.resp, r.pageHTML, err
	}
	r, err := escalateChallenge(reqCtx, blockedURL, requestID, true,
		func() (paginatedResult, error) { return paginatedCrawlOnWorkerTab(reqCtx, blockedURL, args) },
		func() (paginatedResult, error) {
			return performPaginatedCrawlWithNormalSession(reqCtx, blockedURL, args)
		},
	)
	return r.resp, r.pageHTML, err
}

// paginatedArgs is everything about a paginated crawl except where it
// runs — passed unchanged to every tab it may end up running in.
type paginatedArgs struct {
	container         string
	fields            []extractField
	mapping           map[string]string
	nextSelector      string
	requestedMaxPages int
	effectiveMaxPages int
	captureHTML       bool
	requestID         string
	rateLimitKey      string
	// deadline is the AI call's own budget end (aiDeadline); zero for
	// Career's own calls.
	deadline time.Time
}

// performPaginatedCrawlForURL runs a url-bearing paginated crawl in a
// headless worker tab, escalating to escalateChallenge (headed, after
// any other same-domain resolution) only when page 1 stays behind a
// challenge. step 29 — a known-Cloudflare domain skips the headless
// attempt unless the headless browser already holds a cf_clearance for
// url; "known-cloudflare-cache" stays the recorded reason in that case.
// See plan/ai/tools/browser/step-29-skip-headless-for-known-cloudflare-domains.md.
func performPaginatedCrawlForURL(reqCtx context.Context, url string, args paginatedArgs) (paginatedCrawlResponse, []string, error) {
	worker := func() (paginatedResult, error) { return paginatedCrawlOnWorkerTab(reqCtx, url, args) }
	headed := func() (paginatedResult, error) { return performPaginatedCrawlWithNormalSession(reqCtx, url, args) }

	if args.requestID == "" {
		// AI call (extract_from_url): one headless worker attempt within
		// aiCallBudget, never headed.
		r, err := worker()
		recordIfUnresolved(url, err)
		return r.resp, r.pageHTML, err
	}

	if domain, err := hostnameOf(url); err == nil {
		if known, err := isDomainKnownCloudflare(domain); err == nil && known {
			r, err := escalateChallenge(reqCtx, url, args.requestID, shared.HeadlessHasCookie(url, "cf_clearance"), worker, headed)
			return r.resp, r.pageHTML, err
		}
	}

	r, err := worker()
	var cfErr *crawlError
	if !errors.As(err, &cfErr) || cfErr.Code != codeCloudflareUnresolved {
		return r.resp, r.pageHTML, err
	}
	// step 28 — best-effort.
	if domain, hostErr := hostnameOf(url); hostErr == nil {
		_ = recordCloudflareDomain(domain, cfErr.Reason)
	}
	r, err = escalateChallenge(reqCtx, url, args.requestID, false, worker, headed)
	return r.resp, r.pageHTML, err
}

// paginatedCrawlOnWorkerTab navigates a fresh worker tab to url and runs
// the whole paginated loop in it, with page 1's full automatic
// Cloudflare wait. Portal pacing (portal_pacing.go) for page 1 happens
// BEFORE a tab is taken, so a paced request doesn't sit on a tab slot
// while it waits its turn.
func paginatedCrawlOnWorkerTab(reqCtx context.Context, url string, args paginatedArgs) (paginatedResult, error) {
	if capTimeout(paginatedCrawlTimeout, args.deadline) < minWorkerAttempt {
		return paginatedResult{}, newCloudflareUnresolvedError("time budget exhausted before a headless retry", nil)
	}
	awaitPortalCrawlPacing(reqCtx, args.rateLimitKey)
	if reqCtx.Err() != nil {
		return paginatedResult{}, newCrawlCancelledError()
	}

	setCrawlPhase(args.requestID, phaseQueuedForTab, "waiting for a free browser tab")
	tabCtx, release, err := shared.AcquireWorkerTab(reqCtx)
	if err != nil {
		if reqCtx.Err() != nil {
			return paginatedResult{}, newCrawlCancelledError()
		}
		return paginatedResult{}, err
	}
	defer release()

	timeout := paginatedCrawlTimeout
	if args.rateLimitKey != "" {
		timeout = paginatedCrawlTimeoutWithPacing
	}
	ctx, cancel := context.WithTimeout(tabCtx, capTimeout(timeout, args.deadline))
	defer cancel()
	linkRequestCancel(reqCtx, ctx, cancel, tabCtx)

	signals := watchChallengeSignals(ctx)
	setCrawlPhase(args.requestID, phaseNavigating, "navigating to "+url)
	if err := navigateWithHangGuard(ctx, release, chromedp.Navigate(url), chromedp.Sleep(SettleDelay)); err != nil {
		return paginatedResult{}, classifyCancellation(err, reqCtx, tabCtx)
	}

	resp, pageHTML, err := runPaginatedCrawlLoop(ctx, loopTab{signals: signals, autoWait: cloudflareAutoWait}, args)
	return paginatedResult{resp, pageHTML}, classifyCancellation(err, reqCtx, tabCtx)
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
// crawl in a headed tab (shared.AcquireHeadedTab), navigated to
// blockedURL — only ever reached through escalateChallenge, after the
// headless attempts already failed to clear the same challenge on page
// 1. If the headed tab's own automatic wait (page 1 of
// runPaginatedCrawlLoop) also fails, waits for a human via
// waitForHumanToClearCloudflare, then runs the full loop again from
// page 1 — letting pagination proceed through page 2..N in the same
// now-cleared tab. Always closes its tab before returning; on success,
// copies its cookies into the headless browser (importHeadedCookies).
// See plan/ai/tools/browser/step-24-headed-chrome-cloudflare-fallback.md,
// step-25-human-assisted-cloudflare-retry.md, and
// step-26-headed-fallback-for-paginated-crawl.md.
//
// A "solved" verdict can occasionally be premature (a residual widget
// still settling, a transitional re-render): a real, reproduced case
// had the immediate re-extraction hit the same challenge again seconds
// later, which used to surface as an unrecoverable hard failure,
// discarding the entire fallback budget over one bad tick. Bounded at
// maxHumanWaitRounds total rounds.
func performPaginatedCrawlWithNormalSession(reqCtx context.Context, blockedURL string, args paginatedArgs) (paginatedResult, error) {
	requestID := args.requestID
	setCrawlPhase(requestID, phaseQueuedForTab, "waiting for a free headed browser tab")
	tabCtx, release, err := shared.AcquireHeadedTab(reqCtx)
	if err != nil {
		if reqCtx.Err() != nil {
			return paginatedResult{}, newCrawlCancelledError()
		}
		return paginatedResult{}, err
	}
	defer release()

	ctx, cancel := context.WithTimeout(tabCtx, normalSessionPaginatedCrawlTimeout)
	defer cancel()
	// step 63.2 — a Stop click reaches this up-to-32-minute
	// headed/human-solve wait too.
	linkRequestCancel(reqCtx, ctx, cancel, tabCtx)

	result, pageHTML, err := func() (paginatedCrawlResponse, []string, error) {
		signals := watchChallengeSignals(ctx)
		setCrawlPhase(requestID, phaseNavigating, "navigating to "+blockedURL)
		if err := navigateWithHangGuard(ctx, release, chromedp.Navigate(blockedURL), chromedp.Sleep(SettleDelay)); err != nil {
			return paginatedCrawlResponse{}, nil, err
		}
		tab := loopTab{signals: signals, autoWait: cloudflareAutoWait}

		result, pageHTML, err := runPaginatedCrawlLoop(ctx, tab, args)
		var cfErr *crawlError
		if !errors.As(err, &cfErr) || cfErr.Code != codeCloudflareUnresolved {
			return result, pageHTML, err
		}

		for round := 1; round <= maxHumanWaitRounds; round++ {
			outcome, waitErr := waitForHumanToClearCloudflare(ctx, requestID, cfErr)
			if waitErr != nil {
				return paginatedCrawlResponse{}, nil, waitErr
			}
			if outcome.HardBlocked {
				return paginatedCrawlResponse{}, nil, newCloudflareBlockedError(outcome.Reason)
			}
			if !outcome.Cleared {
				return paginatedCrawlResponse{}, nil, cfErr
			}

			result, pageHTML, err = runPaginatedCrawlLoop(ctx, tab, args)
			if !errors.As(err, &cfErr) || cfErr.Code != codeCloudflareUnresolved {
				return result, pageHTML, err
			}
			// "Solved" was premature — cfErr is now this round's own fresh
			// re-detection. Report it distinctly before looping back.
			setCrawlPhase(requestID, phaseAwaitingHumanChallenge, fmt.Sprintf(
				"page reported solved but Cloudflare challenge reappeared (%s) — waiting again (round %d/%d)",
				cfErr.Reason, round, maxHumanWaitRounds,
			))
		}
		return paginatedCrawlResponse{}, nil, cfErr
	}()
	if err == nil {
		importHeadedCookies(ctx, blockedURL)
	}
	return paginatedResult{result, pageHTML}, classifyCancellation(err, reqCtx, tabCtx)
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
