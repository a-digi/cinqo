package phijson

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// The real text of a CV import run (2026-11-03, Phi-4-mini): one "}" short, in a code fence, after a sentence.
const realMalformedCall = "The profile is already stored, so here is the next step:\n\n```json\n" +
	`[{"name": "cv_import_set","arguments":{"runId": "4748b70b-adf8-448a-bd9d-608cbe73c467","section": "profile","value": {"action": "skip", "possibleDuplicateOf": null, "name": "the person's name"}}]` +
	"\n```\n\nPlease replace the name."

func offeredTools() []chatcompleter.ToolDef {
	return []chatcompleter.ToolDef{{Name: "cv_import_set"}, {Name: "list_profiles"}, {Name: "get_persona_details"}}
}

func decodeText(text string) *chatcompleter.ChatCompletionResult {
	r := &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: text}, FinishReason: "stop"}
	Dialect{}.Decode(r, offeredTools())
	return r
}

func TestTheRealMalformedCallOfThePhiRunIsRead(t *testing.T) {
	r := decodeText(realMalformedCall)
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Name != "cv_import_set" || r.FinishReason != "tool_calls" {
		t.Fatalf("calls %+v finish %q", r.ToolCalls, r.FinishReason)
	}
	var args struct {
		RunID   string         `json:"runId"`
		Section string         `json:"section"`
		Value   map[string]any `json:"value"`
	}
	if err := json.Unmarshal(r.ToolCalls[0].Arguments, &args); err != nil {
		t.Fatalf("the arguments must be valid JSON: %v (%s)", err, r.ToolCalls[0].Arguments)
	}
	if args.RunID != "4748b70b-adf8-448a-bd9d-608cbe73c467" || args.Section != "profile" || args.Value["action"] != "skip" || args.Value["name"] != "the person's name" {
		t.Errorf("nothing may be changed but the brackets: %+v", args)
	}
	if r.Message.Content != "The profile is already stored, so here is the next step:" {
		t.Errorf("the sentence before the call stays, without the fence: %q", r.Message.Content)
	}
}

func TestRepairBracketsOnlyPutsBracketsRight(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"one closer short":                 {`[{"a": {"b": 1}]`, `[{"a": {"b": 1}}]`},
		"two closers short":                {`[{"a": {"b": 1}`, `[{"a": {"b": 1}}]`},
		"the text stops early":             {`{"name": "x", "arguments": {"id": 7`, `{"name": "x", "arguments": {"id": 7}}`},
		"a hanging comma":                  {`[{"a": 1},`, `[{"a": 1}]`},
		"a hanging comma and space":        {"[{\"a\": 1} ,  ", `[{"a": 1}]`},
		"a stray closer is skipped":        {`{"a": 1}}`, `{"a": 1}`},
		"a bracket inside a string":        {`[{"a": "}]{"}`, `[{"a": "}]{"}]`},
		"an escaped quote in a string":     {`{"a": "say \"hi\" }"`, `{"a": "say \"hi\" }"}`},
		"an escaped quote hides a closer":  {`{"a": "x\" } y"`, `{"a": "x\" } y"}`},
		"a stray closer inside is skipped": {`{"a": 1] }`, `{"a": 1}`},
		"closers of the wrong kind":        {`{"a": [1, 2}`, `{"a": [1, 2]}`},
		"already complete stays":           {`{"a": [1, 2]}`, `{"a": [1, 2]}`},
		"only the first value is taken":    {`{"a": 1} {"b": 2}`, `{"a": 1}`},
	} {
		got, _, ok := repairBrackets(c.in)
		if !ok || got != c.want {
			t.Errorf("%s: %q ok=%v, want %q", name, got, ok, c.want)
		}
		if ok && json.Unmarshal([]byte(got), new(any)) != nil {
			t.Errorf("%s: the result is not valid JSON: %q", name, got)
		}
	}
	for name, in := range map[string]string{
		"a string that never ends": `{"a": "open`,
		"no bracket first":         `"a": 1}`,
		"nothing":                  ``,
		"text first":               `x {"a": 1}`,
	} {
		if _, _, ok := repairBrackets(in); ok {
			t.Errorf("%s: must not be repaired", name)
		}
	}
	if _, n, _ := repairBrackets(`{"a": 1} tail`); n != len(`{"a": 1}`) {
		t.Errorf("consumed %d, want the length of the value", n)
	}
}

func TestRepairedCallsAreStillCheckedAgainstTheOfferedTools(t *testing.T) {
	for name, text := range map[string]string{
		"a tool nobody offered, one closer short":  `[{"name": "delete_everything", "arguments": {"x": {"y": 1}}]`,
		"one good and one bad, repaired as a list": `[{"name": "list_profiles"}, {"name": "delete_everything", "arguments": {"a": {"b": 1}]`,
		"repaired json that is no call":            `{"answer": {"deep": 42}`,
		"arguments that are text":                  `[{"name": "list_profiles", "arguments": "yes", "x": {"a": 1}]`,
	} {
		r := decodeText(text)
		if len(r.ToolCalls) != 0 || r.Message.Content != text || r.FinishReason != "stop" {
			t.Errorf("%s: must be left as it is: %+v", name, r)
		}
	}
}

func TestARepairedCallAfterANonCallValueIsStillFound(t *testing.T) {
	// "[1]" is not a call and is skipped whole; the malformed call after it is read.
	r := decodeText(`Pick [1]: [{"name": "list_profiles", "arguments": {}]`)
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Name != "list_profiles" || !strings.HasPrefix(r.Message.Content, "Pick [1]:") {
		t.Errorf("%+v", r)
	}
}

func TestTheCallsOfAListThatStopsEarlyAreAllRead(t *testing.T) {
	r := decodeText(`[{"name": "cv_import_set", "arguments": {"section": "profile", "value": {"a": 1}}}, {"name": "list_profiles", "arguments": {}}`)
	if len(r.ToolCalls) != 2 || r.ToolCalls[0].Name != "cv_import_set" || r.ToolCalls[1].Name != "list_profiles" || r.ToolCalls[1].ID != "call_2" {
		t.Errorf("%+v", r.ToolCalls)
	}
}
