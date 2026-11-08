package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/tool/mcp"
)

func TestOnlyArgumentsThatAreNotAJSONObjectAreReplacedForTheReplay(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"object":            {`{"runId":"a"}`, `{"runId":"a"}`},
		"empty object":      {`{}`, `{}`},
		"spaced object":     {"  {\"a\": 1}\n", "  {\"a\": 1}\n"},
		"cut off":           {`{"runId": "0d293c19-d434"`, `{}`},
		"stray bracket":     {`{"runId": "0d293c19")`, `{}`},
		"empty":             {``, `{}`},
		"null":              {`null`, `{}`},
		"array":             {`[1]`, `{}`},
		"string":            {`"{\"a\":1}"`, `{}`},
		"text":              {`hello`, `{}`},
		"nested but broken": {`{"a":{"b":1}`, `{}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			calls := []chatcompleter.ToolCall{{ID: "1", Name: "x", Arguments: json.RawMessage(tc.in)}}
			got := replayableToolCalls(calls)
			if string(got[0].Arguments) != tc.want || got[0].ID != "1" || got[0].Name != "x" {
				t.Errorf("replayed %q, want %q", got[0].Arguments, tc.want)
			}
			if string(calls[0].Arguments) != tc.in {
				t.Error("the calls the loop works with were modified")
			}
		})
	}
}

// brokenThenDone sends one tool call with cut-off arguments, then records the assistant tool call it is sent back.
type brokenThenDone struct {
	calls    int
	replayed json.RawMessage
}

func (b *brokenThenDone) ChatCompletion(_ context.Context, _ *http.Client, _, _, _ string, messages []chatcompleter.Message, _ []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	b.calls++
	if b.calls == 1 {
		return &chatcompleter.ChatCompletionResult{
			Message:   chatcompleter.Message{Role: "assistant"},
			ToolCalls: []chatcompleter.ToolCall{{ID: "c", Name: "crawl_paginated", Arguments: json.RawMessage(`{"runId": "0d293c19-d434"`)}},
		}, nil
	}
	for _, m := range messages {
		for _, c := range m.ToolCalls {
			b.replayed = c.Arguments
		}
	}
	return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: "done"}, FinishReason: "stop"}, nil
}

// The tool still gets the arguments exactly as the model sent them (so its rejection reaches the model), but the next request
// carries `{}` for that call.
func TestACutOffCallIsReplayedAsAnEmptyObjectButTheToolStillGetsWhatWasSent(t *testing.T) {
	var invoked json.RawMessage
	withTools(t, func(string, int) string { return "the arguments are not valid JSON" })
	inner := invokeToolFn
	invokeToolFn = func(ctx context.Context, db2 *sql.DB, scopes []string, user string, call chatcompleter.ToolCall, dir string, port int, conv string) (string, []mcp.ResourceLink, *pendingDelegation) {
		invoked = call.Arguments
		return inner(ctx, db2, scopes, user, call, dir, port, conv)
	}
	s := &brokenThenDone{}
	got, err := runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: s}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, crawlTools(), nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, 0)
	if err != nil || got != "done" {
		t.Fatalf("%q %v", got, err)
	}
	if string(s.replayed) != `{}` {
		t.Errorf("the next request carried %q, want {}", s.replayed)
	}
	if string(invoked) != `{"runId": "0d293c19-d434"` {
		t.Errorf("the tool got %q", invoked)
	}
}
