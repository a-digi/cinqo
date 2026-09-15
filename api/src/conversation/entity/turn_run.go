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
}
