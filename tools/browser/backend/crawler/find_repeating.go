// find_repeating.go implements find_repeating_items (step 94): navigate to a
// URL in a worker tab and report which repeating structures on the rendered
// page look like a list of items (job cards, search results), as CSS
// selectors verified against the live DOM. An agent writing listing crawl
// instructions otherwise sees only the first 20,000 characters of the page
// (the conversation layer's tool-result cap) — usually the header and filters
// — and has to guess a container; this answers "what is the repeating card?"
// in a few hundred characters per candidate. Same worker-tab path as
// extract_from_url (one headless attempt inside aiCallBudget, never headed),
// read-only, submits nothing. See
// plan/ai/tools/browser/step-94-find-repeating-items.md.
package crawler

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"browser-tool-backend/shared"
)

//go:embed find_repeating.js
var findRepeatingJSTemplate string

// findRepeatingPayloadPlaceholder is the token in find_repeating.js replaced
// with the real payload JSON before the script is evaluated.
const findRepeatingPayloadPlaceholder = "__FIND_REPEATING_PAYLOAD__"

const (
	defaultMinItems      = 3
	defaultMaxCandidates = 5
	maxMaxCandidates     = 8
	// findRepeatingMaxAttributeLength matches the 200 the generation prompt
	// already asks fetch_page_html for: longer attribute values are noise.
	findRepeatingMaxAttributeLength = 200
	// findRepeatingRetryWait / findRepeatingRetries: a client-rendered page
	// may not have its list yet right after the settle delay; retry the
	// analysis a couple of times before reporting nothing.
	findRepeatingRetryWait = 2 * time.Second
	findRepeatingRetries   = 2
)

// RepeatingCandidate is one repeating group found on the page.
type RepeatingCandidate struct {
	Selector        string   `json:"selector"`
	Count           int      `json:"count"`
	MembersWithLink int      `json:"membersWithLink"`
	DistinctLinks   int      `json:"distinctLinks"`
	FirstLinks      []string `json:"firstLinks"`
	SampleText      string   `json:"sampleText"`
	SampleHTML      string   `json:"sampleHtml"`
}

// NextCandidate is a control that looks like the "next page" button or link,
// as a selector verified on the live page (step 95).
type NextCandidate struct {
	Selector string `json:"selector"`
	// Matches is how many elements the selector selects (a pagination bar
	// shown above and below the list matches twice; the first is clicked).
	Matches int    `json:"matches"`
	Text    string `json:"text"`
	Label   string `json:"label,omitempty"`
	Href    string `json:"href,omitempty"`
	Reason  string `json:"reason"`
}

type findRepeatingScriptResult struct {
	Candidates     []RepeatingCandidate `json:"candidates"`
	NextCandidates []NextCandidate      `json:"nextCandidates"`
	NextPageURL    string               `json:"nextPageUrl"`
	Truncated      bool                 `json:"truncated"`
}

type findRepeatingRequest struct {
	URL           string `json:"url"`
	MinItems      int    `json:"minItems,omitempty"`
	MaxCandidates int    `json:"maxCandidates,omitempty"`
}

type findRepeatingResponse struct {
	PageTitle  string               `json:"pageTitle"`
	FinalURL   string               `json:"finalUrl"`
	Candidates []RepeatingCandidate `json:"candidates"`
	// NextCandidates (step 95) are the likely next-page controls, best first;
	// use one as pagination.nextSelector. NextPageURL is the page's own
	// rel=next link when it declares one (informational: crawl_paginated
	// clicks a control, it does not navigate to a URL).
	NextCandidates []NextCandidate `json:"nextCandidates"`
	NextPageURL    string          `json:"nextPageUrl,omitempty"`
	Note           string          `json:"note,omitempty"`
}

// normalizeFindRepeating applies the defaults and bounds: minItems at least 2
// (a "list" of one is not a list), maxCandidates at most maxMaxCandidates.
func normalizeFindRepeating(req *findRepeatingRequest) {
	if req.MinItems == 0 {
		req.MinItems = defaultMinItems
	}
	if req.MinItems < 2 {
		req.MinItems = 2
	}
	if req.MaxCandidates <= 0 {
		req.MaxCandidates = defaultMaxCandidates
	}
	if req.MaxCandidates > maxMaxCandidates {
		req.MaxCandidates = maxMaxCandidates
	}
}

// runFindRepeatingScript evaluates the analysis script on whatever page ctx
// currently shows.
func runFindRepeatingScript(ctx context.Context, minItems, maxCandidates int) (findRepeatingScriptResult, error) {
	payload, err := json.Marshal(struct {
		MinItems           int `json:"minItems"`
		MaxCandidates      int `json:"maxCandidates"`
		MaxAttributeLength int `json:"maxAttributeLength"`
	}{minItems, maxCandidates, findRepeatingMaxAttributeLength})
	if err != nil {
		return findRepeatingScriptResult{}, err
	}
	js := strings.Replace(findRepeatingJSTemplate, findRepeatingPayloadPlaceholder, string(payload), 1)
	var result findRepeatingScriptResult
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return findRepeatingScriptResult{}, err
	}
	if result.Candidates == nil {
		result.Candidates = []RepeatingCandidate{}
	}
	if result.NextCandidates == nil {
		result.NextCandidates = []NextCandidate{}
	}
	return result, nil
}

// analyzeCurrentPage runs the script, retrying while the page may still be
// rendering, and builds the response for whatever ctx shows.
func analyzeCurrentPage(ctx context.Context, minItems, maxCandidates int) (findRepeatingResponse, error) {
	var resp findRepeatingResponse
	var script findRepeatingScriptResult
	for attempt := 0; ; attempt++ {
		var err error
		script, err = runFindRepeatingScript(ctx, minItems, maxCandidates)
		if err != nil {
			return findRepeatingResponse{}, err
		}
		if len(script.Candidates) > 0 || attempt >= findRepeatingRetries {
			break
		}
		select {
		case <-ctx.Done():
			return findRepeatingResponse{}, ctx.Err()
		case <-time.After(findRepeatingRetryWait):
		}
	}
	if err := chromedp.Run(ctx, chromedp.Title(&resp.PageTitle), chromedp.Location(&resp.FinalURL)); err != nil {
		return findRepeatingResponse{}, err
	}
	resp.Candidates = script.Candidates
	resp.NextCandidates = script.NextCandidates
	resp.NextPageURL = script.NextPageURL
	if len(resp.Candidates) == 0 {
		resp.Note = fmt.Sprintf("No repeating group of at least %d items with their own links and some text was found. The page may not list items, may not have finished loading, or may be showing a challenge page.", minItems)
	} else if script.Truncated {
		resp.Note = "The page is very large; only part of it was analysed."
	}
	if len(resp.Candidates) > 0 && len(resp.NextCandidates) == 0 {
		msg := "No next-page control was found: the list may be a single page, or paginated by infinite scroll or a \"load more\" button, which crawl_paginated cannot page through (then crawl one page only)."
		if resp.Note != "" {
			msg = resp.Note + " " + msg
		}
		resp.Note = msg
	}
	return resp, nil
}

// performFindRepeating navigates a worker tab to url and analyses it. One
// headless attempt inside the AI call budget, never headed or paced: the
// same shape as extract_from_url's path.
func performFindRepeating(reqCtx context.Context, req findRepeatingRequest) (findRepeatingResponse, error) {
	deadline := aiDeadline("", time.Now())
	if capTimeout(paginatedCrawlTimeout, deadline) < minWorkerAttempt {
		return findRepeatingResponse{}, errors.New("time budget exhausted before a browser tab could be used")
	}
	tabCtx, release, err := shared.AcquireWorkerTab(reqCtx)
	if err != nil {
		return findRepeatingResponse{}, err
	}
	defer release()

	ctx, cancel := context.WithTimeout(tabCtx, capTimeout(paginatedCrawlTimeout, deadline))
	defer cancel()
	linkRequestCancel(reqCtx, ctx, cancel, tabCtx)

	signals := watchChallengeSignals(ctx)
	seedStoredClearance(ctx, req.URL) // step 96
	if err := navigateWithHangGuard(ctx, release, chromedp.Navigate(req.URL), chromedp.Sleep(SettleDelay)); err != nil {
		return findRepeatingResponse{}, classifyCancellation(err, reqCtx, tabCtx)
	}
	resp, err := analyzeCurrentPage(ctx, req.MinItems, req.MaxCandidates)
	if err != nil {
		return findRepeatingResponse{}, classifyCancellation(err, reqCtx, tabCtx)
	}
	// An empty result on a challenge page is a different story from an empty
	// page: report it as the error every other tool reports, which for a
	// challenge a person can clear also asks that person (human_assist.go).
	if len(resp.Candidates) == 0 {
		if tracker, terr := newChallengeTracker(ctx, signals, nil); terr == nil {
			if st, serr := tracker.state(ctx); serr == nil {
				switch {
				case st.blocked:
					return findRepeatingResponse{}, newCloudflareBlockedError(st.reason)
				case st.final != nil:
					return findRepeatingResponse{}, st.final
				case st.challenge:
					return findRepeatingResponse{}, humanAssistedError(req.URL, "", newCloudflareUnresolvedError(st.reason, nil))
				}
			}
		}
	}
	return resp, nil
}

// FindRepeatingItemsHandler handles POST /find-repeating-items — the --mcp
// adapter's own real target for find_repeating_items.
func FindRepeatingItemsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req findRepeatingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	if req.MinItems < 0 || req.MaxCandidates < 0 {
		http.Error(w, "minItems and maxCandidates must not be negative", http.StatusBadRequest)
		return
	}
	// The same SSRF guard every other URL-taking route applies.
	if err := validateCrawlURL(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	normalizeFindRepeating(&req)

	resp, err := performFindRepeating(r.Context(), req)
	if err != nil {
		var cfErr *crawlError
		if errors.As(err, &cfErr) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(cfErr)
			return
		}
		http.Error(w, fmt.Sprintf("find repeating items failed: %v", err), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type findRepeatingItemsArgs struct {
	URL           string `json:"url" jsonschema:"the absolute http(s) URL of the listing page to analyse"`
	MinItems      int    `json:"minItems,omitempty" jsonschema:"fewest items a group must have to count as a list (default 3, at least 2)"`
	MaxCandidates int    `json:"maxCandidates,omitempty" jsonschema:"most candidate groups to return (default 5, at most 8)"`
}

// RegisterFindRepeatingItems adds the find_repeating_items MCP tool — thin,
// like every other MCP-facing registration in this package.
func RegisterFindRepeatingItems(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "find_repeating_items",
		Description: "Find the repeating items on a LISTING page — the job cards or search results — and return the best few candidate groups as CSS selectors verified against the live rendered page, " +
			"each with how many items it selects, the first links, a short text sample and ONE trimmed sample item's HTML. " +
			"Use this FIRST when writing listing crawl instructions, instead of reading raw page HTML: a long page's HTML is cut off long before the job list, so reading it shows only the header and filters. " +
			"Take the candidate whose sample looks like one job posting; its selector is your crawl_paginated `container`, and its sampleHtml shows what to select inside it (title, link, company, location, ...). " +
			"Navigates to the URL itself; read-only, submits nothing. An empty result means no list was found — the page may not list items, may not have finished loading, or may be a challenge page. " +
			"It also returns nextCandidates: the likely next-page controls, each a selector verified on the page — use one as pagination.nextSelector instead of guessing.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args findRepeatingItemsArgs) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(args.URL) == "" {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "url is required"}}, IsError: true}, nil, nil
		}
		body, err := json.Marshal(findRepeatingRequest{URL: args.URL, MinItems: args.MinItems, MaxCandidates: args.MaxCandidates})
		if err != nil {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to build request: %v", err)}}, IsError: true}, nil, nil
		}
		respBody, err := shared.CallSibling("find-repeating-items", body)
		if err != nil {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
		}
		var result findRepeatingResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to parse find_repeating_items response: %v", err)}}, IsError: true}, nil, nil
		}
		text, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to format result: %v", err)}}, IsError: true}, nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}}, nil, nil
	})
}
