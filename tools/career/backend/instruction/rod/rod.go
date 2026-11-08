// Package rod is the Rod instruction engine (plan 192): the AI writes CSS selectors for a link's job list and job pages. It is an ADAPTER
// over the existing Rod code in package portal, which stays where it is and is not edited: RegisterTools calls the same portal.Register…
// functions main.go called before, and the judgements below express what the shared code did for every engine that is not Katana.
//
// Decoupling: this package must not import another engine (instruction/katana); a test reads the import graph.
package rod

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/instruction"
	"career-tool-backend/portal"
)

// ID is the Browser engine name. Every Browser engine other than Katana crawls with AI-written instructions, so Rod is also what the
// shared code falls back to (step 3 decides how; this adapter only serves the name "rod").
const ID = "rod"

const scope = "tool:career:portals"

type engine struct{}

// New returns the Rod engine.
func New() instruction.Engine { return engine{} }

func (engine) ID() string { return ID }

// Tools are the six tools that existed before the engine concept, so they keep their names (Legacy): the AI's saved conversations and
// the prompts name them.
func (engine) Tools() []instruction.ToolSpec {
	names := []string{
		"get_portal_link_crawl_instructions",
		"set_portal_link_crawl_instructions",
		"check_crawl_extraction_result",
		"ask_user_if_page_has_jobs",
		"get_portal_link_job_detail_crawl_instructions",
		"set_portal_link_job_detail_crawl_instructions",
	}
	out := make([]instruction.ToolSpec, len(names))
	for i, n := range names {
		out[i] = instruction.ToolSpec{Name: n, Scope: scope, Legacy: true}
	}
	return out
}

// RegisterTools registers the Rod tools through the existing portal functions, in the order main.go always used.
func (engine) RegisterTools(server *mcp.Server) {
	portal.RegisterGetPortalLinkCrawlInstructions(server)
	portal.RegisterSetPortalLinkCrawlInstructions(server)
	portal.RegisterCheckCrawlExtractionResult(server)
	portal.RegisterAskUserIfPageHasJobs(server)
	portal.RegisterGetPortalLinkJobDetailCrawlInstructions(server)
	portal.RegisterSetPortalLinkJobDetailCrawlInstructions(server)
}

// ListingMethod: the AI-written selectors.
func (engine) ListingMethod() string { return instruction.MethodInstructions }

// HasListing: a Rod link is instructed when the AI wrote its job-list selectors (portal.listingPresent, method "instructions").
func (engine) HasListing(link instruction.LinkFacts) bool { return link.HasAIInstructions }

// ListingReadySQL is portal.crawlableListingSQL("instructions").
func (engine) ListingReadySQL() string {
	return `(crawl_instructions IS NOT NULL AND crawl_instructions != '')`
}

// JobDetailPolicy: the AI writes the selectors, the app may start that run by itself after a crawl, and the modal never waits for jobs.
func (engine) JobDetailPolicy(instruction.LinkFacts) instruction.JobDetailPolicy {
	return instruction.JobDetailPolicy{WrittenBy: instruction.WrittenByAI, AutoAIRequest: true, NeedsCrawledJobs: false}
}

// Events: setting the listing instructions publishes the topic the page and the crawl consume (the consumer is shared by every engine and
// stays in the manifest as it is).
func (engine) Events() []instruction.EventSpec {
	return []instruction.EventSpec{{Topic: "career.portal_link.listing_instructions_ready", Role: instruction.Publishes}}
}
