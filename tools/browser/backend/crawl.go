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
// end to end (process launch + stealth injection + navigate + settle +
// up to cloudflareMaxWait's own wait + HTML read) — deliberately its
// OWN budget, not nested inside or derived from the primary attempt's
// own ctx (already bounded by crawlTimeout, and likely close to
// exhausted by the time a fallback is even considered, having just
// spent up to cloudflareMaxWait waiting inside the headless attempt).
// See plan/ai/tools/browser/step-24-headed-chrome-cloudflare-fallback.md's
// own Open Question 1 for the real tension this creates against
// tool_mcp.Invoke's own 45s invokeTimeout on the AI-driven call path.
const normalSessionCrawlTimeout = 20 * time.Second

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

// crawlPage navigates the one shared headless session to rawURL and
// reads back its rendered HTML/title/final URL. Holds sessionMu for
// the whole operation — see that variable's own doc comment in
// main.go. When the headless session hits an unresolved Cloudflare
// challenge, retries once via crawlWithNormalSession (step 24) instead
// of failing immediately — a real, headed browser is often materially
// harder for Cloudflare to flag as automated than headless Chrome,
// even with this tool's own existing stealth patches applied.
func crawlPage(rawURL string) (crawlResponse, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	ctx, cancel := context.WithTimeout(sessionCtx, crawlTimeout)
	defer cancel()

	result, err := navigateAndReadWithCloudflareCheck(ctx, rawURL)

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
// error) — "close after it is finished." See
// plan/ai/tools/browser/step-24-headed-chrome-cloudflare-fallback.md.
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

	return navigateAndReadWithCloudflareCheck(ctx, rawURL)
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
			"If the page is behind a Cloudflare challenge, this waits briefly for it to clear before reading the page; if it's still blocking once the wait runs out, this automatically retries once in a normal (non-headless) browser before giving up, which can make a blocked call noticeably slower — if it's still blocked after that retry, this call fails with a cloudflare_challenge_unresolved error instead of returning the interstitial as if it were the real page.",
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
