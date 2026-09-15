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
// waiting inside the headless attempt). Raised from step 24's original
// 20s to comfortably contain the human-wait retry budget below on top
// of everything the automated path already needs: process launch +
// navigate/settle (~3s) + the automated cloudflareMaxWait (8s) +
// maxHumanSolveRetries*humanSolveRetryInterval (60s) + a final HTML
// read (~1s) ≈ 72s worst case — 90s leaves real margin. See
// plan/ai/tools/browser/step-25-human-assisted-cloudflare-retry.md's
// own Open Question 2 for the much sharper AI-path timeout conflict
// this creates against tool_mcp.Invoke's own 45s invokeTimeout.
const normalSessionCrawlTimeout = 90 * time.Second

// humanSolveRetryInterval/maxHumanSolveRetries — after the automated
// wait (waitForCloudflareClearance's own cloudflareMaxWait=8s) still
// finds the challenge present in the now-visible headed window, a
// human needs real time to notice it and react — checked again every
// humanSolveRetryInterval (not continuously) rather than one more
// short automated poll. 4 * 15s = 60s of human-wait budget — an
// explicit starting estimate, not verified against a real person's
// own reaction time; see step 25's own Open Question 1.
const (
	humanSolveRetryInterval = 15 * time.Second
	maxHumanSolveRetries    = 4
)

type crawlRequest struct {
	URL string `json:"url"`
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

	result, err := crawlPage(body.URL)
	if err != nil {
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
func navigateAndReadWithCloudflareCheck(ctx context.Context, rawURL string) (crawlResponse, error) {
	if err := chromedp.Run(ctx,
		chromedp.Navigate(rawURL),
		chromedp.Sleep(settleDelay),
	); err != nil {
		return crawlResponse{}, err
	}

	cf, err := waitForCloudflareClearance(ctx)
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
func crawlPage(rawURL string) (crawlResponse, error) {
	result, err := func() (crawlResponse, error) {
		sessionMu.Lock()
		defer sessionMu.Unlock()

		ctx, cancel := context.WithTimeout(sessionCtx, crawlTimeout)
		defer cancel()

		return navigateAndReadWithCloudflareCheck(ctx, rawURL)
	}()

	var cfErr *crawlError
	if errors.As(err, &cfErr) {
		return crawlWithNormalSession(rawURL)
	}
	return result, err
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
func crawlWithNormalSession(rawURL string) (crawlResponse, error) {
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

	result, err := navigateAndReadWithCloudflareCheck(ctx, rawURL)

	var cfErr *crawlError
	if !errors.As(err, &cfErr) {
		return result, err
	}
	return waitForHumanToSolveCloudflare(ctx, cfErr)
}

// waitForHumanToSolveCloudflare gives a person sitting at the now-
// visible headed Chrome window real time to notice a still-present
// Cloudflare challenge and solve it themselves. Sleeps
// humanSolveRetryInterval, then takes one immediate, non-polling check
// (detectCloudflareChallenge — not another full
// waitForCloudflareClearance cycle) — repeated up to
// maxHumanSolveRetries times. Gives up with the same crawlError the
// automated path already produces once that budget is spent. See
// plan/ai/tools/browser/step-25-human-assisted-cloudflare-retry.md.
func waitForHumanToSolveCloudflare(ctx context.Context, fallback *crawlError) (crawlResponse, error) {
	for i := 0; i < maxHumanSolveRetries; i++ {
		if err := chromedp.Run(ctx, chromedp.Sleep(humanSolveRetryInterval)); err != nil {
			return crawlResponse{}, err
		}
		check, err := detectCloudflareChallenge(ctx)
		if err != nil {
			return crawlResponse{}, err
		}
		if !check.Detected {
			return readCrawlResponse(ctx)
		}
	}
	return crawlResponse{}, fallback
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
			"If the page is behind a Cloudflare challenge, this waits briefly for it to clear before reading the page; if it's still blocking once the wait runs out, this automatically retries in a normal (non-headless) browser window and waits up to about a minute more for a person to notice and solve the challenge there before giving up — this call can take up to roughly 90 seconds in that case. If it's still blocked after that, this call fails with a cloudflare_challenge_unresolved error instead of returning the interstitial as if it were the real page.",
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
