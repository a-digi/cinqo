// Package chatcompleter holds the neutral types every platform's chat
// completion call shares (Message, Usage, ChatCompletionResult) and the
// ChatCompleter interface itself. Deliberately its own leaf package —
// not the root `platform` package — so the openai/openrouter/anthropic
// client packages can depend on it without creating an import cycle
// back through platform's own registry (which in turn depends on those
// client packages to populate each Entry's Completer field). See
// plan/ai/platform/step-03-chatcompleter-openai-and-openrouter.md.
package chatcompleter

import (
	"context"
	"encoding/json"
	"net/http"
)

// Message is the neutral shape every platform's chat exchange uses.
// Deliberately shaped like OpenAI's own wire schema — see
// plan/ai/platform/step-03-chatcompleter-openai-and-openrouter.md.
//
// ToolCalls/ToolCallID widen this for real tool-calling (reversing
// that step's own "no tool-calling loop" non-goal — see
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md): a
// plain {Role, Content} message is unchanged from before; ToolCalls is
// set only on an assistant message replaying a prior turn's tool
// calls as history, ToolCallID only on a "tool"-role message carrying
// one call's result. Each ChatCompleter implementation owns
// translating this neutral shape into its own real wire format —
// OpenAI's and Anthropic's are genuinely different (OpenAI: a
// dedicated "tool" message role; Anthropic: no such role at all, a
// tool_result content block inside a user-role message instead) — see
// that step's own "Offering tools to the model, concretely" section.
type Message struct {
	Role       string     `json:"role"` // "system" | "user" | "assistant" | "tool"
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"-"`
	ToolCallID string     `json:"-"`
	// CacheBreakpoint (step 40) marks this message as the end of a
	// cacheable prefix — set by runToolLoop (chat.go) on the last
	// message of its own byte-identical-across-iterations prefix, so a
	// multi-iteration tool-calling turn doesn't pay full price to
	// reprocess the same history on every call. Only the Anthropic
	// client (the one provider with no automatic prompt caching) acts
	// on this; every other client ignores it entirely. See
	// plan/ai/conversation/step-40-anthropic-prompt-caching.md.
	CacheBreakpoint bool `json:"-"`
}

// ToolDef is one tool offered to the model — translated from a
// tool_mcp_tools cache row (itself sourced from a real MCP tools/list
// response), never invented ad hoc.
type ToolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage // JSON Schema, as cached — no reinterpretation
}

// ToolCall is the model's own request to call one tool — Arguments is
// always a parsed JSON object at this neutral layer; each client
// normalizes its own provider's wire quirk (OpenAI's arguments comes
// over as a JSON-encoded *string* and must be re-decoded before
// reaching here; Anthropic's input already isn't a string).
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Usage carries token counts back from a provider — computed but not
// persisted anywhere in this design (see platform.md's non-goals).
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	// CacheCreationTokens/CacheReadTokens (step 40) — Anthropic-only,
	// always 0 for every other provider (and 0 for Anthropic calls
	// that set no CacheBreakpoint). Surfaced so prompt caching's real
	// effect is directly observable from the provider's own reported
	// numbers, not inferred or assumed. See
	// plan/ai/conversation/step-40-anthropic-prompt-caching.md.
	CacheCreationTokens int
	CacheReadTokens     int
}

// ChatCompletionResult is what a ChatCompleter returns for one turn.
type ChatCompletionResult struct {
	Message      Message
	ToolCalls    []ToolCall // non-empty when the model wants to call one or more tools
	FinishReason string
	Usage        Usage
}

// ChatCompleter is what a platform implements. tools is the
// scope-filtered set of currently-offerable MCP tools (may be empty —
// every existing caller before tool-calling passed none, and every
// implementation must keep working identically when it does). A
// platform with no real tool-calling wiring yet (Anthropic,
// OpenRouter, as of this pass) is free to simply ignore tools and
// never populate ChatCompletionResult.ToolCalls — no behavior change
// for those. See
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md.
type ChatCompleter interface {
	ChatCompletion(ctx context.Context, client *http.Client, baseURL, apiKey, model string, messages []Message, tools []ToolDef) (*ChatCompletionResult, error)
}
