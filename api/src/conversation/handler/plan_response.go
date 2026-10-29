package handler

import (
	"database/sql"
	"encoding/json"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// planStepResponse is one step of a composer turn's run_plan call
// (composer step 6). PlanNo groups steps by run_plan call (1 for the
// first plan, 2 for a re-plan); Status is pending, running, completed,
// failed or skipped; SubAgentID links the worker that ran the step and
// is omitted while a step has none (still pending, or skipped without
// ever starting). See
// plan/ai/conversation/step-42-orchestrator-composer-mode.md.
type planStepResponse struct {
	PlanNo     int      `json:"planNo"`
	ID         string   `json:"id"`
	Task       string   `json:"task"`
	DependsOn  []string `json:"dependsOn"`
	Status     string   `json:"status"`
	SubAgentID string   `json:"subAgentId,omitempty"`
}

func toPlanStepResponse(s *conversation_entity.TurnPlanStep) planStepResponse {
	deps := []string{}
	if err := json.Unmarshal([]byte(s.DependsOn), &deps); err != nil || deps == nil {
		deps = []string{}
	}
	resp := planStepResponse{PlanNo: s.PlanNo, ID: s.StepID, Task: s.Task, DependsOn: deps, Status: s.Status}
	if s.SubAgentRunID != nil {
		resp.SubAgentID = *s.SubAgentRunID
	}
	return resp
}

// loadPlan returns every plan step of one turn in plan order, or nil
// when the turn has none (every direct-mode turn) — callers omit the
// field entirely then, so a response for a non-composer turn is
// byte-for-byte what it was before composer mode existed.
func loadPlan(db *sql.DB, turnRunID string) ([]planStepResponse, error) {
	steps, err := conversation_query.NewTurnPlanStepQueryRepo(db).FindByTurnRunID(turnRunID)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return nil, nil
	}
	out := make([]planStepResponse, 0, len(steps))
	for _, s := range steps {
		out = append(out, toPlanStepResponse(s))
	}
	return out, nil
}
