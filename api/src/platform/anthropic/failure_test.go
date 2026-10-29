package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

func anthropicCall(t *testing.T, status int, body string) (*chatcompleter.ChatCompletionResult, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	return Client{}.ChatCompletion(context.Background(), srv.Client(), srv.URL, "k", "m", []chatcompleter.Message{{Role: "user", Content: "x"}}, nil)
}

func TestAnthropicEmptyAnswersWithFailingStopReasons(t *testing.T) {
	for _, reason := range []string{"max_tokens", "refusal"} {
		_, err := anthropicCall(t, 200, `{"content":[],"stop_reason":"`+reason+`"}`)
		if err == nil || chatcompleter.IsRetryable(err) {
			t.Errorf("stop_reason %s: want a non-retryable error, got %v", reason, err)
		}
	}
}

func TestAnthropicNormalAnswerAndOverloadedStatus(t *testing.T) {
	r, err := anthropicCall(t, 200, `{"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn"}`)
	if err != nil || r.Message.Content != "hi" {
		t.Fatalf("normal: %+v %v", r, err)
	}
	// An empty end_turn is not a failing reason: left to the loop's own rule.
	if _, err := anthropicCall(t, 200, `{"content":[],"stop_reason":"end_turn"}`); err != nil {
		t.Errorf("empty end_turn must pass through: %v", err)
	}
	for status, want := range map[int]bool{400: false, 429: true, 529: true} {
		_, err := anthropicCall(t, status, `{"error":{"message":"busy"}}`)
		if err == nil || chatcompleter.IsRetryable(err) != want {
			t.Errorf("status %d: %v retryable=%v want %v", status, err, chatcompleter.IsRetryable(err), want)
		}
	}
}
