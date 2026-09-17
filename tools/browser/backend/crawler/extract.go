// extract.go implements structured extraction — the AI describes,
// per call, which fields it wants read off whatever page the shared
// session currently has loaded (a CSS selector plus an optional
// attribute, one-or-many) and gets back exactly those values,
// structured, instead of the whole page's HTML. Read-only, never
// navigates anywhere itself — same shape as find_login_elements. Kept
// entirely separate from login/login_credentials.go by design — see
// plan/ai/tools/browser/step-09-yaml-instructed-extraction.md.
package crawler

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

	"browser-tool-backend/shared"
)

//go:embed extract.js
var extractJSTemplate string

// extractPayloadPlaceholder is the token in extract.js replaced with
// the real `{"fields": [...], "maxItems": N}` JSON before the script
// is handed to chromedp.Evaluate.
const extractPayloadPlaceholder = "__EXTRACT_PAYLOAD__"

// extractTimeout bounds one extraction — generous even though the DOM
// read itself is fast, matching findLoginElementsTimeout's own
// reasoning (it still has to wait its turn on shared.Mu).
const extractTimeout = 10 * time.Second

// maxExtractedItems bounds how many matches a single `multiple: true`
// field ever returns — an overly broad selector (e.g. "div") could
// otherwise return thousands of values. A plain, non-configurable
// constant, matching MaxHTMLBytes's own first-pass-number precedent.
const maxExtractedItems = 200

// maxExtractedValueLength bounds a single matched value's own length —
// open question 2 from this step's own design: maxExtractedItems only
// ever capped the *count* of matches for a multiple field, not one
// value's own size, so a selector matching a huge container element
// (e.g. accidentally selecting a whole <body>'s text content instead
// of a small title) could still return one enormous string. Measured
// in UTF-16 code units (JS string length, applied browser-side in
// extract.js) — the same "good enough, not grapheme-boundary-precise"
// approximation MaxHTMLBytes's own byte-slicing already accepts for
// crawlPage.
const maxExtractedValueLength = 5000

type extractField struct {
	Label     string `json:"label" jsonschema:"a name for this field in the result, e.g. \"title\""`
	Selector  string `json:"selector" jsonschema:"CSS selector to match"`
	Attribute string `json:"attribute,omitempty" jsonschema:"DOM attribute to read (e.g. href, src); omit or \"text\" for the element's own text content"`
	Multiple  bool   `json:"multiple,omitempty" jsonschema:"true to collect every match, false for just the first"`
}

type extractRequest struct {
	// Container (step 18) is an optional CSS selector for each
	// repeating item's own wrapping element — when set, every field's
	// own selector is evaluated relative to each matched container
	// instead of the whole document, and the response's own Items
	// (not Results) is populated: one correctly-grouped object per
	// real item, instead of separate same-length arrays that may not
	// actually correspond to the same item. See
	// plan/ai/tools/browser/step-18-grouped-container-extraction.md.
	Container string         `json:"container,omitempty"`
	Fields    []extractField `json:"fields"`
	// Mapping (step 20) is an optional {sourceLabel: targetKey} — when
	// set, every key in the response (Results/Items entries, and
	// NotFound) whose original field label appears as a mapping source
	// is renamed to its target before the response is returned. Lets a
	// caller extract under whatever labels are natural for the page
	// while a downstream consumer still gets a specific, fixed set of
	// output keys. A label absent from Mapping passes through
	// unchanged. See
	// plan/ai/tools/browser/step-20-output-field-mapping.md.
	Mapping map[string]string `json:"mapping,omitempty"`
}

type extractResponse struct {
	// Results is populated in flat mode (no Container) — unchanged
	// from before step 18.
	Results map[string]any `json:"results,omitempty"`
	// Items is populated in grouped mode (Container set) — one object
	// per matched container, shaped like Results would be for that one
	// item alone. Exactly one of Results/Items is ever non-nil.
	Items    []map[string]any `json:"items,omitempty"`
	NotFound []string         `json:"notFound"`
}

// ExtractHandler handles POST /extract — the --mcp adapter's own real
// target for extract_page_data.
func ExtractHandler(w http.ResponseWriter, r *http.Request) {
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
	if err := validateMapping(body.Fields, body.Mapping); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := performExtraction(body.Container, body.Fields, body.Mapping)
	if err != nil {
		http.Error(w, fmt.Sprintf("extract failed: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// performExtraction runs the caller-supplied fields against whatever
// page the shared session currently has loaded — never navigates,
// never submits anything. Holds shared.Mu for the whole operation,
// same as crawlPage/findLoginElements. A thin lock+timeout wrapper
// around runExtractionOnCurrentPage — see that function's own doc
// comment for why the split exists.
func performExtraction(container string, fields []extractField, mapping map[string]string) (extractResponse, error) {
	if err := shared.EnsureSharedSession(); err != nil {
		return extractResponse{}, err
	}
	shared.Mu.Lock()
	defer shared.Mu.Unlock()

	// step 47.2/47.3 — fail fast on a tab left wedged by a previous,
	// unrelated caller instead of discovering it only after burning
	// extractTimeout on an evaluation that was never going to complete,
	// and replace the wedged tab immediately (still holding shared.Mu)
	// so the NEXT caller gets a fresh, healthy session instead of
	// inheriting the same wedge.
	if err := shared.ProbeSessionLiveness(shared.Ctx); err != nil {
		recreateErr := shared.RecreateSharedSessionLocked()
		return extractResponse{}, NewSessionWedgedError(err, recreateErr)
	}

	ctx, cancel := context.WithTimeout(shared.Ctx, extractTimeout)
	defer cancel()

	return runExtractionOnCurrentPage(ctx, container, fields, mapping)
}

// runExtractionOnCurrentPage is performExtraction's own actual logic,
// factored out lock-free and context-free (the caller supplies both)
// so a caller that already holds shared.Mu for a longer-lived
// operation — paginate.go's own multi-page crawl loop (step 16) —
// can invoke it directly without deadlocking shared.Mu (sync.Mutex is
// not reentrant; performExtraction locking it again from inside an
// already-locked caller would hang forever). performExtraction's own
// external behavior/contract is unchanged by this split. See
// plan/ai/tools/browser/step-16-paginated-crawl-instructions.md.
func runExtractionOnCurrentPage(ctx context.Context, container string, fields []extractField, mapping map[string]string) (extractResponse, error) {
	payload := struct {
		Container      string         `json:"container,omitempty"`
		Fields         []extractField `json:"fields"`
		MaxItems       int            `json:"maxItems"`
		MaxValueLength int            `json:"maxValueLength"`
	}{Container: container, Fields: fields, MaxItems: maxExtractedItems, MaxValueLength: maxExtractedValueLength}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return extractResponse{}, fmt.Errorf("failed to build extraction payload: %w", err)
	}
	js := strings.Replace(extractJSTemplate, extractPayloadPlaceholder, string(payloadJSON), 1)

	var result extractResponse
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return extractResponse{}, err
	}
	// Default only the mode actually in play — grouped mode (Container
	// set) leaves Results nil/omitted rather than forcing it to {}, and
	// vice versa for Items, so the response's own "exactly one of
	// Results/Items is populated" contract holds even for a page with
	// zero matches.
	if container != "" {
		if result.Items == nil {
			result.Items = []map[string]any{}
		}
	} else if result.Results == nil {
		result.Results = map[string]any{}
	}
	if result.NotFound == nil {
		result.NotFound = []string{}
	}

	applyFieldMapping(&result, mapping)

	return result, nil
}

// applyFieldMapping renames keys in resp.Results/resp.Items (whichever is
// populated) and resp.NotFound from their original field label to
// mapping's own target, in place — step 20's own "schema mapper": lets a
// caller extract under whatever labels are natural for a page while a
// downstream consumer still gets a specific, fixed set of output keys. A
// label absent from mapping passes through unchanged. mapping == nil (or
// empty) is a no-op, preserving the exact pre-step-20 response shape —
// including Results/Items' own nil-ness, which a naive unconditional
// rebuild would otherwise lose. See
// plan/ai/tools/browser/step-20-output-field-mapping.md.
func applyFieldMapping(resp *extractResponse, mapping map[string]string) {
	if len(mapping) == 0 {
		return
	}
	rename := func(m map[string]any) map[string]any {
		out := make(map[string]any, len(m))
		for k, v := range m {
			if target, ok := mapping[k]; ok {
				out[target] = v
			} else {
				out[k] = v
			}
		}
		return out
	}
	if resp.Results != nil {
		resp.Results = rename(resp.Results)
	}
	for i, item := range resp.Items {
		resp.Items[i] = rename(item)
	}
	renamed := make([]string, len(resp.NotFound))
	for i, label := range resp.NotFound {
		if target, ok := mapping[label]; ok {
			renamed[i] = target
		} else {
			renamed[i] = label
		}
	}
	resp.NotFound = renamed
}

// validateMapping rejects a mapping that would produce an ambiguous
// output — two of the caller's declared fields (after mapping is
// applied, when it applies) resolving to the same effective key. Reused
// by both extract_page_data and crawl_paginated's own registration
// handlers (tools/browser/backend/paginate.go). A mapping source label
// that doesn't match any declared field is not an error — a harmless
// no-op in applyFieldMapping, kept that way here too so a caller tweaking
// fields doesn't also have to keep mapping in lockstep. See
// plan/ai/tools/browser/step-20-output-field-mapping.md.
func validateMapping(fields []extractField, mapping map[string]string) error {
	if len(mapping) == 0 {
		return nil
	}
	seenBySource := make(map[string]string, len(fields)) // effective target -> original source label
	for _, f := range fields {
		target := f.Label
		if t, ok := mapping[f.Label]; ok {
			target = t
		}
		if prevSource, exists := seenBySource[target]; exists && prevSource != f.Label {
			return fmt.Errorf("mapping target %q is used by both %q and %q — each target key must be unique", target, prevSource, f.Label)
		}
		seenBySource[target] = f.Label
	}
	return nil
}

type extractPageDataArgs struct {
	Container string            `json:"container,omitempty" jsonschema:"CSS selector for each repeating item's own wrapping element (e.g. one job listing's own <div> or <li>). Set this whenever the page LISTS MULTIPLE similar items at once (a search-results/job-listing/product-catalog page) and more than one field describes each one — this is the correct default for a listing page, not an optional extra. Leave unset only for a page describing a single item. With it, fields are evaluated relative to each item and grouped correctly; without it on a listing page, fields describing multiple items are returned as separate arrays that may NOT actually correspond position-for-position to the same real item."`
	Fields    []extractField    `json:"fields" jsonschema:"one entry per piece of data to extract — relative to each container match when container is set, relative to the whole page otherwise"`
	Mapping   map[string]string `json:"mapping,omitempty" jsonschema:"optional {sourceLabel: targetKey} — renames extracted fields to specific output key names before they're returned, e.g. when a consumer expects a fixed schema (title/url/company/...) but this page's own natural fields are better labeled job_title/link/employer. Fields not listed pass through under their own label."`
}

// RegisterExtractPageData adds the extract_page_data MCP tool — thin,
// like find_login_elements: only ever calls shared.CallSibling and formats
// the result.
func RegisterExtractPageData(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "extract_page_data",
		Description: "Read specific fields (by CSS selector) off the currently loaded page (see fetch_page_html) and return them structured, instead of the whole page's HTML. Read-only; submits nothing. " +
			"Before calling this, check whether the page LISTS MULTIPLE similar items at once (e.g. a job board's own search-results page) or describes just ONE item. For a listing page, always pass container — a selector for one item's own repeating wrapping element — so the result is one correctly-grouped object per item (in `items`); this is the default correct approach for a listing page, not a fallback for when something looks wrong. Omitting container on a listing page returns separate same-length arrays (in `results`) that may NOT actually correspond position-for-position to the same real item. " +
			"Optionally set mapping to rename extracted fields to specific output keys — e.g. a consuming tool expects title/url but this page's own natural fields are better labeled job_title/link.",
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
		if err := validateMapping(args.Fields, args.Mapping); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		reqBody, err := json.Marshal(extractRequest{Container: args.Container, Fields: args.Fields, Mapping: args.Mapping})
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to build request: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		respBody, err := shared.CallSibling("extract", reqBody)
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
