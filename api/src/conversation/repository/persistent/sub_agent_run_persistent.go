// Package persistent — sub_agent_runs table writes. See
// plan/ai/conversation/step-41-sub-agents.md.
package persistent

import (
	"database/sql"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

type SubAgentRunPersistentRepo struct {
	db *sql.DB
}

func NewSubAgentRunPersistentRepo(db *sql.DB) *SubAgentRunPersistentRepo {
	return &SubAgentRunPersistentRepo{db: db}
}

// Insert starts tracking a new sub-agent run — status is always
// "running" at insert time, matching TurnRunPersistentRepo.Insert's
// own convention.
func (r *SubAgentRunPersistentRepo) Insert(s *conversation_entity.SubAgentRun) error {
	_, err := r.db.Exec(
		`INSERT INTO sub_agent_runs (id, parent_turn_run_id, conversation_id, task, status, started_at, log, acceptance_criteria, max_attempts, origin_tool) VALUES (?, ?, ?, ?, 'running', ?, '', ?, ?, ?)`,
		s.ID, s.ParentTurnRunID, s.ConversationID, s.Task, s.StartedAt, s.AcceptanceCriteria, s.MaxAttempts, s.OriginTool,
	)
	return err
}

// AppendLog mirrors TurnRunPersistentRepo.AppendLog exactly — an
// atomic `log = log || ?`, never a read-modify-write.
func (r *SubAgentRunPersistentRepo) AppendLog(id, line string) error {
	_, err := r.db.Exec(`UPDATE sub_agent_runs SET log = log || ? WHERE id = ?`, line, id)
	return err
}

// AddTokenUsage mirrors TurnRunPersistentRepo.AddTokenUsage exactly.
func (r *SubAgentRunPersistentRepo) AddTokenUsage(id string, promptTokens, completionTokens int) error {
	_, err := r.db.Exec(
		`UPDATE sub_agent_runs SET prompt_tokens = prompt_tokens + ?, completion_tokens = completion_tokens + ?, total_tokens = total_tokens + ? WHERE id = ?`,
		promptTokens, completionTokens, promptTokens+completionTokens, id,
	)
	return err
}

// SetTerminalStatus marks a run finished AND records its own final
// report text in the same write — unlike TurnRun (whose own "result"
// is a Markdown turn appended elsewhere), a sub-agent's result has
// nowhere else to live, so this diverges from
// TurnRunPersistentRepo.SetTerminalStatus's own narrower signature by
// necessity, not convention drift. result is nil for "failed"/
// "cancelled" (the caller's own error text goes into Log instead, same
// "never raw model/tool output outside Log" rule TurnRun's own Log
// field already follows).
func (r *SubAgentRunPersistentRepo) SetTerminalStatus(id, status string, result *string, finishedAt string) error {
	_, err := r.db.Exec(`UPDATE sub_agent_runs SET status = ?, result = ?, finished_at = ? WHERE id = ?`, status, result, finishedAt, id)
	return err
}

// SetCancelRequested mirrors TurnRunPersistentRepo.SetCancelRequested
// exactly — observability/audit only (the actual cancellation is
// driven by CancelSubAgent's own in-memory registry); scoped to
// status='running' so a cancel racing a sub-agent that just finished
// on its own is a harmless no-op rather than overwriting a terminal
// row's own history.
func (r *SubAgentRunPersistentRepo) SetCancelRequested(id string) error {
	_, err := r.db.Exec(`UPDATE sub_agent_runs SET cancel_requested = 1 WHERE id = ? AND status = 'running'`, id)
	return err
}

// SetAttempts records which acceptance-loop attempt is now running
// (see acceptance.go).
func (r *SubAgentRunPersistentRepo) SetAttempts(id string, attempts int) error {
	_, err := r.db.Exec(`UPDATE sub_agent_runs SET attempts = ? WHERE id = ?`, attempts, id)
	return err
}

// SetAcceptance records the acceptance loop's final verdict.
func (r *SubAgentRunPersistentRepo) SetAcceptance(id string, accepted bool, feedback string) error {
	_, err := r.db.Exec(`UPDATE sub_agent_runs SET accepted = ?, acceptance_feedback = ? WHERE id = ?`, accepted, feedback, id)
	return err
}
