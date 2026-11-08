// Package persistent — turn_plan_steps table writes. See
// plan/ai/conversation/step-42-orchestrator-composer-mode.md.
package persistent

import (
	"database/sql"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

type TurnPlanStepPersistentRepo struct {
	db *sql.DB
}

func NewTurnPlanStepPersistentRepo(db *sql.DB) *TurnPlanStepPersistentRepo {
	return &TurnPlanStepPersistentRepo{db: db}
}

// InsertAll records every step of one plan, all 'pending', in one
// transaction — a plan is either fully recorded or not at all.
func (r *TurnPlanStepPersistentRepo) InsertAll(steps []*conversation_entity.TurnPlanStep) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, s := range steps {
		if _, err := tx.Exec(
			`INSERT INTO turn_plan_steps (turn_run_id, plan_no, step_id, task, depends_on, status) VALUES (?, ?, ?, ?, ?, 'pending')`,
			s.TurnRunID, s.PlanNo, s.StepID, s.Task, s.DependsOn,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateStatus moves one step to a new status, recording the worker
// that runs it when subAgentRunID is non-nil (nil leaves any existing
// link untouched).
func (r *TurnPlanStepPersistentRepo) UpdateStatus(turnRunID string, planNo int, stepID, status string, subAgentRunID *string) error {
	_, err := r.db.Exec(
		`UPDATE turn_plan_steps SET status = ?, sub_agent_run_id = COALESCE(?, sub_agent_run_id) WHERE turn_run_id = ? AND plan_no = ? AND step_id = ?`,
		status, subAgentRunID, turnRunID, planNo, stepID,
	)
	return err
}
