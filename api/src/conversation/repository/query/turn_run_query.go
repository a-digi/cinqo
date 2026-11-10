// Package query — turn_runs table reads. See
// plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
package query

import (
	"database/sql"
	"strings"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

type TurnRunQueryRepo struct {
	db *sql.DB
}

func NewTurnRunQueryRepo(db *sql.DB) *TurnRunQueryRepo {
	return &TurnRunQueryRepo{db: db}
}

const turnRunColumns = `id, conversation_id, user_content, status, started_at, finished_at, log, cancel_requested, prompt_tokens, completion_tokens, total_tokens, cache_read_tokens, cache_write_tokens, reasoning_tokens, cost_microusd, tool_errors, retry_of, retry_count`

func scanTurnRun(scan func(dest ...any) error) (*conversation_entity.TurnRun, error) {
	var t conversation_entity.TurnRun
	if err := scan(
		&t.ID, &t.ConversationID, &t.UserContent, &t.Status, &t.StartedAt, &t.FinishedAt, &t.Log, &t.CancelRequested,
		&t.PromptTokens, &t.CompletionTokens, &t.TotalTokens, &t.CacheReadTokens, &t.CacheWriteTokens, &t.ReasoningTokens, &t.CostMicroUSD, &t.ToolErrors, &t.RetryOf, &t.RetryCount,
	); err != nil {
		return nil, err
	}
	return &t, nil
}

// FindOptions returns the per-turn settings of a run: fresh (the model is sent the new message only) and the JSON list of the tools the turn is offered
// ("" when the turn uses the conversation's own list). They are read only through here, like the checkpoint, so no query returns them by accident.
// sql.ErrNoRows (unwrapped) when no row has this ID.
func (r *TurnRunQueryRepo) FindOptions(id string) (fresh bool, allowedTools string, err error) {
	var f int
	var tools sql.NullString
	if err = r.db.QueryRow(`SELECT fresh_context, allowed_tools FROM turn_runs WHERE id = ?`, id).Scan(&f, &tools); err != nil {
		return false, "", err
	}
	return f != 0, tools.String, nil
}

// FindUsageByConversationID returns, for every turn of the conversation, what the provider reported besides the prompt and completion
// counts (plan/ai/conversation/step-178, step 3), keyed by the turn run's id. Only the numbers are read (never the log or the text of
// the turns), so it is cheap for a long conversation; a turn whose provider reported nothing has all-zero numbers and a nil cost.
func (r *TurnRunQueryRepo) FindUsageByConversationID(conversationID string) (map[string]conversation_entity.TurnUsage, error) {
	rows, err := r.db.Query(`SELECT id, cache_read_tokens, cache_write_tokens, reasoning_tokens, cost_microusd FROM turn_runs WHERE conversation_id = ?`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]conversation_entity.TurnUsage{}
	for rows.Next() {
		var id string
		var u conversation_entity.TurnUsage
		if err := rows.Scan(&id, &u.CacheReadTokens, &u.CacheWriteTokens, &u.ReasoningTokens, &u.CostMicroUSD); err != nil {
			return nil, err
		}
		out[id] = u
	}
	return out, rows.Err()
}

// FindActiveByConversationID returns the one running turn_runs row for
// conversationID, if any — sql.ErrNoRows (unwrapped) when nothing is
// running. The DB's own turn_runs_one_running_idx partial unique index
// guarantees there is never more than one to find.
func (r *TurnRunQueryRepo) FindActiveByConversationID(conversationID string) (*conversation_entity.TurnRun, error) {
	row := r.db.QueryRow(`SELECT `+turnRunColumns+` FROM turn_runs WHERE conversation_id = ? AND status = 'running' LIMIT 1`, conversationID)
	return scanTurnRun(row.Scan)
}

// started_at has a resolution of one second, so runs started in the same second (a retry right after its failed run) are told apart by
// the order they were inserted in (rowid).
//
// FindMostRecentByConversationID returns conversationID's own most
// recently started turn_runs row, regardless of status — used by the
// polling endpoint (GetActiveTurnHandler), deliberately NOT restricted
// to status='running' the way FindActiveByConversationID is: a
// poller's in-flight request racing the exact moment a run finishes
// must still see that row (now terminal), never a 404 that would
// leave it unable to tell "finished" apart from "never existed" or
// "server restarted and lost it."
func (r *TurnRunQueryRepo) FindMostRecentByConversationID(conversationID string) (*conversation_entity.TurnRun, error) {
	row := r.db.QueryRow(`SELECT `+turnRunColumns+` FROM turn_runs WHERE conversation_id = ? ORDER BY started_at DESC, rowid DESC LIMIT 1`, conversationID)
	return scanTurnRun(row.Scan)
}

// FindByID returns sql.ErrNoRows (unwrapped) if no row has this ID.
// Ownership is enforced by callers via the owning conversation (the
// same conversation_query.FindOwnedByID check every handler in this
// feature already does), not here — matches conversation_query.
// FindByID's own established split between an unfiltered lookup and
// an ownership-scoped one.
func (r *TurnRunQueryRepo) FindByID(id string) (*conversation_entity.TurnRun, error) {
	row := r.db.QueryRow(`SELECT `+turnRunColumns+` FROM turn_runs WHERE id = ? LIMIT 1`, id)
	return scanTurnRun(row.Scan)
}

// FindCheckpoint returns the saved checkpoint text of a turn run, "" when it has none (never saved, or cleared). Read on its own, never as
// part of the turn run row (see TurnRun.RetryOf). sql.ErrNoRows (unwrapped) when no run has this id.
func (r *TurnRunQueryRepo) FindCheckpoint(id string) (string, error) {
	var text sql.NullString
	if err := r.db.QueryRow(`SELECT checkpoint FROM turn_runs WHERE id = ?`, id).Scan(&text); err != nil {
		return "", err
	}
	return text.String, nil
}

// FindActiveSummariesByConversationIDs returns a lean
// {turnRunId, startedAt} projection for every currently-running turn
// among conversationIDs, keyed by conversation_id — used by
// ListHandler to enrich the conversation list with a running-turn
// indicator without pulling every running turn's own (potentially
// sizeable) log/user_content into a response that may be polled every
// few seconds. Returns an empty map without querying when
// conversationIDs is empty (avoids a malformed `IN ()`). See
// plan/ai/conversation/step-26-list-endpoint-active-turn-summary.md.
func (r *TurnRunQueryRepo) FindActiveSummariesByConversationIDs(conversationIDs []string) (map[string]conversation_entity.TurnRunSummary, error) {
	out := map[string]conversation_entity.TurnRunSummary{}
	if len(conversationIDs) == 0 {
		return out, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(conversationIDs)), ",")
	args := make([]any, len(conversationIDs))
	for i, id := range conversationIDs {
		args[i] = id
	}

	rows, err := r.db.Query(`SELECT id, conversation_id, started_at FROM turn_runs WHERE status = 'running' AND conversation_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var s conversation_entity.TurnRunSummary
		if err := rows.Scan(&s.TurnRunID, &s.ConversationID, &s.StartedAt); err != nil {
			return nil, err
		}
		out[s.ConversationID] = s
	}
	return out, rows.Err()
}

// FindAllRunning returns every row still marked "running" — used once
// at boot to reconcile turns orphaned by a server restart. A goroutine
// has no PID to reattach to (unlike tool/manager.go's supervised OS
// subprocesses), so every such row is, by definition, dead the moment
// the process that ran it is gone.
func (r *TurnRunQueryRepo) FindAllRunning() ([]*conversation_entity.TurnRun, error) {
	rows, err := r.db.Query(`SELECT ` + turnRunColumns + ` FROM turn_runs WHERE status = 'running'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*conversation_entity.TurnRun, 0)
	for rows.Next() {
		t, err := scanTurnRun(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RunningTurn is a turn under way and the conversation it belongs to.
type RunningTurn struct {
	ConversationID string
	Title          string
	UserID         string
	Model          string
}

// FindRunningOnPlatform returns the turns still marked "running" whose conversation uses the platform, with the conversation's title, owner and
// model. The local AI asks it which conversations are using the loaded model (a running turn is the reliable sign: between two requests of one
// turn's tool loop the model program is momentarily idle).
func (r *TurnRunQueryRepo) FindRunningOnPlatform(platformID string) ([]RunningTurn, error) {
	rows, err := r.db.Query(`SELECT c.id, c.title, c.user_id, c.model
		FROM turn_runs t JOIN conversations c ON c.id = t.conversation_id
		WHERE t.status = 'running' AND c.platform_id = ?
		ORDER BY t.started_at`, platformID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RunningTurn, 0)
	for rows.Next() {
		var t RunningTurn
		if err := rows.Scan(&t.ConversationID, &t.Title, &t.UserID, &t.Model); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
