// Command career-tool-backend is cinqo's career tool — it holds the
// user's own career profile (skills, experience, what they're looking
// for) and a database of crawled job postings. Requires the browser
// tool to be installed and enabled (enforced by the host, see
// plan/ai/tools/step-14-tool-dependencies.md). For the AI-driven "Crawl
// with AI" path, the two cooperate only through the AI's own
// multi-tool-call orchestration in one conversation (browser crawls,
// this tool stores what was found) — this backend itself never calls
// browser directly for that path. The deterministic "Crawl now" path
// is the one exception (step 37, crawl/crawl_now.go): this backend's
// own detached goroutine calls browser's proxy routes directly over
// HTTP, via CORE_API_URL, so that crawl survives the initiating browser
// tab closing. See plan/ai/tools/career/career.md,
// plan/ai/tools/career/step-01-manifest-and-package-skeleton.md, and
// plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md.
//
// Unlike browser, this tool holds no persistent external resource
// (no shared browser session) across separate calls — every --mcp
// invocation and every HTTP-mode request opens what it needs (steps
// 2/3/4's own two SQLite databases) fresh, does the work, and is
// done. No TOOL_OWN_PORT adapter, no shared session mutex.
//
// This file holds only the wiring: which HTTP route and which MCP tool
// each feature package owns. The features themselves live in real Go
// subpackages, mirroring browser's own layout — the two SQLite handles
// and the small helpers everything needs in shared; the job seeker's
// own profiles/personas/details in profile; Import CV in cvimport; the
// curated company directory and its recruiters in companies; job-board
// sources and their crawl instructions in portals; saved postings and
// Job Match in jobs; and every deterministic crawl mechanism in crawl.
// Each package owns both transports for its own data (its MCP tools
// AND the plain HTTP mirror a human's browser session uses), so the
// single flat http.go that used to hold all fifteen handlers is gone —
// each handler now sits beside the business logic it calls.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/companies"
	"career-tool-backend/crawl"
	"career-tool-backend/cvimport"
	"career-tool-backend/jobs"
	"career-tool-backend/portals"
	"career-tool-backend/profile"
	"career-tool-backend/shared"
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

	if err := shared.InitDatabases(); err != nil {
		log.Fatalf("failed to open career/jobs databases: %v", err)
	}
	// A goroutine, unlike an OS process, has no PID to find or
	// reattach after a restart — any crawl_runs row still 'running'
	// from before this process started is definitely orphaned. See
	// plan/ai/tools/career/step-37-detached-crawl-now-orchestration.md.
	if err := crawl.ReconcileOrphanedCrawlRuns(); err != nil {
		log.Fatalf("failed to reconcile orphaned crawl runs: %v", err)
	}
	// Same reasoning as ReconcileOrphanedCrawlRuns above, one level up
	// — a hidden Job Match conversation's own turn is not a subprocess
	// this Go process could ever reattach to after a restart.
	if err := jobs.ReconcileOrphanedJobMatches(); err != nil {
		log.Fatalf("failed to reconcile orphaned job matches: %v", err)
	}
	// step 47.4 — ReconcileOrphanedCrawlRuns above only ever runs once,
	// at boot, so it can't help a goroutine that's genuinely hung
	// without this process itself restarting. This periodic sweep is
	// the in-process complement: it catches that case too, without
	// requiring a restart.
	crawl.StartStaleCrawlRunReaper()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/profiles", profile.ProfilesHandler)
	http.HandleFunc("/profile-links", profile.ProfileLinksHandler)
	http.HandleFunc("/personas", profile.PersonasHandler)
	http.HandleFunc("/persona-details", profile.PersonaDetailsHandler)
	http.HandleFunc("/skills", profile.SkillsHandler)
	http.HandleFunc("/experience", profile.ExperienceHandler)
	http.HandleFunc("/jobs", jobs.JobsHandler)
	http.HandleFunc("/job-locations", jobs.JobLocationsHandler)
	http.HandleFunc("/jobs/match", jobs.JobMatchHandler)
	http.HandleFunc("/companies", companies.CompaniesHandler)
	http.HandleFunc("/recruiters", companies.RecruitersHandler)
	http.HandleFunc("/portals", portals.PortalsHandler)
	http.HandleFunc("/portal-links", portals.PortalLinksHandler)
	http.HandleFunc("/portal-links/crawl-request", portals.CrawlRequestHandler)
	http.HandleFunc("/portal-links/ingest-crawl-results", crawl.IngestCrawlResultsHandler)
	http.HandleFunc("/portal-links/crawl-now", crawl.CrawlNowHandler)
	http.HandleFunc("/portal-links/crawl-job-details-now", crawl.CrawlJobDetailsNowHandler)
	http.HandleFunc("/portal-links/crawl-runs/active", crawl.CrawlMonitorHandler)
	http.HandleFunc("/portal-links/crawl-now/active", crawl.CrawlNowActiveHandler)
	http.HandleFunc("/portal-links/crawl-now/cancel", crawl.CrawlNowCancelHandler)
	http.HandleFunc("/cv-import/upload", cvimport.UploadCVHandler)
	http.HandleFunc("/cv-import/runs", cvimport.CVImportRunsHandler)

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
	if err := shared.InitDatabases(); err != nil {
		log.Fatalf("failed to open career/jobs databases: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "career", Version: "0.1.0"}, nil)

	profile.RegisterCreateProfile(server)
	profile.RegisterListProfiles(server)
	profile.RegisterUpdateProfile(server)
	profile.RegisterDeleteProfile(server)
	profile.RegisterAddProfileExternalLink(server)
	profile.RegisterRemoveProfileExternalLink(server)
	profile.RegisterCreatePersona(server)
	profile.RegisterListPersonas(server)
	profile.RegisterUpdatePersona(server)
	profile.RegisterDeletePersona(server)
	profile.RegisterGetPersonaDetails(server)
	profile.RegisterUpdatePersonaDetails(server)
	profile.RegisterAddCareerSkill(server)
	profile.RegisterRemoveCareerSkill(server)
	profile.RegisterAddCareerExperience(server)
	profile.RegisterRemoveCareerExperience(server)
	profile.RegisterUpdateCareerExperience(server)
	jobs.RegisterSaveJob(server)
	jobs.RegisterListJobs(server)
	jobs.RegisterSearchJobs(server)
	jobs.RegisterDeleteJob(server)
	jobs.RegisterSaveJobDetailExtraction(server)
	jobs.RegisterGetJob(server)
	jobs.RegisterSaveJobMatch(server)
	jobs.RegisterSavePortalJob(server)
	jobs.RegisterSavePortalJobs(server)
	companies.RegisterCreateCompany(server)
	companies.RegisterListCompanies(server)
	companies.RegisterUpdateCompany(server)
	companies.RegisterDeleteCompany(server)
	jobs.RegisterLinkJobToCompany(server)
	companies.RegisterCreateRecruiter(server)
	companies.RegisterListRecruiters(server)
	companies.RegisterUpdateRecruiter(server)
	companies.RegisterDeleteRecruiter(server)
	portals.RegisterCreatePortal(server)
	portals.RegisterListPortals(server)
	portals.RegisterUpdatePortal(server)
	portals.RegisterDeletePortal(server)
	portals.RegisterAddPortalLink(server)
	portals.RegisterUpdatePortalLink(server)
	portals.RegisterRemovePortalLink(server)
	portals.RegisterGetPortalLinkCrawlInstructions(server)
	portals.RegisterSetPortalLinkCrawlInstructions(server)
	portals.RegisterGetPortalLinkJobDetailCrawlInstructions(server)
	portals.RegisterSetPortalLinkJobDetailCrawlInstructions(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
}
