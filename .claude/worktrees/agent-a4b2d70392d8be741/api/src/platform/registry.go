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
		Info:           entity.Platform{ID: "openai", Name: "OpenAI"},
		DefaultBaseURL: "https://api.openai.com/v1",
		// GPT-4 family removed entirely (plan/ai/platform/step-07-openai-model-list-revision.md).
		// Verified directly against the real page's raw HTML (not a
		// summary) — "gpt-5"/"gpt-5-mini"/"gpt-5-nano" are a real but
		// SUPERSEDED generation (that page's own "More models" bucket);
		// the actual current "Flagship models" section is the
		// 5.6/6-astra generation, whose own lighter tiers are named
		// "Terra"/"Luna", not "-mini"/"-nano". DefaultModel is the
		// current generation's own cost-sensitive tier.
		//
		// gpt-oss-120b/gpt-oss-20b deliberately NOT included — verified
		// directly (each model's own detail page's Endpoints section)
		// that both are hosted on api.openai.com only via the Responses
		// API (v1/responses), never Chat Completions
		// (v1/chat/completions) — the only endpoint openai.Client
		// (step 3) implements. Offering them here would create
		// successfully but fail on every real send. Revisit once
		// Responses-API support is a real, separate design — see that
		// step's own "Open question 3, resolved" section.
		DefaultModel: "gpt-5.6-luna",
		SelectableModels: []string{
			"gpt-6-astra",   // most capable — current flagship
			"gpt-5.6-sol",   // flagship, complex professional work
			"gpt-5.6-terra", // balances intelligence and cost
			"gpt-5.6-luna",  // cost-sensitive workloads — lightest of the current generation
		},
		Completer: openai.Client{},
	},
	{
		Info:           entity.Platform{ID: "anthropic", Name: "Anthropic"},
		DefaultBaseURL: "https://api.anthropic.com/v1",
		// claude-3-x family removed entirely (plan/ai/platform/step-08-anthropic-model-list-revision.md)
		// — several generations behind current; not even in Anthropic's
		// own "Legacy models (still available)" list (which only goes
		// back to 4.x). Verified directly against
		// platform.claude.com/docs/en/models/overview's real comparison
		// table — these are the exact "Claude API ID" column values
		// (the direct Anthropic API's own model identifier, not the
		// Bedrock/Vertex/Foundry variants also listed there).
		// DefaultModel is the current generation's own fastest/
		// cheapest tier.
		DefaultModel: "claude-haiku-4-5-20251001",
		SelectableModels: []string{
			"claude-fable-5-1",          // demanding reasoning, long-horizon agentic work
			"claude-opus-5",             // complex agentic coding and enterprise work
			"claude-sonnet-5",           // best combination of speed and intelligence
			"claude-haiku-4-5-20251001", // fastest, near-frontier intelligence
		},
		Completer: anthropic.Client{},
	},
	{
		Info:           entity.Platform{ID: "openrouter", Name: "OpenRouter"},
		DefaultBaseURL: "https://openrouter.ai/api/v1",
		// "openrouter/free" (plan/ai/platform/step-09-openrouter-free-model.md)
		// — verified directly against openrouter.ai/openrouter/free: a
		// real, distinct router that picks a free model per-request,
		// never a paid one — unlike "openrouter/auto" (never
		// independently verified when first picked), which carries no
		// such guarantee.
		DefaultModel: "openrouter/free", // unchanged; harmless to keep set even though CreateHandler no longer falls back to it now that SelectableModels is non-empty
		// A real select list, starting with just the one verified
		// model (plan/ai/platform/step-10-openrouter-model-selection.md).
		// Four pinned Claude models added (step 40) so
		// openrouter/client.go's own Anthropic prompt-caching support
		// has a real, consistent (non-random) model to actually cache
		// against — openrouter/free's own random-model-per-request
		// routing can never benefit from caching at all. IDs verified
		// directly against OpenRouter's own public, no-auth-required
		// GET https://openrouter.ai/api/v1/models (not guessed, not
		// assumed to mirror the direct Anthropic API's own dated/
		// hyphenated slugs — two of these four don't: OpenRouter's own
		// naming uses dots, not hyphens, for fable-5.1/haiku-4.5, and
		// haiku's own OpenRouter slug carries no date suffix at all).
		// Each confirmed to advertise "tools" in its own
		// supported_parameters at the same time. DefaultModel stays
		// openrouter/free, unchanged — this only adds a choice, it
		// doesn't change what a caller gets without picking one. See
		// plan/ai/conversation/step-40-anthropic-prompt-caching.md.
		SelectableModels: []string{
			"openrouter/free",
			"anthropic/claude-fable-5.1", // demanding reasoning, long-horizon agentic work
			"anthropic/claude-opus-5",    // complex agentic coding and enterprise work
			"anthropic/claude-sonnet-5",  // best combination of speed and intelligence
			"anthropic/claude-haiku-4.5", // fastest, near-frontier intelligence
		},
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
