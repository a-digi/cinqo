// Package instruction is the neutral home of the "instruction engines" of the Career tool (plan 192): everything that depends on HOW a link's
// crawl instructions are made and used, behind one interface. There are two engines, Rod (the AI writes CSS selectors) and Katana (a link
// rule for the job list and picked data for the job page), and each one is a package of its own under this directory (instruction/rod,
// instruction/katana) that implements Engine.
//
// The rules that keep the engines decoupled (enforced by decoupling_test.go and by enginetest):
//   - an engine package never imports another engine package;
//   - this package and enginetest import no engine package and none of portal, crawl, jobs or autodiscovery (shared code such as portal asks
//     THIS package, so this package must stay below it);
//   - the one place that names the engines is backend/engines.go: removing an engine means deleting its package and its registration line;
//   - everything here is neutral data and small interfaces, never behaviour of one engine.
package instruction

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Context is the standard context; the alias keeps the capability interfaces readable.
type Context = context.Context

// ToolSpec names one AI tool an engine registers: the name core shows the model and the scope core checks (manifest.json mcp_tools).
type ToolSpec struct {
	Name  string
	Scope string
	// Legacy marks a tool that existed before the engine was an engine (the Rod tools): it keeps its name. Every other tool of an engine
	// carries the engine's id as a prefix ("katana_…"), so two engines can never claim the same name.
	Legacy bool
}

// Event roles.
const (
	Publishes = "publishes"
	Consumes  = "consumes"
)

// EventSpec is a domain event topic an engine publishes or consumes. A consumed topic is listed in manifest.json event_listeners with its
// path suffix; a published one needs no manifest entry.
type EventSpec struct {
	Topic      string
	Role       string
	PathSuffix string // consumes only
}

// LinkFacts is what an engine needs to know about a portal link to judge it, as plain facts (no database types, so this package stays neutral).
type LinkFacts struct {
	RequiresLogin            bool
	HasRule                  bool // a link-crawler rule is stored
	HasAIInstructions        bool // AI-written job-list selectors are stored
	HasJobDetailInstructions bool
	HasJobs                  bool // at least one job was crawled for the link
}

// How a link's job list is crawled. The values are the ones stored in portal_links.crawl_method and used by the portal package.
const (
	MethodInstructions = "instructions" // the AI-written selectors (Rod)
	MethodLinks        = "links"        // the link crawler's rule (Katana)
)

// Who writes the job-detail instructions.
const (
	WrittenByAI     = "ai"     // the AI writes selectors (Rod)
	WrittenByPicker = "picker" // a person (or the AI, through tools) picks the data of a job page (Katana)
)

// JobDetailPolicy is how an engine instructs a link's job-detail page.
type JobDetailPolicy struct {
	WrittenBy string
	// AutoAIRequest: after a crawl the app starts an AI run for the job details by itself.
	AutoAIRequest bool
	// NeedsCrawledJobs: the job-detail instructor works on a crawled job, so its modal waits until the link has one (plan 191).
	NeedsCrawledJobs bool
}

// Engine is one instruction engine. The core interface is deliberately small; what only some steps need is a capability interface below
// that an engine may implement.
type Engine interface {
	// ID is the Browser engine name this engine serves ("rod", "katana"); lower case letters, digits and underscores.
	ID() string
	// Tools are the AI tools this engine registers; RegisterTools registers exactly those on the MCP server.
	Tools() []ToolSpec
	RegisterTools(server *mcp.Server)
	// ListingMethod is how this engine crawls the job list of a link that needs no login: MethodInstructions or MethodLinks. (A link that
	// needs a login is always read with its AI instructions, in the login window, under every engine; ListingMethodFor handles that.)
	ListingMethod() string
	// HasListing says whether the link is instructed for this engine's job list (a rule, or AI instructions).
	HasListing(link LinkFacts) bool
	// ListingReadySQL is a SQL condition over the portal_links table that is true for a link that has what this engine needs to crawl its
	// job list (the auto-discovery sweep lists links by it). One condition, no semicolon.
	ListingReadySQL() string
	// JobDetailPolicy says how this engine instructs the job-detail page of the link.
	JobDetailPolicy(link LinkFacts) JobDetailPolicy
	// Events are the domain events this engine publishes or consumes.
	Events() []EventSpec
}

// ListingRun is what an engine needs to crawl a link's job list (Crawl now and auto-discovery), as plain values.
type ListingRun struct {
	RunID       string
	LinkID      string
	CoreURL     string
	AccessToken string
}

// ListingCrawler is implemented by an engine that crawls a link's job list itself (step 3 of plan 192 moves the crawl dispatch behind it).
type ListingCrawler interface {
	CrawlListing(ctx Context, run ListingRun) error
}

// PipelineFacts are the parts of a link's pipeline snapshot an engine may change (step 3), as plain values.
type PipelineFacts struct {
	DetailAIRequested bool
	DetailFailedRuns  int
}

// PipelineAdjuster is implemented by an engine whose job-detail instruction makes some pipeline facts meaningless (under Katana the AI's
// job-detail request and failures say nothing about a link).
type PipelineAdjuster interface {
	AdjustPipeline(link LinkFacts, facts *PipelineFacts)
}
