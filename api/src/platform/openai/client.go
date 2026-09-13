// Package openai implements chatcompleter.ChatCompleter against
// api.openai.com's chat completions endpoint. See
// plan/ai/platform/step-03-chatcompleter-openai-and-openrouter.md.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

type Client struct{}

// wireMessage is OpenAI Chat Completions' own real message shape —
// genuinely different from chatcompleter.Message once tool calls are
// involved (a dedicated tool_calls array with a nested function
// object, arguments as a JSON-encoded *string*, and a distinct "tool"
// role carrying tool_call_id), verified directly against OpenAI's own
// docs (developers.openai.com/api/docs/guides/function-calling) —
// see plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md's
// "Offering tools to the model, concretely". No longer safe to reuse
// chatcompleter.Message as the literal wire type once tools are ever
// non-empty, so this package now translates explicitly in both
// directions instead.
type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
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

func (Client) ChatCompletion(ctx context.Context, client *http.Client, baseURL, apiKey, model string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	body, err := json.Marshal(chatRequest{Model: model, Messages: toWireMessages(messages), Tools: toWireTools(tools)})
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("openai: parse response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("openai: response had no choices")
	}

	choice := parsed.Choices[0]
	toolCalls := make([]chatcompleter.ToolCall, 0, len(choice.Message.ToolCalls))
	for _, tc := range choice.Message.ToolCalls {
		toolCalls = append(toolCalls, chatcompleter.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments), // decode the JSON *string* once, here
		})
	}

	return &chatcompleter.ChatCompletionResult{
		Message: chatcompleter.Message{
			Role:      "assistant",
			Content:   choice.Message.Content,
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

// toWireMessages serializes the neutral history into OpenAI's own
// real shape — a ToolCalls-bearing assistant message becomes
// {"role":"assistant","tool_calls":[...]}, a Role=="tool" message
// becomes {"role":"tool","tool_call_id":...,"content":...}.
func toWireMessages(messages []chatcompleter.Message) []wireMessage {
	out := make([]wireMessage, 0, len(messages))
	for _, m := range messages {
		wm := wireMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
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

// toWireTools translates the neutral tool offering into OpenAI's own
// {"type":"function","function":{name,description,parameters}} shape.
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
