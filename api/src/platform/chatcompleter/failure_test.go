package chatcompleter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func res(content, finish string, calls ...ToolCall) *ChatCompletionResult {
	return &ChatCompletionResult{Message: Message{Role: "assistant", Content: content}, ToolCalls: calls, FinishReason: finish}
}

func TestClassifyResult(t *testing.T) {
	call := ToolCall{ID: "1", Name: "x"}
	cases := []struct {
		name      string
		r         *ChatCompletionResult
		wantErr   bool
		retryable bool
	}{
		{"provider error, empty", res("", "error"), true, true},
		{"cut off, empty", res("", "length"), true, false},
		{"anthropic max_tokens, empty", res("", "max_tokens"), true, false},
		{"content filter, empty", res("", "content_filter"), true, false},
		{"refusal, empty", res("", "refusal"), true, false},
		{"whitespace only counts as empty", res("  \n", "error"), true, true},
		{"normal stop", res("done", "stop"), false, false},
		{"anthropic end_turn, empty text", res("", "end_turn"), false, false},
		{"tool calls", res("", "tool_calls", call), false, false},
		{"failing reason but it did say something", res("partial", "length"), false, false},
		{"failing reason but it called a tool", res("", "error", call), false, false},
		{"nil result", nil, false, false},
	}
	for _, c := range cases {
		err := ClassifyResult("p", c.r, "")
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, want error %v", c.name, err, c.wantErr)
			continue
		}
		if c.wantErr && IsRetryable(err) != c.retryable {
			t.Errorf("%s: retryable = %v, want %v", c.name, IsRetryable(err), c.retryable)
		}
	}
}

func TestClassifyResultMessageIsSpecificAndKeepsTheProvidersDetail(t *testing.T) {
	err := ClassifyResult("openrouter", res("", "error"), "upstream timed out")
	for _, want := range []string{"openrouter", "provider reported an error", "finish reason: error", "returned nothing", "upstream timed out"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q lacks %q", err.Error(), want)
		}
	}
}

func TestStatusErrorRetryability(t *testing.T) {
	for status, want := range map[int]bool{400: false, 401: false, 404: false, 429: true, 500: true, 502: true, 529: true} {
		if got := IsRetryable(StatusError("p", status, "x")); got != want {
			t.Errorf("status %d: retryable = %v, want %v", status, got, want)
		}
	}
}

func TestTransportErrorNeverRetriesACancelledContext(t *testing.T) {
	if !IsRetryable(TransportError("p", "request failed", errors.New("connection reset"))) {
		t.Error("a dropped connection should be retried")
	}
	if IsRetryable(TransportError("p", "request failed", context.Canceled)) || IsRetryable(TransportError("p", "request failed", context.DeadlineExceeded)) {
		t.Error("a cancelled or timed-out context must not be retried")
	}
}

func TestNoChoicesIsRetryableAndKeepsDetail(t *testing.T) {
	err := NoChoicesError("openrouter", "provider returned error")
	if !IsRetryable(err) || !strings.Contains(err.Error(), "response had no choices: provider returned error") {
		t.Errorf("no choices: %v retryable=%v", err, IsRetryable(err))
	}
}

func TestSleepStopsAtOnceWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := Sleep(ctx, time.Minute); !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Errorf("Sleep on a cancelled context: %v after %v", err, time.Since(start))
	}
	if err := Sleep(context.Background(), time.Millisecond); err != nil {
		t.Errorf("Sleep: %v", err)
	}
}
