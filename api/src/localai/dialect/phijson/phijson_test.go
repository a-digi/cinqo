package phijson

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

func defs() []chatcompleter.ToolDef {
	return []chatcompleter.ToolDef{
		{Name: "list_profiles", Description: "Use this to list the profiles.", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		{Name: "get_persona_details", Description: "Use this to get one persona.", InputSchema: json.RawMessage(
			`{"type":"object","properties":{"persona_id":{"type":"integer","description":"the id"},"fields":{"type":"array"},"deep":{"type":"boolean"},"ratio":{"type":"number"},"name":{"type":"string"},"opts":{"type":"object"},"odd":{"type":["string","null"]}}}`)},
	}
}

func decoded(text string) *chatcompleter.ChatCompletionResult {
	r := &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: text}, FinishReason: "stop"}
	Dialect{}.Decode(r, defs())
	return r
}

func TestPhiGetsItsToolsInTheSystemMessageInMicrosoftsFormat(t *testing.T) {
	in := []chatcompleter.Message{{Role: "system", Content: "Be brief. "}, {Role: "user", Content: "hi"}}
	out := Dialect{}.Encode(in, defs())
	if len(out) != 2 || out[1].Content != "hi" || out[0].Role != "system" {
		t.Fatalf("out = %+v", out)
	}
	want := "Be brief. " + phiRule + `<|tool|>[{"name":"list_profiles","description":"Use this to list the profiles.","parameters":{}},` +
		`{"name":"get_persona_details","description":"Use this to get one persona.","parameters":{` +
		`"deep":{"type":"bool"},"fields":{"type":"list"},"name":{"type":"str"},"odd":{},"opts":{"type":"dict"},` +
		`"persona_id":{"description":"the id","type":"int"},"ratio":{"type":"float"}}}]<|/tool|>`
	if out[0].Content != want {
		t.Errorf("system message:\n got %s\nwant %s", out[0].Content, want)
	}
	if in[0].Content != "Be brief. " {
		t.Error("the caller's messages must not be changed")
	}
	// No system message: one is put in front.
	out = Dialect{}.Encode([]chatcompleter.Message{{Role: "user", Content: "hi"}}, defs()[:1])
	if len(out) != 2 || out[0].Role != "system" || !strings.HasPrefix(out[0].Content, phiRule+"<|tool|>") || out[1].Content != "hi" {
		t.Errorf("out = %+v", out)
	}
	// Nothing to teach and no tool traffic: the messages are sent as they are.
	same := []chatcompleter.Message{{Role: "user", Content: "hi"}}
	if got := (Dialect{}).Encode(same, nil); !reflect.DeepEqual(got, same) {
		t.Errorf("got %+v", got)
	}
}

func TestToolTrafficIsShownToPhiAsPlainText(t *testing.T) {
	in := []chatcompleter.Message{
		{Role: "user", Content: "list"},
		{Role: "assistant", Content: "Looking.", ToolCalls: []chatcompleter.ToolCall{{ID: "c1", Name: "get_persona_details", Arguments: json.RawMessage(`{"persona_id":7}`)}, {ID: "c2", Name: "list_profiles"}}},
		{Role: "tool", Content: `[{"id":1}]`, ToolCallID: "c1"},
	}
	out := Dialect{}.Encode(in, defs())
	// With tools offered a system message is put in front.
	if len(out) != 4 || out[0].Role != "system" {
		t.Fatalf("out = %+v", out)
	}
	if out[2].Role != "assistant" || len(out[2].ToolCalls) != 0 ||
		out[2].Content != "Looking.\n"+`[{"arguments":{"persona_id":7},"name":"get_persona_details"},{"arguments":{},"name":"list_profiles"}]` {
		t.Errorf("assistant = %+v", out[2])
	}
	if out[3].Role != "tool" || out[3].Content != `[{"id":1}]` || out[3].ToolCallID != "" {
		t.Errorf("tool = %+v", out[3])
	}
	if len(in[1].ToolCalls) != 2 || in[2].ToolCallID != "c1" {
		t.Error("the caller's messages must not be changed")
	}
	// Traffic without tools offered (the last turn) is still shown as text, with no system block.
	if got := (Dialect{}).Encode(in, nil); got[1].Content == "" || len(got) != 3 {
		t.Errorf("got %+v", got)
	}
}

func TestPhiReplyIsReadBackIntoToolCalls(t *testing.T) {
	for name, c := range map[string]struct {
		text    string
		name    string
		args    string
		content string
	}{
		"a bare list":                     {`[{"name": "list_profiles", "arguments": {}}]`, "list_profiles", `{}`, ""},
		"parameters instead":              {`[{"name": "get_persona_details", "parameters": {"persona_id": 7}}]`, "get_persona_details", `{"persona_id":7}`, ""},
		"one object, in a fence":          {"```json\n{\"name\": \"get_persona_details\", \"arguments\": {\"persona_id\": 7}}\n```", "get_persona_details", `{"persona_id":7}`, ""},
		"after a sentence":                {"To list your profiles I can use the tool.\n\n[{\"name\": \"list_profiles\", \"arguments\": {}}]", "list_profiles", `{}`, "To list your profiles I can use the tool."},
		"arguments as a string":           {`[{"name": "get_persona_details", "arguments": "{\"persona_id\": 3}"}]`, "get_persona_details", `{"persona_id":3}`, ""},
		"no arguments at all":             {`[{"name": "list_profiles"}]`, "list_profiles", `{}`, ""},
		"a first json that is not a call": {`Pick [1] or [2]: [{"name": "list_profiles", "arguments": {}}]`, "list_profiles", `{}`, "Pick [1] or [2]:"},
	} {
		r := decoded(c.text)
		if len(r.ToolCalls) != 1 || r.ToolCalls[0].Name != c.name || string(r.ToolCalls[0].Arguments) != c.args || r.ToolCalls[0].ID != "call_1" {
			t.Errorf("%s: calls %+v", name, r.ToolCalls)
			continue
		}
		if !reflect.DeepEqual(r.Message.ToolCalls, r.ToolCalls) || r.FinishReason != "tool_calls" || r.Message.Content != c.content {
			t.Errorf("%s: message %+v finish %q", name, r.Message, r.FinishReason)
		}
	}
	r := decoded(`[{"name":"list_profiles"},{"name":"get_persona_details","arguments":{"persona_id":2}}]`)
	if len(r.ToolCalls) != 2 || r.ToolCalls[0].ID != "call_1" || r.ToolCalls[1].ID != "call_2" || r.ToolCalls[1].Name != "get_persona_details" {
		t.Errorf("two calls: %+v", r.ToolCalls)
	}
}

func TestPhiTextThatIsNotACallIsLeftAlone(t *testing.T) {
	for name, text := range map[string]string{
		"plain answer":             "The capital of France is Paris.",
		"a tool nobody offered":    `[{"name": "delete_everything", "arguments": {}}]`,
		"one good and one bad":     `[{"name": "list_profiles"},{"name": "delete_everything"}]`,
		"a string that never ends": `[{"name": "list_profiles", "arguments": {"a": "never closed}`,
		"no bracket at all":        `"name": "list_profiles"`,
		"an empty list":            `[]`,
		"arguments that are text":  `[{"name": "list_profiles", "arguments": "yes please"}]`,
		"arguments as a list":      `[{"name": "list_profiles", "arguments": [1]}]`,
		"json that is no call":     `{"answer": 42}`,
	} {
		r := decoded(text)
		if len(r.ToolCalls) != 0 || len(r.Message.ToolCalls) != 0 || r.Message.Content != text || r.FinishReason != "stop" {
			t.Errorf("%s: must be left as it is: %+v", name, r)
		}
	}
	// No tools offered, or the server already returned calls: nothing to read.
	r := &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Content: `[{"name":"list_profiles"}]`}}
	Dialect{}.Decode(r, nil)
	if len(r.ToolCalls) != 0 {
		t.Error("no tools offered: no calls")
	}
	r = &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Content: `x [{"name":"list_profiles"}]`}, ToolCalls: []chatcompleter.ToolCall{{ID: "n", Name: "list_profiles"}}}
	Dialect{}.Decode(r, defs())
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].ID != "n" || r.Message.Content != `x [{"name":"list_profiles"}]` {
		t.Errorf("native calls must stay: %+v", r)
	}
	Dialect{}.Decode(nil, defs())
}
