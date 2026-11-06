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
	applicationloghandlers "career-tool-backend/applicationlog/handlers"
	companieshandlers "career-tool-backend/companies/handlers"
	crawlhandlers "career-tool-backend/crawl/handlers"
	cvhandlers "career-tool-backend/cv/handlers"
	educationhandlers "career-tool-backend/education/handlers"
	jobshandlers "career-tool-backend/jobs/handlers"
	"career-tool-backend/match"
	personahandlers "career-tool-backend/persona/handlers"
	portalhandlers "career-tool-backend/portal/handlers"
	profilehandlers "career-tool-backend/profile/handlers"
	recruitershandlers "career-tool-backend/recruiters/handlers"
	"context"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/applicationlog"
	"career-tool-backend/autodiscovery"
	"career-tool-backend/companies"
	"career-tool-backend/companies/companydeletedconsumers"
	"career-tool-backend/crawl"
	"career-tool-backend/crawl/crawlcriteria"
	"career-tool-backend/cv"
	"career-tool-backend/cv/cvbuilder"
	"career-tool-backend/db"
	"career-tool-backend/education"
	"career-tool-backend/jobs"
	"career-tool-backend/jobs/deletedjobs"
	"career-tool-backend/jobs/jobcleanup"
	"career-tool-backend/jobs/jobdeletedconsumers"
	"career-tool-backend/jobs/jobrecrawl"
	"career-tool-backend/jobs/jobrecrawlconsumer"
	"career-tool-backend/match/consumer/jobmatchconsumer"
	"career-tool-backend/match/consumer/personacreatedconsumer"
	"career-tool-backend/match/consumer/personadeletedconsumer"
	"career-tool-backend/match/consumer/personamatchconsumer"
	"career-tool-backend/match/consumer/personarecomputeconsumer"
	"career-tool-backend/match/consumer/profilecreatedconsumer"
	"career-tool-backend/match/consumer/profiledeletedconsumer"
	"career-tool-backend/match/consumer/profilejobmatchconsumer"
	"career-tool-backend/match/matchorphans"
	"career-tool-backend/match/rematchworker"
	"career-tool-backend/persona"
	"career-tool-backend/portal"
	"career-tool-backend/profile"
	"career-tool-backend/profile/profiledetails"
	"career-tool-backend/profile/profileimage"
	"career-tool-backend/profile/profileitems"
	"career-tool-backend/recruiters"
	"career-tool-backend/recruiters/recruiterdeletedconsumers"
	"career-tool-backend/settings"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--mcp" {
		runMCPServer()
		return
	}
	runHTTPServer()
}

// useCrawlerCriteria injects the crawler's "Ignore jobs" rules (step 150): the source that builds the enabled matchers from the settings
// for each crawl, and the log of the jobs they keep out. Called by both processes that insert crawled jobs (the HTTP server and the AI's MCP).
func useCrawlerCriteria() {
	jobs.UseCriteria(crawlcriteria.Source{})
	jobs.UseFilteredOutLog(crawlcriteria.RecordIgnored)
}

func runHTTPServer() {
	port := os.Getenv("PORT")

	if err := db.InitDatabases(); err != nil {
		log.Fatalf("failed to open career/jobs databases: %v", err)
	}
	// The memory of deleted jobs (step 133) is optional: rows made with older URL normalization rules are recomputed
	// here, and a missing database is only a log line (see db.openDeletedJobsDB).
	if _, err := deletedjobs.Rehash(); err != nil && !errors.Is(err, deletedjobs.ErrNoMemory) {
		log.Printf("deleted jobs: could not recompute the keys: %v", err)
	}
	// Step 147: crawls used to store markup and stylesheets in the descriptions; they are rewritten to plain text once, after a
	// copy of jobs.db. Never fatal: the next start tries again when it did not finish.
	if r, err := jobs.CleanStoredDescriptions(); err != nil {
		log.Printf("jobs: could not clean the stored descriptions: %v", err)
	} else if !r.AlreadyDone {
		log.Printf("jobs: cleaned %d of %d crawled descriptions (backup: %q)", r.Changed, r.Scanned, r.Backup)
	}
	// Deleting a CV document must know which jobs use it (step 98); cvbuilder
	// cannot import jobs, so the implementation is registered here.
	cvbuilder.UseJobLinks(jobs.DocumentLinks{})
	useCrawlerCriteria()
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
	if err := match.ReconcileOrphanedJobMatches(); err != nil {
		log.Fatalf("failed to reconcile orphaned job matches: %v", err)
	}
	// Same reasoning as reconcileOrphanedJobMatches above, one level up
	// — a hidden CV-generation conversation's own turn (or the
	// frontend's own post-turn Media persist step) is not a subprocess
	// this Go process could ever reattach to after a restart.
	if err := jobs.ReconcileOrphanedCvGenerations(); err != nil {
		log.Fatalf("failed to reconcile orphaned CV generations: %v", err)
	}
	// step 112 — a persona AI run's own turn lives in the core app too, so a restart here does
	// not orphan it; only a run stuck 'generating' past the stale threshold is ended. Never
	// fatal, like the instruction attempts below.
	if n, err := persona.ReconcileStaleAiRuns(); err != nil {
		log.Printf("failed to reconcile stale persona AI runs: %v", err)
	} else if n > 0 {
		log.Printf("ended %d stale persona AI run(s)", n)
	}
	// step 155 — same for a CV import run (the AI's proposal arrives through suggest_cv_import; the page ends the run).
	if n, err := cv.ReconcileStaleCVImportRuns(); err != nil {
		log.Printf("failed to reconcile stale CV import runs: %v", err)
	} else if n > 0 {
		log.Printf("ended %d stale CV import run(s)", n)
	}
	// step 115 — same for a CV AI run.
	if n, err := cvbuilder.ReconcileStaleCvAiRuns(); err != nil {
		log.Printf("failed to reconcile stale CV AI runs: %v", err)
	} else if n > 0 {
		log.Printf("ended %d stale CV AI run(s)", n)
	}
	// step 86 — an instruction attempt's own run lives in the core app,
	// so a restart here does not orphan it; only ones stuck 'running'
	// past the stale threshold (tab closed, app crashed) are ended.
	// Never fatal: this is a counter, not core state.
	if n, err := portal.ReconcileStaleInstructionAttempts(); err != nil {
		log.Printf("failed to reconcile stale instruction attempts: %v", err)
	} else if n > 0 {
		log.Printf("ended %d stale instruction attempt(s)", n)
	}
	// step 47.4 — reconcileOrphanedCrawlRuns above only ever runs once,
	// at boot, so it can't help a goroutine that's genuinely hung
	// without this process itself restarting. This periodic sweep is
	// the in-process complement: it catches that case too, without
	// requiring a restart.
	crawl.StartStaleCrawlRunReaper()
	autodiscovery.StartAutoDiscoveryManager()
	jobcleanup.StartManager(context.Background())
	jobmatchconsumer.Start(context.Background())
	profilejobmatchconsumer.Start(context.Background())
	personamatchconsumer.Start(context.Background())
	personamatchconsumer.StartSweeper(context.Background())
	personarecomputeconsumer.Start(context.Background())
	personarecomputeconsumer.StartSweeper(context.Background())
	// Step 151: the re-match of the jobs for a new profile or persona, one worker for both consumers.
	rematchworker.Start(context.Background())
	rematchworker.StartSweeper(context.Background())
	// Step 152: the catch-up that removes the matches of deleted profiles and personas (lost events, late AI matches, old leftovers).
	matchorphans.StartSweeper(context.Background())
	// Plan 163, step 5: raises the request flags of the listing/job-detail chain that a lost event should have raised.
	portal.StartFlagSweeper(context.Background())
	jobrecrawlconsumer.Start(context.Background())
	jobrecrawlconsumer.StartSweeper(context.Background())
	// The catch-up for events that were missed (step 134): jobs without any match are queued for the same worker,
	// once shortly after start and then every 10 minutes.
	jobmatchconsumer.StartSweeper(context.Background())
	profilejobmatchconsumer.StartSweeper(context.Background())

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/profiles", profilehandlers.ProfilesHandler)
	http.HandleFunc("/profile-links", profilehandlers.ProfileLinksHandler)
	http.HandleFunc("/profile-image", profileimage.Handler)
	http.HandleFunc("/photos", profileimage.PhotosHandler)
	http.HandleFunc("/profile-photos", profileimage.ProfilePhotosHandler)
	http.HandleFunc("/persona-ai-runs", persona.AiRunsHandler)
	http.HandleFunc("/persona-ai-runs/apply", persona.AiRunApplyHandler)
	http.HandleFunc("/personas", personahandlers.PersonasHandler)
	http.HandleFunc("/persona-details", personahandlers.PersonaDetailsHandler)
	http.HandleFunc("/skills", profilehandlers.SkillsHandler)
	http.HandleFunc("/experience", profilehandlers.ExperienceHandler)
	// Plan 173: the profile library and what a persona uses of it.
	http.HandleFunc("/profile-skills", profilehandlers.ProfileSkillsHandler)
	http.HandleFunc("/profile-experience", profilehandlers.ProfileExperienceHandler)
	http.HandleFunc("/persona-skills", profilehandlers.PersonaSkillsHandler)
	http.HandleFunc("/persona-experience", profilehandlers.PersonaExperienceHandler)
	// Plan 173, step 6: the profile's education.
	http.HandleFunc("/education", educationhandlers.EducationHandler)
	// Plan 176, step 2: the application protocol.
	http.HandleFunc("/application-messages", applicationloghandlers.ApplicationLogHandler)
	// Plan 174, step 3: the summary and goals of a profile.
	http.HandleFunc("/profile-details", profilehandlers.ProfileDetailsHandler)
	http.HandleFunc("/jobs", jobshandlers.JobsHandler)
	http.HandleFunc("/job-locations", jobshandlers.JobLocationsHandler)
	http.HandleFunc("/dashboard/new-jobs", jobs.NewJobsWidgetHandler)
	http.HandleFunc("/dashboard/recent-jobs", jobs.RecentJobsWidgetHandler)
	http.HandleFunc("/jobs/stats", jobs.StatsHandler)
	http.HandleFunc("/jobs/delete", jobs.DeleteManyHandler)
	http.HandleFunc("/jobs/recrawl", jobrecrawl.RequestHandler)
	http.HandleFunc("/jobs/recrawl/status", jobrecrawl.StatusHandler)
	http.HandleFunc("/jobs/recrawl/cancel", jobrecrawl.CancelHandler)
	http.HandleFunc("/jobs/match", jobshandlers.JobMatchHandler)
	http.HandleFunc("/jobs/match/run", jobshandlers.JobMatchRunHandler)
	http.HandleFunc("/job-requirements", jobshandlers.JobRequirementsHandler)
	http.HandleFunc("/job-requirements/suggest", jobshandlers.JobRequirementsSuggestHandler)
	http.HandleFunc("/jobs/cv", cvhandlers.CvPdfHandler)
	http.HandleFunc("/jobs/cv-generations", cvhandlers.CvGenerationsHandler)
	http.HandleFunc("/jobs/cv-link", cvhandlers.CvLinkHandler)
	http.HandleFunc("/job-recruiters", jobshandlers.JobRecruitersHandler)
	http.HandleFunc("/job-board", jobshandlers.JobBoardHandler)
	http.HandleFunc("/job-applications", jobshandlers.JobApplicationsHandler)
	http.HandleFunc("/job-statuses", jobshandlers.JobStatusesHandler)
	http.HandleFunc("/job-board-entries", jobshandlers.JobBoardEntriesHandler)
	http.HandleFunc("/job-board-usage", jobshandlers.JobBoardUsageHandler)
	http.HandleFunc("/job-persona-matches", personahandlers.JobPersonaMatchesHandler)
	http.HandleFunc("/job-persona-matches/recalculate", personahandlers.JobPersonaMatchesRecalculateHandler)
	http.HandleFunc("/profile-match-rules", profilehandlers.ProfileMatchRulesHandler)
	http.HandleFunc("/profile-match-rules/order", profilehandlers.ProfileMatchRulesOrderHandler)
	http.HandleFunc("/profile-match-rules/apply", profilehandlers.ProfileMatchRulesApplyHandler)
	http.HandleFunc("/companies", companieshandlers.CompaniesHandler)
	http.HandleFunc("/recruiters", recruitershandlers.RecruitersHandler)
	http.HandleFunc("/portals", portalhandlers.PortalsHandler)
	http.HandleFunc("/portals/ai", portalhandlers.PortalAIHandler)
	http.HandleFunc("/portal-links", portalhandlers.PortalLinksHandler)
	http.HandleFunc("/portal-links/ai", portalhandlers.PortalLinkAIHandler)
	http.HandleFunc("/portal-links/pipeline", portalhandlers.PortalPipelineHandler)
	http.HandleFunc("/portal-links/crawl-request", crawlhandlers.CrawlRequestHandler)
	http.HandleFunc("/portal-links/ingest-crawl-results", crawlhandlers.IngestCrawlResultsHandler)
	http.HandleFunc("/portal-links/instruction-attempts/start", crawlhandlers.InstructionAttemptStartHandler)
	http.HandleFunc("/portal-links/instruction-attempts/finish", crawlhandlers.InstructionAttemptFinishHandler)
	http.HandleFunc("/portal-links/start-fresh", crawlhandlers.StartFreshHandler)
	http.HandleFunc("/portal-links/crawl-now", crawl.CrawlNowHandler)
	http.HandleFunc("/portal-links/inherit-instructions", crawl.InheritInstructionsHandler)
	http.HandleFunc("/portal-links/crawl-job-details-now", crawl.CrawlJobDetailsNowHandler)
	http.HandleFunc("/portal-links/crawl-runs/active", crawl.CrawlMonitorHandler)
	http.HandleFunc("/portal-links/crawl-now/active", crawl.CrawlNowActiveHandler)
	http.HandleFunc("/portal-links/crawl-now/cancel", crawl.CrawlNowCancelHandler)
	http.HandleFunc("/portal-links/crawl-engine", crawl.CrawlEngineHandler)
	http.HandleFunc("/portal-links/link-crawl/availability", crawl.LinkCrawlAvailabilityHandler)
	http.HandleFunc("/portal-links/link-crawl/rules", crawl.LinkCrawlRulesHandler)
	http.HandleFunc("/portal-links/link-crawl/candidates", crawl.LinkCrawlCandidatesHandler)
	http.HandleFunc("/portal-links/link-crawl/learn", crawl.LinkCrawlLearnHandler)
	http.HandleFunc("/portal-links/link-crawl/learn-pagination", crawl.LinkCrawlLearnPaginationHandler)
	http.HandleFunc("/portal-links/link-crawl/preview", crawl.LinkCrawlPreviewHandler)
	http.HandleFunc("/portal-links/link-crawl/test", crawl.LinkCrawlTestHandler)
	http.HandleFunc("/portal-links/job-detail/candidates", crawl.JobDetailCandidatesHandler)
	http.HandleFunc("/portal-links/job-detail/preview", crawl.JobDetailPreviewHandler)
	http.HandleFunc("/portal-links/job-detail/test", crawl.JobDetailTestHandler)
	http.HandleFunc("/portal-links/job-detail/save", crawl.JobDetailSaveHandler)
	http.HandleFunc("/portal-links/auto-discovery", autodiscovery.SettingsHandler)
	http.HandleFunc("/settings/ai", settings.AIHandler)
	http.HandleFunc("/settings/cleanup", jobcleanup.SettingsHandler)
	http.HandleFunc("/settings/crawler", crawlcriteria.SettingsHandler)
	http.HandleFunc("/settings/crawler/ignored", crawlcriteria.IgnoredHandler)
	http.HandleFunc("/jobs/cleanup/preview", jobcleanup.PreviewHandler)
	http.HandleFunc("/jobs/cleanup/run", jobcleanup.RunHandler)
	http.HandleFunc("/jobs/deleted", deletedjobs.Handler)
	http.HandleFunc("/events/listing-instructions-ready", portal.ListingInstructionsReadyEventHandler)
	http.HandleFunc("/events/jobs-crawled", portal.JobsCrawledEventHandler)
	http.HandleFunc("/events/job-created", jobs.JobCreatedEventHandler)
	http.HandleFunc("/events/job-found", jobmatchconsumer.EventHandler)
	http.HandleFunc("/events/job-description-changed-job-match", jobmatchconsumer.ChangedEventHandler)
	http.HandleFunc("/events/job-found-profile-rules", profilejobmatchconsumer.EventHandler)
	http.HandleFunc("/events/job-found-persona-matches", personamatchconsumer.EventHandler)
	http.HandleFunc("/events/persona-changed-recompute", personarecomputeconsumer.EventHandler)
	http.HandleFunc("/events/profile-created-rematch", profilecreatedconsumer.EventHandler)
	http.HandleFunc("/events/persona-created-rematch", personacreatedconsumer.EventHandler)
	http.HandleFunc("/events/persona-changed-rematch-rearm", personacreatedconsumer.ChangedHandler)
	http.HandleFunc("/events/profile-deleted-matches", profiledeletedconsumer.EventHandler)
	http.HandleFunc("/events/persona-deleted-matches", personadeletedconsumer.EventHandler)
	http.HandleFunc("/events/job-recrawl", jobrecrawlconsumer.EventHandler)
	http.HandleFunc("/events/job-description-changed-persona-matches", personamatchconsumer.EventHandler)
	http.HandleFunc("/events/job-description-changed-profile-rules", profilejobmatchconsumer.EventHandler)
	http.HandleFunc("/events/job-deleted-comments", jobdeletedconsumers.JobCommentsDeleteHandler)
	http.HandleFunc("/events/job-deleted-company", jobdeletedconsumers.CompanyDeleteHandler)
	http.HandleFunc("/events/job-deleted-persona-matches", jobdeletedconsumers.PersonaMatchJobDeletedHandler)
	http.HandleFunc("/events/company-deleted-comments", companydeletedconsumers.CompanyCommentsDeleteHandler)
	// Plan 168, step 7: the comments of a deleted recruiter.
	http.HandleFunc("/events/recruiter-deleted-comments", recruiterdeletedconsumers.RecruiterCommentsDeleteHandler)
	http.HandleFunc("/jobs/match/unmatched", jobmatchconsumer.MatchNowHandler)
	http.HandleFunc("/cv-import/upload", cv.UploadCVHandler)
	http.HandleFunc("/cv-import/runs", cv.CVImportRunsHandler)
	http.HandleFunc("/cv-documents/templates", cvbuilder.TemplatesHandler)
	http.HandleFunc("/cv-documents/persona-defaults", cvbuilder.PersonaDefaultsHandler)
	http.HandleFunc("/cv-documents/photo-status", cvbuilder.PhotoStatusHandler)
	http.HandleFunc("/cv-documents/preview", cvbuilder.PreviewHandler)
	http.HandleFunc("/cv-documents/personas", cvbuilder.PersonaLinksHandler)
	http.HandleFunc("/cv-ai-runs", cvbuilder.CvAiRunsHandler)
	http.HandleFunc("/cv-documents", cvbuilder.DocumentsHandler)

	// autodiscovery.CaptureTokenMiddleware wraps the entire mux here,
	// once, rather than each individual route registration above — see
	// that middleware's own doc comment for why this one choke point is
	// enough. http.DefaultServeMux is what every http.HandleFunc call
	// above registered into (the previous nil handler argument meant
	// exactly that).
	if err := http.ListenAndServe("127.0.0.1:"+port, autodiscovery.CaptureTokenMiddleware(http.DefaultServeMux)); err != nil {
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
	useCrawlerCriteria() // the AI's save tools run in this process: they follow the same rules as the crawl

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
	persona.RegisterSuggestPersona(server)
	cv.RegisterSuggestCVImport(server)
	cv.RegisterCVImportDraftTools(server)
	persona.RegisterUpdatePersonaDetails(server)
	persona.RegisterAddCareerSkill(server)
	persona.RegisterAddCareerSkills(server)
	persona.RegisterRemoveCareerSkill(server)
	persona.RegisterAddCareerExperience(server)
	persona.RegisterAddCareerExperiences(server)
	persona.RegisterRemoveCareerExperience(server)
	persona.RegisterUpdateCareerExperience(server)
	// Plan 173: the profile library and what a persona uses of it.
	profileitems.RegisterListProfileSkills(server)
	profileitems.RegisterAddProfileSkill(server)
	profileitems.RegisterListProfileExperience(server)
	profileitems.RegisterAddProfileExperience(server)
	profileitems.RegisterSetPersonaSkills(server)
	profileitems.RegisterSetPersonaExperience(server)
	education.RegisterListEducation(server)
	education.RegisterAddEducation(server)
	education.RegisterUpdateEducation(server)
	education.RegisterRemoveEducation(server)
	applicationlog.RegisterListApplicationMessages(server)
	applicationlog.RegisterAddApplicationMessage(server)
	applicationlog.RegisterUpdateApplicationMessage(server)
	applicationlog.RegisterRemoveApplicationMessage(server)
	profiledetails.RegisterGetProfileDetails(server)
	profiledetails.RegisterUpdateProfileDetails(server)
	jobs.RegisterSaveJob(server)
	jobs.RegisterListJobs(server)
	jobs.RegisterSearchJobs(server)
	jobs.RegisterDeleteJob(server)
	jobs.RegisterSaveJobDetailExtraction(server)
	match.RegisterGetJob(server)
	match.RegisterSaveJobMatch(server)
	jobs.RegisterSaveCvDocument(server)
	cvbuilder.RegisterListCvTemplates(server)
	cvbuilder.RegisterGetPersonaCvDefaults(server)
	cvbuilder.RegisterRenderCvDocument(server)
	cvbuilder.RegisterSavePersonaCv(server)
	cvbuilder.RegisterSaveCvForPersona(server)
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
	portal.RegisterCheckCrawlExtractionResult(server)
	portal.RegisterAskUserIfPageHasJobs(server)
	portal.RegisterGetPortalLinkJobDetailCrawlInstructions(server)
	portal.RegisterSetPortalLinkJobDetailCrawlInstructions(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(1)
	}
}
