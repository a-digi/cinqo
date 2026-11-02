package crawler

import "unicode/utf8"

// maxAIListingValueRunes is how long a text value of a listing item may be in what crawl_paginated shows an AI.
// A job card's description snippet runs to several hundred characters and, copied by the model into
// check_crawl_extraction_result, made a crawl result tens of kilobytes long (79,000 prompt tokens in one run) and
// easy to mangle. The fields the check and a real crawl depend on stay whole.
const maxAIListingValueRunes = 300

// aiListingProtectedKeys are never shortened: the check needs a title and a url, and company, location and
// postedAt are short and used as they are.
var aiListingProtectedKeys = map[string]bool{"title": true, "url": true, "company": true, "location": true, "postedAt": true}

// capListingItemValues shortens long text values of the grouped listing items (pages[].items) in place, with a
// trailing "…". Flat results (no container: a single detail page, where the full text is the very thing being
// checked) are left alone, and so is everything Career's own "Crawl now" reads: that path does not go through this.
func capListingItemValues(pages []pageExtractResult) {
	for _, page := range pages {
		for _, item := range page.Items {
			for key, value := range item {
				if aiListingProtectedKeys[key] {
					continue
				}
				item[key] = capValue(value)
			}
		}
	}
}

func capValue(v any) any {
	switch t := v.(type) {
	case string:
		return capString(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = capValue(e)
		}
		return out
	default:
		return v
	}
}

func capString(s string) string {
	if utf8.RuneCountInString(s) <= maxAIListingValueRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxAIListingValueRunes]) + "…"
}
