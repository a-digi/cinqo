package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type checkFixtures struct {
	Cases []struct {
		Name     string              `json:"name"`
		Pages    []pageExtractResult `json:"pages"`
		Expected extractionCheck     `json:"expected"`
	} `json:"cases"`
}

func loadCheckFixtures(t *testing.T) checkFixtures {
	t.Helper()
	raw, err := os.ReadFile("../../../testdata/crawl_extraction_check.json")
	if os.IsNotExist(err) {
		// tools/testdata is in .gitignore (the owner's choice), so a fresh checkout has no table: nothing to compare then.
		t.Skip("tools/testdata/crawl_extraction_check.json is not in this checkout")
	}
	if err != nil {
		t.Fatal(err)
	}
	var table checkFixtures
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Cases) < 20 {
		t.Fatalf("only %d cases: the table must not shrink unnoticed", len(table.Cases))
	}
	return table
}

// The same table Career's classifyCrawlResultPages runs: both rules must give the same verdict for every case.
func TestTheVerdictMatchesTheSharedFixtures(t *testing.T) {
	for _, c := range loadCheckFixtures(t).Cases {
		got := extractionCheckForPages(c.Pages)
		// compared as JSON: the expected samples were read from JSON (numbers as float64), the computed ones hold ints
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(c.Expected)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("%s:\n got  %s\n want %s", c.Name, gotJSON, wantJSON)
		}
	}
}

func crawlResultFor(t *testing.T, resp paginatedCrawlResponse) (text string, isErr bool) {
	t.Helper()
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	res := aiCrawlResult(body)
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

// What the AI reads carries the verdict, next to the pages, and the long values are still shortened.
func TestTheCrawlResultCarriesItsVerdict(t *testing.T) {
	long := strings.Repeat("x", 500)
	text, isErr := crawlResultFor(t, paginatedCrawlResponse{
		Pages: []pageExtractResult{{URL: "https://e.com", Items: []map[string]any{
			{"title": "Dev", "url": "https://e.com/1", "description": long},
			{"title": "Ops", "url": "https://e.com/2"},
			{"title": "QA", "url": "https://e.com/3"},
		}}},
		StoppedReason: "max_pages", PagesVisited: 1,
	})
	if isErr {
		t.Fatal(text)
	}
	var got struct {
		Pages           []pageExtractResult `json:"pages"`
		StoppedReason   string              `json:"stoppedReason"`
		ExtractionCheck *extractionCheck    `json:"extractionCheck"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	if got.ExtractionCheck == nil || !got.ExtractionCheck.OK || got.ExtractionCheck.ItemsFound != 3 || got.ExtractionCheck.ItemsExtractable != 3 {
		t.Errorf("verdict %+v", got.ExtractionCheck)
	}
	if got.StoppedReason != "max_pages" || len(got.Pages) != 1 {
		t.Errorf("the crawl response is not unchanged: %+v", got)
	}
	if d, _ := got.Pages[0].Items[0]["description"].(string); len([]rune(d)) > maxAIListingValueRunes+1 {
		t.Errorf("a long value was not shortened: %d runes", len([]rune(d)))
	}
}

// A page whose long, unprotected value is shortened must still count as the same verdict: the verdict is taken on the full values.
func TestTheVerdictIsTakenBeforeTheValuesAreShortened(t *testing.T) {
	text, _ := crawlResultFor(t, paginatedCrawlResponse{Pages: []pageExtractResult{{Items: []map[string]any{
		{"description": strings.Repeat("y", 900)}, {"title": "A", "url": "u"}, {"title": "B", "url": "u"}, {"title": "C", "url": "u"}, {"title": "D", "url": "u"},
	}}}})
	var got struct {
		ExtractionCheck extractionCheck `json:"extractionCheck"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	// a row with only a description is not blank, so it is a broken row
	if got.ExtractionCheck.BlankItems != 0 || got.ExtractionCheck.ItemsSkipped != 1 || got.ExtractionCheck.OK {
		t.Errorf("verdict %+v", got.ExtractionCheck)
	}
}

// No page (a block, an error answer) means no verdict: "no items" would be a wrong thing to tell the AI there.
func TestNoPagesMeansNoVerdict(t *testing.T) {
	text, _ := crawlResultFor(t, paginatedCrawlResponse{StoppedReason: "cloudflare_blocked", BlockedReason: "challenge"})
	if strings.Contains(text, "extractionCheck") {
		t.Errorf("a verdict was attached to a result without pages: %s", text)
	}
}

func TestAnUnreadableAnswerIsAToolError(t *testing.T) {
	res := aiCrawlResult([]byte("not json"))
	if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "failed to parse") {
		t.Errorf("%+v", res)
	}
}

// Through a real MCP session and a stand-in for the browser session (the sibling the tool calls): the whole tool, the YAML in, the
// result with its verdict out.
func TestCrawlPaginatedThroughTheProtocolReturnsTheVerdict(t *testing.T) {
	canned := paginatedCrawlResponse{
		Pages: []pageExtractResult{{URL: "https://e.com", Items: []map[string]any{
			{"title": "Dev", "url": "https://e.com/1"}, {"title": "Ops", "url": "https://e.com/2"}, {"title": "QA"},
		}}},
		StoppedReason: "max_pages", PagesVisited: 1, RequestedMaxPages: 1, EffectiveMaxPages: 1,
	}
	sibling := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/crawl-paginated" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(canned)
	}))
	defer sibling.Close()
	t.Setenv("TOOL_OWN_PORT", strings.TrimPrefix(sibling.URL, "http://127.0.0.1:"))

	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterCrawlPaginated(server)
	ct, st := mcp.NewInMemoryTransports()
	ctx := t.Context()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "crawl_paginated", Arguments: map[string]any{
		"instructions": "container: li.job\nfields:\n  - label: title\n    selector: a\npagination:\n  nextSelector: a.next\n  maxPages: 1\n",
	}})
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	var got struct {
		ExtractionCheck *extractionCheck `json:"extractionCheck"`
	}
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &got); err != nil {
		t.Fatal(err)
	}
	if got.ExtractionCheck == nil || got.ExtractionCheck.OK || got.ExtractionCheck.ItemsSkipped != 1 || !strings.Contains(got.ExtractionCheck.Message, "1 of 3") {
		t.Errorf("verdict %+v", got.ExtractionCheck)
	}
	_ = context.Background
}
