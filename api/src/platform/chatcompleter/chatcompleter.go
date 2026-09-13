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
	"net/http"
)

// Message is the neutral shape every platform's chat exchange uses.
// Deliberately shaped like OpenAI's own wire schema — see
// plan/ai/platform/step-03-chatcompleter-openai-and-openrouter.md.
type Message struct {
	Role    string `json:"role"` // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// Usage carries token counts back from a provider — computed but not
// persisted anywhere in this design (see platform.md's non-goals).
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ChatCompletionResult is what a ChatCompleter returns for one turn.
type ChatCompletionResult struct {
	Message      Message
	FinishReason string
	Usage        Usage
}

// ChatCompleter is what a platform implements. No tools/function-calling
// parameter here — the Conversation feature deliberately does not build
// a tool-calling loop, so this interface stays to exactly what a plain
// chat turn needs. See
// plan/ai/platform/step-03-chatcompleter-openai-and-openrouter.md.
type ChatCompleter interface {
	ChatCompletion(ctx context.Context, client *http.Client, baseURL, apiKey, model string, messages []Message) (*ChatCompletionResult, error)
}
