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
package main

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
)

// paginatedCrawlTimeout bounds the whole multi-page loop — deliberately
// well under the host's own invokeTimeout (45s, api/src/tool/mcp/invoke.go,
// which wraps this call's entire spawn+handshake+logic and can't be
// changed from inside this tool), leaving real headroom so this tool's
// own clean "time_budget_reached" partial result fires before the host
// kills the call outright.
const paginatedCrawlTimeout = 35 * time.Second

// maxAllowedPaginationPages is the real, non-negotiable ceiling on how
// many pages a single call ever visits, regardless of what the
// instruction requests — sized against paginatedCrawlTimeout: a
// conservative ~3s per page/transition (extraction itself is fast;
// settleDelay alone is 1.5s, plus real navigation time) puts 10 pages
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
}

// paginatedCrawlRequest is the internal JSON shape the --mcp adapter
// sends to its own HTTP-mode sibling — already-parsed-and-validated
// by the time it crosses this boundary, same convention as every
// other feature's own callSibling request.
type paginatedCrawlRequest struct {
	Container         string            `json:"container,omitempty"`
	Fields            []extractField    `json:"fields"`
	Mapping           map[string]string `json:"mapping,omitempty"`
	NextSelector      string            `json:"nextSelector"`
	RequestedMaxPages int               `json:"requestedMaxPages"`
	EffectiveMaxPages int               `json:"effectiveMaxPages"`
}

// paginatedCrawlHandler handles POST /crawl-paginated — the --mcp
// adapter's own real target for crawl_paginated.
func paginatedCrawlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body paginatedCrawlRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(body.Fields) == 0 || body.NextSelector == "" || body.EffectiveMaxPages < 1 {
		http.Error(w, "fields, nextSelector, and a positive maxPages are all required", http.StatusBadRequest)
		return
	}

	// Step 22 — read once, up front: both whether to log at all
	// (DebugEnabled) and whether to also capture each page's own raw
	// HTML while crawling (DebugLogHTML) are decided from this single
	// read, not two independent ones later, so the two can't observe a
	// setting change mid-request. A read failure is treated the same as
	// "Debug off" — logging is best-effort observability, never allowed
	// to turn a successful crawl into a failed response. See
	// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
	settings, _ := loadBrowserSettings()
	captureHTML := settings.DebugEnabled && settings.DebugLogHTML

	result, pageHTML, err := performPaginatedCrawl(body.Container, body.Fields, body.Mapping, body.NextSelector, body.RequestedMaxPages, body.EffectiveMaxPages, captureHTML)
	if err != nil {
		http.Error(w, fmt.Sprintf("paginated crawl failed: %v", err), http.StatusBadGateway)
		return
	}

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

// performPaginatedCrawl holds sessionMu for the whole multi-page
// operation — never releasing it between pages, since nothing else
// may touch the shared session mid-loop — and calls
// runExtractionOnCurrentPage (extract.go) directly rather than
// performExtraction, precisely because it already holds the lock
// performExtraction would try to take again. See that function's own
// doc comment.
// captureHTML (step 22) gates one extra chromedp.OuterHTML read per
// page — costs nothing when false (the overwhelming default: Debug or
// "Log HTML" off), only paid when a human has explicitly turned "Log
// HTML" on. pageHTML, the second return value, is parallel to the
// returned response's own Pages (one entry per page, empty string when
// captureHTML is false) and is never embedded in paginatedCrawlResponse
// itself — kept as a separate return value specifically so it cannot
// reach the live AI-facing JSON response; only paginatedCrawlHandler's
// own saveCrawlLog call (crawl_log.go) ever sees it. See
// plan/ai/tools/browser/step-22-debug-mode-and-log-management.md.
func performPaginatedCrawl(container string, fields []extractField, mapping map[string]string, nextSelector string, requestedMaxPages, effectiveMaxPages int, captureHTML bool) (paginatedCrawlResponse, []string, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	ctx, cancel := context.WithTimeout(sessionCtx, paginatedCrawlTimeout)
	defer cancel()

	pages := make([]pageExtractResult, 0, effectiveMaxPages)
	var pageHTML []string
	if captureHTML {
		pageHTML = make([]string, 0, effectiveMaxPages)
	}
	stoppedReason := ""

	for page := 1; ; page++ {
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

		cf, err := detectCloudflareChallenge(ctx)
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
			if len(html) > maxHTMLBytes {
				html = html[:maxHTMLBytes]
			}
			pageHTML = append(pageHTML, html)
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

		if err := chromedp.Run(ctx, chromedp.Click(nextSelector), chromedp.Sleep(settleDelay)); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				stoppedReason = "time_budget_reached"
				break
			}
			stoppedReason = "click_failed"
			break
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
	}, pageHTML, nil
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

// registerCrawlPaginated adds the crawl_paginated MCP tool — thin,
// like the other MCP-facing registrations: parses the YAML
// instructions, validates, builds the internal JSON request, and
// calls callSibling.
func registerCrawlPaginated(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "crawl_paginated",
		Description: "Extract fields from the currently loaded page, then follow a pagination control and repeat, up to a maximum number of pages — instructed via a YAML document (fields + pagination). Read-only; submits nothing. " +
			"Before writing the instructions, check whether the page LISTS MULTIPLE similar items at once or describes just one. For a listing page, always add a top-level container selector for one item's own repeating wrapping element — the default correct approach for a listing page, not a fallback — so the result is one correctly-grouped object per item (in `items`) instead of separate same-length arrays (in `results`) that may NOT actually line up. " +
			"Optionally add a top-level mapping to rename extracted fields to specific output keys — e.g. a consuming tool expects title/url/company but this page's own natural fields are better labeled job_title/link/employer. " +
			"A page's own cloudflareDetected (in its entry under pages) means a Cloudflare challenge was still showing on that page — its results/items may reflect the challenge interstitial, not real content.",
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

		respBody, err := callSibling("crawl-paginated", reqBody)
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
