// Package platform is the compile-time registry of AI providers cinqo
// integrates with — see plan/ai/platform/step-01-platform-registry-and-data-model.md.
// Package-level state, no DI wrapper — same idiom already established
// in this codebase for exactly this kind of static, bootstrap-time
// registry (security/scopes' own registry).
package platform

import (
	"github.com/a-digi/cinqo/src/platform/anthropic"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/platform/entity"
	"github.com/a-digi/cinqo/src/platform/openai"
	"github.com/a-digi/cinqo/src/platform/openrouter"
)

// Entry is one registered platform. Completer is the ChatCompleter this
// platform's chat completion calls go through. SelectableModels is the
// fixed, curated list a caller may choose from at conversation-creation
// time (plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md)
// — empty/nil means no user-facing choice exists and DefaultModel is
// always used (OpenRouter's own case: its "auto" routing already covers
// "give me something free/appropriate").
type Entry struct {
	Info             entity.Platform
	DefaultBaseURL   string
	DefaultModel     string
	SelectableModels []string
	Completer        chatcompleter.ChatCompleter
}

var registry = []Entry{
	{
		Info:             entity.Platform{ID: "openai", Name: "OpenAI"},
		DefaultBaseURL:   "https://api.openai.com/v1",
		DefaultModel:     "gpt-4o-mini",
		SelectableModels: []string{"gpt-4o-mini", "gpt-4o", "gpt-4-turbo"},
		Completer:        openai.Client{},
	},
	{
		Info:             entity.Platform{ID: "anthropic", Name: "Anthropic"},
		DefaultBaseURL:   "https://api.anthropic.com/v1",
		DefaultModel:     "claude-3-5-haiku-latest",
		SelectableModels: []string{"claude-3-5-haiku-latest", "claude-3-5-sonnet-latest", "claude-3-opus-latest"},
		Completer:        anthropic.Client{},
	},
	{
		Info:           entity.Platform{ID: "openrouter", Name: "OpenRouter"},
		DefaultBaseURL: "https://openrouter.ai/api/v1",
		DefaultModel:   "openrouter/auto",
		// SelectableModels intentionally omitted — no user choice.
		Completer: openrouter.Client{},
	},
}

// List returns every registered platform's {id, name} — nothing more,
// matching the reference's own minimal GET /ai/platforms shape.
func List() []entity.Platform {
	out := make([]entity.Platform, 0, len(registry))
	for _, e := range registry {
		out = append(out, e.Info)
	}
	return out
}

// Lookup returns the full registry entry for id, if registered.
func Lookup(id string) (Entry, bool) {
	for _, e := range registry {
		if e.Info.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// IsValid reports whether id is a registered platform.
func IsValid(id string) bool {
	_, ok := Lookup(id)
	return ok
}

// IsSelectableModel reports whether model is one of platformID's own
// curated choices — used at conversation-creation time so an arbitrary
// client-supplied string never reaches the real provider unchecked.
// See plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md.
func IsSelectableModel(platformID, model string) bool {
	entry, ok := Lookup(platformID)
	if !ok {
		return false
	}
	for _, m := range entry.SelectableModels {
		if m == model {
			return true
		}
	}
	return false
}
