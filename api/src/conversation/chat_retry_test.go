package conversation

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// stepCompleter answers its calls from a script: each step is either an
// error or a result.
type stepCompleter struct {
	steps []func() (*chatcompleter.ChatCompletionResult, error)
	calls int
}

func (s *stepCompleter) ChatCompletion(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	i := s.calls
	s.calls++
	if i >= len(s.steps) {
		return nil, errors.New("script exhausted")
	}
	return s.steps[i]()
}

func providerErr() (*chatcompleter.ChatCompletionResult, error) {
	return nil, &chatcompleter.CompletionError{Provider: "p", Retryable: true, Message: "the model provider reported an error (finish reason: error) and returned nothing"}
}

func okAnswer(text string) func() (*chatcompleter.ChatCompletionResult, error) {
	return func() (*chatcompleter.ChatCompletionResult, error) {
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: text}, FinishReason: "stop"}, nil
	}
}

func runRetryLoop(ctx context.Context, c chatcompleter.ChatCompleter, steps *[]string, exchanges *int) (string, error) {
	return runToolLoop(ctx, http.DefaultClient, platform.Entry{Completer: c}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, nil, nil, nil, "u1", "", 0, "conv", nil, "", 0, "direct",
		func(s string) { *steps = append(*steps, s) }, nil,
		func(int, []chatcompleter.Message, []chatcompleter.ToolDef, *chatcompleter.ChatCompletionResult, error) {
			*exchanges++
		}, 0)
}

func fastRetries(t *testing.T) {
	t.Helper()
	old := chatcompleter.RetryWaits
	chatcompleter.RetryWaits = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { chatcompleter.RetryWaits = old })
}

func TestATransientProviderErrorIsRetriedAndTheGoodAnswerIsUsed(t *testing.T) {
	fastRetries(t)
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){providerErr, okAnswer("recovered")}}
	var steps []string
	var exchanges int
	got, err := runRetryLoop(context.Background(), c, &steps, &exchanges)
	if err != nil || got != "recovered" {
		t.Fatalf("got %q, %v", got, err)
	}
	if c.calls != 2 || exchanges != 2 {
		t.Errorf("calls = %d, traced exchanges = %d, want 2 and 2 (every attempt is traced)", c.calls, exchanges)
	}
	if !strings.Contains(strings.Join(steps, "\n"), "provider error") || !strings.Contains(strings.Join(steps, "\n"), "retry 1/2") {
		t.Errorf("the retry must be logged: %v", steps)
	}
}

func TestThreeFailuresInARowEndWithTheSpecificError(t *testing.T) {
	fastRetries(t)
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){providerErr, providerErr, providerErr, okAnswer("never reached")}}
	var steps []string
	var exchanges int
	got, err := runRetryLoop(context.Background(), c, &steps, &exchanges)
	if err == nil || got != "" || !strings.Contains(err.Error(), "finish reason: error") {
		t.Fatalf("got %q, %v", got, err)
	}
	if c.calls != 3 {
		t.Errorf("calls = %d, want 3 (the first plus two retries)", c.calls)
	}
}

func TestADeterministicFailureIsNotRetried(t *testing.T) {
	fastRetries(t)
	cut := func() (*chatcompleter.ChatCompletionResult, error) {
		return nil, &chatcompleter.CompletionError{Provider: "p", Message: "the response was cut off by the token limit (finish reason: length) and returned nothing"}
	}
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){cut, okAnswer("never reached")}}
	var steps []string
	var exchanges int
	if _, err := runRetryLoop(context.Background(), c, &steps, &exchanges); err == nil || c.calls != 1 {
		t.Fatalf("err = %v, calls = %d, want an error after exactly 1 call", err, c.calls)
	}
}

func TestAPlainErrorIsNotRetried(t *testing.T) {
	fastRetries(t)
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){
		func() (*chatcompleter.ChatCompletionResult, error) { return nil, errors.New("something else") },
		okAnswer("never reached"),
	}}
	var steps []string
	var exchanges int
	if _, err := runRetryLoop(context.Background(), c, &steps, &exchanges); err == nil || c.calls != 1 {
		t.Fatalf("err = %v, calls = %d", err, c.calls)
	}
}

func TestCancellingDuringTheWaitStopsAtOnce(t *testing.T) {
	old := chatcompleter.RetryWaits
	chatcompleter.RetryWaits = []time.Duration{time.Minute, time.Minute}
	defer func() { chatcompleter.RetryWaits = old }()

	ctx, cancel := context.WithCancel(context.Background())
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){
		func() (*chatcompleter.ChatCompletionResult, error) { cancel(); return providerErr() },
		okAnswer("never reached"),
	}}
	var steps []string
	var exchanges int
	start := time.Now()
	_, err := runRetryLoop(ctx, c, &steps, &exchanges)
	if !errors.Is(err, context.Canceled) || time.Since(start) > 5*time.Second || c.calls != 1 {
		t.Fatalf("err = %v after %v, calls = %d", err, time.Since(start), c.calls)
	}
}

func TestAnEmptyFinalReplyIsAnError(t *testing.T) {
	fastRetries(t)
	for name, text := range map[string]string{"empty": "", "whitespace": "  \n\t "} {
		// Every attempt is empty: the first call plus the two retries all return nothing, so the error stands.
		c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){okAnswer(text), okAnswer(text), okAnswer(text)}}
		var steps []string
		var exchanges int
		got, err := runRetryLoop(context.Background(), c, &steps, &exchanges)
		if !errors.Is(err, ErrEmptyModelReply) || got != "" {
			t.Errorf("%s: got %q, %v", name, got, err)
		}
	}
}

func TestANonEmptyFinalReplyIsStillASuccess(t *testing.T) {
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){okAnswer("done")}}
	var steps []string
	var exchanges int
	if got, err := runRetryLoop(context.Background(), c, &steps, &exchanges); err != nil || got != "done" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestAnEmptyReplyIsAnErrorAfterToolUseToo(t *testing.T) {
	fastRetries(t)
	db := openMainDB(t)
	seedDomainTool(t, db, "list_jobs", "tool:demo:read")
	scopes := []string{"tool:demo:read"}
	tools, err := offerableToolsForMode(db, scopes, "direct")
	if err != nil {
		t.Fatal(err)
	}
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){
		func() (*chatcompleter.ChatCompletionResult, error) {
			return &chatcompleter.ChatCompletionResult{
				Message:   chatcompleter.Message{Role: "assistant"},
				ToolCalls: []chatcompleter.ToolCall{{ID: "c1", Name: "list_jobs", Arguments: []byte(`{}`)}},
			}, nil
		},
		okAnswer(""), okAnswer(""), okAnswer(""),
	}}
	_, err = runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: c}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, tools, db, scopes, "u1", t.TempDir(), 0, "conv", nil, "", 0, "direct", nil, nil, nil, 0)
	if !errors.Is(err, ErrEmptyModelReply) {
		t.Fatalf("got %v, want ErrEmptyModelReply", err)
	}
}

func emptyAnswer() (*chatcompleter.ChatCompletionResult, error) {
	return &chatcompleter.ChatCompletionResult{
		Message: chatcompleter.Message{Role: "assistant", Content: "  \n"}, FinishReason: "stop",
		Usage: chatcompleter.Usage{PromptTokens: 10, CompletionTokens: 1},
	}, nil
}

func runRetryLoopUsage(c chatcompleter.ChatCompleter, steps *[]string, prompt *int) (string, error) {
	return runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: c}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, nil, nil, nil, "u1", "", 0, "conv", nil, "", 0, "direct",
		func(s string) { *steps = append(*steps, s) },
		func(u chatcompleter.Usage) { *prompt += u.PromptTokens }, nil, 0)
}

func TestAnEmptyReplyIsRetriedAndTheGoodAnswerIsUsed(t *testing.T) {
	fastRetries(t)
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){emptyAnswer, okAnswer("recovered")}}
	var steps []string
	var exchanges int
	got, err := runRetryLoop(context.Background(), c, &steps, &exchanges)
	if err != nil || got != "recovered" {
		t.Fatalf("got %q, %v", got, err)
	}
	if c.calls != 2 || exchanges != 2 {
		t.Errorf("calls = %d, traced exchanges = %d, want 2 and 2", c.calls, exchanges)
	}
	if joined := strings.Join(steps, "\n"); !strings.Contains(joined, "empty response, retry 1/2") {
		t.Errorf("the retry must be logged: %v", steps)
	}
}

func TestThreeEmptyRepliesInARowEndWithTheEmptyReplyError(t *testing.T) {
	fastRetries(t)
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){emptyAnswer, emptyAnswer, emptyAnswer, okAnswer("never reached")}}
	var steps []string
	var exchanges int
	got, err := runRetryLoop(context.Background(), c, &steps, &exchanges)
	if !errors.Is(err, ErrEmptyModelReply) || got != "" {
		t.Fatalf("got %q, %v", got, err)
	}
	if c.calls != 3 {
		t.Errorf("calls = %d, want 3 (the first plus two retries)", c.calls)
	}
}

func TestDiscardedEmptyAttemptsStayInTheTurnUsage(t *testing.T) {
	fastRetries(t)
	c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){emptyAnswer, emptyAnswer, emptyAnswer}}
	var steps []string
	var prompt int
	if _, err := runRetryLoopUsage(c, &steps, &prompt); !errors.Is(err, ErrEmptyModelReply) {
		t.Fatal(err)
	}
	if prompt != 30 {
		t.Errorf("prompt tokens counted = %d, want 30 (all three paid attempts)", prompt)
	}
}

func TestAnAnswerWithOnlyAToolCallIsNotAnEmptyReply(t *testing.T) {
	r := &chatcompleter.ChatCompletionResult{ToolCalls: []chatcompleter.ToolCall{{Name: "x"}}}
	if isEmptyReply(r, 0) {
		t.Error("a tool call is not empty")
	}
	if isEmptyReply(&chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Content: "hi"}}, 0) {
		t.Error("text is not empty")
	}
	if isEmptyReply(&chatcompleter.ChatCompletionResult{}, 1) {
		t.Error("a collected link is not empty")
	}
	if !isEmptyReply(&chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Content: " \t\n"}}, 0) {
		t.Error("white space only is empty")
	}
}
