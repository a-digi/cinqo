package chatcompleter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CompletionError is a model call that failed in a way the caller can act
// on. Retryable marks the transient ones (an upstream provider error, a
// rate limit, a 5xx, a dropped connection), which are worth trying again;
// a deterministic failure (a token cutoff, a refusal, a 4xx) would fail the
// same way again and is not. Message is plain text, safe to show a user:
// it never contains the request (so never the API key). See
// plan/ai/tools/career/step-92-model-failures-and-low-item-counts.md.
type CompletionError struct {
	Provider  string
	Status    int // HTTP status, 0 when the failure was not an HTTP status
	Retryable bool
	Message   string
	Err       error
}

func (e *CompletionError) Error() string {
	msg := e.Provider + ": " + e.Message
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *CompletionError) Unwrap() error { return e.Err }

// StatusError is a non-2xx HTTP answer. 429 and 5xx are transient.
func StatusError(provider string, status int, detail string) *CompletionError {
	return &CompletionError{
		Provider:  provider,
		Status:    status,
		Retryable: status == 429 || status >= 500,
		Message:   fmt.Sprintf("unexpected status %d: %s", status, detail),
	}
}

// TransportError is a request or read that failed before a usable answer
// arrived. A cancelled or timed-out context is the caller's own decision and
// is never retried.
func TransportError(provider, what string, err error) *CompletionError {
	return &CompletionError{
		Provider:  provider,
		Retryable: !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded),
		Message:   what,
		Err:       err,
	}
}

// NoChoicesError is a 2xx answer without any choice, which providers send
// when the upstream model failed. detail is the provider's own description
// when the body carried one.
func NoChoicesError(provider, detail string) *CompletionError {
	msg := "response had no choices"
	if detail != "" {
		msg += ": " + detail
	}
	return &CompletionError{Provider: provider, Retryable: true, Message: msg}
}

// failingFinishReasons maps a finish or stop reason that, with an empty
// answer, means the call failed rather than finished. The bool is whether
// trying again could help.
var failingFinishReasons = map[string]struct {
	text      string
	retryable bool
}{
	"error":          {"the model provider reported an error", true},
	"length":         {"the response was cut off by the token limit", false},
	"max_tokens":     {"the response was cut off by the token limit", false},
	"content_filter": {"the response was blocked by a content filter", false},
	"refusal":        {"the model refused to answer", false},
}

// ClassifyResult turns a completion that carries nothing — no text and no
// tool calls — with a failing finish reason into an error. A normal stop, a
// tool-call response, or a failing reason that still produced text or tool
// calls is returned untouched (nil): the caller keeps whatever the model
// did say. detail is the provider's own description of the failure, if any.
func ClassifyResult(provider string, r *ChatCompletionResult, detail string) error {
	if r == nil || len(r.ToolCalls) > 0 || strings.TrimSpace(r.Message.Content) != "" {
		return nil
	}
	f, ok := failingFinishReasons[r.FinishReason]
	if !ok {
		return nil
	}
	msg := fmt.Sprintf("%s (finish reason: %s) and returned nothing", f.text, r.FinishReason)
	if detail != "" {
		msg += ": " + detail
	}
	return &CompletionError{Provider: provider, Retryable: f.retryable, Message: msg}
}

// IsRetryable reports whether err is a transient CompletionError.
func IsRetryable(err error) bool {
	var ce *CompletionError
	return errors.As(err, &ce) && ce.Retryable
}

// RetryWaits is the pause before each retry: two retries, so three calls at
// most. A var so tests can shrink it.
var RetryWaits = []time.Duration{time.Second, 3 * time.Second}

// Sleep waits d or until ctx is done, whichever is first.
func Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
