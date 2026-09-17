// Package query — sub_agent_runs table reads. See
// plan/ai/conversation/step-41-sub-agents.md.
package query

import (
	"database/sql"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

type SubAgentRunQueryRepo struct {
	db *sql.DB
}

func NewSubAgentRunQueryRepo(db *sql.DB) *SubAgentRunQueryRepo {
	return &SubAgentRunQueryRepo{db: db}
}

const subAgentRunColumns = `id, parent_turn_run_id, conversation_id, task, status, result, started_at, finished_at, log, cancel_requested, prompt_tokens, completion_tokens, total_tokens`

func scanSubAgentRun(scan func(dest ...any) error) (*conversation_entity.SubAgentRun, error) {
	var s conversation_entity.SubAgentRun
	if err := scan(
		&s.ID, &s.ParentTurnRunID, &s.ConversationID, &s.Task, &s.Status, &s.Result, &s.StartedAt, &s.FinishedAt, &s.Log,
		&s.CancelRequested, &s.PromptTokens, &s.CompletionTokens, &s.TotalTokens,
	); err != nil {
		return nil, err
	}
	return &s, nil
}

// FindByParentTurnRunID returns every sub-agent spawned by
// parentTurnRunID, oldest first — GetActiveTurnHandler's own data
// source for the "sub-agents working" panel.
func (r *SubAgentRunQueryRepo) FindByParentTurnRunID(parentTurnRunID string) ([]*conversation_entity.SubAgentRun, error) {
	rows, err := r.db.Query(`SELECT `+subAgentRunColumns+` FROM sub_agent_runs WHERE parent_turn_run_id = ? ORDER BY started_at ASC`, parentTurnRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*conversation_entity.SubAgentRun, 0)
	for rows.Next() {
		s, err := scanSubAgentRun(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// FindByID returns sql.ErrNoRows (unwrapped) if no row has this ID —
// checkSubAgentCall's own data source.
func (r *SubAgentRunQueryRepo) FindByID(id string) (*conversation_entity.SubAgentRun, error) {
	row := r.db.QueryRow(`SELECT `+subAgentRunColumns+` FROM sub_agent_runs WHERE id = ? LIMIT 1`, id)
	return scanSubAgentRun(row.Scan)
}

// CountByParentTurnRunID returns how many sub-agents have EVER been
// spawned under parentTurnRunID — every status, not just "running" —
// since every one of them (even an already-finished one) already
// happened and cost real tokens. subagent.go's own maxSubAgentsPerTurn
// check reads this before allowing another spawn; counting only
// currently-running rows would let a turn that spawns-then-waits-then-
// spawns-again bypass the cap entirely by never having more than one
// in flight at once.
func (r *SubAgentRunQueryRepo) CountByParentTurnRunID(parentTurnRunID string) (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM sub_agent_runs WHERE parent_turn_run_id = ?`, parentTurnRunID).Scan(&count)
	return count, err
}

// FindAllRunning returns every row still marked "running" — used once
// at boot to reconcile sub-agent runs orphaned by a server restart,
// the same reasoning as TurnRunQueryRepo.FindAllRunning: a goroutine
// has no PID to reattach to.
func (r *SubAgentRunQueryRepo) FindAllRunning() ([]*conversation_entity.SubAgentRun, error) {
	rows, err := r.db.Query(`SELECT ` + subAgentRunColumns + ` FROM sub_agent_runs WHERE status = 'running'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*conversation_entity.SubAgentRun, 0)
	for rows.Next() {
		s, err := scanSubAgentRun(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
