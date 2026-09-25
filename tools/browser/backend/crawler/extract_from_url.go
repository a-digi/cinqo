// extract_from_url.go implements extract_from_url — navigate to a URL
// and extract fields from it in ONE atomic operation, unlike
// fetch_page_html + extract_page_data as two separate calls. Built
// specifically for a caller that may be running CONCURRENTLY with
// other, unrelated callers driving the same shared browser session
// (e.g. several sub-agents, each crawling a different job's own
// detail page at the same time, api/src/conversation's own
// crawl_urls_with_subagents) — two separate calls each independently
// acquiring/releasing shared.Mu leave a window where a different
// caller's own navigate can land in between this caller's own
// navigate and its own extract, silently extracting the WRONG page's
// content. This tool closes that window entirely by reusing
// performPaginatedCrawl's own url-then-extract path (paginate.go, step
// 37, plan/ai/tools/browser/step-37-atomic-navigate-and-extract.md),
// which runs start to finish in its own worker tab — so concurrent
// sub-agents also actually run in parallel instead of queuing — with
// pagination effectively disabled (a single page, no next-page
// control) — no new locking/navigation logic of its own. See
// plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
package crawler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"browser-tool-backend/shared"
)

type extractFromURLArgs struct {
	URL     string            `json:"url" jsonschema:"the absolute http(s) URL of the single page to navigate to and extract from"`
	Fields  []extractField    `json:"fields" jsonschema:"one entry per piece of data to extract off this page — label + selector, optionally attribute/multiple. Same shape as extract_page_data's own fields."`
	Mapping map[string]string `json:"mapping,omitempty" jsonschema:"optional {sourceLabel: targetKey} — renames extracted fields to specific output key names before they're returned, same as extract_page_data's own mapping."`
}

// RegisterExtractFromURL adds the extract_from_url MCP tool — thin,
// like every other MCP-facing registration in this package: validates,
// builds paginatedCrawlRequest with pagination disabled (a single
// page, no nextSelector), and calls the exact same /crawl-paginated
// sibling route crawl_paginated itself uses.
func RegisterExtractFromURL(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "extract_from_url",
		Description: "Navigate to a URL and extract specific fields from it, as ONE atomic operation. " +
			"Always prefer this over calling fetch_page_html and then extract_page_data as two separate calls " +
			"whenever you might be running at the same time as another, independent caller sharing this same " +
			"browser session (e.g. one of several sub-agents each crawling a different page concurrently) — two " +
			"separate calls leave a window where a different caller's own navigate can land in between yours, " +
			"silently extracting the wrong page's content; this tool closes that window entirely. Read-only; " +
			"submits nothing. Same field shape as extract_page_data (label + selector, optionally " +
			"attribute/multiple) and the same optional mapping to rename extracted fields to specific output " +
			"keys. This is for a single page — no container, no pagination; use crawl_paginated instead for a " +
			"listing page or one that needs to be paginated through.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args extractFromURLArgs) (*mcp.CallToolResult, any, error) {
		if args.URL == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "url is required"}},
				IsError: true,
			}, nil, nil
		}
		if len(args.Fields) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "fields is required"}},
				IsError: true,
			}, nil, nil
		}
		for _, f := range args.Fields {
			if f.Label == "" || f.Selector == "" {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "every field requires both a label and a selector"}},
					IsError: true,
				}, nil, nil
			}
		}
		if err := validateMapping(args.Fields, args.Mapping); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}
		if err := validateCrawlURL(args.URL); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		reqBody, err := json.Marshal(paginatedCrawlRequest{
			URL:               args.URL,
			Fields:            args.Fields,
			Mapping:           args.Mapping,
			NextSelector:      "",
			RequestedMaxPages: 1,
			EffectiveMaxPages: 1,
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
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to parse response: %v", err)}},
				IsError: true,
			}, nil, nil
		}
		if len(result.Pages) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("extraction produced no page (stoppedReason: %s)", result.StoppedReason)}},
				IsError: true,
			}, nil, nil
		}

		text, err := json.MarshalIndent(result.Pages[0], "", "  ")
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to format result: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}}, nil, nil
	})
}
