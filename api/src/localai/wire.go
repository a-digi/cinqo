package localai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// The local AI's own chat client (plan 253, step 1). The model program (llama.cpp's server) answers the OpenAI chat-completions wire format, but this module does
// not borrow the OpenAI platform's client for it: a change made for OpenAI (a parameter, a header, retry or error handling) must never change how the local AI
// talks to its own program, and a change here must never reach a remote platform. OpenRouter keeps its own copy for the same reason. Only the neutral contract
// (chatcompleter: the message, tool and result types and the error helpers) is shared. boundary_test.go fails if this module imports a platform package again.

// wireMessage is the chat-completions message: a tool call is a nested function object whose arguments are a JSON-encoded string, and a tool's answer has its own role
// with the id of the call it answers.
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

type wireRequest struct {
	Model    string        `json:"model"`
	Messages []wireMessage `json:"messages"`
	Tools    []wireTool    `json:"tools,omitempty"`
}

type wireResponse struct {
	Choices []struct {
		Message      wireMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

// localChat sends one chat request to the model program at baseURL and reads its answer. token is sent as a bearer token only when there is one (the program is
// started on this computer and needs none unless one was configured).
func localChat(ctx context.Context, client *http.Client, baseURL, token, model string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	body, err := json.Marshal(wireRequest{Model: model, Messages: toWireMessages(messages), Tools: toWireTools(tools)})
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", providerName, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", providerName, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, chatcompleter.TransportError(providerName, "request failed", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, chatcompleter.TransportError(providerName, "read response", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, chatcompleter.StatusError(providerName, resp.StatusCode, string(respBody))
	}

	var parsed wireResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("%s: parse response: %w", providerName, err)
	}
	if len(parsed.Choices) == 0 {
		return nil, chatcompleter.NoChoicesError(providerName, "")
	}
	choice := parsed.Choices[0]
	calls := make([]chatcompleter.ToolCall, 0, len(choice.Message.ToolCalls))
	for _, tc := range choice.Message.ToolCalls {
		calls = append(calls, chatcompleter.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: json.RawMessage(tc.Function.Arguments)})
	}
	result := &chatcompleter.ChatCompletionResult{
		Message:      chatcompleter.Message{Role: "assistant", Content: choice.Message.Content, ToolCalls: calls},
		ToolCalls:    calls,
		FinishReason: choice.FinishReason,
		Usage: chatcompleter.Usage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
			CacheReadTokens:  parsed.Usage.PromptTokensDetails.CachedTokens,
		},
	}
	// An empty answer that ended with a failing finish reason is a failed call, not the model's final reply.
	if err := chatcompleter.ClassifyResult(providerName, result, ""); err != nil {
		return nil, err
	}
	return result, nil
}

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

func toWireTools(tools []chatcompleter.ToolDef) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]wireTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, wireTool{Type: "function", Function: wireFunctionDef{Name: t.Name, Description: t.Description, Parameters: t.InputSchema}})
	}
	return out
}
