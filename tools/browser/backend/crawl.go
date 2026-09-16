// crawl.go implements the HTML crawler feature — the AI navigates the
// shared browser session to a URL and gets back the page's rendered
// HTML. See plan/ai/tools/browser/step-03-html-crawler-feature.md.
package main

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
)

// maxHTMLBytes bounds how much of a crawled page's HTML is ever
// returned — an arbitrary page's rendered HTML can be megabytes, far
// more than worth spending an LLM's own context budget on in one tool
// result. A fixed, non-configurable cap for this first pass, matching
// this codebase's own established precedent (defaultMaxTokens in
// api/src/platform/anthropic) of plain constants over
// premature configurability.
const maxHTMLBytes = 200_000

// crawlTimeout bounds one navigation — shorter than pdf_generator's
// own 30s render budget, since crawling has no print-to-PDF step of
// its own to also account for.
const crawlTimeout = 20 * time.Second

const settleDelay = 1500 * time.Millisecond

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
	// flight. Absent for every AI-driven call (callSibling never sets
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
}

type crawlResponse struct {
	HTML      string `json:"html"`
	Title     string `json:"title"`
	FinalURL  string `json:"finalUrl"`
	Truncated bool   `json:"truncated"`
}

// crawlHandler handles POST /crawl — the --mcp adapter's own real
// target for fetch_page_html, reached only via callSibling.
func crawlHandler(w http.ResponseWriter, r *http.Request) {
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

	result, err := crawlPage(body.URL, body.RequestID, body.ExpectedSelectors)
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
func readCrawlResponse(ctx context.Context) (crawlResponse, error) {
	var html, title, finalURL string
	if err := chromedp.Run(ctx,
		chromedp.Title(&title),
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		return crawlResponse{}, err
	}

	truncated := false
	if len(html) > maxHTMLBytes {
		html = html[:maxHTMLBytes]
		truncated = true
	}

	return crawlResponse{
		HTML:      html,
		Title:     title,
		FinalURL:  finalURL,
		Truncated: truncated,
	}, nil
}

// navigateAndReadWithCloudflareCheck runs the shared navigate → settle
// → Cloudflare-wait → read-HTML sequence against ctx — extracted so
// both the primary (shared headless session) and fallback (ephemeral
// headed session, step 24) crawl attempts share one implementation
// instead of two copies that could drift apart. Returns the same
// crawlError (cloudflare_challenge_unresolved) as before when the
// challenge is still present once ctx's own wait budget is spent. See
// plan/ai/tools/browser/step-24-headed-chrome-cloudflare-fallback.md.
func navigateAndReadWithCloudflareCheck(ctx context.Context, rawURL, requestID string, expectedSelectors []string) (crawlResponse, error) {
	setCrawlPhase(requestID, phaseNavigating, "navigating to "+rawURL)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(rawURL),
		chromedp.Sleep(settleDelay),
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

	return readCrawlResponse(ctx)
}

// crawlPage navigates the one shared headless session to rawURL and
// reads back its rendered HTML/title/final URL. The headless attempt
// itself is scoped to an inner function so sessionMu (see that
// variable's own doc comment in main.go) is released the moment that
// attempt concludes — BEFORE crawlWithNormalSession (a wholly separate
// browser, guarded by its own normalSessionMu) ever starts. Without
// this, sessionMu would stay held for the fallback's own up-to-90s
// retry budget (step 25) too, serializing every other crawl request
// behind one slow, unrelated Cloudflare fallback. When the headless
// attempt hits an unresolved Cloudflare challenge, retries via
// crawlWithNormalSession (step 24) instead of failing immediately — a
// real, headed browser is often materially harder for Cloudflare to
// flag as automated than headless Chrome, even with this tool's own
// existing stealth patches applied.
func crawlPage(rawURL, requestID string, expectedSelectors []string) (crawlResponse, error) {
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
			return crawlWithNormalSession(rawURL, requestID, expectedSelectors)
		}
	}

	result, err := func() (crawlResponse, error) {
		sessionMu.Lock()
		defer sessionMu.Unlock()

		ctx, cancel := context.WithTimeout(sessionCtx, crawlTimeout)
		defer cancel()

		return navigateAndReadWithCloudflareCheck(ctx, rawURL, requestID, expectedSelectors)
	}()

	var cfErr *crawlError
	if errors.As(err, &cfErr) {
		// step 28 — best-effort: a failed cache write must never turn
		// an otherwise-working fallback into a failure.
		if domain, hostErr := hostnameOf(rawURL); hostErr == nil {
			_ = recordCloudflareDomain(domain, cfErr.Reason)
		}
		return crawlWithNormalSession(rawURL, requestID, expectedSelectors)
	}
	// step 38 — deliberately wrapped only here, after the Cloudflare
	// dispatch above: a session-interrupted error must never trigger
	// crawlWithNormalSession's own fresh headed-Chrome fallback (the
	// whole subprocess — and its one shared session — is what just
	// died; launching a second, unrelated browser fixes nothing here).
	return result, wrapIfSessionInterrupted(err)
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
func crawlWithNormalSession(rawURL, requestID string, expectedSelectors []string) (crawlResponse, error) {
	normalSessionMu.Lock()
	defer normalSessionMu.Unlock()

	ctx, cancels, err := startSharedNormalSession()
	if err != nil {
		return crawlResponse{}, fmt.Errorf("normal-session fallback: failed to start: %w", err)
	}
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, normalSessionCrawlTimeout)
	defer cancel()

	result, err := navigateAndReadWithCloudflareCheck(ctx, rawURL, requestID, expectedSelectors)

	var cfErr *crawlError
	if !errors.As(err, &cfErr) {
		return result, err
	}
	return waitForHumanToSolveCloudflare(ctx, cfErr, requestID, expectedSelectors)
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
// expectedSelectors (step 36) corroborates BOTH directions here, on
// purpose asymmetrically:
//   - forward: already handled inside detectCloudflareChallenge itself
//     — the moment the crawl's own target content is visibly present,
//     the heuristic's own "detected" result is overridden before this
//     loop ever sees it.
//   - reverse: handled explicitly in this loop below — a heuristic
//     "clear" result does NOT immediately end the wait; it must also
//     be corroborated by the expected content actually being found.
//     Deliberately scoped to THIS function alone, never
//     waitForCloudflareClearance's own short automated wait, and never
//     a fresh crawl's very first check: by the time this loop is
//     running at all, a real Cloudflare issue was already independently
//     confirmed moments earlier, so "clear but content missing" is
//     being asked in a context that's already Cloudflare-adjacent —
//     not on an arbitrary, unrelated page, where "content not found
//     yet" is the ordinary, everyday signature of a wrong selector or
//     an empty results page, not evidence of a hidden block. This does
//     NOT extend maxHumanSolveDuration — it only changes what counts as
//     "cleared" within the wait that's already happening. See
//     plan/ai/tools/browser/step-36-content-based-cloudflare-override.md.
func waitForHumanToClearCloudflare(ctx context.Context, requestID, initialReason string, expectedSelectors []string) (bool, error) {
	setCrawlPhase(requestID, phaseAwaitingHumanChallenge, fmt.Sprintf(
		"Cloudflare challenge detected (%s) — waiting for a person to solve it in the open browser window", initialReason,
	))
	deadline := time.Now().Add(maxHumanSolveDuration)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		if err := chromedp.Run(ctx, chromedp.Sleep(humanSolveRetryInterval)); err != nil {
			return false, err
		}
		check, err := detectCloudflareChallenge(ctx, expectedSelectors)
		if err != nil {
			return false, err
		}
		if !check.Detected {
			if len(expectedSelectors) == 0 {
				return true, nil
			}
			found, err := expectedContentVisible(ctx, expectedSelectors)
			if err != nil {
				return false, err
			}
			if found {
				return true, nil
			}
			setCrawlPhase(requestID, phaseAwaitingHumanChallenge, fmt.Sprintf(
				"Cloudflare check reports clear, but expected content not yet found — check #%d, continuing to wait", attempt,
			))
			continue
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
func waitForHumanToSolveCloudflare(ctx context.Context, fallback *crawlError, requestID string, expectedSelectors []string) (crawlResponse, error) {
	cleared, err := waitForHumanToClearCloudflare(ctx, requestID, fallback.Reason, expectedSelectors)
	if err != nil {
		return crawlResponse{}, err
	}
	if !cleared {
		return crawlResponse{}, fallback
	}
	return readCrawlResponse(ctx)
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
	URL string `json:"url" jsonschema:"the absolute http(s) URL to navigate to and read"`
}

// registerFetchPageHTML adds the fetch_page_html MCP tool — thin: it
// only ever calls callSibling and formats the result, never touches
// sessionCtx directly (this --mcp subprocess never holds it — see
// main.go's own runMCPServer doc comment).
func registerFetchPageHTML(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "fetch_page_html",
		Description: "Navigate the shared browser session to a URL and return the page's rendered HTML, title, and final URL (after any redirect). " +
			"If the page is behind a Cloudflare challenge, this waits briefly for it to clear before reading the page; if it's still blocking once the wait runs out, this automatically retries in a normal (non-headless) browser window and can wait up to about 30 minutes for a person to notice and solve the challenge there before giving up — meaning this call can take up to roughly 30 minutes in that case, almost certainly longer than this AI tool-calling session's own timeout, so a Cloudflare-blocked page is effectively only recoverable through this path by a human watching for the window, not by an AI call waiting on the result. If it's still blocked after that, this call fails with a cloudflare_challenge_unresolved error instead of returning the interstitial as if it were the real page.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args fetchPageHTMLArgs) (*mcp.CallToolResult, any, error) {
		if args.URL == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "url is required"}},
				IsError: true,
			}, nil, nil
		}

		reqBody, err := json.Marshal(crawlRequest{URL: args.URL})
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to build request: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		respBody, err := callSibling("crawl", reqBody)
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
