// Command career-tool-backend is cinqo's career tool — it holds the
// user's own career profile (skills, experience, what they're looking
// for) and a database of crawled job postings. Requires the browser
// tool to be installed and enabled (enforced by the host, see
// plan/ai/tools/step-14-tool-dependencies.md). For the AI-driven "Crawl
// with AI" path, the two cooperate only through the AI's own
// multi-tool-call orchestration in one conversation (browser crawls,
// this tool stores what was found) — this backend itself never calls
// browser directly for that path. The deterministic "Crawl now" path
// is the one exception (step 37, crawl_now.go): this backend's own
// detached goroutine calls browser's proxy routes directly over HTTP,
// via CORE_API_URL, so that crawl survives the initiating browser tab
// closing. See plan/ai/tools/career/career.md,
// plan/ai/tools/career/step-01-manifest-and-package-skeleton.md, and
// plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md.
//
// Unlike browser, this tool holds no persistent external resource
// (no shared browser session) across separate calls — every --mcp
// invocation and every HTTP-mode request opens what it needs (steps
// 2/3/4's own two SQLite databases) fresh, does the work, and is
// done. No TOOL_OWN_PORT adapter, no shared session mutex.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

	if err := initDatabases(); err != nil {
		log.Fatalf("failed to open career/jobs databases: %v", err)
	}
	// A goroutine, unlike an OS process, has no PID to find or
	// reattach after a restart — any crawl_runs row still 'running'
	// from before this process started is definitely orphaned. See
	// plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md.
	if err := reconcileOrphanedCrawlRuns(); err != nil {
		log.Fatalf("failed to reconcile orphaned crawl runs: %v", err)
	}
	// step 47.4 — reconcileOrphanedCrawlRuns above only ever runs once,
	// at boot, so it can't help a goroutine that's genuinely hung
	// without this process itself restarting. This periodic sweep is
	// the in-process complement: it catches that case too, without
	// requiring a restart.
	startStaleCrawlRunReaper()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/profiles", profilesHandler)
	http.HandleFunc("/profile-links", profileLinksHandler)
	http.HandleFunc("/personas", personasHandler)
	http.HandleFunc("/persona-details", personaDetailsHandler)
	http.HandleFunc("/skills", skillsHandler)
	http.HandleFunc("/experience", experienceHandler)
	http.HandleFunc("/jobs", jobsHandler)
	http.HandleFunc("/job-locations", jobLocationsHandler)
	http.HandleFunc("/companies", companiesHandler)
	http.HandleFunc("/recruiters", recruitersHandler)
	http.HandleFunc("/portals", portalsHandler)
	http.HandleFunc("/portal-links", portalLinksHandler)
	http.HandleFunc("/portal-links/crawl-request", crawlRequestHandler)
	http.HandleFunc("/portal-links/ingest-crawl-results", ingestCrawlResultsHandler)
	http.HandleFunc("/portal-links/crawl-now", crawlNowHandler)
	http.HandleFunc("/portal-links/crawl-now/active", crawlNowActiveHandler)
	http.HandleFunc("/portal-links/crawl-now/cancel", crawlNowCancelHandler)
	http.HandleFunc("/cv-import/upload", uploadCVHandler)
	http.HandleFunc("/cv-import/uploads", listCVUploadsHandler)

	if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil {
		os.Exit(1)
	}
}

// runMCPServer speaks MCP over stdin/stdout for exactly one spawn ->
// exchange -> exit cycle, matching every other tool's own --mcp
// contract. Opens both databases too — unlike browser's own --mcp
// adapter (which never touches its shared session directly and
// relays through TOOL_OWN_PORT instead), this tool has no HTTP-mode
// sibling to relay through at all, so its own --mcp subprocess
// reads/writes career.db and jobs.db directly, the same as HTTP mode
// does.
func runMCPServer() {
	if err := initDatabases(); err != nil {
		log.Fatalf("failed to open career/jobs databases: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "career", Version: "0.1.0"}, nil)

	registerCreateProfile(server)
	registerListProfiles(server)
	registerUpdateProfile(server)
	registerDeleteProfile(server)
	registerAddProfileExternalLink(server)
	registerRemoveProfileExternalLink(server)
	registerCreatePersona(server)
	registerListPersonas(server)
	registerUpdatePersona(server)
	registerDeletePersona(server)
	registerGetPersonaDetails(server)
	registerUpdatePersonaDetails(server)
	registerAddCareerSkill(server)
	registerRemoveCareerSkill(server)
	registerAddCareerExperience(server)
	registerRemoveCareerExperience(server)
	registerUpdateCareerExperience(server)
	registerSaveJob(server)
	registerListJobs(server)
	registerSearchJobs(server)
	registerDeleteJob(server)
	registerSavePortalJob(server)
	registerSavePortalJobs(server)
	registerCreateCompany(server)
	registerListCompanies(server)
	registerUpdateCompany(server)
	registerDeleteCompany(server)
	registerLinkJobToCompany(server)
	registerCreateRecruiter(server)
	registerListRecruiters(server)
	registerUpdateRecruiter(server)
	registerDeleteRecruiter(server)
	registerCreatePortal(server)
	registerListPortals(server)
	registerUpdatePortal(server)
	registerDeletePortal(server)
	registerAddPortalLink(server)
	registerUpdatePortalLink(server)
	registerRemovePortalLink(server)
	registerGetPortalLinkCrawlInstructions(server)
	registerSetPortalLinkCrawlInstructions(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
}
