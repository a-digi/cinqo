// Package katana is the Katana instruction engine (plan 192): the job list is crawled with a link-crawler RULE (URL patterns) and the job
// page is read by picking its data. It is an ADAPTER over what already exists (the rule, the pure packages portal/linkrule and
// portal/detailpick, the picker pipeline rules of plans 190 and 191); later steps add the AI tools, the internal service and the event
// here, and nowhere else.
//
// Decoupling: this package must not import another engine (instruction/rod); a test reads the import graph.
package katana

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/instruction"
)

// ID is the Browser engine name.
const ID = "katana"

type engine struct{}

// New returns the Katana engine.
func New() instruction.Engine { return engine{} }

func (engine) ID() string { return ID }

// Tools: the job-list tools (tools.go); step 8 adds the job-detail ones.
func (engine) Tools() []instruction.ToolSpec { return toolSpecs() }

func (engine) RegisterTools(server *mcp.Server) { registerTools(server) }

// ListingMethod: the link crawler's rule.
func (engine) ListingMethod() string { return instruction.MethodLinks }

// HasListing: a link that needs no login is instructed when it has a rule; a login link is read with its AI instructions, in the login
// window, under every engine.
func (engine) HasListing(link instruction.LinkFacts) bool {
	if link.RequiresLogin {
		return link.HasAIInstructions
	}
	return link.HasRule
}

// ListingReadySQL is portal.crawlableListingSQL("links").
func (engine) ListingReadySQL() string { return `(link_rules IS NOT NULL AND link_rules != '')` }

// JobDetailPolicy: the job page is read by picking its data, on a crawled job, and the AI's automatic run does not start (plan 190, 191). A
// link that needs a login has no picker and keeps the AI.
func (engine) JobDetailPolicy(link instruction.LinkFacts) instruction.JobDetailPolicy {
	if link.RequiresLogin {
		return instruction.JobDetailPolicy{WrittenBy: instruction.WrittenByAI, AutoAIRequest: true, NeedsCrawledJobs: false}
	}
	return instruction.JobDetailPolicy{WrittenBy: instruction.WrittenByPicker, AutoAIRequest: false, NeedsCrawledJobs: true}
}

// AdjustPipeline: under Katana the AI's job-detail request and failures say nothing about a link that is not a login link (portal.PipelineForLinks).
func (engine) AdjustPipeline(link instruction.LinkFacts, facts *instruction.PipelineFacts) {
	if link.RequiresLogin {
		return
	}
	facts.DetailAIRequested = false
	facts.DetailFailedRuns = 0
}

// Events: saving a rule publishes the engine's own topic, and the engine's handler turns it into the request to crawl (service/events.go).
// The topic and the path are spelled here and in the service; a test keeps the two equal.
func (engine) Events() []instruction.EventSpec {
	return []instruction.EventSpec{
		{Topic: "career.portal_link.katana_rule_saved", Role: instruction.Publishes},
		{Topic: "career.portal_link.katana_rule_saved", Role: instruction.Consumes, PathSuffix: "events/katana-rule-saved"},
	}
}
