package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

func callWith(t *testing.T, status int, body string) (*chatcompleter.ChatCompletionResult, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	return Client{}.ChatCompletion(context.Background(), srv.Client(), srv.URL, "key", "m", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
}

// The failure from the Stepstone run: a normal-looking 200 whose only choice
// finished with "error" and carries nothing.
func TestFinishReasonErrorWithNothingIsARetryableError(t *testing.T) {
	_, err := callWith(t, 200, `{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"error"}],"usage":{"prompt_tokens":34752,"completion_tokens":107}}`)
	if err == nil || !chatcompleter.IsRetryable(err) || !strings.Contains(err.Error(), "finish reason: error") {
		t.Fatalf("want a retryable finish-reason error, got %v", err)
	}
}

func TestChoiceErrorDescriptionIsKept(t *testing.T) {
	_, err := callWith(t, 200, `{"choices":[{"message":{"content":""},"finish_reason":"error","error":{"message":"upstream timed out"}}]}`)
	if err == nil || !strings.Contains(err.Error(), "upstream timed out") {
		t.Fatalf("the provider's own description should be in the error: %v", err)
	}
}

func TestNoChoicesKeepsTheTopLevelErrorAndIsRetryable(t *testing.T) {
	_, err := callWith(t, 200, `{"error":{"message":"provider returned error"}}`)
	if err == nil || !chatcompleter.IsRetryable(err) || !strings.Contains(err.Error(), "response had no choices: provider returned error") {
		t.Fatalf("got %v", err)
	}
}

func TestLengthCutoffIsAnErrorButNotRetryable(t *testing.T) {
	_, err := callWith(t, 200, `{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`)
	if err == nil || chatcompleter.IsRetryable(err) {
		t.Fatalf("want a non-retryable error, got %v", err)
	}
}

func TestHTTPStatusRetryability(t *testing.T) {
	for status, want := range map[int]bool{400: false, 401: false, 429: true, 500: true, 503: true} {
		_, err := callWith(t, status, `{"error":"x"}`)
		if err == nil || chatcompleter.IsRetryable(err) != want {
			t.Errorf("status %d: err=%v retryable=%v want %v", status, err, chatcompleter.IsRetryable(err), want)
		}
	}
}

func TestNormalAnswersAreUntouched(t *testing.T) {
	r, err := callWith(t, 200, `{"choices":[{"message":{"content":"all good"},"finish_reason":"stop"}]}`)
	if err != nil || r.Message.Content != "all good" || r.FinishReason != "stop" {
		t.Fatalf("normal stop: %+v %v", r, err)
	}
	r, err = callWith(t, 200, `{"choices":[{"message":{"content":"","tool_calls":[{"id":"1","type":"function","function":{"name":"x","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
	if err != nil || len(r.ToolCalls) != 1 {
		t.Fatalf("tool call: %+v %v", r, err)
	}
	// A cut-off that still said something is kept: the caller decides.
	r, err = callWith(t, 200, `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`)
	if err != nil || r.Message.Content != "partial" {
		t.Fatalf("partial answer: %+v %v", r, err)
	}
}
