// Package anthropic implements chatcompleter.ChatCompleter against
// Anthropic's Messages API — genuinely different wire shape from
// OpenAI/OpenRouter (auth header, top-level system prompt, max_tokens
// required, array-of-content-blocks response, differently-named usage
// fields), so this package does real translation to/from the neutral
// chatcompleter types rather than a near-copy of the openai client.
// See plan/ai/platform/step-04-chatcompleter-anthropic.md.
//
// Real tool-calling wiring — verified directly against Anthropic's own
// docs (platform.claude.com/docs/en/agents-and-tools/tool-use/overview),
// not assumed: tools are declared as {name, description, input_schema}
// (no nested "function" key, unlike OpenAI); a tool call comes back as
// a tool_use content BLOCK inside the assistant message
// (stop_reason:"tool_use", content:[{"type":"tool_use","id","name","input"}]
// — input is already a parsed object, not a JSON string like OpenAI's
// arguments); critically, there is no dedicated "tool" role at all — a
// result is fed back as a tool_result content block inside a
// user-role message instead. See
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md's own
// "Offering tools to the model, concretely" section.
package anthropic

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

const anthropicVersion = "2023-06-01"

// No per-conversation/per-platform override in this design — see
// step-04's open question 1.
const defaultMaxTokens = 4096

type Client struct{}

// anthropicContentBlock is used for BOTH request and response content
// — every field anthropic ever needs across text/tool_use/tool_result
// blocks in either direction, `omitempty` everywhere it doesn't apply
// to a given block Type.
type anthropicContentBlock struct {
	Type string `json:"type"` // "text" | "tool_use" | "tool_result"

	Text string `json:"text,omitempty"` // "text"

	ID    string          `json:"id,omitempty"`    // "tool_use"
	Name  string          `json:"name,omitempty"`  // "tool_use"
	Input json.RawMessage `json:"input,omitempty"` // "tool_use" — already a parsed object on the wire, never a string

	ToolUseID string `json:"tool_use_id,omitempty"` // "tool_result"
	Content   string `json:"content,omitempty"`     // "tool_result"
}

type anthropicMessage struct {
	Role    string                  `json:"role"` // "user" | "assistant" — never "tool", never "system"
	Content []anthropicContentBlock `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (Client) ChatCompletion(ctx context.Context, client *http.Client, baseURL, apiKey, model string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	system, turns := toAnthropicMessages(messages)

	reqBody, err := json.Marshal(anthropicRequest{
		Model:     model,
		MaxTokens: defaultMaxTokens,
		System:    system,
		Messages:  turns,
		Tools:     toAnthropicTools(tools),
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/messages", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var wireErr anthropicError
		if json.Unmarshal(respBody, &wireErr) == nil && wireErr.Error.Message != "" {
			return nil, fmt.Errorf("anthropic: unexpected status %d: %s", resp.StatusCode, wireErr.Error.Message)
		}
		return nil, fmt.Errorf("anthropic: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var wire anthropicResponse
	if err := json.Unmarshal(respBody, &wire); err != nil {
		return nil, fmt.Errorf("anthropic: parse response: %w", err)
	}

	toolCalls := make([]chatcompleter.ToolCall, 0, len(wire.Content))
	if wire.StopReason == "tool_use" {
		for _, b := range wire.Content {
			if b.Type != "tool_use" {
				continue
			}
			toolCalls = append(toolCalls, chatcompleter.ToolCall{ID: b.ID, Name: b.Name, Arguments: b.Input})
		}
	}

	return &chatcompleter.ChatCompletionResult{
		Message: chatcompleter.Message{
			Role:      "assistant",
			Content:   concatenateTextBlocks(wire.Content),
			ToolCalls: toolCalls,
		},
		ToolCalls:    toolCalls,
		FinishReason: wire.StopReason,
		Usage: chatcompleter.Usage{
			PromptTokens:     wire.Usage.InputTokens,
			CompletionTokens: wire.Usage.OutputTokens,
			TotalTokens:      wire.Usage.InputTokens + wire.Usage.OutputTokens,
		},
	}, nil
}

// toAnthropicMessages pulls any {role:"system"} entries out into
// Anthropic's own top-level system field (Anthropic's messages array
// only ever contains user/assistant turns), and translates the rest:
//
//   - a plain {role, content} turn becomes one text block.
//   - an assistant message carrying ToolCalls (replaying a prior
//     turn's tool_use) gets one tool_use block per call, alongside a
//     text block if Content is non-empty.
//   - a {role:"tool", ToolCallID, Content} message — a real concept at
//     the neutral layer, but Anthropic has no "tool" role at all — is
//     translated into a user-role message with one tool_result block.
//     Consecutive tool-role messages (the model called more than one
//     tool in a single turn) are merged into ONE user-role message
//     carrying multiple tool_result blocks, matching the pattern
//     Anthropic's own docs show, rather than emitting several
//     back-to-back user messages for a single turn's results.
func toAnthropicMessages(messages []chatcompleter.Message) (string, []anthropicMessage) {
	var systemParts []string
	turns := make([]anthropicMessage, 0, len(messages))

	for _, m := range messages {
		switch {
		case m.Role == "system":
			systemParts = append(systemParts, m.Content)

		case m.Role == "tool":
			block := anthropicContentBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content}
			if last := len(turns) - 1; last >= 0 && turns[last].Role == "user" && isToolResultOnly(turns[last]) {
				turns[last].Content = append(turns[last].Content, block)
			} else {
				turns = append(turns, anthropicMessage{Role: "user", Content: []anthropicContentBlock{block}})
			}

		case len(m.ToolCalls) > 0:
			blocks := make([]anthropicContentBlock, 0, len(m.ToolCalls)+1)
			if m.Content != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropicContentBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Arguments})
			}
			turns = append(turns, anthropicMessage{Role: m.Role, Content: blocks})

		default:
			turns = append(turns, anthropicMessage{Role: m.Role, Content: []anthropicContentBlock{{Type: "text", Text: m.Content}}})
		}
	}

	return strings.Join(systemParts, "\n\n"), turns
}

// isToolResultOnly reports whether msg is a merge-eligible tool_result
// carrier (i.e., built by the m.Role == "tool" branch above) — used so
// a real, ordinary user text turn immediately followed by a tool
// result is never mistaken for another tool result to merge into.
func isToolResultOnly(msg anthropicMessage) bool {
	for _, b := range msg.Content {
		if b.Type != "tool_result" {
			return false
		}
	}
	return len(msg.Content) > 0
}

// toAnthropicTools translates the neutral tool offering into
// Anthropic's own {name, description, input_schema} shape — no
// "function" nesting, unlike OpenAI's.
func toAnthropicTools(tools []chatcompleter.ToolDef) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, anthropicTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return out
}

// concatenateTextBlocks joins every type:"text" content block's text —
// Anthropic can return multiple, alongside any tool_use blocks (which
// this deliberately skips here; those are extracted separately above).
func concatenateTextBlocks(blocks []anthropicContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type != "text" {
			continue
		}
		sb.WriteString(b.Text)
	}
	return sb.String()
}
