// Package platform is the compile-time registry of AI providers cinqo
// integrates with — see plan/ai/platform/step-01-platform-registry-and-data-model.md.
// Package-level state, no DI wrapper — same idiom already established
// in this codebase for exactly this kind of static, bootstrap-time
// registry (security/scopes' own registry).
package platform

import (
	"context"
	"errors"
	"sync"

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

	// The fields below are optional seams. They are all nil/false for the
	// three built-in cloud platforms, which then behave exactly as before.
	// A platform added with Register may set them; nothing in this
	// package or in the conversation code knows any platform by name.

	// Credentials, when set, says where this platform is reached and with
	// which token, instead of DefaultBaseURL plus a stored API key. A
	// platform with Credentials needs no API key (RequiresKey is false).
	// An error means the platform cannot answer right now.
	Credentials func(ctx context.Context) (baseURL, token string, err error)
	// Models, when set, is the list of selectable models read at call
	// time (instead of the fixed SelectableModels).
	Models func() []string
	// Available, when set, says whether the platform is offered at all
	// right now; an unavailable platform is not listed and cannot be
	// chosen for a new conversation (existing conversations still find it
	// and fail with a clear reason).
	Available func() bool
	// NoTools means conversations on this platform are offered no tools
	// and run in direct mode only.
	NoTools bool
	// ToolsFor, when set, decides per model how tools are offered (instead of NoTools): ToolsNone, or ToolsExplicit
	// (only the tools a conversation names in its allowlist, in direct mode). A platform that sets it never offers
	// every tool.
	ToolsFor func(model string) ToolPolicy
	// ModelNames, when set, gives a readable name for the models this platform offers (the local AI: "Qwen2.5-Coder-3B-Instruct, 2.0 GiB"
	// for an id such as qwen2.5-coder-3b-instruct-q4_k_m), read at call time. A model without a name is shown by its id. Optional: the cloud
	// platforms set none. It is listed as modelNames, only for the models that are selectable right now.
	ModelNames func() map[string]string
	// ModelUnsuitable, when set, names for each model the tasks it was measured NOT to manage (the local AI: the 1.5B Qwen cannot do Career's
	// CV import), read at call time, so a tool can warn before it starts. A model missing from the map, or with no tasks, says nothing.
	// Optional: the cloud platforms set none. It is listed as unsuitableFor, only for the models that are selectable right now.
	ModelUnsuitable func() map[string][]string
	// ModelDriver, when set, names for each model how a caller should run a long task for it ("code": the caller does the steps in code and asks
	// the model only small questions with no tools; a model that is missing, or has no driver, is left to the platform: ShortSteps or one prompt).
	// Read at call time; listed as drivers, only for the models that are selectable right now. Optional.
	ModelDriver func() map[string]string
	// ShortSteps says the models of this platform follow short, one-action-at-a-time instructions better than a long
	// multi-step prompt (a small local model never called a tool when given the long import prompt, and always did when
	// given the same work as small steps). A caller that can split its work offers it that way; it is listed as
	// needsShortSteps.
	ShortSteps bool
	// ContextTokens, when set, is how much a conversation on this platform can hold at once (the context of the model that
	// answers), read at call time; 0 or nil means the platform states none (the cloud platforms). A caller whose work needs a
	// minimum can then say so before it starts, instead of failing half way. It is listed as contextTokens.
	ContextTokens func() int
	// AnswerRepeatedCalls says a model of this platform may repeat a tool call it already made in the same turn (a real run of a small
	// local model called the same PDF reader five times in a row, each answer the whole document, until the context was full). An
	// identical call (same tool, same arguments) that was already answered in the turn is then not run again: the model is told its
	// result is above and to go on.
	AnswerRepeatedCalls bool
	// ToolHint, when set, is a system message added to a turn that is offered tools (a small model needs telling).
	ToolHint string
}

// ToolPolicy says which tools a conversation on a platform may be offered.
type ToolPolicy int

const (
	// ToolsAll: every tool the caller may use (the built-in platforms).
	ToolsAll ToolPolicy = iota
	// ToolsNone: no tools; direct mode only.
	ToolsNone
	// ToolsExplicit: only the tools the conversation's allowlist names (none without one); direct mode only.
	ToolsExplicit
)

// ToolPolicyFor is how tools are offered to a conversation of this platform that uses model.
func (e Entry) ToolPolicyFor(model string) ToolPolicy {
	if e.NoTools {
		return ToolsNone
	}
	if e.ToolsFor != nil {
		return e.ToolsFor(model)
	}
	return ToolsAll
}

// OffersAllTools is false for a platform whose conversations are never offered every tool (so a form that cannot name an
// allowlist must not offer tool modes for it).
func (e Entry) OffersAllTools() bool { return !e.NoTools && e.ToolsFor == nil }

// RequiresKey reports whether the platform is reached with a stored API
// key (true for every platform without its own Credentials).
func (e Entry) RequiresKey() bool { return e.Credentials == nil }

// IsAvailable reports whether the platform is offered right now.
func (e Entry) IsAvailable() bool { return e.Available == nil || e.Available() }

// SelectableModelList is the platform's selectable models: Models() when
// set, otherwise the fixed list. Never nil.
func (e Entry) SelectableModelList() []string {
	var models []string
	if e.Models != nil {
		models = e.Models()
	} else {
		models = e.SelectableModels
	}
	if models == nil {
		return []string{}
	}
	return models
}

// ErrInvalidEntry is returned by Register for an entry that cannot work.
var ErrInvalidEntry = errors.New("platform: invalid entry")

// mu guards registry: Register may add entries while requests read them.
var mu sync.RWMutex

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

// Register adds a platform to the registry at start-up. The ID must be
// non-empty and not yet registered, and the entry needs a Completer.
func Register(e Entry) error {
	if e.Info.ID == "" || e.Completer == nil {
		return ErrInvalidEntry
	}
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range registry {
		if existing.Info.ID == e.Info.ID {
			return ErrInvalidEntry
		}
	}
	registry = append(registry, e)
	return nil
}

// List returns every registered platform's {id, name} — nothing more,
// matching the reference's own minimal GET /ai/platforms shape.
func List() []entity.Platform {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]entity.Platform, 0, len(registry))
	for _, e := range registry {
		out = append(out, e.Info)
	}
	return out
}

// Lookup returns the full registry entry for id, if registered.
func Lookup(id string) (Entry, bool) {
	mu.RLock()
	defer mu.RUnlock()
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
	for _, m := range entry.SelectableModelList() {
		if m == model {
			return true
		}
	}
	return false
}
