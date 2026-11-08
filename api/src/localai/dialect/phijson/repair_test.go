package phijson

import (
	"encoding/json"
	"reflect"
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

// The shape of a real run (2026-11-04, Phi-4-mini, the CV import): the call object was closed after "items", so its "name" came after
// it, and the list closed on the wrong brace. The words are made up; the description has a "}" and a quote inside, as a real one can.
const realClosedTooEarly = `[{"arguments":{"action":"insert","possibleDuplicateOf":null,"company":"Acme Corp","description":"Built tools.\n\n- Used a } brace and a \"quote\".","endDate":"","startDate":"Dec 2022-Present","title":"Software Engineer"},"items":"1"},"name":"cv_import_add_experience"}]`

func decodeWith(text string, names ...string) *chatcompleter.ChatCompletionResult {
	var tools []chatcompleter.ToolDef
	for _, n := range names {
		tools = append(tools, chatcompleter.ToolDef{Name: n})
	}
	r := &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: text}, FinishReason: "stop"}
	Dialect{}.Decode(r, tools)
	return r
}

func TestACallClosedTooEarlyIsReadWithItsNameAfterIt(t *testing.T) {
	r := decodeWith(realClosedTooEarly, "cv_import_add_experience")
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].Name != "cv_import_add_experience" || r.FinishReason != "tool_calls" {
		t.Fatalf("calls %+v finish %q", r.ToolCalls, r.FinishReason)
	}
	// The values are exactly what the model wrote (the keys are re-ordered by the decoder, as for every call): compare as values.
	want := `{"action":"insert","possibleDuplicateOf":null,"company":"Acme Corp","description":"Built tools.\n\n- Used a } brace and a \"quote\".","endDate":"","startDate":"Dec 2022-Present","title":"Software Engineer"}`
	var got, exp map[string]any
	if json.Unmarshal(r.ToolCalls[0].Arguments, &got) != nil || json.Unmarshal([]byte(want), &exp) != nil || !reflect.DeepEqual(got, exp) {
		t.Errorf("the arguments must be what the model wrote, only the brackets moved:\n got %s\nwant %s", r.ToolCalls[0].Arguments, want)
	}
	if _, has := got["items"]; has {
		t.Error("\"items\" belongs to the call object, not to its arguments")
	}
	if r.Message.Content != "" {
		t.Errorf("nothing but the call: %q", r.Message.Content)
	}
}

func TestACallClosedTooEarlyIsOnlyACallWhenItsToolIsOffered(t *testing.T) {
	r := decodeWith(realClosedTooEarly, "list_profiles")
	if len(r.ToolCalls) != 0 || r.Message.Content != realClosedTooEarly || r.FinishReason != "stop" {
		t.Errorf("a tool nobody offered must stay text: %+v", r)
	}
}

func TestACallClosedTooEarlyOutsideAListIsLeftAsText(t *testing.T) {
	// A complete valid object followed by more text: not the slip seen in a real run, and not guessed at.
	text := `Calling it now: {"arguments":{"id":7},"note":"x"},"name":"list_profiles"}`
	if r := decodeWith(text, "list_profiles"); len(r.ToolCalls) != 0 || r.Message.Content != text {
		t.Errorf("%+v", r)
	}
}

func TestRepairReopensAnObjectClosedBeforeItsNextKeyAndOnlyThen(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"in a list":                     {`[{"arguments":{"a":1},"b":2},"name":"t"}]`, `[{"arguments":{"a":1},"b":2,"name":"t"}]`},
		"at the top it is not reopened": {`{"arguments":{"a":1},"b":2},"name":"t"}`, `{"arguments":{"a":1},"b":2}`},
		"white space around the key":    {"[{\"arguments\":{\"a\":1}}\n ,\n \"name\" : \"t\"}]", "[{\"arguments\":{\"a\":1}\n ,\n \"name\" : \"t\"}]"},
		"a key with an escaped quote":   {`[{"a":1},"k\"q":2}]`, `[{"a":1,"k\"q":2}]`},
		"two keys after the close":      {`[{"a":1},"k":2,"m":3}]`, `[{"a":1,"k":2,"m":3}]`},
		"a closer inside a string":      {`[{"a": "x},\"k\": 1"}`, `[{"a": "x},\"k\": 1"}]`},
		// Not this slip: the object is inside another object, so ", "key":" is that object's next key.
		"a nested object keeps its parent's next key": {`[{"name":"t","arguments":{"a":{"x":1},"b":2}]`, `[{"name":"t","arguments":{"a":{"x":1},"b":2}}]`},
		"a value after the comma is not a key":        {`[{"a":{"z":1}},"b"`, `[{"a":{"z":1}},"b"]`},
		"a string that never gets a colon":            {`[{"a":1},"b" "c"`, `[{"a":1},"b" "c"]`},
		"a comma and no string":                       {`[{"a":1},{"b":2}`, `[{"a":1},{"b":2}]`},
		"a closing square bracket is never reopened":  {`[[1],"k":2]`, `[[1],"k":2]`},
		"no comma between the close and the key":      {`[{"a":1} x"k":2}]`, `[{"a":1} x"k":2]`},
		"a bare word is not a key":                    {`[{"a":1},x":2"}]`, `[{"a":1},x":2"]`},
		"brackets left open inside are closed first":  {`[{"a":[[1},"k":2]`, `[{"a":[[1]]},"k":2]`},
	} {
		got, _, ok := repairBrackets(c.in)
		if !ok || got != c.want {
			t.Errorf("%s: %q ok=%v, want %q", name, got, ok, c.want)
		}
	}
}

func TestValidJSONIsNeverTouchedByTheNewRule(t *testing.T) {
	r := decodeWith(`[{"name":"list_profiles","arguments":{"a":{"x":1},"b":2}}]`, "list_profiles")
	if len(r.ToolCalls) != 1 || string(r.ToolCalls[0].Arguments) != `{"a":{"x":1},"b":2}` {
		t.Errorf("%+v", r.ToolCalls)
	}
}
