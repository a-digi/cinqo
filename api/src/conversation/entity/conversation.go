// Package entity holds the conversation feature's plain data shapes —
// see plan/ai/conversation/step-01-data-model-and-separate-database.md.
package entity

// Conversation is metadata only — its message content lives entirely
// in a Markdown log file (see log.go), never in this table. FilePath is
// always server-derived from the conversation's own ID, never
// client-supplied, and json:"-" since the frontend never needs the
// path itself, only content the backend already read out of the file.
//
// PlatformID/Model are set once at creation and never updated by any
// code path afterward — immutability is enforced structurally (no
// UpdatePlatform/UpdateModel method exists anywhere), not by a runtime
// check. See
// plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md.
type Conversation struct {
	_          struct{} `table:"conversations"`
	ID         string   `db:"id" dbtype:"UUID" nullable:"false" json:"id"`
	UserID     string   `db:"user_id" dbtype:"TEXT" nullable:"false" json:"-"`
	Title      string   `db:"title" dbtype:"TEXT" nullable:"false" json:"title"`
	StartedAt  string   `db:"started_at" dbtype:"DATETIME" nullable:"false" json:"startedAt"`
	FilePath   string   `db:"file_path" dbtype:"TEXT" nullable:"false" json:"-"`
	PlatformID string   `db:"platform_id" dbtype:"TEXT" nullable:"false" json:"platformId"`
	Model      string   `db:"model" dbtype:"TEXT" nullable:"false" json:"model"`
	// Hidden (step 30) excludes this conversation from ListOwnedBy —
	// used by callers that want an AI conversation running without it
	// ever showing up in the caller's own normal conversation list
	// (e.g. Career's own "generate crawl instructions with AI").
	// json:"-" — never echoed back in any response; a caller that
	// creates a hidden conversation already knows it did so, and every
	// other route (FindOwnedByID/FindByID) stays fully unfiltered by
	// this, so a hidden conversation is exactly as usable as a normal
	// one to anything that already has its ID. See
	// plan/ai/conversation/step-30-hidden-conversations.md.
	Hidden bool `db:"hidden" dbtype:"INTEGER" nullable:"false" json:"-"`
	// ToolSlug (step 83-style tool attribution) is set once, at
	// creation, by whichever caller created this conversation on behalf
	// of a tool (e.g. Career's own Job Match/Generate CV PDF/crawl-
	// instructions flows pass "career") — empty for a conversation
	// created directly by a person via the chat widget/full page's own
	// "+ New conversation", which isn't tied to any tool. Never
	// validated against the tools table (a tool can be uninstalled and
	// reinstalled; this is an attribution tag for filtering, not a
	// referential integrity requirement) — same convention Media's own
	// tool_slug column already established. Nothing ever updates it
	// after creation. See
	// plan/ai/conversation/step-41-search-and-filter-by-tool.md.
	ToolSlug string `db:"tool_slug" dbtype:"TEXT" nullable:"true" json:"toolSlug,omitempty"`
	// OrchestrationMode (composer step 1) is OrchestrationModeDirect —
	// the orchestrator model works with the tools itself, exactly as
	// before this field existed — or OrchestrationModeComposer, where
	// the orchestrator only plans and sub-agents do the work. Set once
	// at creation and never updated by any code path afterward
	// (structurally: no Update method exists), the same "fixed at
	// creation" rule as PlatformID/Model. Step 1 only stores and
	// exposes it; nothing branches on it yet. See
	// plan/ai/conversation/step-42-orchestrator-composer-mode.md.
	OrchestrationMode string `db:"orchestration_mode" dbtype:"TEXT" nullable:"false" json:"orchestrationMode"`
	// ToolIterationLimit (step 93) caps how many model calls one turn of
	// this conversation may make while calling tools. nil means the global
	// default; a conversation can only LOWER its budget (the create handler
	// rejects anything above the global ceiling). Set once at creation,
	// like PlatformID and Model. See
	// plan/ai/tools/career/step-93-tool-call-budget-and-weak-models.md.
	ToolIterationLimit *int `db:"tool_iteration_limit" dbtype:"INTEGER" nullable:"true" json:"toolIterationLimit,omitempty"`
}

// MaxToolIterationLimit mirrors the conversation package's own ceiling
// (conversation.MaxToolIterations; the entity package cannot import it).
// A test in the conversation package pins the two together.
const MaxToolIterationLimit = 15

// ValidToolIterationLimit reports whether a client-supplied limit is
// acceptable: absent (nil) means "use the default"; otherwise 1 to
// MaxToolIterationLimit.
func ValidToolIterationLimit(limit *int) bool {
	return limit == nil || (*limit >= 1 && *limit <= MaxToolIterationLimit)
}

const (
	OrchestrationModeDirect   = "direct"
	OrchestrationModeComposer = "composer"
)

// ParseOrchestrationMode normalizes a client-supplied mode: empty means
// the default (direct, today's behavior). ok is false for anything
// that is not exactly one of the two known modes.
func ParseOrchestrationMode(raw string) (mode string, ok bool) {
	switch raw {
	case "", OrchestrationModeDirect:
		return OrchestrationModeDirect, true
	case OrchestrationModeComposer:
		return OrchestrationModeComposer, true
	}
	return "", false
}
