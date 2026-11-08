// Package persistent — turn_runs table writes. See
// plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
package persistent

import (
	"database/sql"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

type TurnRunPersistentRepo struct {
	db *sql.DB
}

func NewTurnRunPersistentRepo(db *sql.DB) *TurnRunPersistentRepo {
	return &TurnRunPersistentRepo{db: db}
}

// Insert starts tracking a new turn run — status is always "running"
// at insert time; nothing else ever creates one. The DB's own
// turn_runs_one_running_idx partial unique index is the real
// concurrency guard against two turns running at once for the same
// conversation: this returns that constraint violation unchanged
// rather than attempting its own check-then-insert, which would race
// under two concurrent POSTs to the same conversation.
func (r *TurnRunPersistentRepo) Insert(t *conversation_entity.TurnRun) error {
	_, err := r.db.Exec(
		`INSERT INTO turn_runs (id, conversation_id, user_content, status, started_at, log) VALUES (?, ?, ?, 'running', ?, '')`,
		t.ID, t.ConversationID, t.UserContent, t.StartedAt,
	)
	return err
}

// AppendLog appends one line to the run's own progress log via an
// atomic `log = log || ?` — never a read-modify-write, so it can never
// clobber a concurrent append (today there is only ever one goroutine
// writing a given run's log, but the atomicity costs nothing and is
// correct regardless).
func (r *TurnRunPersistentRepo) AppendLog(id, line string) error {
	_, err := r.db.Exec(`UPDATE turn_runs SET log = log || ? WHERE id = ?`, line, id)
	return err
}

// SetTerminalStatus marks a run finished — status must be one of
// "completed"/"failed"/"cancelled" (never "running", which only
// Insert ever sets). Does not touch prompt_tokens/completion_tokens/
// total_tokens — AddUsage (below) already accumulated the
// correct final totals live, once per iteration, by the time a run
// reaches a terminal status.
func (r *TurnRunPersistentRepo) SetTerminalStatus(id, status, finishedAt string) error {
	_, err := r.db.Exec(`UPDATE turn_runs SET status = ?, finished_at = ? WHERE id = ?`, status, finishedAt, id)
	return err
}

// AddUsage atomically increments this run's running usage by one model response's (step 34, widened by plan/ai/conversation/
// step-178, step 2): the prompt and completion counts and what the provider reported for its prompt cache and its hidden reasoning,
// and the cost. Each is a `col = col + ?` in one UPDATE, never a read-modify-write, so the totals are right and readable live while
// the run is still running, and concurrent additions never lose one. The cost is added only when the response carried one: a NULL
// total stays NULL until the first reported cost, then sums.
func (r *TurnRunPersistentRepo) AddUsage(id string, u conversation_entity.UsageAmounts) error {
	_, err := r.db.Exec(
		`UPDATE turn_runs SET prompt_tokens = prompt_tokens + ?1, completion_tokens = completion_tokens + ?2, total_tokens = total_tokens + ?3,
		   cache_read_tokens = cache_read_tokens + ?4, cache_write_tokens = cache_write_tokens + ?5, reasoning_tokens = reasoning_tokens + ?6,
		   cost_microusd = CASE WHEN ?7 IS NULL THEN cost_microusd ELSE COALESCE(cost_microusd, 0) + ?7 END
		 WHERE id = ?8`,
		u.PromptTokens, u.CompletionTokens, u.PromptTokens+u.CompletionTokens,
		u.CacheReadTokens, u.CacheWriteTokens, u.ReasoningTokens, u.CostMicroUSD, id,
	)
	return err
}

// SaveCheckpoint stores the checkpoint text of a failed turn run (plan/ai/conversation/retry-last-step.md, step 3). At most one run of a
// conversation keeps one: saving clears every other run's of the same conversation in the same statement, so a retry only ever finds the
// state of the last failed turn. A no-op for an unknown run.
func (r *TurnRunPersistentRepo) SaveCheckpoint(id, text string) error {
	_, err := r.db.Exec(
		`UPDATE turn_runs SET checkpoint = CASE WHEN id = ?1 THEN ?2 ELSE NULL END
		 WHERE conversation_id = (SELECT conversation_id FROM turn_runs WHERE id = ?1) AND (id = ?1 OR checkpoint IS NOT NULL)`,
		id, text,
	)
	return err
}

// ClearCheckpoint forgets one run's checkpoint (after a successful retry).
func (r *TurnRunPersistentRepo) ClearCheckpoint(id string) error {
	_, err := r.db.Exec(`UPDATE turn_runs SET checkpoint = NULL WHERE id = ?`, id)
	return err
}

// ClearConversationCheckpoints forgets every checkpoint of one conversation (a new turn of it starts, or it is deleted).
func (r *TurnRunPersistentRepo) ClearConversationCheckpoints(conversationID string) error {
	_, err := r.db.Exec(`UPDATE turn_runs SET checkpoint = NULL WHERE conversation_id = ? AND checkpoint IS NOT NULL`, conversationID)
	return err
}

// SetOptions stores the per-turn settings of a run (the migration 31_10_2026_16_00_00): fresh says the model is sent the new message only, allowedTools
// is the JSON list of the tools the turn is offered ("" for the conversation's own list, stored as NULL).
func (r *TurnRunPersistentRepo) SetOptions(id string, fresh bool, allowedTools string) error {
	var tools any
	if allowedTools != "" {
		tools = allowedTools
	}
	f := 0
	if fresh {
		f = 1
	}
	_, err := r.db.Exec(`UPDATE turn_runs SET fresh_context = ?, allowed_tools = ? WHERE id = ?`, f, tools, id)
	return err
}

// CopyOptions gives the run toID the per-turn settings of the run fromID (a retry keeps the settings of the turn it retries).
func (r *TurnRunPersistentRepo) CopyOptions(fromID, toID string) error {
	_, err := r.db.Exec(
		`UPDATE turn_runs SET fresh_context = (SELECT fresh_context FROM turn_runs WHERE id = ?), allowed_tools = (SELECT allowed_tools FROM turn_runs WHERE id = ?) WHERE id = ?`,
		fromID, fromID, toID,
	)
	return err
}

// SetRetry marks a run as the retry of another (the place in the chain: 1 for the first retry).
func (r *TurnRunPersistentRepo) SetRetry(id, retryOf string, retryCount int) error {
	_, err := r.db.Exec(`UPDATE turn_runs SET retry_of = ?, retry_count = ? WHERE id = ?`, retryOf, retryCount, id)
	return err
}

// AddToolError counts one refused or rejected tool call of this run (a single atomic increment, readable live like the usage).
func (r *TurnRunPersistentRepo) AddToolError(id string) error {
	_, err := r.db.Exec(`UPDATE turn_runs SET tool_errors = tool_errors + 1 WHERE id = ?`, id)
	return err
}

// SetCancelRequested records that a stop was requested for this run —
// mainly for observability/audit (see the entity field's own doc
// comment); the actual cancellation happens via runner.go's in-memory
// cancel-func registry, not by anything reading this column back.
// Scoped to status='running' so a stop request racing a run that just
// finished on its own is a harmless no-op rather than overwriting a
// terminal row's own history.
func (r *TurnRunPersistentRepo) SetCancelRequested(id string) error {
	_, err := r.db.Exec(`UPDATE turn_runs SET cancel_requested = 1 WHERE id = ? AND status = 'running'`, id)
	return err
}
