package conversation

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

type fakeCompleter struct {
	replies []string
	errs    []error
	calls   int
}

func (f *fakeCompleter) ChatCompletion(ctx context.Context, _ *http.Client, _, _, _ string, _ []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	i := f.calls
	f.calls++
	if len(tools) != 0 {
		return nil, errors.New("judge must not be offered tools")
	}
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	return &chatcompleter.ChatCompletionResult{
		Message: chatcompleter.Message{Role: "assistant", Content: f.replies[i]},
		Usage:   chatcompleter.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
	}, nil
}

func TestNewAcceptanceSpec(t *testing.T) {
	if newAcceptanceSpec("   ", 3) != nil {
		t.Fatal("blank criteria must disable the loop")
	}
	cases := []struct{ in, want int }{{0, 3}, {-1, 3}, {1, 1}, {4, 4}, {99, 5}}
	for _, c := range cases {
		if got := newAcceptanceSpec("x", c.in).MaxAttempts; got != c.want {
			t.Errorf("maxAttempts %d: got %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseVerdict(t *testing.T) {
	v, err := parseVerdict("```json\n{\"accepted\": false, \"feedback\": \" add sources \"}\n```")
	if err != nil || v.Accepted || v.Feedback != "add sources" {
		t.Fatalf("got %+v, %v", v, err)
	}
	if _, err := parseVerdict("looks fine"); err == nil {
		t.Fatal("expected error for reply without JSON")
	}
}

func TestJudgeResultRetriesOnceThenSucceeds(t *testing.T) {
	f := &fakeCompleter{replies: []string{"not json", `{"accepted": true}`}}
	v, usage, err := judgeResult(context.Background(), nil, platform.Entry{Completer: f}, "k", "m", "task", "crit", "res")
	if err != nil || !v.Accepted {
		t.Fatalf("got %+v, %v", v, err)
	}
	if f.calls != 2 || usage.TotalTokens != 24 {
		t.Fatalf("calls=%d usage=%+v", f.calls, usage)
	}
}

func TestJudgeResultGivesUpAfterTwoFailures(t *testing.T) {
	f := &fakeCompleter{replies: []string{"", ""}, errs: []error{errors.New("boom"), errors.New("boom")}}
	if _, _, err := judgeResult(context.Background(), nil, platform.Entry{Completer: f}, "k", "m", "t", "c", "r"); err == nil {
		t.Fatal("expected error")
	}
	if f.calls != 2 {
		t.Fatalf("calls=%d", f.calls)
	}
}

func TestJudgeResultDoesNotRetryWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fakeCompleter{replies: []string{""}, errs: []error{context.Canceled}}
	if _, _, err := judgeResult(ctx, nil, platform.Entry{Completer: f}, "k", "m", "t", "c", "r"); err == nil {
		t.Fatal("expected error")
	}
	if f.calls != 1 {
		t.Fatalf("calls=%d", f.calls)
	}
}

func TestBuildRetryMessages(t *testing.T) {
	m := buildRetryMessages("do x", "prev", "too short")
	if len(m) != 3 || m[0].Role != "user" || m[1].Role != "assistant" || m[1].Content != "prev" || m[2].Role != "user" {
		t.Fatalf("unexpected messages: %+v", m)
	}
}
