package crawler

import (
	"fmt"
	"strings"
)

// extraction_check.go (plan/ai/conversation/step-45, step 2) gives crawl_paginated's result for an AI its own verdict, so the model
// no longer has to retype the whole crawl into check_crawl_extraction_result (the retyped copies cost 6,000 to 12,000 completion
// tokens each and were re-read in every later request). The rule is a copy of Career's classifyCrawlResultPages
// (tools/career/backend/portal/crawl_extraction_check.go, plan 80 and 81): the same fields, the same title-and-url requirement, the
// same tolerance for completely empty rows, the same low-count warning and the same texts. The two copies are kept equal by one
// table of pages and expected verdicts, tools/testdata/crawl_extraction_check.json, that the tests of both modules run.
// Career stays the authority for what a real crawl saves; this verdict only says what such a crawl will most likely do.

// extractionCheck is the verdict that comes with the crawl: the same JSON as Career's CrawlExtractionReport.
type extractionCheck struct {
	ItemsFound       int              `json:"itemsFound"`
	ItemsExtractable int              `json:"itemsExtractable"`
	ItemsSkipped     int              `json:"itemsSkipped"`
	BlankItems       int              `json:"blankItems,omitempty"`
	OK               bool             `json:"ok"`
	SkippedSamples   []map[string]any `json:"skippedSamples,omitempty"`
	Message          string           `json:"message"`
	Warning          string           `json:"warning,omitempty"`
}

// lowItemCountThreshold: 1 or 2 items in total warn, 3 or more do not, 0 has its own message.
const lowItemCountThreshold = 2

// blankItemsToleratedPercent: the largest share of all items (in percent) that may be completely empty for ok to stay true.
const blankItemsToleratedPercent = 25

func extractionCheckForPages(pages []pageExtractResult) extractionCheck {
	var report extractionCheck
	count := func(pageNo, index int, title, sourceURL, company, location, description, postedAt string) {
		report.ItemsFound++
		if title != "" && sourceURL != "" {
			report.ItemsExtractable++
			return
		}
		report.ItemsSkipped++
		if title == "" && sourceURL == "" && company == "" && location == "" && description == "" && postedAt == "" {
			report.BlankItems++
		}
		if len(report.SkippedSamples) < 3 {
			report.SkippedSamples = append(report.SkippedSamples, map[string]any{
				"page": pageNo, "index": index,
				"title": title, "url": sourceURL, "company": company, "location": location, "description": description, "postedAt": postedAt,
			})
		}
	}
	for pi, page := range pages {
		if len(page.Items) > 0 {
			for i, item := range page.Items {
				title, sourceURL, company, location, description, postedAt := checkItemFields(item)
				count(pi+1, i+1, title, sourceURL, company, location, description, postedAt)
			}
			continue
		}
		titles := checkStrings(page.Results["title"])
		for i := range titles {
			title, sourceURL, company, location, description, postedAt := checkIndexedFields(page, i)
			count(pi+1, i+1, title, sourceURL, company, location, description, postedAt)
		}
	}
	broken := report.ItemsSkipped - report.BlankItems
	blankTolerated := report.BlankItems*100 <= report.ItemsFound*blankItemsToleratedPercent
	report.OK = report.ItemsFound > 0 && broken == 0 && blankTolerated
	switch {
	case report.ItemsFound == 0:
		report.Message = "No items were extracted at all — check the container selector (it matched nothing), or the field selectors if this is a single detail page. The page's real content may also not be present in a plain fetch_page_html (client-side-rendered content) — re-inspect the actual HTML you fetched. This is a failure to resolve, never a result to save: make ONE fix attempt, and if a second test still finds 0 items, call ask_user_if_page_has_jobs (once) instead of retrying further."
	case broken > 0:
		report.Message = fmt.Sprintf("%d of %d extracted item(s) are missing a title and/or a url — see skippedSamples for the raw fields that came back empty. Every item must produce both before these instructions can be saved.", report.ItemsSkipped, report.ItemsFound)
		if report.BlankItems > 0 {
			report.Message += fmt.Sprintf(" (%d of them are completely empty rows, probably ads or placeholders: those alone would not stop saving, the others point to a wrong field selector.)", report.BlankItems)
		}
	case !blankTolerated:
		report.Message = fmt.Sprintf("%d of %d extracted item(s) are completely empty, more than %d%%: the container probably matches the wrong elements (ads, wrappers, placeholders), not just a few stray rows. Narrow the container so it matches only the job entries, for example with :has(<your title selector>), and test again.", report.BlankItems, report.ItemsFound, blankItemsToleratedPercent)
	case report.BlankItems > 0:
		report.Message = fmt.Sprintf("%d of %d extracted item(s) are completely empty (probably ads or placeholders the container also matches). A real crawl skips them and saves the other %d, so these instructions can be saved. To avoid them anyway, narrow the container, for example with :has(<your title selector>).", report.BlankItems, report.ItemsFound, report.ItemsExtractable)
	default:
		report.Message = fmt.Sprintf("All %d extracted item(s) produced both a title and a url — ready to save.", report.ItemsFound)
	}
	if report.ItemsFound > 0 && report.ItemsFound <= lowItemCountThreshold {
		report.Warning = fmt.Sprintf("Only %d item(s) were extracted in total. A listing page usually shows many. Compare this with the number of repeating job entries you saw in the page HTML: if the page shows more, your container selector matches only a wrapper or part of the page and must be fixed before you save. If the page really lists this few, you may save.", report.ItemsFound)
	}
	return report
}

// checkStrings normalizes one extracted value (a string, a list of strings or absent) to the non-empty strings in it.
func checkStrings(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func checkAt(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return ""
}

func checkItemFields(item map[string]any) (title, sourceURL, company, location, description, postedAt string) {
	title = checkSanitizeTitle(checkAt(checkStrings(item["title"]), 0))
	sourceURL = checkAt(checkStrings(item["url"]), 0)
	company = checkAt(checkStrings(item["company"]), 0)
	location = checkAt(checkStrings(item["location"]), 0)
	description = checkAt(checkStrings(item["description"]), 0)
	postedAt = checkSanitizePostedAt(checkAt(checkStrings(item["postedAt"]), 0))
	return
}

func checkIndexedFields(page pageExtractResult, i int) (title, sourceURL, company, location, description, postedAt string) {
	title = checkSanitizeTitle(checkAt(checkStrings(page.Results["title"]), i))
	sourceURL = checkAt(checkStrings(page.Results["url"]), i)
	company = checkAt(checkStrings(page.Results["company"]), i)
	location = checkAt(checkStrings(page.Results["location"]), i)
	description = checkAt(checkStrings(page.Results["description"]), i)
	postedAt = checkSanitizePostedAt(checkAt(checkStrings(page.Results["postedAt"]), i))
	return
}

// checkSanitizeTitle strips digits and colons from a scraped title (a stray index badge like "2:") and collapses the whitespace,
// exactly as Career's sanitizeTitle does before it decides whether a title is empty.
func checkSanitizeTitle(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if (r >= '0' && r <= '9') || r == ':' {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// checkSanitizePostedAt keeps only digits and the delimiters . / - and gives "" when nothing date-shaped is left, exactly as
// Career's sanitizePostedAt does.
func checkSanitizePostedAt(raw string) string {
	var b strings.Builder
	hasDigit, hasDelimiter := false, false
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
			b.WriteRune(r)
		case r == '.' || r == '/' || r == '-':
			hasDelimiter = true
			b.WriteRune(r)
		}
	}
	if !hasDigit || !hasDelimiter {
		return ""
	}
	return b.String()
}
