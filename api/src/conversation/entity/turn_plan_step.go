package entity

// TurnPlanStep is one step of a composer orchestrator's run_plan call
// (conversation/composer_plan.go): the self-contained task a worker
// sub-agent is given, which other steps it waits on, and where it
// stands. PlanNo is 1 for the turn's first run_plan call, 2 for a
// re-plan, and so on. SubAgentRunID links the worker once the step has
// started; "skipped" lives only here (sub_agent_runs' own status CHECK
// has no such value). See
// plan/ai/conversation/step-42-orchestrator-composer-mode.md.
type TurnPlanStep struct {
	_             struct{} `table:"turn_plan_steps"`
	TurnRunID     string   `db:"turn_run_id" dbtype:"TEXT" nullable:"false" json:"-"`
	PlanNo        int      `db:"plan_no" dbtype:"INTEGER" nullable:"false" json:"planNo"`
	StepID        string   `db:"step_id" dbtype:"TEXT" nullable:"false" json:"stepId"`
	Task          string   `db:"task" dbtype:"TEXT" nullable:"false" json:"task"`
	DependsOn     string   `db:"depends_on" dbtype:"TEXT" nullable:"false" json:"-"`
	Status        string   `db:"status" dbtype:"TEXT" nullable:"false" json:"status"`
	SubAgentRunID *string  `db:"sub_agent_run_id" dbtype:"TEXT" nullable:"true" json:"subAgentId,omitempty"`
}

const (
	PlanStepPending   = "pending"
	PlanStepRunning   = "running"
	PlanStepCompleted = "completed"
	PlanStepFailed    = "failed"
	PlanStepSkipped   = "skipped"
)
