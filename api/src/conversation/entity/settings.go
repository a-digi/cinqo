package entity

// Settings is the conversation feature's own single, global settings
// row (id always 1 — enforced by the table's own CHECK constraint) —
// same "one object, one row" shape as tools/browser/backend's own
// browser_settings. AITraceLogsEnabled gates whether
// runner.go's runDetachedTurn ever writes anything to
// conversation.TraceLogPath at all: off (the default), a turn's own
// tool-calling loop produces no trace file whatsoever, not merely a
// hidden one. See plan/ai/conversation/step-37-ai-debug-logging-toggle.md.
type Settings struct {
	_                  struct{} `table:"conversation_settings"`
	ID                 int      `db:"id" dbtype:"INTEGER" nullable:"false" json:"-"`
	AITraceLogsEnabled bool     `db:"ai_trace_logs_enabled" dbtype:"INTEGER" nullable:"false" json:"aiTraceLogsEnabled"`
	UpdatedAt          *string  `db:"updated_at" dbtype:"TEXT" nullable:"true" json:"-"`
}
