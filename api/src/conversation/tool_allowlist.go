package conversation

import (
	"context"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

// A conversation can be created with an allowlist of tools (step 113): the model is offered only
// those, and a call to anything else is refused as "tool unavailable" before it can reach a real
// tool, whatever the model says or whatever text it was shown. Without a list nothing changes.
// The list only narrows: the caller's scope check still applies to every tool that is offered.
// See plan/ai/tools/career/step-113-human-in-the-loop-for-ai-personas.md.

type allowlistKey struct{}

// applyToolAllowlist returns the context and the tool list a turn must use for conv: unchanged
// when the conversation has no allowlist, otherwise the offered tools cut down to the allowlist and
// a context that makes runToolLoop refuse every other call.
func applyToolAllowlist(ctx context.Context, tools []chatcompleter.ToolDef, conv *conversation_entity.Conversation) (context.Context, []chatcompleter.ToolDef) {
	names := conv.AllowedToolNames()
	if names == nil {
		return ctx, tools
	}
	allowed := make(map[string]struct{}, len(names))
	for _, n := range names {
		allowed[n] = struct{}{}
	}
	kept := make([]chatcompleter.ToolDef, 0, len(tools))
	for _, t := range tools {
		if _, ok := allowed[t.Name]; ok {
			kept = append(kept, t)
		}
	}
	return context.WithValue(ctx, allowlistKey{}, allowed), kept
}

// refusesDisallowedTool reports whether a call must be refused because the turn's conversation has
// an allowlist and name is not on it.
func refusesDisallowedTool(ctx context.Context, name string) bool {
	allowed, ok := ctx.Value(allowlistKey{}).(map[string]struct{})
	if !ok {
		return false
	}
	_, on := allowed[name]
	return !on
}
