package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

func TestOpenAIEmptyAnswersWithFailingFinishReasons(t *testing.T) {
	for reason, retryable := range map[string]bool{"error": true, "length": false, "content_filter": false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""},"finish_reason":"` + reason + `"}]}`))
		}))
		_, err := Client{}.ChatCompletion(context.Background(), srv.Client(), srv.URL, "k", "m", []chatcompleter.Message{{Role: "user", Content: "x"}}, nil)
		srv.Close()
		if err == nil || chatcompleter.IsRetryable(err) != retryable {
			t.Errorf("finish_reason %s: err=%v retryable=%v want %v", reason, err, chatcompleter.IsRetryable(err), retryable)
		}
	}
}

func TestOpenAIStatusAndNoChoices(t *testing.T) {
	for status, want := range map[int]bool{401: false, 429: true, 502: true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		_, err := Client{}.ChatCompletion(context.Background(), srv.Client(), srv.URL, "k", "m", nil, nil)
		srv.Close()
		if err == nil || chatcompleter.IsRetryable(err) != want {
			t.Errorf("status %d: %v retryable=%v want %v", status, err, chatcompleter.IsRetryable(err), want)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"choices":[]}`)) }))
	defer srv.Close()
	if _, err := (Client{}).ChatCompletion(context.Background(), srv.Client(), srv.URL, "k", "m", nil, nil); err == nil || !chatcompleter.IsRetryable(err) {
		t.Errorf("no choices: %v", err)
	}
}
