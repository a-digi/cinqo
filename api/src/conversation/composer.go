// composer.go — the orchestrator's "composer" mode (plan/ai/conversation/
// step-42-orchestrator-composer-mode.md): in a composer conversation the
// orchestrator never does domain work itself; it plans, and sub-agents
// do the work. This file enforces the capability restriction in code —
// what the composer orchestrator is offered and allowed to call;
// composer_plan.go holds its one tool, run_plan.
package conversation

import (
	"database/sql"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

// composerTools is everything the composer orchestrator is offered —
// deliberately NOT derived from the installed tools or the built-in
// sub-agent tools: no domain (MCP) tool, no spawn_subagent, no
// crawl_urls_with_subagents. Just run_plan (composer_plan.go): the
// orchestrator plans, workers do the work.
func composerTools() []chatcompleter.ToolDef {
	return []chatcompleter.ToolDef{runPlanToolDef}
}

// offerableToolsForMode is the orchestrator's tool set for a
// conversation's mode: a composer gets composerTools() and never
// touches the tool tables at all; every other mode is exactly the
// behavior before composer mode existed (offerableTools with the
// sub-agent tools included).
func offerableToolsForMode(mainDB *sql.DB, callerScopes []string, orchestrationMode string) ([]chatcompleter.ToolDef, error) {
	if orchestrationMode == conversation_entity.OrchestrationModeComposer {
		return composerTools(), nil
	}
	return offerableTools(mainDB, callerScopes, true)
}

// refusesUnofferedCall reports whether a tool call must be refused
// because the name is not in the set actually offered to this loop. It
// applies to (a) the composer orchestrator (depth 0 in composer mode)
// and (b) every sub-agent loop (depth > 0), so a worker's allowedTools
// and nesting-depth limit bind what it can CALL, not just what it was
// shown. The direct orchestrator (depth 0) is deliberately untouched:
// invokeToolCall's own checks remain its only gate, as before.
func refusesUnofferedCall(orchestrationMode string, depth int, name string, offered []chatcompleter.ToolDef) bool {
	if depth == 0 && orchestrationMode != conversation_entity.OrchestrationModeComposer {
		return false
	}
	for _, t := range offered {
		if t.Name == name {
			return false
		}
	}
	return true
}
