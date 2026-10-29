// Package query — turn_plan_steps table reads. See
// plan/ai/conversation/step-42-orchestrator-composer-mode.md.
package query

import (
	"database/sql"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

type TurnPlanStepQueryRepo struct {
	db *sql.DB
}

func NewTurnPlanStepQueryRepo(db *sql.DB) *TurnPlanStepQueryRepo {
	return &TurnPlanStepQueryRepo{db: db}
}

// NextPlanNo returns the plan number the turn's next run_plan call
// should use: 1 for the first, then one more than the highest so far.
func (r *TurnPlanStepQueryRepo) NextPlanNo(turnRunID string) (int, error) {
	var maxNo sql.NullInt64
	if err := r.db.QueryRow(`SELECT MAX(plan_no) FROM turn_plan_steps WHERE turn_run_id = ?`, turnRunID).Scan(&maxNo); err != nil {
		return 0, err
	}
	return int(maxNo.Int64) + 1, nil
}

// FindByTurnRunID returns every plan step of a turn, in plan order and
// the order each plan's steps were given.
func (r *TurnPlanStepQueryRepo) FindByTurnRunID(turnRunID string) ([]*conversation_entity.TurnPlanStep, error) {
	rows, err := r.db.Query(
		`SELECT turn_run_id, plan_no, step_id, task, depends_on, status, sub_agent_run_id FROM turn_plan_steps WHERE turn_run_id = ? ORDER BY plan_no ASC, rowid ASC`,
		turnRunID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*conversation_entity.TurnPlanStep, 0)
	for rows.Next() {
		var s conversation_entity.TurnPlanStep
		if err := rows.Scan(&s.TurnRunID, &s.PlanNo, &s.StepID, &s.Task, &s.DependsOn, &s.Status, &s.SubAgentRunID); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}
