// Package qwenjson is the tool dialect of Qwen2.5-Coder: the model's chat template renders the tools natively (the tools stay in the
// request), but the model often answers a tool call as TEXT, a JSON object {"name": ..., "arguments": {...}} (alone, in a code fence, or
// after a sentence) instead of the <tool_call> tags the server reads, so no call reaches the conversation. This dialect only reads that
// text back into tool calls, for the tools that were offered. Measured 2026-11-06 on Qwen2.5-Coder-3B-Instruct Q4_K_M with the real
// Katana recipes (plan 196): without it 0 of 3 job-detail runs made a call; with it the job-detail recipe finished 10 of 10.
package qwenjson

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/a-digi/cinqo/src/localai/dialect"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// Name is the name the catalog uses for it.
const Name = "qwen-json"

// Dialect reads text calls; it changes nothing that is sent.
type Dialect struct{}

// New is the dialect, ready to register.
func New() dialect.Dialect { return Dialect{} }

// NativeTools: the tools stay in the request, the template renders them.
func (Dialect) NativeTools() bool { return true }

// Encode sends the messages as they are.
func (Dialect) Encode(messages []chatcompleter.Message, _ []chatcompleter.ToolDef) []chatcompleter.Message {
	return messages
}

// Decode turns a text answer that holds a call (or a list of calls) of an offered tool into tool calls. An answer that already has
// tool calls, and any text that is not such a call, is left as it is.
func (Dialect) Decode(result *chatcompleter.ChatCompletionResult, tools []chatcompleter.ToolDef) {
	if result == nil || len(result.ToolCalls) > 0 {
		return
	}
	offered := make(map[string]bool, len(tools))
	for _, t := range tools {
		offered[t.Name] = true
	}
	text := result.Message.Content
	for i := 0; i < len(text); i++ {
		if text[i] != '[' && text[i] != '{' {
			continue
		}
		var raw json.RawMessage
		dec := json.NewDecoder(strings.NewReader(text[i:]))
		if err := dec.Decode(&raw); err != nil {
			continue
		}
		calls, ok := callsOf(raw, offered)
		if !ok {
			i += int(dec.InputOffset()) - 1 // a value that is not a call is skipped whole
			continue
		}
		result.ToolCalls = calls
		result.Message.ToolCalls = calls
		result.Message.Content = stripFence(text[:i])
		result.FinishReason = "tool_calls"
		return
	}
}

// callsOf reads one JSON value as a call or a list of calls of offered tools; anything else is not a call.
func callsOf(raw json.RawMessage, offered map[string]bool) ([]chatcompleter.ToolCall, bool) {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		list = []json.RawMessage{raw}
	}
	if len(list) == 0 {
		return nil, false
	}
	calls := make([]chatcompleter.ToolCall, 0, len(list))
	for n, item := range list {
		var c struct {
			Name       string          `json:"name"`
			Arguments  json.RawMessage `json:"arguments"`
			Parameters json.RawMessage `json:"parameters"`
		}
		if json.Unmarshal(item, &c) != nil || !offered[c.Name] {
			return nil, false
		}
		args := c.Arguments
		if len(args) == 0 || string(args) == "null" {
			args = c.Parameters
		}
		args, ok := argumentsObject(args)
		if !ok {
			return nil, false
		}
		calls = append(calls, chatcompleter.ToolCall{ID: fmt.Sprintf("call_%d", n+1), Name: c.Name, Arguments: args})
	}
	return calls, true
}

// argumentsObject is the arguments as a JSON object: absent means none, and a string that holds an object (some models write it that
// way) is unwrapped. Anything else is not usable.
func argumentsObject(raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("{}"), true
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		raw = json.RawMessage(asString)
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil, false
	}
	compact, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	return compact, true
}

// stripFence is the sentence before a call, without the code fence the model may have opened in front of it.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	for _, f := range []string{"```json", "```JSON", "```"} {
		s = strings.TrimSpace(strings.TrimSuffix(s, f))
	}
	return s
}
