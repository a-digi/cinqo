// Package anthropic implements chatcompleter.ChatCompleter against
// Anthropic's Messages API — genuinely different wire shape from
// OpenAI/OpenRouter (auth header, top-level system prompt, max_tokens
// required, array-of-content-blocks response, differently-named usage
// fields), so this package does real translation to/from the neutral
// chatcompleter types rather than a near-copy of the openai client.
// See plan/ai/platform/step-04-chatcompleter-anthropic.md.
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

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
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

func (Client) ChatCompletion(ctx context.Context, client *http.Client, baseURL, apiKey, model string, messages []chatcompleter.Message) (*chatcompleter.ChatCompletionResult, error) {
	system, turns := splitSystemPrompt(messages)

	reqBody, err := json.Marshal(anthropicRequest{
		Model:     model,
		MaxTokens: defaultMaxTokens,
		System:    system,
		Messages:  turns,
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

	return &chatcompleter.ChatCompletionResult{
		Message:      chatcompleter.Message{Role: "assistant", Content: concatenateTextBlocks(wire.Content)},
		FinishReason: wire.StopReason,
		Usage: chatcompleter.Usage{
			PromptTokens:     wire.Usage.InputTokens,
			CompletionTokens: wire.Usage.OutputTokens,
			TotalTokens:      wire.Usage.InputTokens + wire.Usage.OutputTokens,
		},
	}, nil
}

// splitSystemPrompt pulls any {role:"system"} entries out of the
// neutral message list into Anthropic's own top-level system field —
// Anthropic's messages array only ever contains user/assistant turns.
// Multiple system messages are joined with a blank line.
func splitSystemPrompt(messages []chatcompleter.Message) (string, []anthropicMessage) {
	var systemParts []string
	turns := make([]anthropicMessage, 0, len(messages))

	for _, m := range messages {
		if m.Role == "system" {
			systemParts = append(systemParts, m.Content)
			continue
		}
		turns = append(turns, anthropicMessage{Role: m.Role, Content: m.Content})
	}

	return strings.Join(systemParts, "\n\n"), turns
}

// concatenateTextBlocks joins every type:"text" content block's text —
// Anthropic can return multiple; no tool-use blocks are ever produced
// here since this design has no tool-calling loop.
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
