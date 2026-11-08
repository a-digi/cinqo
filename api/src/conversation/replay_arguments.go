package conversation

import (
	"bytes"
	"encoding/json"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// replayableToolCalls is the copy of a model's tool calls that goes back into the history sent with the next request. The
// providers refuse a request whose assistant message holds a tool call with arguments that are not a JSON object ("arguments must
// be a valid JSON object string", HTTP 400), and a weak model does send cut-off arguments such as `{"runId": "..."`; replayed as
// they are, one such call fails the whole turn (the CV import runs of plan 165). A call whose arguments are not a JSON object is
// replayed with `{}`; the tool itself is still invoked with the arguments as the model sent them, so its rejection (which the
// model sees as the tool result) is unchanged. Valid calls and the slice the loop works with are never modified.
func replayableToolCalls(calls []chatcompleter.ToolCall) []chatcompleter.ToolCall {
	out := make([]chatcompleter.ToolCall, len(calls))
	for i, call := range calls {
		if !isJSONObject(call.Arguments) {
			call.Arguments = json.RawMessage(`{}`)
		}
		out[i] = call
	}
	return out
}

func isJSONObject(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	var obj map[string]json.RawMessage
	return json.Unmarshal(trimmed, &obj) == nil
}
