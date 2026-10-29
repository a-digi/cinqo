package entity

// SubAgentRun tracks one sub-agent spawned by an orchestrator turn's
// own tool-calling loop (the "Sub Agents" feature) — a fresh,
// independent tool-calling loop (conversation/subagent.go) seeded only
// with Task, never the orchestrator's own conversation history, so the
// orchestrator's context grows only by Result, not by anything the
// sub-agent did to produce it. Mirrors TurnRun's own shape field-for-
// field deliberately — same live-progress-log/token-tracking/terminal-
// status story, so the polling and reconciliation code paths need no
// new patterns invented. Runs under its own independent context, not
// its parent turn's own — the parent turn is marked terminal (and its
// own reply persisted) the moment its own model call finishes, without
// waiting for a row like this one to reach a terminal status of its
// own. See plan/ai/conversation/step-41-sub-agents.md and
// conversation/subagent.go's own "the reply returns early" doc comment.
type SubAgentRun struct {
	_               struct{} `table:"sub_agent_runs"`
	ID              string   `db:"id" dbtype:"TEXT" nullable:"false" json:"id"`
	ParentTurnRunID string   `db:"parent_turn_run_id" dbtype:"TEXT" nullable:"false" json:"-"`
	ConversationID  string   `db:"conversation_id" dbtype:"TEXT" nullable:"false" json:"-"`
	Task            string   `db:"task" dbtype:"TEXT" nullable:"false" json:"task"`
	Status          string   `db:"status" dbtype:"TEXT" nullable:"false" json:"status"`
	// Result is the sub-agent's own final report text — set only once
	// Status leaves "running". This is the ONLY thing that ever flows
	// back into the orchestrator's own conversation context.
	Result     *string `db:"result" dbtype:"TEXT" nullable:"true" json:"result,omitempty"`
	StartedAt  string  `db:"started_at" dbtype:"TEXT" nullable:"false" json:"startedAt"`
	FinishedAt *string `db:"finished_at" dbtype:"TEXT" nullable:"true" json:"finishedAt,omitempty"`
	Log        string  `db:"log" dbtype:"TEXT" nullable:"false" json:"-"`
	// CancelRequested mirrors TurnRun's own field of the same name —
	// observability/audit only; actual cancellation propagates via the
	// parent turn's own context (see subagent.go).
	CancelRequested  bool `db:"cancel_requested" dbtype:"INTEGER" nullable:"false" json:"-"`
	PromptTokens     int  `db:"prompt_tokens" dbtype:"INTEGER" nullable:"false" json:"promptTokens"`
	CompletionTokens int  `db:"completion_tokens" dbtype:"INTEGER" nullable:"false" json:"completionTokens"`
	TotalTokens      int  `db:"total_tokens" dbtype:"INTEGER" nullable:"false" json:"totalTokens"`
	// Acceptance loop (conversation/acceptance.go) — all unset/default
	// for a spawn without acceptanceCriteria. Accepted is nil when no
	// criteria were given, otherwise the final verdict; a run that
	// exhausted its attempts is Status "completed" with Accepted false.
	AcceptanceCriteria *string `db:"acceptance_criteria" dbtype:"TEXT" nullable:"true" json:"acceptanceCriteria,omitempty"`
	MaxAttempts        int     `db:"max_attempts" dbtype:"INTEGER" nullable:"false" json:"maxAttempts"`
	Attempts           int     `db:"attempts" dbtype:"INTEGER" nullable:"false" json:"attempts"`
	Accepted           *bool   `db:"accepted" dbtype:"INTEGER" nullable:"true" json:"accepted,omitempty"`
	AcceptanceFeedback *string `db:"acceptance_feedback" dbtype:"TEXT" nullable:"true" json:"acceptanceFeedback,omitempty"`
	// OriginTool is the installed tool's MCP tool name whose delegation
	// directive started this sub-agent (conversation/delegation.go);
	// nil when the orchestrator model itself spawned it.
	OriginTool *string `db:"origin_tool" dbtype:"TEXT" nullable:"true" json:"originTool,omitempty"`
}
