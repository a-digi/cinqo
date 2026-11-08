package source

import (
	"regexp"
	"strings"
)

// uuid is the exact shape of a conversation id and a turn run id (both generated as lowercase UUIDs).
var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// IsUUID reports whether s is a lowercase UUID, the only shape accepted in a trace-log reference.
func IsUUID(s string) bool { return uuid.MatchString(s) }

// AITraceID is the id of the AI trace log source.
const AITraceID = "ai-trace"

type aiTrace struct{ conversationsDir string }

// NewAITrace serves the per-turn AI trace logs, <conversationsDir>/<conversationId>/<turnRunId>.txt, to
// administrators (the same scope as the AI Logs overview). The reference is "<conversationId>/<turnRunId>".
func NewAITrace(conversationsDir string) Source { return aiTrace{conversationsDir: conversationsDir} }

func (aiTrace) ID() string       { return AITraceID }
func (aiTrace) Scopes() []string { return []string{AdminScope} }

func (a aiTrace) Resolve(ref string) (string, error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 2 || !IsUUID(parts[0]) || !IsUUID(parts[1]) {
		return "", ErrNotFound
	}
	return ResolveUnder(a.conversationsDir, parts[0], parts[1]+".txt")
}
