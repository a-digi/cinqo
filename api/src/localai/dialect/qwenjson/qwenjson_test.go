package qwenjson

import (
	"encoding/json"
	"testing"

	"github.com/a-digi/cinqo/src/localai/dialect"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

var tools = []chatcompleter.ToolDef{{Name: "katana_get_instruction_state"}, {Name: "list_profiles"}}

func decode(text string) *chatcompleter.ChatCompletionResult {
	r := &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: text}, FinishReason: "stop"}
	Dialect{}.Decode(r, tools)
	return r
}

func TestItReadsTheCallsAModelWritesAsText(t *testing.T) {
	for name, text := range map[string]string{
		"a fenced object":          "```json\n{\n  \"name\": \"katana_get_instruction_state\",\n  \"arguments\": {\"portalLinkId\": \"abc\"}\n}\n```",
		"a bare object":            `{"name": "katana_get_instruction_state", "arguments": {"portalLinkId": "abc"}}`,
		"after a sentence":         `I will look at the state first. {"name": "katana_get_instruction_state", "arguments": {"portalLinkId": "abc"}}`,
		"a list":                   `[{"name": "katana_get_instruction_state", "arguments": {"portalLinkId": "abc"}}]`,
		"parameters for arguments": `{"name": "katana_get_instruction_state", "parameters": {"portalLinkId": "abc"}}`,
		"arguments as a string":    `{"name": "katana_get_instruction_state", "arguments": "{\"portalLinkId\": \"abc\"}"}`,
	} {
		r := decode(text)
		if len(r.ToolCalls) != 1 || r.ToolCalls[0].Name != "katana_get_instruction_state" || r.FinishReason != "tool_calls" || len(r.Message.ToolCalls) != 1 {
			t.Errorf("%s: %+v", name, r)
			continue
		}
		var args map[string]string
		if err := json.Unmarshal(r.ToolCalls[0].Arguments, &args); err != nil || args["portalLinkId"] != "abc" {
			t.Errorf("%s: arguments %s", name, r.ToolCalls[0].Arguments)
		}
	}
}

func TestOnlyACallOfAnOfferedToolIsRead(t *testing.T) {
	for name, text := range map[string]string{
		"another tool":          `{"name": "delete_everything", "arguments": {}}`,
		"plain text":            "The job list is saved.",
		"json that is no call":  `{"verdict": "ok", "jobs": 3}`,
		"arguments not object":  `{"name": "list_profiles", "arguments": [1, 2]}`,
		"a list with a bad one": `[{"name": "list_profiles", "arguments": {}}, {"name": "delete_everything", "arguments": {}}]`,
		"an unfinished call":    `{"name": "list_profiles", "arguments": {`,
	} {
		r := decode(text)
		if len(r.ToolCalls) != 0 || r.Message.Content != text || r.FinishReason != "stop" {
			t.Errorf("%s must be left alone: %+v", name, r)
		}
	}
}

func TestAnAnswerThatAlreadyHasCallsIsLeftAsItIs(t *testing.T) {
	r := &chatcompleter.ChatCompletionResult{
		Message:   chatcompleter.Message{Content: `{"name": "list_profiles", "arguments": {}}`},
		ToolCalls: []chatcompleter.ToolCall{{ID: "x", Name: "katana_get_instruction_state", Arguments: json.RawMessage(`{}`)}},
	}
	Dialect{}.Decode(r, tools)
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Name != "katana_get_instruction_state" {
		t.Errorf("%+v", r.ToolCalls)
	}
}

func TestItSendsTheMessagesAsTheyAreAndKeepsTheToolsNative(t *testing.T) {
	msgs := []chatcompleter.Message{{Role: "user", Content: "hi"}}
	if got := (Dialect{}).Encode(msgs, tools); len(got) != 1 || got[0].Content != "hi" {
		t.Errorf("%+v", got)
	}
	var d dialect.Dialect = Dialect{}
	n, ok := d.(dialect.NativeToolsDialect)
	if !ok || !n.NativeTools() {
		t.Error("the tools must stay in the request")
	}
	if !dialect.ValidName(Name) {
		t.Errorf("%q is not a valid dialect name", Name)
	}
}
