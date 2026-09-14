// extract.go implements structured extraction — the AI describes,
// per call, which fields it wants read off whatever page the shared
// session currently has loaded (a CSS selector plus an optional
// attribute, one-or-many) and gets back exactly those values,
// structured, instead of the whole page's HTML. Read-only, never
// navigates anywhere itself — same shape as find_login_elements. Kept
// entirely separate from login/login_credentials.go by design — see
// plan/ai/tools/browser/step-09-yaml-instructed-extraction.md.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed extract.js
var extractJSTemplate string

// extractPayloadPlaceholder is the token in extract.js replaced with
// the real `{"fields": [...], "maxItems": N}` JSON before the script
// is handed to chromedp.Evaluate.
const extractPayloadPlaceholder = "__EXTRACT_PAYLOAD__"

// extractTimeout bounds one extraction — generous even though the DOM
// read itself is fast, matching findLoginElementsTimeout's own
// reasoning (it still has to wait its turn on sessionMu).
const extractTimeout = 10 * time.Second

// maxExtractedItems bounds how many matches a single `multiple: true`
// field ever returns — an overly broad selector (e.g. "div") could
// otherwise return thousands of values. A plain, non-configurable
// constant, matching maxHTMLBytes's own first-pass-number precedent.
const maxExtractedItems = 200

// maxExtractedValueLength bounds a single matched value's own length —
// open question 2 from this step's own design: maxExtractedItems only
// ever capped the *count* of matches for a multiple field, not one
// value's own size, so a selector matching a huge container element
// (e.g. accidentally selecting a whole <body>'s text content instead
// of a small title) could still return one enormous string. Measured
// in UTF-16 code units (JS string length, applied browser-side in
// extract.js) — the same "good enough, not grapheme-boundary-precise"
// approximation maxHTMLBytes's own byte-slicing already accepts for
// crawlPage.
const maxExtractedValueLength = 5000

type extractField struct {
	Label     string `json:"label" jsonschema:"a name for this field in the result, e.g. \"title\""`
	Selector  string `json:"selector" jsonschema:"CSS selector to match"`
	Attribute string `json:"attribute,omitempty" jsonschema:"DOM attribute to read (e.g. href, src); omit or \"text\" for the element's own text content"`
	Multiple  bool   `json:"multiple,omitempty" jsonschema:"true to collect every match, false for just the first"`
}

type extractRequest struct {
	Fields []extractField `json:"fields"`
}

type extractResponse struct {
	Results  map[string]any `json:"results"`
	NotFound []string       `json:"notFound"`
}

// extractHandler handles POST /extract — the --mcp adapter's own real
// target for extract_page_data.
func extractHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body extractRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(body.Fields) == 0 {
		http.Error(w, "fields is required", http.StatusBadRequest)
		return
	}
	for _, f := range body.Fields {
		if f.Label == "" || f.Selector == "" {
			http.Error(w, "every field requires both a label and a selector", http.StatusBadRequest)
			return
		}
	}

	result, err := performExtraction(body.Fields)
	if err != nil {
		http.Error(w, fmt.Sprintf("extract failed: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// performExtraction runs the caller-supplied fields against whatever
// page the shared session currently has loaded — never navigates,
// never submits anything. Holds sessionMu for the whole operation,
// same as crawlPage/findLoginElements. A thin lock+timeout wrapper
// around runExtractionOnCurrentPage — see that function's own doc
// comment for why the split exists.
func performExtraction(fields []extractField) (extractResponse, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	ctx, cancel := context.WithTimeout(sessionCtx, extractTimeout)
	defer cancel()

	return runExtractionOnCurrentPage(ctx, fields)
}

// runExtractionOnCurrentPage is performExtraction's own actual logic,
// factored out lock-free and context-free (the caller supplies both)
// so a caller that already holds sessionMu for a longer-lived
// operation — paginate.go's own multi-page crawl loop (step 16) —
// can invoke it directly without deadlocking sessionMu (sync.Mutex is
// not reentrant; performExtraction locking it again from inside an
// already-locked caller would hang forever). performExtraction's own
// external behavior/contract is unchanged by this split. See
// plan/ai/tools/browser/step-16-paginated-crawl-instructions.md.
func runExtractionOnCurrentPage(ctx context.Context, fields []extractField) (extractResponse, error) {
	payload := struct {
		Fields         []extractField `json:"fields"`
		MaxItems       int            `json:"maxItems"`
		MaxValueLength int            `json:"maxValueLength"`
	}{Fields: fields, MaxItems: maxExtractedItems, MaxValueLength: maxExtractedValueLength}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return extractResponse{}, fmt.Errorf("failed to build extraction payload: %w", err)
	}
	js := strings.Replace(extractJSTemplate, extractPayloadPlaceholder, string(payloadJSON), 1)

	var result extractResponse
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return extractResponse{}, err
	}
	if result.Results == nil {
		result.Results = map[string]any{}
	}
	if result.NotFound == nil {
		result.NotFound = []string{}
	}

	return result, nil
}

type extractPageDataArgs struct {
	Fields []extractField `json:"fields" jsonschema:"one entry per piece of data to extract from the currently loaded page"`
}

// registerExtractPageData adds the extract_page_data MCP tool — thin,
// like find_login_elements: only ever calls callSibling and formats
// the result.
func registerExtractPageData(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "extract_page_data",
		Description: "Read specific fields (by CSS selector) off the currently loaded page (see fetch_page_html) and return them structured, instead of the whole page's HTML. Read-only; submits nothing.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args extractPageDataArgs) (*mcp.CallToolResult, any, error) {
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

		reqBody, err := json.Marshal(extractRequest{Fields: args.Fields})
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to build request: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		respBody, err := callSibling("extract", reqBody)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		var result extractResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to parse extract response: %v", err)}},
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
