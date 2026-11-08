package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"browser-tool-backend/shared"
)

func TestIsBlankPageURL(t *testing.T) {
	for in, want := range map[string]bool{
		"":                              true,
		"   ":                           true,
		"about:blank":                   true,
		" about:blank ":                 true,
		"https://de.indeed.com/jobs":    false,
		"http://127.0.0.1:1234/":        false,
		"about:blank#x":                 false, // a real navigation to the blank page with a fragment is not "nothing loaded" by this rule's definition
		"chrome-error://chromewebdata/": false,
	} {
		if got := isBlankPageURL(in); got != want {
			t.Errorf("isBlankPageURL(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestTheMessageSaysWhatToDo(t *testing.T) {
	msg := errNoPageLoaded.Error()
	for _, part := range []string{"no page is loaded", "url:", "fetch_page_html"} {
		if !strings.Contains(msg, part) {
			t.Errorf("the message should mention %q: %s", part, msg)
		}
	}
}

// blankTab is a private browser tab nothing was loaded into.
func blankTab(t *testing.T) (context.Context, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	chrome, err := shared.ChromePath()
	if err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><p>hello</p></body></html>`))
	}))
	t.Cleanup(srv.Close)
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome))
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	t.Cleanup(func() { cancelTimeout(); cancelCtx(); cancelAlloc() })
	return ctx, srv.URL + "/"
}

func TestEnsurePageLoadedOnARealTab(t *testing.T) {
	ctx, url := blankTab(t)
	if err := ensurePageLoaded(ctx); !errors.Is(err, errNoPageLoaded) {
		t.Fatalf("a fresh tab: err = %v, want errNoPageLoaded", err)
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil {
		t.Fatal(err)
	}
	if err := ensurePageLoaded(ctx); err != nil {
		t.Fatalf("after loading a page: err = %v, want nil", err)
	}
	// A challenge page is left by navigating to about:blank (leavePrimaryTabLocked): blank again.
	if err := chromedp.Run(ctx, chromedp.Navigate("about:blank")); err != nil {
		t.Fatal(err)
	}
	if err := ensurePageLoaded(ctx); !errors.Is(err, errNoPageLoaded) {
		t.Fatalf("after leaving to about:blank: err = %v, want errNoPageLoaded", err)
	}
}

// crawlRequestSent calls the crawl_paginated MCP tool against a stub of the browser tool's own HTTP side and
// returns the /crawl-paginated request body it sent (nil when none).
func crawlRequestSent(t *testing.T, instructions string) (map[string]any, *mcp.CallToolResult, error) {
	t.Helper()
	body, res, err := callViaMCP(t, "crawl_paginated", RegisterCrawlPaginated, map[string]any{"instructions": instructions})
	if body == nil {
		return nil, res, err
	}
	var sent map[string]any
	if jerr := json.Unmarshal(body, &sent); jerr != nil {
		t.Fatalf("unreadable request body %q: %v", body, jerr)
	}
	return sent, res, err
}

const crawlYAMLBody = `container: ".job"
fields:
  - label: title
    selector: h2
pagination:
  nextSelector: a.next
  maxPages: 1
`

func TestCrawlPaginatedSendsTheUrlOfTheYAML(t *testing.T) {
	sent, res, err := crawlRequestSent(t, "url: \"  https://de.indeed.com/q-software-engineer-l-hamburg-jobs.html \"\n"+crawlYAMLBody)
	if err != nil || res == nil || sent == nil {
		t.Fatalf("the call did not reach the browser tool: err=%v res=%+v", err, res)
	}
	if got := sent["url"]; got != "https://de.indeed.com/q-software-engineer-l-hamburg-jobs.html" {
		t.Errorf("url sent = %v, want the trimmed address", got)
	}
	if sent["container"] != ".job" || sent["nextSelector"] != "a.next" {
		t.Errorf("the rest of the instructions must arrive unchanged: %v", sent)
	}
}

func TestCrawlPaginatedWithoutAUrlSendsNone(t *testing.T) {
	sent, res, err := crawlRequestSent(t, crawlYAMLBody)
	if err != nil || res == nil || sent == nil {
		t.Fatalf("the call did not reach the browser tool: err=%v res=%+v", err, res)
	}
	if _, has := sent["url"]; has {
		t.Errorf("no url was given, none should be sent: %v", sent["url"])
	}
}

func TestACommentedOrEmptyUrlSendsNone(t *testing.T) {
	for name, yaml := range map[string]string{"empty": "url: \"\"\n" + crawlYAMLBody, "blank": "url: \"   \"\n" + crawlYAMLBody} {
		sent, _, _ := crawlRequestSent(t, yaml)
		if sent == nil {
			t.Fatalf("%s: nothing sent", name)
		}
		if _, has := sent["url"]; has {
			t.Errorf("%s: no url should be sent: %v", name, sent["url"])
		}
	}
}

func TestTheToolDescriptionsExplainTheUrl(t *testing.T) {
	raw, err := json.Marshal(mustToolSchema(t, "crawl_paginated", RegisterCrawlPaginated))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if d := schema.Properties["instructions"].Description; !strings.Contains(d, "url: <the page address>") || !strings.Contains(d, "find_repeating_items") {
		t.Errorf("the instructions argument should explain the url key: %s", d)
	}
	desc := toolDescription(t, "crawl_paginated", RegisterCrawlPaginated)
	if !strings.Contains(desc, "url: <the page address>") || !strings.Contains(desc, "find_repeating_items") || !strings.Contains(desc, "the browser is blank") {
		t.Errorf("the tool description should explain the url key and find_repeating_items: %s", desc)
	}
}

// toolDescription and mustToolSchema read what an AI is shown for one tool, through a real MCP session.
func listedTool(t *testing.T, name string, register func(*mcp.Server)) *mcp.Tool {
	t.Helper()
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
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range list.Tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %s not listed", name)
	return nil
}

func toolDescription(t *testing.T, name string, register func(*mcp.Server)) string {
	return listedTool(t, name, register).Description
}

func mustToolSchema(t *testing.T, name string, register func(*mcp.Server)) any {
	return listedTool(t, name, register).InputSchema
}

// The two tools that read "the page already loaded" run against the shared browser session, which a unit test
// must not start (it would use the real browser profile). So the guard's call sites are pinned by reading the
// source: each function must call ensurePageLoaded after it holds the session, and before it reads the page.
func TestTheCurrentPageToolsCheckThatAPageIsLoaded(t *testing.T) {
	for _, c := range []struct{ file, fn, before string }{
		{"extract.go", "func performExtraction(", "runExtractionOnCurrentPage(ctx"},
		{"paginate.go", "func performPaginatedCrawl(", "runPaginatedCrawlLoop(ctx, tab, args)"},
	} {
		raw, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		start := strings.Index(src, c.fn)
		if start < 0 {
			t.Fatalf("%s: %s not found", c.file, c.fn)
		}
		body := src[start:]
		if end := strings.Index(body[1:], "\nfunc "); end > 0 {
			body = body[:end+1]
		}
		guard, read := strings.Index(body, "ensurePageLoaded(ctx)"), strings.Index(body, c.before)
		if guard < 0 || read < 0 || guard > read {
			t.Errorf("%s: %s must call ensurePageLoaded(ctx) before %s (guard at %d, read at %d)", c.file, c.fn, c.before, guard, read)
		}
		if lock := strings.Index(body, "shared.Mu.Lock()"); lock < 0 || lock > guard {
			t.Errorf("%s: %s must check the page while holding shared.Mu", c.file, c.fn)
		}
	}
}
