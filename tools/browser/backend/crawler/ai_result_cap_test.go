package crawler

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCapStringKeepsShortAndCutsLongOnRuneBoundaries(t *testing.T) {
	exact := strings.Repeat("a", 300)
	if got := capString(exact); got != exact {
		t.Errorf("exactly 300 characters must stay whole")
	}
	got := capString(strings.Repeat("a", 301))
	if utf8.RuneCountInString(got) != 301 || !strings.HasSuffix(got, "…") || !strings.HasPrefix(got, strings.Repeat("a", 300)) {
		t.Errorf("301 characters: want 300 plus an ellipsis, got %d runes", utf8.RuneCountInString(got))
	}
	// multi-byte text is cut by characters, never in the middle of one
	umlauts := capString(strings.Repeat("ü", 500))
	if !utf8.ValidString(umlauts) || utf8.RuneCountInString(umlauts) != 301 {
		t.Errorf("umlauts: valid=%v runes=%d", utf8.ValidString(umlauts), utf8.RuneCountInString(umlauts))
	}
	if capString("") != "" || capString("short") != "short" {
		t.Error("short strings unchanged")
	}
}

func TestCapListingItemValuesProtectsTheFieldsTheCheckNeeds(t *testing.T) {
	long := strings.Repeat("x", 450)
	pages := []pageExtractResult{{Items: []map[string]any{{
		"title": long, "url": "https://de.indeed.com/rc/clk?jk=" + long, "company": long, "location": long, "postedAt": long,
		"description": long, "snippet": long, "tags": []any{long, "short", 7}, "count": 5, "flag": true, "nothing": nil,
	}}}}
	capListingItemValues(pages)
	item := pages[0].Items[0]
	for _, k := range []string{"title", "url", "company", "location", "postedAt"} {
		if s := item[k].(string); strings.Contains(s, "…") || len(s) < 450 {
			t.Errorf("%s must stay whole, got %d bytes", k, len(s))
		}
	}
	for _, k := range []string{"description", "snippet"} {
		if s := item[k].(string); utf8.RuneCountInString(s) != 301 || !strings.HasSuffix(s, "…") {
			t.Errorf("%s must be shortened, got %d runes", k, utf8.RuneCountInString(s))
		}
	}
	tags := item["tags"].([]any)
	if utf8.RuneCountInString(tags[0].(string)) != 301 || tags[1] != "short" || tags[2] != 7 {
		t.Errorf("array values: first shortened, the rest unchanged: %v", tags)
	}
	if item["count"] != 5 || item["flag"] != true || item["nothing"] != nil {
		t.Errorf("non-text values must be untouched: %v", item)
	}
}

func TestCapListingItemValuesLeavesFlatResultsAlone(t *testing.T) {
	long := strings.Repeat("y", 900)
	pages := []pageExtractResult{{Results: map[string]any{"description": long, "title": "T"}}}
	capListingItemValues(pages)
	if pages[0].Results["description"] != long {
		t.Error("a flat result (a detail page: the full text is what is checked) must not be shortened")
	}
}

// callWithReply calls a registered tool through a real MCP session against a stub that answers the browser tool's
// own HTTP side with the given JSON.
func callWithReply(t *testing.T, tool string, register func(*mcp.Server), args map[string]any, reply string) string {
	t.Helper()
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(stub.Close)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(stub.URL, "http://"))
	t.Setenv("TOOL_OWN_PORT", port)
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	register(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil || res.IsError || len(res.Content) == 0 {
		t.Fatalf("call failed: err=%v res=%+v", err, res)
	}
	return res.Content[0].(*mcp.TextContent).Text
}

func TestCrawlPaginatedShowsTheAIShortenedListingItems(t *testing.T) {
	desc := strings.Repeat("d", 600)
	longURL := "https://de.indeed.com/rc/clk?jk=" + strings.Repeat("u", 400)
	reply, _ := json.Marshal(map[string]any{
		"pages":         []any{map[string]any{"url": "https://x/", "items": []any{map[string]any{"title": "Engineer", "url": longURL, "description": desc, "company": "Acme"}}, "notFound": []any{}}},
		"stoppedReason": "max_pages_reached", "pagesVisited": 1, "requestedMaxPages": 1, "effectiveMaxPages": 1,
	})
	text := callWithReply(t, "crawl_paginated", RegisterCrawlPaginated, map[string]any{"instructions": crawlYAMLBody}, string(reply))
	var got paginatedCrawlResponse
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
	item := got.Pages[0].Items[0]
	if item["url"] != longURL || item["title"] != "Engineer" || item["company"] != "Acme" {
		t.Errorf("title, url and company must arrive whole: %v", item)
	}
	if d := item["description"].(string); utf8.RuneCountInString(d) != 301 || !strings.HasSuffix(d, "…") {
		t.Errorf("description must be shortened to 300 characters, got %d", utf8.RuneCountInString(d))
	}
	if strings.Contains(text, desc) {
		t.Error("the full description must not reach the AI")
	}
}

func TestCrawlPaginatedKeepsAFlatDetailResultWhole(t *testing.T) {
	desc := strings.Repeat("d", 2000)
	reply, _ := json.Marshal(map[string]any{
		"pages":         []any{map[string]any{"url": "https://x/job/1", "results": map[string]any{"description": desc}, "notFound": []any{}}},
		"stoppedReason": "max_pages_reached", "pagesVisited": 1, "requestedMaxPages": 1, "effectiveMaxPages": 1,
	})
	text := callWithReply(t, "crawl_paginated", RegisterCrawlPaginated, map[string]any{"instructions": crawlYAMLBody}, string(reply))
	if !strings.Contains(text, desc) {
		t.Error("a detail page's full text must reach the AI whole")
	}
}

func TestExtractToolsAreNotShortened(t *testing.T) {
	desc := strings.Repeat("e", 1200)
	// extract_from_url takes a crawl-paginated reply and shows the first page: it is used for job detail pages
	reply, _ := json.Marshal(map[string]any{
		"pages":         []any{map[string]any{"url": "https://x/job/1", "results": map[string]any{"description": desc}, "notFound": []any{}}},
		"stoppedReason": "max_pages_reached", "pagesVisited": 1, "requestedMaxPages": 1, "effectiveMaxPages": 1,
	})
	text := callWithReply(t, "extract_from_url", RegisterExtractFromURL, map[string]any{
		"url": "https://example.com/job/1", "fields": []any{map[string]any{"label": "description", "selector": "main"}},
	}, string(reply))
	if !strings.Contains(text, desc) {
		t.Error("extract_from_url must keep full text (job detail pages)")
	}
	text = callWithReply(t, "extract_page_data", RegisterExtractPageData, map[string]any{
		"container": ".job", "fields": []any{map[string]any{"label": "description", "selector": "p"}},
	}, `{"items":[{"description":"`+desc+`"}],"notFound":[]}`)
	if !strings.Contains(text, desc) {
		t.Error("extract_page_data must keep full text")
	}
}
