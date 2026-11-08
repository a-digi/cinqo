package portal

import (
	"encoding/json"
	"strings"
	"testing"
)

// Step 160: an item whose six fields are all empty is a row the container also matches (an ad, a placeholder),
// not a job with a wrong selector. A few of them must not stop ok: a real crawl skips them and saves the rest.

func goodItem(i int) map[string]any {
	return map[string]any{"title": "Software Engineer", "url": "https://x/job/" + string(rune('a'+i%26)) + string(rune('a'+i/26)), "company": "Acme"}
}

// pageOf builds one page of `good` real items followed by `blank` empty ones and `broken` items that have a
// company but no title or url.
func pageOf(good, blank, broken int) []CrawlResultPage {
	var items []map[string]any
	for i := 0; i < good; i++ {
		items = append(items, goodItem(i))
	}
	for i := 0; i < blank; i++ {
		items = append(items, map[string]any{}) // what the crawl returned for the four ad rows on Indeed
	}
	for i := 0; i < broken; i++ {
		items = append(items, map[string]any{"company": "Acme only"})
	}
	return []CrawlResultPage{{Items: items}}
}

func TestSixteenRealAndFourBlankIsOK(t *testing.T) {
	r := classifyCrawlResultPages(pageOf(16, 4, 0)) // the Indeed run that ran out of rounds
	if !r.OK || r.ItemsFound != 20 || r.ItemsExtractable != 16 || r.ItemsSkipped != 4 || r.BlankItems != 4 {
		t.Fatalf("16 real + 4 blank: %+v", r)
	}
	for _, part := range []string{"4 of 20", "completely empty", "ads or placeholders", "saves the other 16", ":has(<your title selector>)", "can be saved"} {
		if !strings.Contains(r.Message, part) {
			t.Errorf("the message should say %q: %s", part, r.Message)
		}
	}
	if r.Warning != "" {
		t.Errorf("blank items must not use the low-count warning (step 6b would misread it): %q", r.Warning)
	}
}

func TestTheBlankShareLimitIsAQuarterOfAllItems(t *testing.T) {
	for _, c := range []struct {
		good, blank int
		ok          bool
	}{
		{20, 0, true},
		{19, 1, true},
		{15, 5, true},  // 25% exactly passes
		{14, 6, false}, // 30%
		{1, 3, false},  // 75%
		{3, 1, true},   // 25% of 4
		{2, 1, false},  // 33%
		{0, 5, false},  // nothing real at all
	} {
		r := classifyCrawlResultPages(pageOf(c.good, c.blank, 0))
		want := c.ok
		if r.OK != want {
			t.Errorf("%d real + %d blank: ok = %v, want %v (%s)", c.good, c.blank, r.OK, want, r.Message)
		}
		if !want && c.good+c.blank > 0 && (!strings.Contains(r.Message, "more than 25%") || !strings.Contains(r.Message, ":has(<your title selector>)")) {
			t.Errorf("%d real + %d blank: the message should say it is more than 25%% and how to narrow the container: %s", c.good, c.blank, r.Message)
		}
	}
}

func TestBrokenItemsAreNeverTolerated(t *testing.T) {
	r := classifyCrawlResultPages(pageOf(19, 0, 1))
	if r.OK || r.BlankItems != 0 || r.ItemsSkipped != 1 {
		t.Fatalf("one item with a company but no title/url must still block: %+v", r)
	}
	if !strings.Contains(r.Message, "1 of 20 extracted item(s) are missing a title and/or a url") || strings.Contains(r.Message, "completely empty") {
		t.Errorf("the original message, without a blank note: %s", r.Message)
	}
	// Every single field counts as data: a row with only one of the six filled in is broken, not blank.
	for _, field := range []string{"title", "url", "company", "location", "description", "postedAt"} {
		value := "x"
		if field == "postedAt" {
			value = "2026-10-30" // a postedAt without digits and a date delimiter is dropped by sanitizePostedAt, as in a real crawl
		}
		page := []CrawlResultPage{{Items: append(pageOf(19, 0, 0)[0].Items, map[string]any{field: value})}}
		got := classifyCrawlResultPages(page)
		if got.BlankItems != 0 || got.OK {
			t.Errorf("a row holding only %q is broken, not blank: %+v", field, got)
		}
	}
}

func TestBlankAndBrokenTogetherStillBlockAndSayWhy(t *testing.T) {
	r := classifyCrawlResultPages(pageOf(16, 3, 1))
	if r.OK || r.BlankItems != 3 || r.ItemsSkipped != 4 {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(r.Message, "4 of 20 extracted item(s) are missing a title and/or a url") ||
		!strings.Contains(r.Message, "3 of them are completely empty rows") || !strings.Contains(r.Message, "wrong field selector") {
		t.Errorf("the message should keep the original sentence and explain the split: %s", r.Message)
	}
}

func TestAllBlankIsNotOK(t *testing.T) {
	r := classifyCrawlResultPages(pageOf(0, 6, 0))
	if r.OK || r.ItemsFound != 6 || r.BlankItems != 6 || r.ItemsExtractable != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestTheParallelArraysShapeDropsEmptyEntriesAndFlagsAMissingUrl(t *testing.T) {
	// No container: results come as parallel arrays and empty entries are dropped before items are counted (as
	// ExtractResultStrings always did), so a blank row cannot show up there; a title with no url at its index
	// is a broken item, never a blank one.
	page := CrawlResultPage{Results: map[string]any{
		"title": []any{"A", "B", "C", "D", "E", "F", "G", "H", "", "I"},
		"url":   []any{"/a", "/b", "/c", "/d", "/e", "/f", "/g", "/h", ""},
	}}
	r := classifyCrawlResultPages([]CrawlResultPage{page})
	if r.ItemsFound != 9 || r.ItemsExtractable != 8 || r.ItemsSkipped != 1 || r.BlankItems != 0 || r.OK {
		t.Fatalf("title I has no url: %+v", r)
	}
	page.Results["url"] = []any{"/a", "/b", "/c", "/d", "/e", "/f", "/g", "/h", "/i"}
	if r := classifyCrawlResultPages([]CrawlResultPage{page}); !r.OK || r.ItemsFound != 9 || r.BlankItems != 0 {
		t.Fatalf("complete arrays: %+v", r)
	}
}

func TestSkippedSamplesSayWhereTheItemSits(t *testing.T) {
	pages := append(pageOf(2, 0, 0), CrawlResultPage{Items: []map[string]any{goodItem(5), {}, goodItem(6), {}}})
	for i := 0; i < 8; i++ {
		pages = append(pages, CrawlResultPage{Items: []map[string]any{goodItem(i + 10)}})
	}
	r := classifyCrawlResultPages(pages)
	if len(r.SkippedSamples) != 2 {
		t.Fatalf("two blank items, two samples: %+v", r.SkippedSamples)
	}
	want := [][2]int{{2, 2}, {2, 4}}
	for i, w := range want {
		if r.SkippedSamples[i]["page"] != w[0] || r.SkippedSamples[i]["index"] != w[1] {
			t.Errorf("sample %d: page/index = %v/%v, want %d/%d", i, r.SkippedSamples[i]["page"], r.SkippedSamples[i]["index"], w[0], w[1])
		}
		for _, k := range []string{"title", "url", "company", "location", "description", "postedAt"} {
			if _, ok := r.SkippedSamples[i][k]; !ok {
				t.Errorf("sample %d lost its %q field", i, k)
			}
		}
	}
	// at most three samples, as before
	if r := classifyCrawlResultPages(pageOf(30, 5, 0)); len(r.SkippedSamples) != 3 {
		t.Errorf("at most 3 samples: %d", len(r.SkippedSamples))
	}
}

func TestBlankItemsAreOmittedFromTheJSONWhenThereAreNone(t *testing.T) {
	b, _ := json.Marshal(classifyCrawlResultPages(pageOf(5, 0, 0)))
	if strings.Contains(string(b), "blankItems") {
		t.Errorf("no blankItems expected: %s", b)
	}
	b, _ = json.Marshal(classifyCrawlResultPages(pageOf(16, 4, 0)))
	if !strings.Contains(string(b), `"blankItems":4`) || !strings.Contains(string(b), `"ok":true`) {
		t.Errorf("blankItems 4 and ok true expected: %s", b)
	}
}

func TestTheInheritTrialUsesTheSameRule(t *testing.T) {
	pass := ClassifyTrialPages(pageOf(16, 4, 0))
	if !ListingTrialPasses(pass) || ListingTrialFailureReason(pass) != "" {
		t.Errorf("a trial with 4 blank of 20 passes like check_crawl_extraction_result: %+v", pass)
	}
	fail := ClassifyTrialPages(pageOf(14, 6, 0))
	if ListingTrialPasses(fail) || ListingTrialFailureReason(fail) == "" {
		t.Errorf("6 blank of 20 does not pass: %+v", fail)
	}
	if got := ListingTrialFailureReason(ClassifyTrialPages(pageOf(19, 0, 1))); got != "1 of 20 items had no title or url" {
		t.Errorf("a broken item keeps its reason: %q", got)
	}
}

func TestUnchangedVerdictsStayUnchanged(t *testing.T) {
	if r := classifyCrawlResultPages(pageOf(5, 0, 0)); !r.OK || !strings.Contains(r.Message, "All 5 extracted item(s) produced both a title and a url — ready to save.") {
		t.Errorf("all good: %+v", r)
	}
	if r := classifyCrawlResultPages(nil); r.OK || !strings.Contains(r.Message, "No items were extracted at all") || !strings.Contains(r.Message, "call ask_user_if_page_has_jobs (once) instead of retrying further.") {
		t.Errorf("no items keeps its own message: %+v", r)
	}
}
