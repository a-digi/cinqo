package portal

import (
	"encoding/json"
	"os"
	"testing"
)

// The shared table of pages and verdicts (tools/testdata/crawl_extraction_check.json) is also run by Browser's extractionCheckForPages
// (tools/browser/backend/crawler): crawl_paginated gives an AI its verdict with the crawl, computed by that copy of this rule, so the
// two must answer alike for every case (plan/ai/conversation/step-45, step 2).
func TestTheSharedExtractionFixturesGiveTheirExpectedVerdicts(t *testing.T) {
	raw, err := os.ReadFile("../../../testdata/crawl_extraction_check.json")
	if os.IsNotExist(err) {
		// tools/testdata is in .gitignore (the owner's choice), so a fresh checkout has no table: nothing to compare then.
		t.Skip("tools/testdata/crawl_extraction_check.json is not in this checkout")
	}
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Cases []struct {
			Name     string                `json:"name"`
			Pages    []CrawlResultPage     `json:"pages"`
			Expected CrawlExtractionReport `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Cases) < 20 {
		t.Fatalf("only %d cases: the table must not shrink unnoticed", len(table.Cases))
	}
	for _, c := range table.Cases {
		got := classifyCrawlResultPages(c.Pages)
		// compared as JSON: the expected samples were read from JSON (numbers as float64), the computed ones hold ints
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(c.Expected)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("%s:\n got  %s\n want %s", c.Name, gotJSON, wantJSON)
		}
	}
}
