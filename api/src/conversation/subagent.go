// subagent.go — "Sub Agents": lets an orchestrator turn's own
// tool-calling loop delegate a self-contained sub-task to a separate,
// independent tool-calling loop, keeping the orchestrator's own
// context small (only a sub-agent's final report ever flows back —
// never its intermediate reasoning or tool calls). See
// plan/ai/conversation/step-41-sub-agents.md.
//
// Phase 2 (this file, superseding Phase 1's synchronous behavior):
// spawn_subagent returns immediately with an id instead of blocking
// the orchestrator's own tool-loop iteration — the sub-agent's own
// loop runs in a detached goroutine. check_subagent/list_subagents let
// the orchestrator come back for the result later, so it can spawn
// several sub-agents and let them run concurrently before checking
// any of them. The sub-agent's own progress is independently,
// visually observable the whole time via its own sub_agent_runs row
// (GetActiveTurnHandler's own subAgents[] field) regardless of which
// dispatch model is in play.
//
// Recursive sub-agents: a sub-agent MAY spawn its own sub-agents, up to
// maxSubAgentDepth levels below the orchestrator — depth is a plain
// runtime counter threaded through runToolLoop/invokeSubAgentCall/
// runSubAgentLoop, gating whether offerableTools includes the
// sub-agent tools at all for that level's own loop; a deeper level
// simply never sees spawn_subagent in its own tool list; there is no
// separate "reject the call" path to get wrong. Every sub-agent at
// every depth is still recorded flat, under the ORIGINAL orchestrator
// turn's own parent_turn_run_id (never its own immediate parent
// sub-agent) — deliberate, not an oversight: it keeps the existing
// data model and UI (one flat list per turn) working unchanged for
// arbitrary depth, rather than needing a real tree (a self-referencing
// parent column) for a feature whose whole point is a small, bounded
// depth in the first place.
//
// The reply returns early: runDetachedTurn (runner.go) no longer waits
// for a turn's own spawned sub-agents before marking it terminal — the
// orchestrator's own reply is persisted and visible the moment its own
// model call finishes, exactly when it finishes, regardless of
// whatever sub-agents are still working in the background. This is
// what makes that safe, and why it wasn't safe before this: every
// sub-agent gets its OWN independent root context
// (context.WithTimeout(context.Background(), maxTurnRunDuration) in
// invokeSubAgentCall, below) — NOT one derived from the parent turn's
// own ctx the way an earlier version of this file did. A derived
// context is cancelled the instant its parent is, so as long as a
// sub-agent's context descended from the orchestrator's own, finishing
// the orchestrator's turn early would have killed every sub-agent
// still running under it mid-task. With independent roots, the two
// lifetimes are genuinely unrelated: a sub-agent keeps running, is
// still visible via its own sub_agent_runs row, and still finishes (or
// times out) entirely on its own, long after its own parent turn
// already shows "completed". Deliberate consequence: stopping a turn
// (StopActiveTurnHandler) no longer stops its sub-agents "for free" via
// context propagation — CancelActiveTurn (runner.go) now explicitly
// cascades to every sub-agent a turn spawned instead, so "stop" still
// means "stop everything this turn started," turn included, regardless
// of whether the turn itself is still the one shown as "active".
package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// subAgentToolName/checkSubAgentToolName/listSubAgentsToolName are
// built-in pseudo-tools — never installed (no manifest.json, no
// subprocess), unlike every other entry offerableTools assembles.
// Appended there only when includeSubAgent is true.
const (
	subAgentToolName       = "spawn_subagent"
	checkSubAgentToolName  = "check_subagent"
	listSubAgentsToolName  = "list_subagents"
	cancelSubAgentToolName = "cancel_subagent"
)

// maxSubAgentDepth bounds recursive spawning: the orchestrator itself
// is depth 0 and can always spawn (depth 1); a depth-1 sub-agent can
// still spawn its own (depth 2); a depth-2 sub-agent cannot — it never
// sees spawn_subagent in its own offered tools at all. A plain
// constant, not configurable, matching this codebase's own "constants
// over premature configurability" convention — bounds the worst-case
// fan-out (and cost) of one careless top-level spawn tree without
// needing a separate per-conversation/per-turn budget concept.
const maxSubAgentDepth = 2

// maxSubAgentsPerTurn caps the TOTAL number of sub-agents ever spawned
// under one top-level orchestrator turn — every one spawned across the
// WHOLE tree (any depth), counted for the lifetime of the turn, not
// just currently-running ones (see
// SubAgentRunQueryRepo.CountByParentTurnRunID's own doc comment for
// why: a spawn-wait-spawn-again cycle must not be able to bypass this
// by never having more than one in flight at once). maxSubAgentDepth,
// above, bounds recursion depth; this bounds breadth/fan-out at any
// single depth — the two are independent failure modes (a model could
// spawn 500 sub-agents all at depth 1 without ever going deeper), so
// depth alone doesn't cap total cost. A plain constant, matching this
// codebase's own "constants over premature configurability"
// convention — generous enough for genuine parallel fan-out, small
// enough to bound one turn's worst-case token cost.
const maxSubAgentsPerTurn = 10

// maxConcurrentSubAgents caps how many sub-agent loops may be
// EXECUTING at once — across every conversation and every turn on this
// whole running instance, at every nesting depth alike. A materially
// different resource from maxSubAgentsPerTurn's own per-turn LIFETIME
// count above: that one bounds one turn's own total cost over time;
// this one bounds the host's own concurrent capacity right now
// (goroutines, outbound LLM/tool calls all in flight simultaneously) —
// a turn that spawns 10 sub-agents one at a time, waiting for each to
// finish before spawning the next, never worries this limit at all,
// while 10 turns each spawning one sub-agent at the exact same moment
// would. See subAgentSlots, below, for how this is tracked and
// enforced.
const maxConcurrentSubAgents = 10

// subAgentSlotsMu/runningSubAgents track how many sub-agent loops are
// executing RIGHT NOW, process-wide — a plain counter, not a map,
// since nothing here ever needs to look up or act on one specific
// entry by id, only "how many, total, at this instant." Same
// package-level, mutex-protected registry idiom this codebase already
// establishes for exactly this kind of long-lived, cross-request
// state with no natural DI home — see runner.go's own `active` map and
// tool/manager.go's own process registry, both the same shape for a
// different resource.
var (
	subAgentSlotsMu  sync.Mutex
	runningSubAgents int
)

// tryAcquireSubAgentSlot reports whether a slot was available and, if
// so, claims it (increments runningSubAgents) atomically with the
// check — never blocks and never queues: a spawn that can't get a slot
// right now is refused outright (see invokeSubAgentCall, below), the
// same "no separate reject-later path" shape maxSubAgentsPerTurn's own
// check already established. Every successful acquire MUST be matched
// by exactly one releaseSubAgentSlot call once that sub-agent's own
// loop actually finishes, however it finishes.
func tryAcquireSubAgentSlot() bool {
	subAgentSlotsMu.Lock()
	defer subAgentSlotsMu.Unlock()
	if runningSubAgents >= maxConcurrentSubAgents {
		return false
	}
	runningSubAgents++
	return true
}

func releaseSubAgentSlot() {
	subAgentSlotsMu.Lock()
	defer subAgentSlotsMu.Unlock()
	runningSubAgents--
}

// subAgentCancelMu/subAgentCancelFuncs — one cancel func per currently-
// running sub-agent, keyed by its own sub_agent_runs.id. Same
// package-level, mutex-protected registry idiom as runner.go's own
// `active` map (turn_runs.id -> cancel func) and CancelActiveTurn,
// just one level down: every sub-agent gets its OWN context derived
// from whatever context it was started under (context.WithCancel), so
// cancel_subagent can stop exactly one sub-agent without touching its
// siblings or the parent turn — cancelling the parent turn still
// cancels every sub-agent transitively (a derived context always
// observes its parent's own cancellation), so CancelActiveTurn's
// existing behavior is unchanged.
var (
	subAgentCancelMu    sync.Mutex
	subAgentCancelFuncs = map[string]context.CancelFunc{}
)

// CancelSubAgent requests cancellation of one specific sub-agent —
// the direct analog of CancelActiveTurn (runner.go), one level down.
// Returns false if id isn't currently tracked (already finished, or
// never existed), the same "nothing left to do" contract
// CancelActiveTurn's own doc comment establishes. Asynchronous, same
// caveat as CancelActiveTurn: this returning true means the signal was
// sent, not that the sub-agent has actually stopped yet.
func CancelSubAgent(id string) bool {
	subAgentCancelMu.Lock()
	cancel, ok := subAgentCancelFuncs[id]
	subAgentCancelMu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

var subAgentToolDef = chatcompleter.ToolDef{
	Name:        subAgentToolName,
	Description: "Delegate a self-contained sub-task to a separate sub-agent that runs its own independent tool-calling loop in the background and reports back only its final result. Returns immediately with the sub-agent's own id — it does NOT wait for the sub-agent to finish. Use this to keep your own context small and to run several sub-tasks concurrently: spawn as many as you need, keep doing other work (or spawn more), then call check_subagent (or list_subagents) later to collect each result. The sub-agent starts with NO memory of this conversation. A sub-agent may itself spawn further sub-agents, up to a small fixed nesting limit — beyond that limit, this tool simply won't be offered to it. There is also a fixed limit on the total number of sub-agents one turn may spawn in total, and a separate fixed limit on how many may be running at the same time system-wide — once either is reached, further spawn attempts will be refused until one finishes.",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "A clear, self-contained description of the sub-task to delegate. The sub-agent has no memory of this conversation — include everything it needs to know to complete the task on its own."
			},
			"allowedTools": {
				"type": "array",
				"items": { "type": "string" },
				"description": "Optional. Restrict the sub-agent to only these tool names (e.g. only tools that read data, not ones that change anything), for a task where you want to bound what it's able to do. Omit to give it the same full set of tools you have. Naming a tool you don't have access to has no effect — this can only narrow what the sub-agent can do, never widen it."
			}
		},
		"required": ["task"]
	}`),
}

var checkSubAgentToolDef = chatcompleter.ToolDef{
	Name:        checkSubAgentToolName,
	Description: "Check on a sub-agent you previously started with spawn_subagent. Returns its final result if it has completed, a plain statement that it's still running (call back later) if not, or the failure reason if it failed or was cancelled.",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {
				"type": "string",
				"description": "The sub-agent id returned by spawn_subagent."
			}
		},
		"required": ["id"]
	}`),
}

var listSubAgentsToolDef = chatcompleter.ToolDef{
	Name:        listSubAgentsToolName,
	Description: "List every sub-agent you have started so far in this turn, with its id, current status, and task. Use this if you've lost track of which sub-agent ids are still outstanding.",
	InputSchema: json.RawMessage(`{"type": "object", "properties": {}}`),
}

var cancelSubAgentToolDef = chatcompleter.ToolDef{
	Name:        cancelSubAgentToolName,
	Description: "Cancel a sub-agent you no longer need — e.g. you spawned several in parallel and one has already given you enough information, or a task turned out to be unnecessary. Frees its slot immediately for another sub-agent to use (there is a limit on how many may run at the same time). Has no effect on a sub-agent that has already finished (completed/failed/cancelled) or on an unknown id.",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {
				"type": "string",
				"description": "The sub-agent id returned by spawn_subagent."
			}
		},
		"required": ["id"]
	}`),
}

type spawnSubAgentArgs struct {
	Task string `json:"task"`
	// AllowedTools optionally restricts which tools the sub-agent may
	// call, by name. Names not already present in the orchestrator's
	// own offered tool set are silently ignored, never granted — see
	// filterToolsByName's own doc comment for why this can only ever
	// narrow access, never widen it. nil/empty means "the same full
	// tool set the orchestrator itself has" — this feature's original
	// default behavior, unchanged for any caller that doesn't set this.
	AllowedTools []string `json:"allowedTools,omitempty"`
}

type checkSubAgentArgs struct {
	ID string `json:"id"`
}

// invokeSubAgentCall starts a sub-agent's own independent tool-calling
// loop in a detached goroutine and returns immediately — it never
// blocks the orchestrator's own tool-loop iteration waiting for the
// sub-agent to finish. Never returns a bare error — matches
// invokeToolCall's own "never leave a tool_call unanswered" convention.
func invokeSubAgentCall(
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
	var args spawnSubAgentArgs
	if err := json.Unmarshal(call.Arguments, &args); err != nil || args.Task == "" {
		return "task is required"
	}

	// maxSubAgentsPerTurn — checked here, not as a static offered-tools
	// decision like maxSubAgentDepth: the count can only be known at
	// call time (it changes with every spawn across the whole tree,
	// including ones this exact model call can't see coming), so this
	// is a real per-call rejection, not merely "the tool was never
	// offered." parentTurnRunID is the ORIGINAL orchestrator turn at
	// every depth (see this file's own "Recursive sub-agents" doc
	// comment), so this count is already correctly whole-tree, not
	// just this level's own children.
	existing, err := conversation_query.NewSubAgentRunQueryRepo(conversationDB).CountByParentTurnRunID(parentTurnRunID)
	if err != nil {
		return fmt.Sprintf("failed to check sub-agent limit: %v", err)
	}
	if existing >= maxSubAgentsPerTurn {
		return fmt.Sprintf("Cannot start a new sub-agent: this turn has already reached its limit of %d sub-agents.", maxSubAgentsPerTurn)
	}

	_, message := spawnSubAgent(httpClient, entry, apiKey, model, mainDB, conversationDB, callerScopes, userID, dataDir, corePort, conversationID, parentTurnRunID, args.Task, args.AllowedTools, depth)
	return message
}

// spawnSubAgent claims a concurrency slot, inserts the sub_agent_runs
// row, and launches this sub-agent's own detached, independently
// rooted execution loop — the actual spawn mechanics (slot accounting,
// run bookkeeping, context lifetime) shared by every caller that ever
// starts a sub-agent: invokeSubAgentCall above (one spawn_subagent
// call, one sub-agent) and invokeCrawlURLsWithSubAgentsCall (crawl_
// fanout.go — many, one per URL, from a single tool call) alike.
// Returns "" for id (never a valid uuid) when the slot couldn't be
// claimed or the run couldn't be recorded — callers distinguish
// "started" from "refused" by checking id, never by parsing message.
func spawnSubAgent(
	httpClient *http.Client,
	entry platform.Entry,
	apiKey, model string,
	mainDB, conversationDB *sql.DB,
	callerScopes []string,
	userID string,
	dataDir string,
	corePort int,
	conversationID, parentTurnRunID string,
	task string,
	allowedTools []string,
	depth int,
) (id string, message string) {
	// maxConcurrentSubAgents — acquired here, right before this
	// sub-agent would actually start executing, and released (below/in
	// the goroutine) the moment it stops, however it stops.
	if !tryAcquireSubAgentSlot() {
		return "", fmt.Sprintf(
			"Cannot start a new sub-agent right now: the maximum of %d sub-agents running at the same time has been reached. Try again shortly, or check on ones already running with check_subagent/list_subagents.",
			maxConcurrentSubAgents,
		)
	}

	runs := conversation_persistent.NewSubAgentRunPersistentRepo(conversationDB)
	run := &conversation_entity.SubAgentRun{
		ID:              uuid.NewString(),
		ParentTurnRunID: parentTurnRunID,
		ConversationID:  conversationID,
		Task:            task,
		StartedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if err := runs.Insert(run); err != nil {
		releaseSubAgentSlot() // never actually started — free the slot immediately, not just on the goroutine's own exit
		return "", fmt.Sprintf("failed to start sub-agent: %v", err)
	}

	// subCtx/cancel — this sub-agent's OWN INDEPENDENT root context, not
	// derived from the orchestrator's own ctx (which this function no
	// longer even receives) — see this file's own "the reply returns
	// early" doc comment for why that independence is exactly what
	// makes it safe for the orchestrator's own turn to finish without
	// waiting for this sub-agent. Same maxTurnRunDuration ceiling a
	// top-level turn itself gets (runner.go), for the same reasoning:
	// bounded by maxToolIterations x a worst-case per-iteration cost.
	// Registered by id so cancel_subagent — or CancelActiveTurn's own
	// explicit cascade — can still stop exactly this one sub-agent.
	subCtx, cancel := context.WithTimeout(context.Background(), maxTurnRunDuration)
	subAgentCancelMu.Lock()
	subAgentCancelFuncs[run.ID] = cancel
	subAgentCancelMu.Unlock()

	go func() {
		defer releaseSubAgentSlot()
		defer func() {
			subAgentCancelMu.Lock()
			delete(subAgentCancelFuncs, run.ID)
			subAgentCancelMu.Unlock()
			cancel() // release this context's own resources even on the success path
		}()
		runSubAgentLoop(subCtx, httpClient, entry, apiKey, model, mainDB, conversationDB, callerScopes, userID, dataDir, corePort, conversationID, run.ID, task, allowedTools, depth+1, parentTurnRunID)
	}()

	return run.ID, fmt.Sprintf(
		"Sub-agent %s started in the background for task: %q. It is running concurrently — continue with other work (or spawn more sub-agents), then call check_subagent with id=%q once you need its result.",
		run.ID, task, run.ID,
	)
}

// filterToolsByName narrows tools down to only the named entries, when
// allowed is non-empty — a name in allowed that isn't present in tools
// is silently dropped, never added: this can only ever REMOVE tools
// the caller (offerableTools' own scope-filtered result) already had,
// never grant one it didn't. An empty allowed list is "no restriction
// requested" and returns tools unchanged, not "allow nothing" — an
// empty spawn_subagent.allowedTools array is indistinguishable from
// omitting the field entirely (both unmarshal to a nil/empty slice),
// which is the intended, safer default.
func filterToolsByName(tools []chatcompleter.ToolDef, allowed []string) []chatcompleter.ToolDef {
	if len(allowed) == 0 {
		return tools
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	out := make([]chatcompleter.ToolDef, 0, len(tools))
	for _, t := range tools {
		if _, ok := allowedSet[t.Name]; ok {
			out = append(out, t)
		}
	}
	return out
}

// runSubAgentLoop is the sub-agent's own execution body, run detached
// in its own goroutine by invokeSubAgentCall. ctx is THIS sub-agent's
// OWN independent root context (invokeSubAgentCall's own subCtx) —
// never the orchestrator's, and never any ancestor sub-agent's either
// — see this file's own "the reply returns early" doc comment for why.
func runSubAgentLoop(
	ctx context.Context,
	httpClient *http.Client,
	entry platform.Entry,
	apiKey, model string,
	mainDB, conversationDB *sql.DB,
	callerScopes []string,
	userID string,
	dataDir string,
	corePort int,
	conversationID, runID, task string,
	allowedTools []string,
	depth int,
	// parentTurnRunID is passed straight through from
	// invokeSubAgentCall's own — every sub-agent at every depth is
	// recorded flat under the ORIGINAL orchestrator turn (see this
	// file's own "Recursive sub-agents" doc comment).
	parentTurnRunID string,
) {
	runs := conversation_persistent.NewSubAgentRunPersistentRepo(conversationDB)
	logStep := func(step string) {
		line := fmt.Sprintf("%s\t%s\n", time.Now().UTC().Format(time.RFC3339), step)
		_ = runs.AppendLog(runID, line)
	}
	reportUsage := func(promptTokens, completionTokens int) {
		_ = runs.AddTokenUsage(runID, promptTokens, completionTokens)
	}

	// The same scope-filtered tool set the orchestrator itself was
	// offered — includeSubAgent is now depth-gated, not always false:
	// this sub-agent may itself spawn further sub-agents as long as
	// it's still under maxSubAgentDepth, so recursion bottoms out by
	// simply never offering the tool at all past that point, never by
	// rejecting a call. Never more privilege than the parent regardless
	// (offerableTools' own scope filter still applies identically).
	// filterToolsByName then optionally narrows it further, per this
	// specific spawn_subagent call's own allowedTools argument.
	tools, err := offerableTools(mainDB, callerScopes, depth < maxSubAgentDepth)
	if err != nil {
		finishSubAgent(runs, runID, "failed", nil)
		return
	}
	tools = filterToolsByName(tools, allowedTools)

	messages := []chatcompleter.Message{{Role: "user", Content: task}}

	result, err := runToolLoop(ctx, httpClient, entry, apiKey, model, messages, tools, mainDB, callerScopes, userID, dataDir, corePort, conversationID, conversationDB, parentTurnRunID, depth, logStep, reportUsage, nil)
	if err != nil {
		status := "failed"
		if errors.Is(ctx.Err(), context.Canceled) {
			status = "cancelled"
		}
		finishSubAgent(runs, runID, status, nil)
		return
	}

	finishSubAgent(runs, runID, "completed", &result)
}

// checkSubAgentCall looks up one sub-agent by id — any id, not just
// ones belonging to the calling turn, matching this feature's own
// established "no per-user/per-turn data scoping inside a single
// conversation" convention (the same trust level as every other tool
// argument already flowing through invokeToolCall).
func checkSubAgentCall(conversationDB *sql.DB, call chatcompleter.ToolCall) string {
	var args checkSubAgentArgs
	if err := json.Unmarshal(call.Arguments, &args); err != nil || args.ID == "" {
		return "id is required"
	}

	run, err := conversation_query.NewSubAgentRunQueryRepo(conversationDB).FindByID(args.ID)
	if err != nil {
		return "unknown sub-agent id"
	}

	switch run.Status {
	case "running":
		return fmt.Sprintf("Sub-agent %s is still running. Check back later.", run.ID)
	case "completed":
		if run.Result != nil {
			return *run.Result
		}
		return ""
	default: // "failed" / "cancelled"
		return fmt.Sprintf("Sub-agent %s %s.", run.ID, run.Status)
	}
}

// cancelSubAgentCall requests cancellation of one specific sub-agent —
// any id, same "no per-user/per-turn data scoping" trust level as
// checkSubAgentCall above. Cancellation is asynchronous (same caveat
// as StopActiveTurnHandler's own): this confirms the signal was sent,
// not that the sub-agent has actually stopped by the time this
// returns — a later check_subagent call will show its real terminal
// status once it settles.
func cancelSubAgentCall(conversationDB *sql.DB, call chatcompleter.ToolCall) string {
	var args checkSubAgentArgs
	if err := json.Unmarshal(call.Arguments, &args); err != nil || args.ID == "" {
		return "id is required"
	}

	if !CancelSubAgent(args.ID) {
		return fmt.Sprintf("Sub-agent %s is not currently running (already finished, or unknown id) — nothing to cancel.", args.ID)
	}
	_ = conversation_persistent.NewSubAgentRunPersistentRepo(conversationDB).SetCancelRequested(args.ID)
	return fmt.Sprintf("Cancellation requested for sub-agent %s.", args.ID)
}

// listSubAgentsCall summarizes every sub-agent the CURRENT parent turn
// has spawned — unlike checkSubAgentCall, scoped to parentTurnRunID,
// since "everything I've started so far" is inherently a per-turn
// question.
func listSubAgentsCall(conversationDB *sql.DB, parentTurnRunID string) string {
	runs, err := conversation_query.NewSubAgentRunQueryRepo(conversationDB).FindByParentTurnRunID(parentTurnRunID)
	if err != nil {
		return fmt.Sprintf("failed to list sub-agents: %v", err)
	}
	if len(runs) == 0 {
		return "No sub-agents have been started yet."
	}

	var sb strings.Builder
	for _, r := range runs {
		fmt.Fprintf(&sb, "%s: %s — %s\n", r.ID, r.Status, r.Task)
	}
	return sb.String()
}

func finishSubAgent(runs *conversation_persistent.SubAgentRunPersistentRepo, id, status string, result *string) {
	_ = runs.SetTerminalStatus(id, status, result, time.Now().UTC().Format(time.RFC3339))
}

// ReconcileOrphanedSubAgentRuns mirrors ReconcileOrphanedTurnRuns
// (runner.go) exactly, for the same reason: a sub-agent run is a
// goroutine, not a subprocess, so it has no PID to reattach to after a
// server restart — every row still "running" at boot is, by
// definition, dead. Simpler than its turn_runs counterpart: a
// sub-agent's own "result" lives entirely in this row (never a
// conversation's own Markdown log), so there is no matching
// failed-turn append to also perform here. Called once at boot,
// alongside ReconcileOrphanedTurnRuns. See cinqo.Start.
func ReconcileOrphanedSubAgentRuns(conversationDB *sql.DB, warn func(format string, args ...any)) {
	runningRuns, err := conversation_query.NewSubAgentRunQueryRepo(conversationDB).FindAllRunning()
	if err != nil {
		warn("conversation: failed to list orphaned sub-agent runs at boot: %v", err)
		return
	}
	if len(runningRuns) == 0 {
		return
	}

	runs := conversation_persistent.NewSubAgentRunPersistentRepo(conversationDB)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, run := range runningRuns {
		if err := runs.SetTerminalStatus(run.ID, "failed", nil, now); err != nil {
			warn("conversation: failed to mark orphaned sub-agent run %q failed: %v", run.ID, err)
		}
	}
}
