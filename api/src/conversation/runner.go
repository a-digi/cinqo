// runner.go detaches a turn's own tool-calling loop from the HTTP
// request that started it — see
// plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
// Package-level, mutex-protected state (a map of cancel funcs, no
// struct instance threaded through DI) — same idiom already
// established in this codebase by tool/manager.go's own process
// registry, since this feature's DI container
// (api/config/di.ContextBag, a flat map[string]interface{}) has no
// mechanism for a long-lived service with its own lifecycle.
package conversation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	platform_crypto "github.com/a-digi/cinqo/src/platform/crypto"
	platform_query "github.com/a-digi/cinqo/src/platform/repository/query"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// maxTurnRunDuration bounds one detached turn end-to-end — the new
// outer ceiling that replaces "however long the client stays
// connected" (step-12-remove-send-timeout.md's own conclusion,
// superseded here — see step 23's own doc). Sized against
// maxToolIterations (15) x a worst-case per-iteration cost (one LLM
// call + tool_mcp.Invoke's own 45s invokeTimeout), already >11
// minutes worst case — 15 minutes leaves real margin without being
// unbounded.
const maxTurnRunDuration = 15 * time.Minute

// ErrTurnAlreadyRunning fires when a turn is already running for this
// conversation — the turn_runs_one_running_idx partial unique index is
// the actual, race-free enforcement; this just gives StartTurnRun's
// caller a recognizable error to map to 409.
var ErrTurnAlreadyRunning = errors.New("conversation: a turn is already running for this conversation")

var (
	runnerMu sync.Mutex
	active   = map[string]context.CancelFunc{} // keyed by turn_runs.id
)

// StartTurnRun inserts a new turn_runs row (status "running") and
// launches its own tool-calling loop in a detached goroutine — rooted
// in context.Background(), NOT the caller's own request context, so
// the loop keeps running after the HTTP handler that started it
// returns a response, and even if the client that made that request
// disconnects entirely. Returns as soon as the row is inserted and the
// goroutine is launched — it never waits for the loop itself to do
// anything.
func StartTurnRun(
	httpClient *http.Client,
	mainDB *sql.DB,
	conversationDB *sql.DB,
	encryptionKey []byte,
	conversationID, content string,
	callerScopes []string,
	dataDir string,
	corePort int,
) (*conversation_entity.TurnRun, error) {
	if content == "" {
		return nil, ErrEmptyContent
	}
	if len(content) > maxMessageContentLength {
		return nil, ErrContentTooLong
	}

	// Fail fast on an unknown conversation before ever inserting a
	// turn_runs row for it — mirrors SendMessage's own first lookup.
	if _, err := conversation_query.NewConversationQueryRepo(conversationDB).FindByID(conversationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, fmt.Errorf("conversation: look up conversation: %w", err)
	}

	run := &conversation_entity.TurnRun{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		UserContent:    content,
		Status:         "running",
		StartedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	if err := conversation_persistent.NewTurnRunPersistentRepo(conversationDB).Insert(run); err != nil {
		if isUniqueConstraintErr(err) {
			return nil, ErrTurnAlreadyRunning
		}
		return nil, fmt.Errorf("conversation: start turn run: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), maxTurnRunDuration)
	runnerMu.Lock()
	active[run.ID] = cancel
	runnerMu.Unlock()

	go runDetachedTurn(ctx, run.ID, httpClient, mainDB, conversationDB, encryptionKey, conversationID, content, callerScopes, dataDir, corePort)

	return run, nil
}

// runDetachedTurn is today's SendMessage body, adapted to run off a
// detached context and to persist progress/terminal state into
// turn_runs as it goes, instead of only returning a value once
// everything is done. Always deregisters its own cancel func on the
// way out (every exit path), and always calls cancel() itself to
// release the context's own resources even on the success path (the
// timeout's own eventual firing would do the same, but there's no
// reason to wait for it).
func runDetachedTurn(
	ctx context.Context,
	turnRunID string,
	httpClient *http.Client,
	mainDB *sql.DB,
	conversationDB *sql.DB,
	encryptionKey []byte,
	conversationID, content string,
	callerScopes []string,
	dataDir string,
	corePort int,
) {
	defer func() {
		runnerMu.Lock()
		if cancel, ok := active[turnRunID]; ok {
			cancel()
			delete(active, turnRunID)
		}
		runnerMu.Unlock()
	}()

	runs := conversation_persistent.NewTurnRunPersistentRepo(conversationDB)
	logStep := func(step string) {
		line := fmt.Sprintf("%s\t%s\n", time.Now().UTC().Format(time.RFC3339), step)
		_ = runs.AppendLog(turnRunID, line)
	}

	conv, err := conversation_query.NewConversationQueryRepo(conversationDB).FindByID(conversationID)
	if err != nil {
		finishTurnRun(runs, turnRunID, "failed")
		return
	}

	entry, ok := platform.Lookup(conv.PlatformID)
	if !ok {
		finishTurnRun(runs, turnRunID, "failed")
		return
	}
	if entry.Completer == nil {
		finishTurnRun(runs, turnRunID, "failed")
		return
	}
	model := conv.Model

	key, err := platform_query.NewKeyQueryRepo(mainDB).FindMostRecentForPlatform(conv.PlatformID)
	if err != nil {
		finishTurnRun(runs, turnRunID, "failed")
		return
	}
	plainKey, err := platform_crypto.Decrypt(key.EncryptedKey, encryptionKey)
	if err != nil {
		finishTurnRun(runs, turnRunID, "failed")
		return
	}

	recent, err := ReadRecentTurns(conv.FilePath, ContextTurns)
	if err != nil {
		finishTurnRun(runs, turnRunID, "failed")
		return
	}

	messages := make([]chatcompleter.Message, 0, len(recent)*2+1)
	for _, t := range recent {
		if t.Failed {
			continue
		}
		messages = append(messages,
			chatcompleter.Message{Role: "user", Content: t.UserContent},
			chatcompleter.Message{Role: "assistant", Content: t.AssistantContent},
		)
	}
	userTimestamp := time.Now().UTC().Format(time.RFC3339)
	messages = append(messages, chatcompleter.Message{Role: "user", Content: content})

	tools, err := offerableTools(mainDB, callerScopes)
	if err != nil {
		finishTurnRun(runs, turnRunID, "failed")
		return
	}

	// totalPromptTokens/totalCompletionTokens (step 35) accumulate this
	// turn's own real usage across its whole tool-calling loop, in
	// addition to reportUsage's own existing live write into turn_runs
	// (step 34) — reused, not duplicated: this is the same callback,
	// just also summing locally so the Markdown-log Turn appended below
	// (success or failure) can carry the same total that's already
	// live in turn_runs by the time either AppendTurn call happens. See
	// plan/ai/conversation/step-35-persist-per-turn-token-usage.md.
	var totalPromptTokens, totalCompletionTokens int
	reportUsage := func(promptTokens, completionTokens int) {
		totalPromptTokens += promptTokens
		totalCompletionTokens += completionTokens
		_ = runs.AddTokenUsage(turnRunID, promptTokens, completionTokens)
	}
	// logExchange (step 36) writes this turn's own full raw request/
	// response trace, one .txt file per turn, one appended block per
	// iteration — a diagnostic aid for understanding excessive token
	// usage, distinct from reportUsage's own aggregate-numbers-only
	// tracking above. filepath.Dir(conv.FilePath) is the conversations/
	// directory itself (conv.FilePath is
	// <conversationsDir>/<id>_conversation.md) — TraceLogPath then adds
	// the new <id>/<turnRunID>.txt layout underneath it. Best-effort:
	// a trace-log write failure must never fail the turn itself.
	//
	// Gated by conversation_settings.ai_trace_logs_enabled (step 37) —
	// checked ONCE per turn here, not once per iteration, since the
	// setting doesn't need mid-turn-granularity live-toggling and a
	// turn can run up to maxToolIterations model calls. Off (the
	// default): logExchange stays nil, and runToolLoop's own existing
	// nil-guard means literally nothing is written — not merely
	// hidden. A settings-load failure is treated the same as "off,"
	// never as a reason to fail the turn.
	var logExchange func(iteration int, msgs []chatcompleter.Message, toolDefs []chatcompleter.ToolDef, result *chatcompleter.ChatCompletionResult, callErr error)
	if settings, settingsErr := conversation_query.NewSettingsQueryRepo(conversationDB).Load(); settingsErr == nil && settings.AITraceLogsEnabled {
		tracePath := TraceLogPath(filepath.Dir(conv.FilePath), conversationID, turnRunID)
		logExchange = func(iteration int, msgs []chatcompleter.Message, toolDefs []chatcompleter.ToolDef, result *chatcompleter.ChatCompletionResult, callErr error) {
			_ = appendTraceEntry(tracePath, iteration, msgs, toolDefs, result, callErr)
		}
	}
	assistantContent, err := runToolLoop(ctx, httpClient, entry, plainKey, model, messages, tools, mainDB, callerScopes, dataDir, corePort, logStep, reportUsage, logExchange)
	if err != nil {
		failedTurn := Turn{
			UserTimestamp:    userTimestamp,
			UserContent:      content,
			Failed:           true,
			ErrorTimestamp:   time.Now().UTC().Format(time.RFC3339),
			ErrorMessage:     truncateError(errorMessageFor(ctx, err)),
			PromptTokens:     totalPromptTokens,
			CompletionTokens: totalCompletionTokens,
		}
		if appendErr := AppendTurn(conv.FilePath, failedTurn); appendErr != nil {
			finishTurnRun(runs, turnRunID, "failed")
			return
		}
		status := "failed"
		if errors.Is(ctx.Err(), context.Canceled) {
			status = "cancelled"
		}
		finishTurnRun(runs, turnRunID, status)
		return
	}

	turn := Turn{
		UserTimestamp:      userTimestamp,
		UserContent:        content,
		AssistantTimestamp: time.Now().UTC().Format(time.RFC3339),
		AssistantContent:   assistantContent,
		PromptTokens:       totalPromptTokens,
		CompletionTokens:   totalCompletionTokens,
	}
	if err := AppendTurn(conv.FilePath, turn); err != nil {
		finishTurnRun(runs, turnRunID, "failed")
		return
	}

	finishTurnRun(runs, turnRunID, "completed")
}

// CancelActiveTurn requests cancellation of turnRunID's own detached
// goroutine — the direct analog of tool/manager.go's own Stop, adapted
// for an in-process goroutine (no PID/signal involved) rather than a
// supervised OS subprocess. Returns false if turnRunID isn't currently
// tracked (already finished, or belongs to a run this process never
// started — e.g. a restart already reconciled it away), in which case
// the caller has nothing left to do. Cancellation is asynchronous: the
// goroutine only observes ctx.Done() at its next context-aware
// checkpoint (the in-flight LLM HTTP call or MCP tool invocation
// returning) — this returning true means "the stop signal was sent,"
// not "the run has stopped." See
// plan/ai/conversation/step-25-cancel-in-progress-turn.md.
func CancelActiveTurn(turnRunID string) bool {
	runnerMu.Lock()
	cancel, ok := active[turnRunID]
	runnerMu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// errorMessageFor prefers a recognizable "cancelled" message over the
// raw context.Canceled error text a cancelled HTTP/tool call would
// otherwise surface as (e.g. "context canceled" bubbling up from deep
// inside an HTTP client).
func errorMessageFor(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.Canceled) {
		return "Cancelled"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("Turn exceeded the maximum allowed duration (%s)", maxTurnRunDuration)
	}
	return err.Error()
}

// finishTurnRun marks a run terminal. Matches this codebase's own
// established convention (send_message_handler.go's own comment on
// this exact point) of domain functions just returning/recording
// outcomes, never touching a logger themselves — the real failure
// detail, when there is one, already went into the conversation's own
// Markdown log via AppendTurn's "## error —" block before this is
// called; this only ever records the coarse run status.
func finishTurnRun(runs *conversation_persistent.TurnRunPersistentRepo, turnRunID, status string) {
	_ = runs.SetTerminalStatus(turnRunID, status, time.Now().UTC().Format(time.RFC3339))
}

// isUniqueConstraintErr recognizes the sqlite3 driver's own unique
// constraint violation — the only place in this codebase that needs
// to distinguish "insert failed because a row like this already
// exists" from any other insert failure, since turn_runs is also the
// first table here to actually rely on a constraint racing correctly
// under concurrent writers (turn_runs_one_running_idx).
func isUniqueConstraintErr(err error) bool {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code == sqlite3.ErrConstraint
	}
	return false
}

// ReconcileOrphanedTurnRuns marks every still-"running" turn_runs row
// as "failed" and records a matching error turn in that conversation's
// own Markdown log — run once at application boot
// (backendapp.Start), since a goroutine has no PID to reattach to
// after a process restart the way tool/manager.go's supervised OS
// subprocesses do; every row still "running" when this runs is, by
// definition, dead. Best-effort per row, matching
// tool_manager.StartAllEnabled's own convention: one row failing to
// reconcile is warned about and does not stop the rest from being
// attempted.
func ReconcileOrphanedTurnRuns(conversationDB *sql.DB, warn func(format string, args ...any)) {
	runningRuns, err := conversation_query.NewTurnRunQueryRepo(conversationDB).FindAllRunning()
	if err != nil {
		warn("conversation: failed to list orphaned turn runs at boot: %v", err)
		return
	}
	if len(runningRuns) == 0 {
		return
	}

	runs := conversation_persistent.NewTurnRunPersistentRepo(conversationDB)
	convQuery := conversation_query.NewConversationQueryRepo(conversationDB)
	now := time.Now().UTC().Format(time.RFC3339)

	for _, run := range runningRuns {
		if err := runs.SetTerminalStatus(run.ID, "failed", now); err != nil {
			warn("conversation: failed to mark orphaned turn run %q failed: %v", run.ID, err)
			continue
		}

		conv, err := convQuery.FindByID(run.ConversationID)
		if err != nil {
			warn("conversation: orphaned turn run %q marked failed, but its conversation %q could not be found to record the error: %v", run.ID, run.ConversationID, err)
			continue
		}

		failedTurn := Turn{
			UserTimestamp:    run.StartedAt,
			UserContent:      run.UserContent,
			Failed:           true,
			ErrorTimestamp:   now,
			ErrorMessage:     "Interrupted by a server restart",
			PromptTokens:     run.PromptTokens,
			CompletionTokens: run.CompletionTokens,
		}
		if err := AppendTurn(conv.FilePath, failedTurn); err != nil {
			warn("conversation: orphaned turn run %q marked failed, but recording the error turn failed: %v", run.ID, err)
		}
	}
}
