package conversation

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// alwaysCallsATool answers every call with a tool call, until it is offered
// no tools — the wrap-up call — when it answers with wrapUp (or fails).
type alwaysCallsATool struct {
	calls       int
	offered     []int // number of tools offered on each call
	lastMessage chatcompleter.Message
	wrapUp      string
	wrapUpErr   error
}

func (a *alwaysCallsATool) ChatCompletion(_ context.Context, _ *http.Client, _, _, _ string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	a.calls++
	a.offered = append(a.offered, len(tools))
	a.lastMessage = messages[len(messages)-1]
	if len(tools) == 0 {
		if a.wrapUpErr != nil {
			return nil, a.wrapUpErr
		}
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: a.wrapUp}, FinishReason: "stop"}, nil
	}
	return &chatcompleter.ChatCompletionResult{
		Message:   chatcompleter.Message{Role: "assistant"},
		ToolCalls: []chatcompleter.ToolCall{{ID: "c", Name: "list_jobs", Arguments: []byte(`{}`)}},
	}, nil
}

func runLimited(t *testing.T, a *alwaysCallsATool, limit int) (string, error) {
	t.Helper()
	db := openMainDB(t)
	seedDomainTool(t, db, "list_jobs", "tool:demo:read")
	scopes := []string{"tool:demo:read"}
	tools, err := offerableToolsForMode(db, scopes, conversation_entity.OrchestrationModeDirect)
	if err != nil {
		t.Fatal(err)
	}
	return runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: a}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, tools, db, scopes, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, limit)
}

func TestALowerLimitStopsEarlyAndAsksTheModelToExplainWithoutTools(t *testing.T) {
	a := &alwaysCallsATool{wrapUp: "I could not find a selector that matches the job entries."}
	_, err := runLimited(t, a, 3)

	if !errors.Is(err, ErrToolIterationLimitReached) {
		t.Fatalf("err = %v, want ErrToolIterationLimitReached", err)
	}
	var le *ToolLimitError
	if !errors.As(err, &le) || le.Limit != 3 {
		t.Fatalf("want a ToolLimitError with Limit 3, got %v", err)
	}
	if a.calls != 4 {
		t.Errorf("calls = %d, want 3 tool rounds + exactly 1 wrap-up call", a.calls)
	}
	if a.offered[3] != 0 || a.offered[0] == 0 {
		t.Errorf("tools offered per call = %v: the wrap-up call must be offered none, the others some", a.offered)
	}
	if a.lastMessage.Role != "user" || a.lastMessage.Content != toolLimitWrapUpPrompt {
		t.Errorf("the wrap-up call must end with the fixed instruction, got %+v", a.lastMessage)
	}
	for _, want := range []string{"stopped after 3 rounds", "The model said: I could not find a selector"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err.Error(), want)
		}
	}
}

func TestAFailingOrEmptyWrapUpStillGivesTheSpecificError(t *testing.T) {
	for name, a := range map[string]*alwaysCallsATool{
		"failing": {wrapUpErr: errors.New("provider down")},
		"empty":   {wrapUp: "   "},
	} {
		_, err := runLimited(t, a, 2)
		if !errors.Is(err, ErrToolIterationLimitReached) || !strings.Contains(err.Error(), "stopped after 2 rounds") {
			t.Errorf("%s: %v", name, err)
		}
		if strings.Contains(err.Error(), "The model said") {
			t.Errorf("%s: no explanation expected: %v", name, err)
		}
	}
}

func TestTheWrapUpExplanationIsTruncated(t *testing.T) {
	a := &alwaysCallsATool{wrapUp: strings.Repeat("x", 5000)}
	_, err := runLimited(t, a, 1)
	var le *ToolLimitError
	if !errors.As(err, &le) || len([]rune(le.Explanation)) > maxToolLimitExplanationLen+1 {
		t.Fatalf("explanation not bounded: %d runes", len([]rune(le.Explanation)))
	}
}

func TestNoLimitMeansTheGlobalDefault(t *testing.T) {
	for _, limit := range []int{0, -1, 16, 1000} {
		a := &alwaysCallsATool{wrapUp: "x"}
		_, err := runLimited(t, a, limit)
		var le *ToolLimitError
		if !errors.As(err, &le) || le.Limit != maxToolIterations || a.calls != maxToolIterations+1 {
			t.Errorf("limit %d: calls = %d, err = %v; want %d rounds + 1 wrap-up", limit, a.calls, err, maxToolIterations)
		}
	}
}

func TestEffectiveToolLimit(t *testing.T) {
	for in, want := range map[int]int{0: 15, -3: 15, 1: 1, 10: 10, 15: 15, 16: 15} {
		if got := effectiveToolLimit(in); got != want {
			t.Errorf("effectiveToolLimit(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestToolLimitOf(t *testing.T) {
	ten := 10
	if toolLimitOf(nil) != 0 || toolLimitOf(&conversation_entity.Conversation{}) != 0 || toolLimitOf(&conversation_entity.Conversation{ToolIterationLimit: &ten}) != 10 {
		t.Error("toolLimitOf must read the conversation's own limit, 0 when unset")
	}
}

// The entity package cannot import this one, so it keeps its own copy of the
// ceiling; this pins the two together.
func TestTheEntityCeilingMatchesTheLoopCeiling(t *testing.T) {
	if conversation_entity.MaxToolIterationLimit != MaxToolIterations || MaxToolIterations != maxToolIterations {
		t.Errorf("ceilings differ: entity %d, exported %d, loop %d", conversation_entity.MaxToolIterationLimit, MaxToolIterations, maxToolIterations)
	}
}
