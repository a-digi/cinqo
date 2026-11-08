// Command browser-tool-backend is cinqo's browser tool — a headless-
// Chrome-backed tool giving the AI a persistent browser session across
// several separate tool calls (fetch page HTML, inspect for login
// elements, crawl). See
// plan/ai/tools/browser/step-01-manifest-and-package-skeleton.md.
//
// This file holds this process's own --mcp adapter and the HTTP-mode
// route wiring for each feature. The shared headless session itself
// (originally step 2's own architecture) and this tool's own SQLite
// handle/sibling-HTTP-call helper live in the shared package. The
// find-login-elements feature and the human-only login window (plan 182:
// the person logs in on the site itself; NO login or credential is stored
// anywhere) live in the auth package. The crawl-adjacent feature set (crawl.go for step
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
	// Plan 182: delete the throw-away profile folders of any login window an earlier run of this tool left behind (a crash, a kill).
	if n := shared.SweepLoginWindowFolders(); n > 0 {
		log.Printf("removed %d login window folder(s) left by an earlier run", n)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		log.Print("shutting down, closing browser session")
		auth.StopLoginWindows()
		shared.StopSharedSession()
		crawler.StopRodEngine()
		crawler.StopKatanaEngine()
		crawler.StopKatanaWindow() // plan 198
		os.Exit(0)
	}()

	shared.StartIdleReaper()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/crawl", crawler.CrawlHandler)
	http.HandleFunc("/find-login-elements", auth.FindLoginElementsHandler)
	http.HandleFunc("/extract", crawler.ExtractHandler)
	http.HandleFunc("/crawl-paginated", crawler.PaginatedCrawlHandler)
	http.HandleFunc("/find-repeating-items", crawler.FindRepeatingItemsHandler)
	http.HandleFunc("/fetch-links", crawler.FetchLinksHandler)
	http.HandleFunc("/katana-window-cleared", crawler.KatanaWindowClearedHandler) // plan 198
	http.HandleFunc("/katana-window-hosts", crawler.KatanaWindowHostsHandler)
	http.HandleFunc("/katana-window-host", crawler.KatanaWindowHostHandler)
	http.HandleFunc("/katana-window-open", crawler.KatanaWindowOpenHandler)
	http.HandleFunc("/fetch-detail-candidates", crawler.FetchDetailCandidatesHandler)
	http.HandleFunc("/capabilities", crawler.CapabilitiesHandler)
	http.HandleFunc("/diagnose-url", crawler.DiagnoseURLHandler)
	http.HandleFunc("/dashboard/status", crawler.DashboardStatusHandler)
	http.HandleFunc("/domains", crawler.DomainsHandler)
	http.HandleFunc("/domains/clear", crawler.DomainsClearHandler)
	http.HandleFunc("/crawl-logs", crawler.CrawlLogsHandler)
	http.HandleFunc("/browser-settings", shared.BrowserSettingsHandler)
	http.HandleFunc("/engine-status", crawler.EngineStatusHandler)
	http.HandleFunc("/chrome-status", shared.ChromeStatusHandler)
	http.HandleFunc("/engine-test", crawler.EngineTestHandler)
	http.HandleFunc("/cloudflare-domains", crawler.CloudflareDomainsHandler)
	http.HandleFunc("/crawl-status", crawler.CrawlStatusHandler)
	http.HandleFunc("/crawl-cancel", crawler.CrawlCancelHandler)
	// Plan 182: the human-only login window (the person logs in on the site's own page; nothing is stored). Deliberately not MCP tools,
	// the AI cannot open, confirm or end a login. See auth/login_window.go.
	http.HandleFunc("/login-window/open", auth.LoginWindowOpenHandler)
	http.HandleFunc("/login-window/status", auth.LoginWindowStatusHandler)
	http.HandleFunc("/login-window/confirm", auth.LoginWindowConfirmHandler)
	http.HandleFunc("/login-window/cancel", auth.LoginWindowCancelHandler)
	// Plan 195: a login SESSION (sealed cookies of one site) the person chose to keep so Katana can read the logged-in area. Never a password,
	// never an MCP tool; the routes answer state only, never a cookie. See shared/loginsession.go.
	http.HandleFunc("/login-session/state", auth.LoginSessionStateHandler)
	http.HandleFunc("/login-session", auth.LoginSessionForgetHandler)
	// Plan 197: the engine an AI conversation of a Career link set to Rod reads its pages with. Never an MCP tool.
	http.HandleFunc("/conversation-engine", crawler.ConversationEngineHandler)

	log.Printf("browser tool listening on 127.0.0.1:%s", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil {
		auth.StopLoginWindows()
		shared.StopSharedSession()
		crawler.StopRodEngine()
		crawler.StopKatanaEngine()
		crawler.StopKatanaWindow() // plan 198
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
	crawler.RegisterFindRepeatingItems(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
