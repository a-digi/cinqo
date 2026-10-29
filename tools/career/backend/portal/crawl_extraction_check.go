// crawl_extraction_check.go (step 81) adds check_crawl_extraction_result
// — a purely computational MCP tool the AI calls, during "Generate
// instructions," against its own crawl_paginated test output, so the
// AI never has to trust its own arithmetic on a raw JSON blob to decide
// whether a draft listing crawl instructions document actually works.
// See plan/ai/tools/career/step-81-check-crawl-extraction-result-tool.md
// and, for the root-cause story this whole feature exists to close,
// plan/ai/tools/career/step-79-ai-crawl-instructions-acceptance-loop.md.
package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// CrawlExtractionReport is check_crawl_extraction_result's own return
// shape — see that tool's registration for the full contract. Also
// usable internally as a diagnostic if IngestCrawlResults itself ever
// wants to log a richer summary than its own plain JobsSkipped count
// (not done in this step — out of scope, mentioned only so a future
// reader knows this type isn't test-only).
type CrawlExtractionReport struct {
	ItemsFound       int              `json:"itemsFound"`
	ItemsExtractable int              `json:"itemsExtractable"`
	ItemsSkipped     int              `json:"itemsSkipped"`
	OK               bool             `json:"ok"`
	SkippedSamples   []map[string]any `json:"skippedSamples,omitempty"`
	Message          string           `json:"message"`
	// Warning (step 92) is set when only a handful of items came back in
	// total. ok stays true (a page can really list one job), but a listing
	// page usually shows many, so one or two items most often means the
	// container selector matches a wrapper or part of the page. It never
	// changes ok; the agent must compare the count with the page's HTML.
	Warning string `json:"warning,omitempty"`
}

// lowItemCountThreshold is the highest total item count that triggers
// CrawlExtractionReport.Warning: 1 or 2 items warn, 3 or more do not, and 0
// has its own message.
const lowItemCountThreshold = 2

// classifyCrawlResultPages applies IngestCrawlResults' own title/url-
// required rule (via step 80's extractCrawlResultItemFields/
// extractCrawlResultIndexedFields) WITHOUT persisting anything — the
// single source of truth both IngestCrawlResults (which saves) and
// check_crawl_extraction_result (which only reports, during
// instruction authoring) rely on, so a "ready to save" verdict can
// never diverge from what a real crawl actually does. See
// plan/ai/tools/career/step-81-check-crawl-extraction-result-tool.md.
func classifyCrawlResultPages(pages []CrawlResultPage) CrawlExtractionReport {
	var report CrawlExtractionReport
	sample := func(fields map[string]any) {
		if len(report.SkippedSamples) < 3 {
			report.SkippedSamples = append(report.SkippedSamples, fields)
		}
	}
	for _, page := range pages {
		if len(page.Items) > 0 {
			for _, item := range page.Items {
				title, sourceURL, company, location, description, postedAt := extractCrawlResultItemFields(item)
				report.ItemsFound++
				if title == "" || sourceURL == "" {
					report.ItemsSkipped++
					sample(map[string]any{"title": title, "url": sourceURL, "company": company, "location": location, "description": description, "postedAt": postedAt})
					continue
				}
				report.ItemsExtractable++
			}
			continue
		}
		titles := ExtractResultStrings(page.Results["title"])
		for i := range titles {
			title, sourceURL, company, location, description, postedAt := extractCrawlResultIndexedFields(page, i)
			report.ItemsFound++
			if title == "" || sourceURL == "" {
				report.ItemsSkipped++
				sample(map[string]any{"title": title, "url": sourceURL, "company": company, "location": location, "description": description, "postedAt": postedAt})
				continue
			}
			report.ItemsExtractable++
		}
	}
	report.OK = report.ItemsFound > 0 && report.ItemsSkipped == 0
	switch {
	case report.ItemsFound == 0:
		report.Message = "No items were extracted at all — check the container selector (it matched nothing), or the field selectors if this is a single detail page. The page's real content may also not be present in a plain fetch_page_html (client-side-rendered content) — re-inspect the actual HTML you fetched. This is a failure to resolve, never a result to save: make ONE fix attempt, and if a second test still finds 0 items, call ask_user_if_page_has_jobs (once) instead of retrying further."
	case report.ItemsSkipped > 0:
		report.Message = fmt.Sprintf("%d of %d extracted item(s) are missing a title and/or a url — see skippedSamples for the raw fields that came back empty. Every item must produce both before these instructions can be saved.", report.ItemsSkipped, report.ItemsFound)
	default:
		report.Message = fmt.Sprintf("All %d extracted item(s) produced both a title and a url — ready to save.", report.ItemsFound)
	}
	if report.ItemsFound > 0 && report.ItemsFound <= lowItemCountThreshold {
		report.Warning = fmt.Sprintf("Only %d item(s) were extracted in total. A listing page usually shows many. Compare this with the number of repeating job entries you saw in the page HTML: if the page shows more, your container selector matches only a wrapper or part of the page and must be fixed before you save. If the page really lists this few, you may save.", report.ItemsFound)
	}
	return report
}

// CrawlPagesArg is the pages argument: the array crawl_paginated returned, or
// (step 93) that same array as a JSON string. Weaker models often retype the
// array as a string; accepting it costs nothing and saves them a rejected
// call. The published schema allows both (see the registration below), so the
// framework does not reject the string before this ever runs.
type CrawlPagesArg []CrawlResultPage

func (p *CrawlPagesArg) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return err
		}
		var pages []CrawlResultPage
		if err := json.Unmarshal([]byte(text), &pages); err != nil {
			return fmt.Errorf("pages was a string but not a JSON array of pages — pass the 'pages' array from crawl_paginated unchanged: %w", err)
		}
		*p = pages
		return nil
	}
	var pages []CrawlResultPage
	if err := json.Unmarshal(trimmed, &pages); err != nil {
		return err
	}
	*p = pages
	return nil
}

type checkCrawlExtractionResultArgs struct {
	Pages CrawlPagesArg `json:"pages" jsonschema:"the exact 'pages' array from a crawl_paginated call you just made with your DRAFT instructions against this same URL — pass it through unchanged, do not retype or summarize it by hand."`
}

// checkCrawlExtractionResultSchema is the schema inferred from the args
// struct, with the string form of pages allowed in addition to the array.
func checkCrawlExtractionResultSchema() (*jsonschema.Schema, error) {
	schema, err := jsonschema.For[checkCrawlExtractionResultArgs](nil)
	if err != nil {
		return nil, err
	}
	pages := schema.Properties["pages"]
	if pages == nil {
		return nil, fmt.Errorf("check_crawl_extraction_result: inferred schema has no pages property")
	}
	switch {
	case len(pages.Types) > 0:
		pages.Types = append(pages.Types, "string")
	case pages.Type != "":
		pages.Types = []string{pages.Type, "string"}
		pages.Type = ""
	}
	return schema, nil
}

func RegisterCheckCrawlExtractionResult(server *mcp.Server) {
	schema, err := checkCrawlExtractionResultSchema()
	if err != nil {
		panic(err) // a programming error caught by the first test run, not a runtime condition
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_crawl_extraction_result",
		InputSchema: schema,
		Description: "Check whether a crawl_paginated result (pass its own " +
			"'pages' array here, unchanged) would actually produce usable job " +
			"postings — the same title-and-url-required rule the deterministic " +
			"\"Crawl now\" button itself applies when deciding what to save versus " +
			"skip. Call this after every crawl_paginated test run of your DRAFT " +
			"listing crawl instructions, before calling " +
			"set_portal_link_crawl_instructions — do not count skipped/extracted " +
			"items yourself from the raw JSON; this tool's own count is the one " +
			"that matters, since it is the exact rule a real crawl will use. " +
			"ok: true means every extracted item produced both a title and a url " +
			"(and at least one item was found at all) — only then are these " +
			"instructions ready to save. ok: false means fix your container/" +
			"field selectors/mapping using skippedSamples (the raw fields of up " +
			"to 3 items that failed) and test again. A \"warning\" field (only 1 or 2 " +
			"items extracted in total) does not change ok, but it means your " +
			"container selector may match just a wrapper or part of the page: compare " +
			"the count with the repeating job entries in the HTML you fetched and fix " +
			"the container before saving if the page shows more.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args checkCrawlExtractionResultArgs) (*mcp.CallToolResult, any, error) {
		if len(args.Pages) == 0 {
			return db.ErrResult("pages is required — pass the 'pages' array from a crawl_paginated call"), nil, nil
		}
		report := classifyCrawlResultPages([]CrawlResultPage(args.Pages))
		return db.JSONResult(report)
	})
}
