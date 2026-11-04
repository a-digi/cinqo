package entity

// TurnRun tracks one in-flight (or just-finished) AI turn independently
// of the HTTP request that started it — see
// plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
// A completed turn is still recorded in the conversation's own
// Markdown log (see ../log.go) exactly as before; this row only tracks
// execution state for a turn that may still be running, so a reopened
// page (or a fresh poll) can tell whether one is in flight and since
// when.
//
// FinishedAt is nil while Status is "running". Log is a newline-
// delimited, append-only progress trace (one line per tool-loop
// iteration and per tool invocation) — a coarse trace of the loop's
// own steps, never raw model/tool output; see this step's own
// Security considerations for why.
type TurnRun struct {
	_              struct{} `table:"turn_runs"`
	ID             string   `db:"id" dbtype:"TEXT" nullable:"false" json:"id"`
	ConversationID string   `db:"conversation_id" dbtype:"TEXT" nullable:"false" json:"-"`
	UserContent    string   `db:"user_content" dbtype:"TEXT" nullable:"false" json:"-"`
	Status         string   `db:"status" dbtype:"TEXT" nullable:"false" json:"status"`
	StartedAt      string   `db:"started_at" dbtype:"TEXT" nullable:"false" json:"startedAt"`
	FinishedAt     *string  `db:"finished_at" dbtype:"TEXT" nullable:"true" json:"finishedAt,omitempty"`
	Log            string   `db:"log" dbtype:"TEXT" nullable:"false" json:"-"`
	// CancelRequested (step 25) is set the moment a stop is requested —
	// mainly for observability/audit, since the actual cancellation is
	// driven by runner.go's own in-memory cancel-func registry: a
	// process that has already restarted has lost the run entirely
	// regardless of this flag (ReconcileOrphanedTurnRuns already
	// handles that case by marking any still-"running" row "failed").
	CancelRequested bool `db:"cancel_requested" dbtype:"INTEGER" nullable:"false" json:"-"`
	// PromptTokens/CompletionTokens/TotalTokens (step 34) are the real,
	// provider-reported token counts accumulated across this turn's own
	// tool-calling loop — incremented atomically once per iteration
	// (TurnRunPersistentRepo.AddUsage), so they're already correct
	// and readable live while the run is still "running", not only
	// once it finishes. Zero for every turn run before this column
	// existed. See plan/ai/conversation/step-34-realtime-token-usage-
	// budget-and-display.md.
	// ToolErrors is how many of this turn's tool calls the framework refused or rejected (a tool that was not available, or arguments
	// that did not pass the schema), counted live like the token counts.
	ToolErrors       int `db:"tool_errors" dbtype:"INTEGER" nullable:"false" json:"toolErrors"`
	PromptTokens     int `db:"prompt_tokens" dbtype:"INTEGER" nullable:"false" json:"promptTokens"`
	CompletionTokens int `db:"completion_tokens" dbtype:"INTEGER" nullable:"false" json:"completionTokens"`
	TotalTokens      int `db:"total_tokens" dbtype:"INTEGER" nullable:"false" json:"totalTokens"`
	// CacheReadTokens/CacheWriteTokens/ReasoningTokens/CostMicroUSD (plan/ai/conversation/step-178, step 2) are what the provider reported
	// besides the two counts above, accumulated the same way: the prompt cache, the hidden reasoning (already inside CompletionTokens)
	// and the cost in whole micro-dollars. CostMicroUSD is nil when no response of the turn carried a cost (unknown), 0 for free calls.
	// Not in the JSON yet (step 3 decides what the handlers show).
	CacheReadTokens  int    `db:"cache_read_tokens" dbtype:"INTEGER" nullable:"false" json:"-"`
	CacheWriteTokens int    `db:"cache_write_tokens" dbtype:"INTEGER" nullable:"false" json:"-"`
	ReasoningTokens  int    `db:"reasoning_tokens" dbtype:"INTEGER" nullable:"false" json:"-"`
	CostMicroUSD     *int64 `db:"cost_microusd" dbtype:"INTEGER" nullable:"true" json:"-"`
	// RetryOf and RetryCount (plan/ai/conversation/retry-last-step.md, step 3): the failed turn run this run retries (nil for a fresh turn)
	// and its place in that chain (0 for a fresh turn). The saved checkpoint is a column too (checkpoint) but not a field here: it is
	// read and written only through the repositories' own checkpoint methods, so no query can return it by accident.
	RetryOf    *string `db:"retry_of" dbtype:"TEXT" nullable:"true" json:"-"`
	RetryCount int     `db:"retry_count" dbtype:"INTEGER" nullable:"false" json:"-"`
}

// TurnUsage is what the provider reported for one finished turn besides the prompt and completion counts (the lean projection the
// conversation read uses to show it next to the turn's message, plan/ai/conversation/step-178, step 3): no log, no text, only numbers.
type TurnUsage struct {
	CacheReadTokens  int
	CacheWriteTokens int
	ReasoningTokens  int
	CostMicroUSD     *int64
}

// UsageAmounts is one model response's usage as the repositories add it to a run (plan/ai/conversation/step-178, step 2): the counts the
// provider reported and the cost in whole micro-dollars, nil when the response carried none. Plain numbers, so the repositories do not
// depend on the provider packages.
type UsageAmounts struct {
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	CacheWriteTokens int
	ReasoningTokens  int
	CostMicroUSD     *int64
}

// TurnRunSummary is a lean projection of TurnRun for read paths that
// only need to say "this conversation has a turn running since X" —
// e.g. the conversation list endpoint — without paying for
// UserContent/Log on every row. See
// plan/ai/conversation/step-26-list-endpoint-active-turn-summary.md.
type TurnRunSummary struct {
	TurnRunID      string
	ConversationID string
	StartedAt      string
}
