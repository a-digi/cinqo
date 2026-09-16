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
	// (TurnRunPersistentRepo.AddTokenUsage), so they're already correct
	// and readable live while the run is still "running", not only
	// once it finishes. Zero for every turn run before this column
	// existed. See plan/ai/conversation/step-34-realtime-token-usage-
	// budget-and-display.md.
	PromptTokens     int `db:"prompt_tokens" dbtype:"INTEGER" nullable:"false" json:"promptTokens"`
	CompletionTokens int `db:"completion_tokens" dbtype:"INTEGER" nullable:"false" json:"completionTokens"`
	TotalTokens      int `db:"total_tokens" dbtype:"INTEGER" nullable:"false" json:"totalTokens"`
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
