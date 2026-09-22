// crawl_fanout.go — crawl_urls_with_subagents: a built-in pseudo-tool
// (same shape as spawn_subagent and friends, subagent.go — never
// installed, no manifest.json, no subprocess) that fans a batch of
// URLs out to one independent sub-agent per URL, spawned
// DETERMINISTICALLY by this tool's own Go implementation rather than
// left to the calling model's own discretion. This is the actual
// enforcement mechanism behind "crawling many individual pages must
// always use sub-agents, not model choice": a model that wants this
// tool's own result has no path to it that skips spawning sub-agents,
// because the spawning happens in code, inside this file, not via N
// separate spawn_subagent calls the model could choose to make or
// skip.
//
// Deliberately domain-agnostic — this package (api/src/conversation)
// knows nothing about Career, jobs, or portal links; urls,
// extractFields, and which tool to call with each extraction (
// saveToolName/saveToolArgsKey/saveToolArgsByUrl) are all supplied by
// the caller. Career (or any future tool needing "visit many pages,
// extract fields, save each result somewhere") is simply this tool's
// first caller — the same "generic core, tool-specific caller"
// precedent the Media feature already established. See
// plan/ai/conversation/step-XX-crawl-urls-with-subagents.md and
// plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
//
// Every sub-agent this tool spawns is restricted to EXACTLY
// ["extract_from_url", saveToolName] — never the caller's own full
// tool set, and never caller-supplied (unlike spawn_subagent's own
// optional allowedTools) — see crawlURLsWithSubagentsAllowedTools,
// below. extract_from_url (tools/browser/backend/crawler/
// extract_from_url.go) does navigate+extract as ONE atomic operation
// specifically so that many of these sub-agents can safely share
// browser's own single shared browser tab at the same time — the
// separate fetch_page_html + extract_page_data two-call sequence
// cannot safely be used this way (a concurrent sibling's own navigate
// can land in between the two calls, silently extracting the wrong
// page).
package conversation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"

	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

const crawlURLsWithSubagentsToolName = "crawl_urls_with_subagents"

var crawlURLsWithSubagentsToolDef = chatcompleter.ToolDef{
	Name: crawlURLsWithSubagentsToolName,
	Description: "Crawl a batch of individual page URLs and extract fields from each — ONE INDEPENDENT SUB-AGENT " +
		"PER URL, spawned automatically by this tool itself. This is not optional or a matter of your own " +
		"judgment: whenever you have several individual pages to visit and extract data from (e.g. job detail " +
		"pages once you have their URLs from a listing crawl), you MUST use this tool instead of calling " +
		"extract_from_url yourself in a loop — doing it yourself defeats the isolation and concurrency this tool " +
		"exists to provide. Each spawned sub-agent is restricted to only extract_from_url and the saveToolName " +
		"you name, and does exactly two things: extract the given fields from its own URL, then call " +
		"saveToolName with the extracted values. Returns immediately once every sub-agent has been started (or " +
		"refused, e.g. the concurrency limit was reached) — it does NOT wait for them to finish; check on them " +
		"with check_subagent/list_subagents. Subject to the same total-sub-agents-per-turn limit as " +
		"spawn_subagent — a batch that would exceed it is refused outright; call this again with a smaller " +
		"batch instead of retrying the same one. saveToolName must already be one of your own available tools; " +
		"naming one you don't have access to is refused.",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"urls": {
				"type": "array",
				"items": { "type": "string" },
				"description": "The URLs to crawl — one independent sub-agent is spawned per URL."
			},
			"extractFields": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"label": { "type": "string" },
						"selector": { "type": "string" },
						"attribute": { "type": "string" },
						"multiple": { "type": "boolean" }
					},
					"required": ["label", "selector"]
				},
				"description": "Which fields to extract off EACH url (same shape as extract_from_url's own fields) — applied identically to every URL in this batch."
			},
			"saveToolName": {
				"type": "string",
				"description": "The name of the tool each sub-agent calls with its own extracted values, once per URL — must already be one of your own available tools."
			},
			"saveToolArgsKey": {
				"type": "string",
				"description": "The argument name on saveToolName that identifies which record this URL's own extraction belongs to (e.g. \"jobId\")."
			},
			"saveToolArgsByUrl": {
				"type": "object",
				"additionalProperties": { "type": "string" },
				"description": "Maps each url (must match one entry in urls exactly) to the value for saveToolArgsKey on that url's own save call (e.g. its own job id)."
			}
		},
		"required": ["urls", "extractFields", "saveToolName", "saveToolArgsKey", "saveToolArgsByUrl"]
	}`),
}

// crawlURLFieldSpec mirrors browser's own extractField
// (tools/browser/backend/crawler/extract.go) and career's own
// crawlInstructionsField (tools/career/backend/portals.go) —
// independently declared, not imported, same "two fully separate Go
// modules/binaries, no shared package" reasoning career's own
// crawlRequestField already established.
type crawlURLFieldSpec struct {
	Label     string `json:"label"`
	Selector  string `json:"selector"`
	Attribute string `json:"attribute,omitempty"`
	Multiple  bool   `json:"multiple,omitempty"`
}

type crawlURLsWithSubagentsArgs struct {
	URLs              []string            `json:"urls"`
	ExtractFields     []crawlURLFieldSpec `json:"extractFields"`
	SaveToolName      string              `json:"saveToolName"`
	SaveToolArgsKey   string              `json:"saveToolArgsKey"`
	SaveToolArgsByURL map[string]string   `json:"saveToolArgsByUrl"`
}

// crawlURLsWithSubagentsAllowedTools is the FIXED allowedTools list
// every sub-agent this tool spawns is restricted to — computed here,
// never taken from the caller (unlike spawn_subagent's own optional
// allowedTools argument). This is the actual enforcement mechanism: a
// model that calls this tool cannot cause its own spawned sub-agents
// to do anything other than extract one page and save the result,
// regardless of what else the calling model itself might have access
// to.
func crawlURLsWithSubagentsAllowedTools(saveToolName string) []string {
	return []string{"extract_from_url", saveToolName}
}

// invokeCrawlURLsWithSubAgentsCall validates the batch, then spawns one
// sub-agent per URL via spawnSubAgent (subagent.go) — the same
// underlying spawn mechanics spawn_subagent itself uses, just called
// N times in a row from Go code instead of relying on N separate
// model-issued spawn_subagent calls. Never returns a bare error —
// matches invokeSubAgentCall's own "never leave a tool_call unanswered"
// convention.
func invokeCrawlURLsWithSubAgentsCall(
	httpClient *http.Client,
	entry platform.Entry,
	apiKey, model string,
	mainDB, conversationDB *sql.DB,
	callerScopes []string,
	userID string,
	dataDir string,
	corePort int,
	conversationID, parentTurnRunID string,
	call chatcompleter.ToolCall,
	depth int,
) string {
	var args crawlURLsWithSubagentsArgs
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return fmt.Sprintf("invalid arguments: %v", err)
	}
	if len(args.URLs) == 0 {
		return "urls is required and must have at least one entry"
	}
	if len(args.ExtractFields) == 0 {
		return "extractFields is required and must have at least one entry"
	}
	for _, f := range args.ExtractFields {
		if f.Label == "" || f.Selector == "" {
			return "every extractFields entry requires both a label and a selector"
		}
	}
	if args.SaveToolName == "" {
		return "saveToolName is required"
	}
	if args.SaveToolArgsKey == "" {
		return "saveToolArgsKey is required"
	}

	// saveToolName must already be one of the caller's own offerable
	// tools — this can only ever narrow what a spawned sub-agent can do
	// (crawlURLsWithSubagentsAllowedTools, above), never grant access to
	// a tool the caller itself couldn't otherwise reach.
	callerTools, err := offerableTools(mainDB, callerScopes, false)
	if err != nil {
		return fmt.Sprintf("failed to check saveToolName: %v", err)
	}
	saveToolKnown := false
	for _, t := range callerTools {
		if t.Name == args.SaveToolName {
			saveToolKnown = true
			break
		}
	}
	if !saveToolKnown {
		return fmt.Sprintf("unknown or unauthorized saveToolName %q", args.SaveToolName)
	}

	// maxSubAgentsPerTurn — checked once for the WHOLE batch (existing
	// plus every URL this call would spawn), not per-URL: a batch that
	// would blow through the turn's own remaining budget is refused
	// entirely, rather than silently spawning only however many fit and
	// leaving the caller to guess which URLs were skipped.
	existing, err := conversation_query.NewSubAgentRunQueryRepo(conversationDB).CountByParentTurnRunID(parentTurnRunID)
	if err != nil {
		return fmt.Sprintf("failed to check sub-agent limit: %v", err)
	}
	if existing+len(args.URLs) > maxSubAgentsPerTurn {
		return fmt.Sprintf(
			"Cannot crawl %d URLs: this turn has already spawned %d of a maximum %d sub-agents, leaving room for only %d more. Call this tool again with a smaller batch.",
			len(args.URLs), existing, maxSubAgentsPerTurn, maxSubAgentsPerTurn-existing,
		)
	}

	fieldsJSON, err := json.Marshal(args.ExtractFields)
	if err != nil {
		return fmt.Sprintf("failed to build extraction task: %v", err)
	}
	allowedTools := crawlURLsWithSubagentsAllowedTools(args.SaveToolName)

	var sb strings.Builder
	spawned := 0
	fmt.Fprintf(&sb, "Crawling %d URL(s):\n", len(args.URLs))
	for _, url := range args.URLs {
		// Backend-authored task text, not model-authored — the spawned
		// sub-agent's own two-step job (extract, then save) is fully
		// determined by this tool's own arguments, leaving it nothing to
		// decide beyond the mechanics of calling the two named tools.
		task := fmt.Sprintf(
			"Call extract_from_url with url=%q and fields=%s. Then take the resulting extracted values "+
				"(keyed by each field's own label) and call %s with %s=%q and values set to those extracted "+
				"values. Report back a short confirmation of what you saved.",
			url, string(fieldsJSON), args.SaveToolName, args.SaveToolArgsKey, args.SaveToolArgsByURL[url],
		)
		id, message := spawnSubAgent(httpClient, entry, apiKey, model, mainDB, conversationDB, callerScopes, userID, dataDir, corePort, conversationID, parentTurnRunID, task, allowedTools, depth)
		if id == "" {
			fmt.Fprintf(&sb, "- %s: NOT started — %s\n", url, message)
			continue
		}
		spawned++
		fmt.Fprintf(&sb, "- %s: sub-agent %s started\n", url, id)
	}
	fmt.Fprintf(&sb, "\n%d of %d sub-agent(s) started. Check on them with check_subagent/list_subagents once you need their results.", spawned, len(args.URLs))
	return sb.String()
}
