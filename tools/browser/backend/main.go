// Command browser-tool-backend is cinqo's browser tool — a headless-
// Chrome-backed tool giving the AI a persistent browser session across
// several separate tool calls (fetch page HTML, inspect for login
// elements, log in). See
// plan/ai/tools/browser/step-01-manifest-and-package-skeleton.md.
//
// This file holds step 2's own architecture — the shared session
// itself and the --mcp adapter that reaches it
// (plan/ai/tools/browser/step-02-shared-browser-session.md) — plus
// the HTTP-mode wiring for each feature, implemented in its own file
// (crawl.go for step 3, login_elements.go for step 4, login.go
// originally for step 5, login_credentials.go + crypto.go for step 7
// — step 7 retired step 5's own allowlist.go entirely, see
// plan/ai/tools/browser/step-07-login-profiles-and-credential-isolation.md
// — login.go rewritten again by step 8 around domain + selectors
// only, no credential fields on the wire at all, see
// plan/ai/tools/browser/step-08-ai-instructed-login.md — extract.go
// for step 9, deliberately independent of login/
// login_credentials.go, see
// plan/ai/tools/browser/step-09-yaml-instructed-extraction.md —
// paginate.go for step 16, a multi-page crawl loop built on top of
// extract.go's own per-page extraction, see
// plan/ai/tools/browser/step-16-paginated-crawl-instructions.md).
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--mcp" {
		runMCPServer()
		return
	}
	runHTTPServer()
}

// sessionMu guards every access to sessionCtx — every HTTP handler
// that touches the shared page must hold this for the duration of its
// own chromedp actions, so two requests never race on the same tab.
// Nothing acquires it yet (no feature exists), but every future
// handler (steps 3-5) must.
var (
	sessionMu     sync.Mutex
	sessionCtx    context.Context
	sessionCancel []context.CancelFunc
)

func runHTTPServer() {
	port := os.Getenv("PORT")

	// The shared browser session is created ONCE here, before the HTTP
	// server ever starts listening — deliberately different from
	// pdf_generator's own per-call chromedp usage. manager.go's own
	// health check can't succeed until ListenAndServe below actually
	// binds, so a healthy /healthz already implies a live browser
	// session, with no extra logic needed inside the handler itself.
	// See plan/ai/tools/browser/step-02-shared-browser-session.md.
	if err := startSharedSession(); err != nil {
		log.Fatalf("failed to start shared browser session: %v", err)
	}
	if err := initBrowserDB(); err != nil {
		log.Fatalf("failed to open browser database: %v", err)
	}

	// A bare `defer stopSharedSession()` here would never actually run:
	// manager.go stops a tool by sending SIGTERM (escalating to
	// SIGKILL), and Go's default behavior for an unhandled SIGTERM is
	// immediate process termination — deferred functions do not run on
	// signal death, and log.Fatal below has the identical problem (it
	// calls os.Exit internally, which also skips defers). Verified
	// directly: without this signal handler, the child Chrome process
	// was confirmed to survive a plain `kill -TERM` of this process,
	// left running as an orphan. Catching the signal explicitly and
	// cleaning up before exiting is the only way this actually works.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		log.Print("shutting down, closing browser session")
		stopSharedSession()
		os.Exit(0)
	}()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/crawl", crawlHandler)
	http.HandleFunc("/find-login-elements", findLoginElementsHandler)
	http.HandleFunc("/extract", extractHandler)
	http.HandleFunc("/crawl-paginated", paginatedCrawlHandler)
	http.HandleFunc("/crawl-logs", crawlLogsHandler)
	http.HandleFunc("/login", loginHandler)
	// Deliberately not exposed as an MCP tool — see
	// login_credentials.go's own top comment. Reachable only via the
	// ordinary tool proxy a signed-in human's own request hits, never
	// by the AI's own tool-calling loop (which only ever calls
	// registered MCP tools).
	http.HandleFunc("/login-credentials", loginCredentialsHandler)
	// Unlike /login-credentials above, THIS one is deliberately also
	// an MCP tool (has_login_credential, registered below) — the one
	// deliberate, narrow crack in "the AI never touches login data":
	// it can only ever report a yes/no existence check, never a
	// credential. See
	// plan/ai/tools/browser/step-11-ai-facing-credential-existence-check.md.
	http.HandleFunc("/has-login-credential", hasCredentialHandler)

	log.Printf("browser tool listening on 127.0.0.1:%s", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil {
		stopSharedSession()
		log.Fatal(err)
	}
}

// startSharedSession creates the one chromedp browser context this
// whole process holds for its entire lifetime. A real, empty
// navigation (not just allocator/context creation) forces the browser
// to actually launch now rather than lazily on first real use —
// confirming a genuinely live browser before this process ever reports
// itself healthy, not just that the Go-side handles were constructed.
func startSharedSession() error {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocatorOptions()...)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)

	if err := chromedp.Run(ctx, chromedp.Navigate("about:blank")); err != nil {
		ctxCancel()
		allocCancel()
		return err
	}

	sessionCtx = ctx
	sessionCancel = []context.CancelFunc{ctxCancel, allocCancel}
	return nil
}

func stopSharedSession() {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	for _, cancel := range sessionCancel {
		cancel()
	}
}

// allocatorOptions falls back to chromedp's own default auto-discovery
// unless BROWSER_TOOL_CHROME_PATH is set — the same escape hatch
// pdf_generator's own PDF_GENERATOR_CHROME_PATH already established,
// generalized under this tool's own name. See
// plan/ai/tools/browser/step-06-multi-os-packaging.md.
func allocatorOptions() []chromedp.ExecAllocatorOption {
	opts := chromedp.DefaultExecAllocatorOptions[:]
	if p := os.Getenv("BROWSER_TOOL_CHROME_PATH"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	return opts
}

// runMCPServer speaks MCP over stdin/stdout for exactly one spawn ->
// exchange -> exit cycle, matching every other tool's own --mcp
// contract (see api/src/tool/mcp). Each registered tool calls
// callSibling below rather than touching sessionCtx directly (this
// subprocess never holds sessionCtx itself — that only ever exists
// inside the long-running HTTP-mode sibling). Credential management
// itself (POST/GET/DELETE /login-credentials) is deliberately never
// registered here — see login_credentials.go's own top comment;
// has_login_credential below is the one narrow, deliberate exception.
func runMCPServer() {
	server := mcp.NewServer(&mcp.Implementation{Name: "browser", Version: "0.1.0"}, nil)

	registerFetchPageHTML(server)
	registerFindLoginElements(server)
	registerExtractPageData(server)
	registerCrawlPaginated(server)
	registerHasLoginCredential(server)
	registerLogin(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

// callSibling relays one request to this tool's own already-running
// HTTP-mode sibling — the process actually holding the shared browser
// session — over the loopback TOOL_OWN_PORT env var
// install_handler.go/chat.go both set when this tool is running. The
// returned error is always safe to hand back to the model as the tool
// result text (matching tool_mcp.Invoke's own "never strand an
// unanswered call" contract) rather than propagating a bare Go error.
// See plan/ai/tools/browser/step-02-shared-browser-session.md.
func callSibling(route string, body []byte) ([]byte, error) {
	portStr := os.Getenv("TOOL_OWN_PORT")
	if portStr == "" {
		return nil, fmt.Errorf("browser session is not currently running — enable the tool first")
	}

	url := fmt.Sprintf("http://127.0.0.1:%s/%s", portStr, route)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("browser session is not reachable: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read browser session response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("browser session returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
