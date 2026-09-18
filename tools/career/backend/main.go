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

	"career-tool-backend/companies"
	"career-tool-backend/crawl"
	"career-tool-backend/cv"
	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/persona"
	"career-tool-backend/portal"
	"career-tool-backend/profile"
	"career-tool-backend/recruiters"
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

	if err := db.InitDatabases(); err != nil {
		log.Fatalf("failed to open career/jobs databases: %v", err)
	}
	// A goroutine, unlike an OS process, has no PID to find or
	// reattach after a restart — any crawl_runs row still 'running'
	// from before this process started is definitely orphaned. See
	// plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md.
	if err := crawl.ReconcileOrphanedCrawlRuns(); err != nil {
		log.Fatalf("failed to reconcile orphaned crawl runs: %v", err)
	}
	// Same reasoning as reconcileOrphanedCrawlRuns above, one level up
	// — a hidden Job Match conversation's own turn is not a subprocess
	// this Go process could ever reattach to after a restart.
	if err := jobs.ReconcileOrphanedJobMatches(); err != nil {
		log.Fatalf("failed to reconcile orphaned job matches: %v", err)
	}
	// Same reasoning as reconcileOrphanedJobMatches above, one level up
	// — a hidden CV-generation conversation's own turn (or the
	// frontend's own post-turn Media persist step) is not a subprocess
	// this Go process could ever reattach to after a restart.
	if err := jobs.ReconcileOrphanedCvGenerations(); err != nil {
		log.Fatalf("failed to reconcile orphaned CV generations: %v", err)
	}
	// step 47.4 — reconcileOrphanedCrawlRuns above only ever runs once,
	// at boot, so it can't help a goroutine that's genuinely hung
	// without this process itself restarting. This periodic sweep is
	// the in-process complement: it catches that case too, without
	// requiring a restart.
	crawl.StartStaleCrawlRunReaper()

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
	http.HandleFunc("/jobs/match", jobMatchHandler)
	http.HandleFunc("/jobs/cv", cvPdfHandler)
	http.HandleFunc("/jobs/cv/persist", cvPdfPersistHandler)
	http.HandleFunc("/companies", companiesHandler)
	http.HandleFunc("/recruiters", recruitersHandler)
	http.HandleFunc("/portals", portalsHandler)
	http.HandleFunc("/portal-links", portalLinksHandler)
	http.HandleFunc("/portal-links/crawl-request", crawlRequestHandler)
	http.HandleFunc("/portal-links/ingest-crawl-results", ingestCrawlResultsHandler)
	http.HandleFunc("/portal-links/crawl-now", crawl.CrawlNowHandler)
	http.HandleFunc("/portal-links/crawl-job-details-now", crawl.CrawlJobDetailsNowHandler)
	http.HandleFunc("/portal-links/crawl-runs/active", crawl.CrawlMonitorHandler)
	http.HandleFunc("/portal-links/crawl-now/active", crawl.CrawlNowActiveHandler)
	http.HandleFunc("/portal-links/crawl-now/cancel", crawl.CrawlNowCancelHandler)
	http.HandleFunc("/cv-import/upload", cv.UploadCVHandler)
	http.HandleFunc("/cv-import/runs", cv.CVImportRunsHandler)

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
	if err := db.InitDatabases(); err != nil {
		log.Fatalf("failed to open career/jobs databases: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "career", Version: "0.1.0"}, nil)

	profile.RegisterCreateProfile(server)
	profile.RegisterListProfiles(server)
	profile.RegisterUpdateProfile(server)
	profile.RegisterDeleteProfile(server)
	profile.RegisterAddProfileExternalLink(server)
	profile.RegisterRemoveProfileExternalLink(server)
	persona.RegisterCreatePersona(server)
	persona.RegisterListPersonas(server)
	persona.RegisterUpdatePersona(server)
	persona.RegisterDeletePersona(server)
	persona.RegisterGetPersonaDetails(server)
	persona.RegisterUpdatePersonaDetails(server)
	persona.RegisterAddCareerSkill(server)
	persona.RegisterRemoveCareerSkill(server)
	persona.RegisterAddCareerExperience(server)
	persona.RegisterRemoveCareerExperience(server)
	persona.RegisterUpdateCareerExperience(server)
	jobs.RegisterSaveJob(server)
	jobs.RegisterListJobs(server)
	jobs.RegisterSearchJobs(server)
	jobs.RegisterDeleteJob(server)
	jobs.RegisterSaveJobDetailExtraction(server)
	jobs.RegisterGetJob(server)
	jobs.RegisterSaveJobMatch(server)
	jobs.RegisterSaveCvPdf(server)
	jobs.RegisterSavePortalJob(server)
	jobs.RegisterSavePortalJobs(server)
	companies.RegisterCreateCompany(server)
	companies.RegisterListCompanies(server)
	companies.RegisterUpdateCompany(server)
	companies.RegisterDeleteCompany(server)
	jobs.RegisterLinkJobToCompany(server)
	recruiters.RegisterCreateRecruiter(server)
	recruiters.RegisterListRecruiters(server)
	recruiters.RegisterUpdateRecruiter(server)
	recruiters.RegisterDeleteRecruiter(server)
	portal.RegisterCreatePortal(server)
	portal.RegisterListPortals(server)
	portal.RegisterUpdatePortal(server)
	portal.RegisterDeletePortal(server)
	portal.RegisterAddPortalLink(server)
	portal.RegisterUpdatePortalLink(server)
	portal.RegisterRemovePortalLink(server)
	portal.RegisterGetPortalLinkCrawlInstructions(server)
	portal.RegisterSetPortalLinkCrawlInstructions(server)
	portal.RegisterGetPortalLinkJobDetailCrawlInstructions(server)
	portal.RegisterSetPortalLinkJobDetailCrawlInstructions(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
}
