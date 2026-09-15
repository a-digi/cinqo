// crawl.go implements the HTML crawler feature — the AI navigates the
// shared browser session to a URL and gets back the page's rendered
// HTML. See plan/ai/tools/browser/step-03-html-crawler-feature.md.
package main

import (
	"context"
	"encoding/json"
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

type crawlRequest struct {
	URL string `json:"url"`
}

type crawlResponse struct {
	HTML      string `json:"html"`
	Title     string `json:"title"`
	FinalURL  string `json:"finalUrl"`
	Truncated bool   `json:"truncated"`
	// CloudflareDetected/CloudflareReason (step 21) — set when
	// waitForCloudflareClearance (cloudflare.go) still found a
	// Cloudflare challenge interstitial present after its own wait
	// budget was exhausted; empty/false when no challenge was ever
	// seen, or one was seen but cleared within budget (in which case
	// HTML/Title/FinalURL above already reflect the real, resolved
	// page). See
	// plan/ai/tools/browser/step-21-cloudflare-challenge-detection.md.
	CloudflareDetected bool   `json:"cloudflareDetected,omitempty"`
	CloudflareReason   string `json:"cloudflareReason,omitempty"`
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
		http.Error(w, fmt.Sprintf("crawl failed: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// crawlPage navigates the one shared session to rawURL and reads back
// its rendered HTML/title/final URL. Holds sessionMu for the whole
// operation — see that variable's own doc comment in main.go.
func crawlPage(rawURL string) (crawlResponse, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	ctx, cancel := context.WithTimeout(sessionCtx, crawlTimeout)
	defer cancel()

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
		HTML:               html,
		Title:              title,
		FinalURL:           finalURL,
		Truncated:          truncated,
		CloudflareDetected: cf.Detected,
		CloudflareReason:   cf.Reason,
	}, nil
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
			"If the page is behind a Cloudflare challenge, this waits briefly for it to clear before reading the page; a returned cloudflareDetected warning means it was still blocking when the wait ran out — the HTML returned may be the challenge interstitial, not the real page.",
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
		if result.CloudflareDetected {
			text += fmt.Sprintf("⚠️ Cloudflare challenge still present after waiting — this page's own content may be the interstitial, not the real page (reason: %s)\n", result.CloudflareReason)
		}
		text += "\n" + result.HTML

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	})
}
