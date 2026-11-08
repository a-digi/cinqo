// Package openrouter implements chatcompleter.ChatCompleter against
// openrouter.ai — an OpenAI-compatible proxy, same request/response
// shape as the openai package, tool-calling included. HTTP-Referer/
// X-Title attribution headers deliberately not carried forward — see
// open question 1 in
// plan/ai/platform/step-03-chatcompleter-openai-and-openrouter.md.
//
// Verified directly, not assumed, that "OpenRouter should use MCP"
// resolves to this same OpenAI-compatible wire shape, not a separate
// or novel one: OpenRouter's own docs
// (openrouter.ai/docs/use-cases/mcp-servers) describe exactly cinqo's
// own architecture — connect to an MCP server, list its tools,
// convert them to OpenAI-compatible tool definitions, and send those
// in a normal chat completions request — their own sample code points
// the plain OpenAI Python client's base_url at
// "https://openrouter.ai/api/v1" and uses standard OpenAI tool-calling
// with no OpenRouter-specific wire differences at all. See
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md.
package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

type Client struct{}

// wireMessage/wireToolCall/wireTool/wireFunctionDef mirror
// openai.Client's own identical types exactly — same real wire shape,
// verified above, so no new format to design here. Not shared code
// (each platform client package stays independent, per this
// codebase's own established convention) — deliberately duplicated,
// not imported from the openai package.
//
// Content is `any`, not `string` (step 40's own extension) — normally
// still a plain string (identical wire output to before), but becomes
// a one-element content-part array carrying a cache_control marker
// for the one message runToolLoop (chat.go) flags with
// CacheBreakpoint, when routing to a model OpenRouter can actually
// apply it for (see supportsAnthropicCacheControl below). Go's
// encoding/json marshals either underlying value correctly with no
// custom MarshalJSON needed.
type wireMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// wireContentPart/wireCacheControl (step 40) — OpenRouter's own
// documented shape for marking a prompt-caching breakpoint when the
// request is routed to an Anthropic model: content becomes an array
// of parts instead of a plain string, and the part to cache up through
// carries cache_control. Mirrors anthropic.Client's own identical
// concept (a separate, independent implementation — see this
// package's own top comment on why nothing is shared across platform
// clients).
type wireContentPart struct {
	Type         string            `json:"type"` // "text"
	Text         string            `json:"text"`
	CacheControl *wireCacheControl `json:"cache_control,omitempty"`
}

type wireCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// supportsAnthropicCacheControl reports whether model is routed to an
// Anthropic model — OpenRouter's own model-ID convention is
// "provider/model-name" (e.g. "anthropic/claude-sonnet-5"), verified
// against this app's own registry.go, which uses exactly that shape
// for every non-OpenRouter-native model slug. Gated this narrowly
// (not "always emit cache_control regardless of model") since sending
// an Anthropic-specific marker to a non-Claude provider's own API is
// unverified territory — could be silently ignored, could be rejected
// by a stricter provider; this codebase doesn't guess at behavior it
// hasn't confirmed.
//
// Flagged, not silently assumed: this app's own OpenRouter registry
// entry (registry.go) currently offers exactly one selectable model,
// "openrouter/free" — a router that picks a random free underlying
// model on every call, with no guarantee the same one serves two
// consecutive requests. This function correctly returns false for it
// (no "anthropic/" prefix), so this whole mechanism is dormant under
// today's actual configuration — implemented for when a real, pinned
// Claude-via-OpenRouter model is added to SelectableModels, not
// something that does anything with the config as it stands. See
// plan/ai/conversation/step-40-anthropic-prompt-caching.md.
func supportsAnthropicCacheControl(model string) bool {
	return strings.HasPrefix(model, "anthropic/")
}

type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireTool struct {
	Type     string          `json:"type"`
	Function wireFunctionDef `json:"function"`
}

type wireFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []wireMessage `json:"messages"`
	Tools    []wireTool    `json:"tools,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message      wireMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Real tool-calling wiring, mirroring openai.Client's own translation
// exactly. One structural caveat remains, unaddressed by this wiring
// and not addressable by it: openrouter/free (step 09) picks a random
// underlying free model per request, so tool-calling support isn't
// guaranteed for any given call the way it is for a fixed OpenAI/
// Anthropic model — if the picked model doesn't support tools, it
// simply never returns tool_calls (a plain text reply, same as today),
// not an error.
func (Client) ChatCompletion(ctx context.Context, client *http.Client, baseURL, apiKey, model string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	body, err := json.Marshal(chatRequest{Model: model, Messages: toWireMessages(messages, supportsAnthropicCacheControl(model)), Tools: toWireTools(tools)})
	if err != nil {
		return nil, fmt.Errorf("openrouter: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openrouter: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openrouter: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openrouter: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("openrouter: parse response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("openrouter: response had no choices")
	}

	choice := parsed.Choices[0]
	toolCalls := make([]chatcompleter.ToolCall, 0, len(choice.Message.ToolCalls))
	for _, tc := range choice.Message.ToolCalls {
		toolCalls = append(toolCalls, chatcompleter.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}

	// choice.Message.Content is always a plain JSON string in a real
	// OpenRouter response (the array-of-parts shape is a request-only
	// concept, for cache_control) — a failed assertion (content
	// somehow absent/non-string) degrades to "", never a panic.
	responseContent, _ := choice.Message.Content.(string)

	return &chatcompleter.ChatCompletionResult{
		Message: chatcompleter.Message{
			Role:      "assistant",
			Content:   responseContent,
			ToolCalls: toolCalls,
		},
		ToolCalls:    toolCalls,
		FinishReason: choice.FinishReason,
		Usage: chatcompleter.Usage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		},
	}, nil
}

func toWireMessages(messages []chatcompleter.Message, cacheControlSupported bool) []wireMessage {
	out := make([]wireMessage, 0, len(messages))
	for _, m := range messages {
		wm := wireMessage{Role: m.Role, ToolCallID: m.ToolCallID}
		if m.CacheBreakpoint && cacheControlSupported {
			wm.Content = []wireContentPart{{Type: "text", Text: m.Content, CacheControl: &wireCacheControl{Type: "ephemeral"}}}
		} else {
			wm.Content = m.Content
		}
		for _, tc := range m.ToolCalls {
			wtc := wireToolCall{ID: tc.ID, Type: "function"}
			wtc.Function.Name = tc.Name
			wtc.Function.Arguments = string(tc.Arguments)
			wm.ToolCalls = append(wm.ToolCalls, wtc)
		}
		out = append(out, wm)
	}
	return out
}

func toWireTools(tools []chatcompleter.ToolDef) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]wireTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, wireTool{
			Type: "function",
			Function: wireFunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return out
}
