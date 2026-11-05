package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"browser-tool-backend/auth"
	"browser-tool-backend/crawler"
	"browser-tool-backend/shared"
)

// TestAnAIToolCallRunsInTheWindowAPersonOpenedForItsConversation (opt-in, opens a real visible Chrome window):
//
//	BROWSER_TOOL_LIVE_CHROME_TEST=1 go test . -run TestAnAIToolCallRunsInTheWindowAPersonOpenedForItsConversation -v
//
// plan 183, step 1, end to end except core: the real MCP tool fetch_page_html -> shared.CallSibling (which copies TOOL_CONVERSATION_ID into the
// request) -> the real routes -> the real login window. The person is this test: it opens the window for the conversation (aiMayUse) and
// confirms it; the page is example.com (no login form).
func TestAnAIToolCallRunsInTheWindowAPersonOpenedForItsConversation(t *testing.T) {
	if os.Getenv("BROWSER_TOOL_LIVE_CHROME_TEST") == "" {
		t.Skip("opens a real visible Chrome window; set BROWSER_TOOL_LIVE_CHROME_TEST=1 to run it")
	}
	root := t.TempDir()
	t.Setenv("TOOL_DB_DIR", filepath.Join(root, "db"))
	t.Setenv("TOOL_LOGS_DIR", filepath.Join(root, "logs"))
	t.Setenv("TOOL_CACHE_DIR", filepath.Join(root, "cache"))
	if err := shared.InitDB(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shared.DB.Close() })

	mux := http.NewServeMux()
	mux.HandleFunc("/crawl", crawler.CrawlHandler)
	mux.HandleFunc("/find-repeating-items", crawler.FindRepeatingItemsHandler)
	mux.HandleFunc("/crawl-status", crawler.CrawlStatusHandler)
	mux.HandleFunc("/login-window/open", auth.LoginWindowOpenHandler)
	mux.HandleFunc("/login-window/status", auth.LoginWindowStatusHandler)
	mux.HandleFunc("/login-window/confirm", auth.LoginWindowConfirmHandler)
	mux.HandleFunc("/login-window/cancel", auth.LoginWindowCancelHandler)
	srv := httptest.NewServer(mux)
	u, _ := url.Parse(srv.URL)
	t.Setenv("TOOL_OWN_PORT", u.Port())

	postJSON := func(path, body string) (int, string) {
		resp, err := http.Post(srv.URL+path, "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	const conv = "conv-live-183"
	// The window is closed before the test server, which is closed last.
	t.Cleanup(func() {
		if resp, err := http.Post(srv.URL+"/login-window/cancel", "application/json", bytes.NewBufferString(`{"requestId":"`+conv+`"}`)); err == nil {
			resp.Body.Close()
		}
		srv.Close()
	})

	// The person: opens the window for this conversation, then confirms it.
	if code, body := postJSON("/login-window/open", `{"requestId":"`+conv+`","url":"https://example.com/","aiMayUse":true}`); code != 200 {
		t.Fatalf("open: %d %s", code, body)
	}

	// The AI's tool, through MCP, as core runs it: the environment carries the conversation id.
	t.Setenv("TOOL_CONVERSATION_ID", conv)
	call, callTool := mcpFetch(t)

	// Not confirmed yet: the tool answers that a login is needed, it does not read the page headless.
	text, isErr := call("https://example.com/")
	if !isErr || !strings.Contains(text, "login_required") {
		t.Fatalf("before the confirmation: isErr=%v %s", isErr, text)
	}
	// and it tells the model to hand over to the person
	if !strings.Contains(text, "tell them the login window needs them") {
		t.Errorf("the model is not told to hand over: %s", text)
	}
	if text, isErr := callTool("find_repeating_items", map[string]any{"url": "https://example.com/"}); !isErr || !strings.Contains(text, "login_required") {
		t.Errorf("find_repeating_items before the confirmation: isErr=%v %s", isErr, text)
	}

	if code, body := postJSON("/login-window/confirm", `{"requestId":"`+conv+`"}`); code != 200 {
		t.Fatalf("confirm: %d %s", code, body)
	}
	text, isErr = call("https://example.com/")
	if isErr || !strings.Contains(text, "Example Domain") {
		t.Fatalf("in the window: isErr=%v %.300s", isErr, text)
	}
	// It ran IN the window: the run's phase under the conversation id says so (a headless call leaves no phase).
	resp, err := http.Get(srv.URL + "/crawl-status?requestId=" + conv)
	if err != nil {
		t.Fatal(err)
	}
	statusBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(statusBody), "login window") {
		t.Errorf("crawl-status: %d %s", resp.StatusCode, statusBody)
	}

	// Step 2: find_repeating_items (the AI's first tool when it writes listing instructions) also analyses the page IN the window.
	repeating, repErr := callTool("find_repeating_items", map[string]any{"url": "https://example.com/"})
	if repErr || !strings.Contains(repeating, "Example Domain") {
		t.Errorf("find_repeating_items in the window: isErr=%v %.300s", repErr, repeating)
	}
	if text, isErr := callTool("find_repeating_items", map[string]any{"url": "https://www.iana.org/"}); !isErr || !strings.Contains(text, "login_window_other_site") {
		t.Errorf("find_repeating_items for another site: isErr=%v %s", isErr, text)
	}

	// Another site than the window's is refused, not read on the headless tab.
	text, isErr = call("https://www.iana.org/")
	if !isErr || !strings.Contains(text, "login_window_other_site") || !strings.Contains(text, "another site") {
		t.Errorf("other site: isErr=%v %s", isErr, text)
	}

	// Another conversation has no window: unchanged (headless).
	t.Setenv("TOOL_CONVERSATION_ID", "conv-without-a-window")
	text, isErr = call("https://example.com/")
	if isErr || !strings.Contains(text, "Example Domain") {
		t.Fatalf("another conversation: isErr=%v %.300s", isErr, text)
	}
	if resp, err := http.Get(srv.URL + "/crawl-status?requestId=conv-without-a-window"); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("a headless AI call left a phase: %d", resp.StatusCode)
		}
	}
	t.Log("fetch_page_html ran in the confirmed window of its conversation, was refused for another site, and stayed headless for another conversation")
}

// mcpFetch connects an in-memory MCP client to a server with the real fetch_page_html and find_repeating_items tools. It returns a function
// that calls fetch_page_html for a url and one that calls any of the two tools.
func mcpFetch(t *testing.T) (func(pageURL string) (string, bool), func(tool string, args map[string]any) (string, bool)) {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "browser-test", Version: "1"}, nil)
	crawler.RegisterFetchPageHTML(server)
	crawler.RegisterFindRepeatingItems(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	callTool := func(tool string, args map[string]any) (string, bool) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text += tc.Text
			}
		}
		return text, res.IsError
	}
	return func(pageURL string) (string, bool) {
		return callTool("fetch_page_html", map[string]any{"url": pageURL})
	}, callTool
}
