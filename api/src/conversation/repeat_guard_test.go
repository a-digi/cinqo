package conversation

import (
	"context"
	"database/sql"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/tool/mcp"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// argScript calls the given tool calls in order, one per model call, and records every tool result the model got back.
type argScript struct {
	calls   []chatcompleter.ToolCall
	next    int
	results []string
}

func (s *argScript) ChatCompletion(_ context.Context, _ *http.Client, _, _, _ string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	if m := messages[len(messages)-1]; m.Role == "tool" {
		s.results = append(s.results, m.Content)
	}
	if s.next >= len(s.calls) || len(tools) == 0 {
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: "done"}, FinishReason: "stop"}, nil
	}
	call := s.calls[s.next]
	s.next++
	return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant"}, ToolCalls: []chatcompleter.ToolCall{call}}, nil
}

func callOf(name, args string) chatcompleter.ToolCall {
	return chatcompleter.ToolCall{ID: "c", Name: name, Arguments: []byte(args)}
}

func runRepeat(t *testing.T, entry platform.Entry, calls ...chatcompleter.ToolCall) (*argScript, []string) {
	t.Helper()
	var ran []string
	old := invokeToolFn
	invokeToolFn = func(_ context.Context, _ *sql.DB, _ []string, _ string, call chatcompleter.ToolCall, _ string, _ int, _ string) (string, []mcp.ResourceLink, *pendingDelegation) {
		ran = append(ran, call.Name+string(call.Arguments))
		if call.Name == "rejected" {
			return `validating "arguments": validating root: missing property`, nil, nil
		}
		return "result of " + call.Name, nil, nil
	}
	t.Cleanup(func() { invokeToolFn = old })
	s := &argScript{calls: calls}
	entry.Completer = s
	_, err := runToolLoop(context.Background(), http.DefaultClient, entry, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, offered(), nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	return s, ran
}

func TestAnIdenticalCallAlreadyAnsweredInTheTurnIsNotRunAgain(t *testing.T) {
	same := callOf("list_personas", `{"id":"1"}`)
	s, ran := runRepeat(t, platform.Entry{AnswerRepeatedCalls: true}, same, same, same)
	if !reflect.DeepEqual(ran, []string{`list_personas{"id":"1"}`}) {
		t.Fatalf("the tool ran %v, want once", ran)
	}
	if len(s.results) != 3 || s.results[0] != "result of list_personas" {
		t.Fatalf("results = %v", s.results)
	}
	for _, r := range s.results[1:] {
		if !strings.Contains(r, "You already made this exact list_personas call in round 1 of this turn") || len(r) > 250 {
			t.Errorf("the repeated answer must be short and name the round: %q", r)
		}
	}
}

func TestWithoutTheSeamEveryCallRunsAsBefore(t *testing.T) {
	same := callOf("list_personas", `{"id":"1"}`)
	_, ran := runRepeat(t, platform.Entry{}, same, same, same)
	if len(ran) != 3 {
		t.Errorf("a platform that did not ask runs every call: %v", ran)
	}
}

func TestCallsThatDifferAreNotRepeats(t *testing.T) {
	_, ran := runRepeat(t, platform.Entry{AnswerRepeatedCalls: true},
		callOf("list_personas", `{"id":"1"}`), callOf("list_personas", `{"id":"2"}`), callOf("get_persona_details", `{"id":"1"}`), callOf("list_personas", `{"id":"1"}`))
	if len(ran) != 3 {
		t.Errorf("ran %v: only the last call repeats the first", ran)
	}
}

func TestTheSameArgumentsInAnotherOrderAreTheSameCall(t *testing.T) {
	_, ran := runRepeat(t, platform.Entry{AnswerRepeatedCalls: true},
		callOf("list_personas", `{"a":1,"b":2}`), callOf("list_personas", `{ "b": 2, "a": 1 }`))
	if len(ran) != 1 {
		t.Errorf("ran %v: key order and spacing do not make a different call", ran)
	}
}

func TestARejectedCallIsNotRememberedSoTheModelCanCorrectItself(t *testing.T) {
	bad := callOf("rejected", `{}`)
	_, ran := runRepeat(t, platform.Entry{AnswerRepeatedCalls: true}, bad, bad)
	if len(ran) != 2 {
		t.Errorf("ran %v: a call whose arguments were refused must be allowed again", ran)
	}
}

func TestTheGuardDoesNotCarryOverToAnotherTurn(t *testing.T) {
	same := callOf("list_personas", `{"id":"1"}`)
	_, ran1 := runRepeat(t, platform.Entry{AnswerRepeatedCalls: true}, same)
	_, ran2 := runRepeat(t, platform.Entry{AnswerRepeatedCalls: true}, same)
	if len(ran1) != 1 || len(ran2) != 1 {
		t.Errorf("each turn runs its own first call: %v %v", ran1, ran2)
	}
}
