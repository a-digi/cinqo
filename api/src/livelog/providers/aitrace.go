// Package providers holds one file per kind of log the live log module can follow. Each file is an adapter: it is the only place that
// imports the thing it wraps, and it hands the module a Provider and a Follower. Adding a kind of log is one new file here and one line
// in the composition root (api/src/cinqo/livelog.go); nothing else changes.
package providers

import (
	"github.com/a-digi/cinqo/src/filereader/source"
	"github.com/a-digi/cinqo/src/livelog"
	"github.com/a-digi/cinqo/src/livelog/filetail"
)

// AITraceID is the id of the AI trace log provider.
const AITraceID = "ai-trace"

type aiTrace struct {
	resolver source.Source
	opts     filetail.Options
}

// NewAITrace follows the per-turn AI trace logs, <conversationsDir>/<conversationId>/<turnRunId>.txt, which the conversation runner
// appends to while a turn runs. The reference is "<conversationId>/<turnRunId>" and is resolved by the file reader's own path rule
// (lower-case UUIDs only, symlinks resolved and kept inside the root), so the two readers can never disagree about which file a reference
// names. Administrators only, like the AI Logs overview.
func NewAITrace(conversationsDir string) livelog.Provider {
	return aiTrace{resolver: source.NewAITrace(conversationsDir)}
}

func (aiTrace) ID() string       { return AITraceID }
func (aiTrace) Title() string    { return "AI trace logs" }
func (aiTrace) Scopes() []string { return []string{livelog.AdminScope} }

func (a aiTrace) Open(ref string) (livelog.Follower, error) {
	path, err := a.resolver.Resolve(ref)
	if err != nil {
		return nil, livelog.ErrNotFound // every way a reference can fail is the same answer
	}
	return filetail.New(path, a.opts), nil
}
