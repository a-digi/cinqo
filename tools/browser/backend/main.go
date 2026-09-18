// Command browser-tool-backend is cinqo's browser tool — a headless-
// Chrome-backed tool giving the AI a persistent browser session across
// several separate tool calls (fetch page HTML, inspect for login
// elements, log in). See
// plan/ai/tools/browser/step-01-manifest-and-package-skeleton.md.
//
// This file holds this process's own --mcp adapter and the HTTP-mode
// route wiring for each feature. The shared headless session itself
// (originally step 2's own architecture) and this tool's own SQLite
// handle/sibling-HTTP-call helper live in the shared package. The
// login/find-login-elements/login-credentials feature (originally
// steps 4/5/7/8, retiring step 5's own allowlist.go entirely — see
// plan/ai/tools/browser/step-07-login-profiles-and-credential-isolation.md
// and plan/ai/tools/browser/step-08-ai-instructed-login.md) lives in
// the auth package. The crawl-adjacent feature set (crawl.go for step
// 3, extract.go for step 9, paginate.go for step 16, and the rest)
// lives in the crawler package. Both were carved out of this package
// so each can be its own package without importing this one. See
// plan/ai/tools/browser/step-65-crawler-package-extraction.md.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"browser-tool-backend/auth"
	"browser-tool-backend/crawler"
	"browser-tool-backend/shared"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--mcp" {
		runMCPServer()
		return
	}
	runHTTPServer()
}

func runHTTPServer() {
	port := os.Getenv("PORT")

	// The shared headless session is no longer started here — see
	// shared.EnsureSharedSession's own doc comment. /healthz (and every
	// other route) is reachable the moment this process is listening,
	// regardless of whether a browser has ever been launched.
	if err := shared.InitDB(); err != nil {
		log.Fatalf("failed to open browser database: %v", err)
	}
	if err := auth.InitCryptoKey(); err != nil {
		log.Fatalf("failed to load credential encryption key: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		log.Print("shutting down, closing browser session")
		shared.StopSharedSession()
		os.Exit(0)
	}()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/crawl", crawler.CrawlHandler)
	http.HandleFunc("/find-login-elements", auth.FindLoginElementsHandler)
	http.HandleFunc("/extract", crawler.ExtractHandler)
	http.HandleFunc("/crawl-paginated", crawler.PaginatedCrawlHandler)
	http.HandleFunc("/crawl-logs", crawler.CrawlLogsHandler)
	http.HandleFunc("/browser-settings", shared.BrowserSettingsHandler)
	http.HandleFunc("/cloudflare-domains", crawler.CloudflareDomainsHandler)
	http.HandleFunc("/crawl-status", crawler.CrawlStatusHandler)
	http.HandleFunc("/crawl-cancel", crawler.CrawlCancelHandler)
	http.HandleFunc("/login", auth.LoginHandler)
	// Deliberately not exposed as an MCP tool — see
	// login_credentials.go's own top comment. Reachable only via the
	// ordinary tool proxy a signed-in human's own request hits, never
	// by the AI's own tool-calling loop (which only ever calls
	// registered MCP tools).
	http.HandleFunc("/login-credentials", auth.LoginCredentialsHandler)
	// Unlike /login-credentials above, THIS one is deliberately also
	// an MCP tool (has_login_credential, registered below) — the one
	// deliberate, narrow crack in "the AI never touches login data":
	// it can only ever report a yes/no existence check, never a
	// credential. See
	// plan/ai/tools/browser/step-11-ai-facing-credential-existence-check.md.
	http.HandleFunc("/has-login-credential", auth.HasCredentialHandler)

	log.Printf("browser tool listening on 127.0.0.1:%s", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil {
		shared.StopSharedSession()
		log.Fatal(err)
	}
}

func runMCPServer() {
	server := mcp.NewServer(&mcp.Implementation{Name: "browser", Version: "0.1.0"}, nil)

	crawler.RegisterFetchPageHTML(server)
	auth.RegisterFindLoginElements(server)
	crawler.RegisterExtractPageData(server)
	crawler.RegisterCrawlPaginated(server)
	crawler.RegisterExtractFromURL(server)
	auth.RegisterHasLoginCredential(server)
	auth.RegisterLogin(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
