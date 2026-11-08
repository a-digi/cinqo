package portal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func checkSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterCheckCrawlExtractionResult(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

const twoPages = `[{"url":"https://x/1","items":[{"title":"A","url":"https://x/a"},{"title":"B","url":"https://x/b"},{"title":"C","url":"https://x/c"}]}]`

func callCheck(t *testing.T, session *mcp.ClientSession, pages any) (*mcp.CallToolResult, error) {
	t.Helper()
	return session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "check_crawl_extraction_result",
		Arguments: map[string]any{"pages": pages},
	})
}

func reportOf(t *testing.T, res *mcp.CallToolResult) CrawlExtractionReport {
	t.Helper()
	var r CrawlExtractionReport
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &r); err != nil {
		t.Fatalf("not a report: %v", err)
	}
	return r
}

func TestPagesAsAnArrayStillWorks(t *testing.T) {
	var pages []any
	if err := json.Unmarshal([]byte(twoPages), &pages); err != nil {
		t.Fatal(err)
	}
	res, err := callCheck(t, checkSession(t), pages)
	if err != nil || res.IsError {
		t.Fatalf("array form: %v %+v", err, res)
	}
	if r := reportOf(t, res); r.ItemsFound != 3 || !r.OK {
		t.Errorf("report: %+v", r)
	}
}

// The slip from the Stepstone run: the model sends the array as a string.
func TestPagesAsAJSONStringWorksEndToEnd(t *testing.T) {
	res, err := callCheck(t, checkSession(t), twoPages)
	if err != nil || res.IsError {
		t.Fatalf("string form must not be rejected: %v %+v", err, res)
	}
	if r := reportOf(t, res); r.ItemsFound != 3 || !r.OK {
		t.Errorf("the string form must give the same report as the array: %+v", r)
	}
}

func TestAStringThatIsNotAPagesArrayIsAClearError(t *testing.T) {
	res, err := callCheck(t, checkSession(t), `[{"url": "https://x, }]`)
	text := ""
	if err != nil {
		text = err.Error()
	} else if res.IsError {
		text = res.Content[0].(*mcp.TextContent).Text
	} else {
		t.Fatalf("a broken string must be an error, got %+v", res)
	}
	if !strings.Contains(text, "pass the 'pages' array from crawl_paginated unchanged") {
		t.Errorf("error should say what to do: %q", text)
	}
}

func TestNoPagesAtAllKeepsItsOwnError(t *testing.T) {
	res, err := callCheck(t, checkSession(t), nil)
	if err != nil {
		return // rejected as a protocol error: also fine
	}
	if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "pages is required") {
		t.Errorf("null pages: %+v", res)
	}
}

func TestPublishedSchemaAllowsTheStringForm(t *testing.T) {
	session := checkSession(t)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 1 {
		t.Fatalf("tools: %v %v", tools, err)
	}
	raw, _ := json.Marshal(tools.Tools[0].InputSchema)
	var schema struct {
		Properties struct {
			Pages struct {
				Type  any `json:"type"`
				Types any `json:"types"`
			} `json:"pages"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"string"`) || !strings.Contains(string(raw), `"array"`) {
		t.Errorf("pages must be allowed as an array and as a string, schema: %s", raw)
	}
}

func TestCrawlPagesArgDecoding(t *testing.T) {
	var a CrawlPagesArg
	if err := json.Unmarshal([]byte(`null`), &a); err != nil || len(a) != 0 {
		t.Errorf("null: %v %v", a, err)
	}
	if err := json.Unmarshal([]byte(`"[]"`), &a); err != nil || len(a) != 0 {
		t.Errorf("empty array string: %v %v", a, err)
	}
	if err := json.Unmarshal([]byte(`123`), &a); err == nil {
		t.Error("a number is not pages")
	}
}
