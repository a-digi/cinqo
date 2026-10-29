// delegation.go — tool-decided sub-agents: an installed MCP tool (one
// whose manifest declared `delegates`) can return a delegation
// directive in its result's structuredContent, and the host — here, in
// code, inside the tool-calling loop — spawns the sub-agents itself, so
// the orchestrator model never has to choose to. Each task goes
// through the same spawnSubAgent every model-initiated spawn uses, so
// the per-turn, concurrency and depth limits and the acceptance loop
// all apply unchanged. See tool/mcp/invoke.go's Delegate.
package conversation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/a-digi/cinqo/src/platform"
	tool_mcp "github.com/a-digi/cinqo/src/tool/mcp"

	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// maxDelegatedTaskLength bounds one tool-supplied task string.
const maxDelegatedTaskLength = maxMessageContentLength

// pendingDelegation is a directive from a tool that opted in via its
// manifest, together with the manifest-declared ceiling on what its
// sub-agents may be granted.
type pendingDelegation struct {
	ToolName     string
	AllowedTools []string
	Directive    *tool_mcp.Delegate
}

// parseDelegateAllowedTools decodes the tool_mcp_tools column; any
// malformed value yields nil, which grants nothing.
func parseDelegateAllowedTools(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func isBuiltinSubAgentTool(name string) bool {
	switch name {
	case subAgentToolName, checkSubAgentToolName, listSubAgentsToolName, cancelSubAgentToolName, crawlURLsWithSubagentsToolName:
		return true
	}
	return false
}

// effectiveDelegateTools returns the tool names one delegated task's
// sub-agent may use: the manifest ceiling, narrowed (never widened) by
// the task's own list, never including a built-in sub-agent tool. The
// result is empty only when nothing is grantable — callers must refuse
// then, since an empty list to spawnSubAgent would mean "no
// restriction".
func effectiveDelegateTools(ceiling, requested []string) []string {
	allowed := make(map[string]struct{}, len(ceiling))
	for _, name := range ceiling {
		if !isBuiltinSubAgentTool(name) {
			allowed[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(allowed))
	if len(requested) == 0 {
		for _, name := range ceiling {
			if _, ok := allowed[name]; ok {
				out = append(out, name)
			}
		}
		return out
	}
	for _, name := range requested {
		if _, ok := allowed[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

// executeDelegation spawns every task in p's directive and returns text
// to append to the tool's own result so the model knows what was
// started. A batch that would exceed maxSubAgentsPerTurn is refused
// whole, matching crawl_urls_with_subagents; a directive arriving where
// sub-agents can't be spawned (depth limit) is refused with a note.
func executeDelegation(
	httpClient *http.Client,
	entry platform.Entry,
	apiKey, model string,
	mainDB, conversationDB *sql.DB,
	callerScopes []string,
	userID string,
	dataDir string,
	corePort int,
	conversationID, parentTurnRunID string,
	p *pendingDelegation,
	depth int,
) string {
	tasks := p.Directive.Tasks
	if depth >= maxSubAgentDepth {
		return "\n\n[Delegation ignored: the maximum sub-agent nesting depth has been reached.]"
	}
	existing, err := conversation_query.NewSubAgentRunQueryRepo(conversationDB).CountByParentTurnRunID(parentTurnRunID)
	if err != nil {
		return fmt.Sprintf("\n\n[Delegation failed: could not check the sub-agent limit: %v]", err)
	}
	if existing+len(tasks) > maxSubAgentsPerTurn {
		return fmt.Sprintf("\n\n[Delegation refused: starting %d sub-agents would exceed this turn's limit of %d (%d already started).]", len(tasks), maxSubAgentsPerTurn, existing)
	}

	var b strings.Builder
	b.WriteString("\n\n")
	if msg := strings.TrimSpace(p.Directive.Message); msg != "" {
		b.WriteString(truncateToolResult(msg))
		b.WriteString("\n")
	}
	started := 0
	for i, t := range tasks {
		task := strings.TrimSpace(t.Task)
		if task == "" {
			fmt.Fprintf(&b, "Task %d refused: empty task.\n", i+1)
			continue
		}
		if len(task) > maxDelegatedTaskLength {
			task = task[:maxDelegatedTaskLength]
		}
		tools := effectiveDelegateTools(p.AllowedTools, t.AllowedTools)
		if len(tools) == 0 {
			fmt.Fprintf(&b, "Task %d refused: none of its requested tools may be granted.\n", i+1)
			continue
		}
		id, message := spawnSubAgent(httpClient, entry, apiKey, model, mainDB, conversationDB, callerScopes, userID, dataDir, corePort, conversationID, parentTurnRunID, task, tools, newAcceptanceSpec(t.AcceptanceCriteria, t.MaxAttempts), p.ToolName, depth)
		if id == "" {
			fmt.Fprintf(&b, "Task %d not started: %s\n", i+1, message)
			continue
		}
		started++
		fmt.Fprintf(&b, "Sub-agent %s started for task %d.\n", id, i+1)
	}
	if started > 0 {
		b.WriteString("These sub-agents were started automatically by the tool and run concurrently; collect each result with check_subagent (or list_subagents) once you need it. Do not spawn duplicates.")
	}
	return strings.TrimRight(b.String(), "\n")
}
