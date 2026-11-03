// Package phijson is the tool dialect of Phi-4-mini.
package phijson

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/a-digi/cinqo/src/localai/dialect"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// Dialect is Microsoft's documented format for Phi-4-mini: the tools as a JSON list between <|tool|> and <|/tool|> at the end
// of the system message, and the model answers with a JSON list of calls as text (measured: sometimes in a code fence, sometimes
// after a sentence, with "arguments" or "parameters"). Both are read.
type Dialect struct{}

// Name is the name the catalog uses for it.
const Name = "phi-json"

// New is the dialect, ready to register.
func New() dialect.Dialect { return Dialect{} }

// phiRule is told to the model in front of the tools. Measured on Phi-4-mini: 10 of 10 calls for an action ("list my profiles"),
// for the same need as a question, for a call with an argument and with a list, and no call for a question that needs none.
const phiRule = `Use a tool whenever the user asks for something a tool can do, and for any question about the user's own data. To call a tool, reply with ONLY a JSON array like [{"name": "tool_name", "arguments": {"parameter": "value"}}] and nothing else. If no tool is needed, answer normally.`

type phiParam struct {
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

type phiTool struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Parameters  map[string]phiParam `json:"parameters"`
}

// phiTypes maps JSON Schema types to the names of Microsoft's example.
var phiTypes = map[string]string{"string": "str", "integer": "int", "number": "float", "boolean": "bool", "array": "list", "object": "dict"}

func phiToolOf(t chatcompleter.ToolDef) phiTool {
	out := phiTool{Name: t.Name, Description: t.Description, Parameters: map[string]phiParam{}}
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
			Type        any    `json:"type"`
		} `json:"properties"`
	}
	if len(t.InputSchema) == 0 || json.Unmarshal(t.InputSchema, &schema) != nil {
		return out
	}
	for name, p := range schema.Properties {
		typ, _ := p.Type.(string)
		if mapped, ok := phiTypes[typ]; ok {
			typ = mapped
		}
		out.Parameters[name] = phiParam{Description: p.Description, Type: typ}
	}
	return out
}

func (Dialect) Encode(messages []chatcompleter.Message, tools []chatcompleter.ToolDef) []chatcompleter.Message {
	traffic := false
	for _, m := range messages {
		if len(m.ToolCalls) > 0 || m.Role == "tool" {
			traffic = true
		}
	}
	if len(tools) == 0 && !traffic {
		return messages
	}
	out := make([]chatcompleter.Message, 0, len(messages)+1)
	for _, m := range messages {
		switch {
		case len(m.ToolCalls) > 0:
			// The call the model made is shown to it as the text it wrote.
			calls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, c := range m.ToolCalls {
				var args any = map[string]any{}
				if len(c.Arguments) > 0 {
					var parsed any
					if json.Unmarshal(c.Arguments, &parsed) == nil {
						args = parsed
					}
				}
				calls = append(calls, map[string]any{"name": c.Name, "arguments": args})
			}
			text, _ := json.Marshal(calls)
			content := strings.TrimSpace(m.Content)
			if content != "" {
				content += "\n"
			}
			out = append(out, chatcompleter.Message{Role: "assistant", Content: content + string(text)})
		case m.Role == "tool":
			out = append(out, chatcompleter.Message{Role: "tool", Content: m.Content})
		default:
			out = append(out, m)
		}
	}
	if len(tools) == 0 {
		return out
	}
	list := make([]phiTool, 0, len(tools))
	for _, t := range tools {
		list = append(list, phiToolOf(t))
	}
	block, _ := json.Marshal(list)
	text := phiRule + "<|tool|>" + string(block) + "<|/tool|>"
	for i := range out {
		if out[i].Role == "system" {
			out[i].Content = strings.TrimSpace(out[i].Content) + " " + text
			return out
		}
	}
	return append([]chatcompleter.Message{{Role: "system", Content: text}}, out...)
}

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
		dec := json.NewDecoder(strings.NewReader(text[i:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			continue
		}
		calls, ok := phiCallsOf(raw, offered)
		if !ok {
			// A value that is not a call is skipped whole: the calls inside a list with one bad entry are not calls either.
			i += int(dec.InputOffset()) - 1
			continue
		}
		result.ToolCalls = calls
		result.Message.ToolCalls = calls
		result.Message.Content = stripFence(text[:i])
		result.FinishReason = "tool_calls"
		return
	}
}

// phiCallsOf reads one JSON value as a call or a list of calls of offered tools; anything else is not a call.
func phiCallsOf(raw json.RawMessage, offered map[string]bool) ([]chatcompleter.ToolCall, bool) {
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

// argumentsObject is the arguments as a JSON object: absent means none, and a string that holds an object (some models write
// it that way) is unwrapped. Anything else is not usable.
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
