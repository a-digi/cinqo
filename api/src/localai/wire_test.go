package localai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// The local AI's own chat client (plan 253, step 1), checked against a fake model program.

type seen struct {
	path, auth, contentType string
	body                    map[string]any
}

func fakeProgram(t *testing.T, status int, answer string) (*httptest.Server, *seen) {
	t.Helper()
	got := &seen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path, got.auth, got.contentType = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

const plainAnswer = `{"choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10,"prompt_tokens_details":{"cached_tokens":4}}}`

func askLocal(t *testing.T, srv *httptest.Server, token string, msgs []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	t.Helper()
	return localChat(context.Background(), srv.Client(), srv.URL, token, "phi", msgs, tools)
}

func TestTheRequestIsTheChatCompletionsShape(t *testing.T) {
	srv, got := fakeProgram(t, 200, plainAnswer)
	msgs := []chatcompleter.Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []chatcompleter.ToolCall{{ID: "c1", Name: "look", Arguments: json.RawMessage(`{"a":1}`)}}},
		{Role: "tool", ToolCallID: "c1", Content: "found"},
	}
	tools := []chatcompleter.ToolDef{{Name: "look", Description: "looks", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	if _, err := askLocal(t, srv, "secret", msgs, tools); err != nil {
		t.Fatal(err)
	}
	if got.path != "/chat/completions" || got.contentType != "application/json" || got.auth != "Bearer secret" {
		t.Errorf("path %q, content type %q, auth %q", got.path, got.contentType, got.auth)
	}
	if got.body["model"] != "phi" {
		t.Errorf("model = %v", got.body["model"])
	}
	m := got.body["messages"].([]any)
	if len(m) != 4 {
		t.Fatalf("messages = %v", m)
	}
	call := m[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	fn := call["function"].(map[string]any)
	if call["id"] != "c1" || call["type"] != "function" || fn["name"] != "look" || fn["arguments"] != `{"a":1}` {
		t.Errorf("the tool call is %v (its arguments must be a JSON-encoded string)", call)
	}
	answer := m[3].(map[string]any)
	if answer["role"] != "tool" || answer["tool_call_id"] != "c1" || answer["content"] != "found" {
		t.Errorf("the tool answer is %v", answer)
	}
	tl := got.body["tools"].([]any)[0].(map[string]any)
	tf := tl["function"].(map[string]any)
	if tl["type"] != "function" || tf["name"] != "look" || tf["description"] != "looks" || tf["parameters"].(map[string]any)["type"] != "object" {
		t.Errorf("the tool is %v", tl)
	}
}

func TestNoToolsMeansNoToolsField(t *testing.T) {
	srv, got := fakeProgram(t, 200, plainAnswer)
	if _, err := askLocal(t, srv, "", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, has := got.body["tools"]; has {
		t.Errorf("a request without tools must not carry a tools field: %v", got.body)
	}
	if _, has := got.body["tool_calls"]; has {
		t.Errorf("unexpected tool_calls: %v", got.body)
	}
	m := got.body["messages"].([]any)[0].(map[string]any)
	if _, has := m["tool_calls"]; has {
		t.Errorf("a plain message must not carry tool_calls (not even null): %v", m)
	}
	if _, has := m["tool_call_id"]; has {
		t.Errorf("a plain message must not carry tool_call_id: %v", m)
	}
}

func TestTheBearerTokenIsSentOnlyWhenThereIsOne(t *testing.T) {
	srv, got := fakeProgram(t, 200, plainAnswer)
	if _, err := askLocal(t, srv, "", []chatcompleter.Message{{Role: "user", Content: "x"}}, nil); err != nil {
		t.Fatal(err)
	}
	if got.auth != "" {
		t.Errorf("no token, yet Authorization = %q", got.auth)
	}
}

func TestTheAnswerIsReadWithItsUsageAndFinishReason(t *testing.T) {
	srv, _ := fakeProgram(t, 200, plainAnswer)
	r, err := askLocal(t, srv, "k", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Message.Role != "assistant" || r.Message.Content != "hello" || r.FinishReason != "stop" || len(r.ToolCalls) != 0 {
		t.Errorf("result = %+v", r)
	}
	u := r.Usage
	if u.PromptTokens != 7 || u.CompletionTokens != 3 || u.TotalTokens != 10 || u.CacheReadTokens != 4 || u.CacheCreationTokens != 0 {
		t.Errorf("usage = %+v", u)
	}
}

func TestToolCallsOfTheAnswerKeepTheirArgumentsAsJSON(t *testing.T) {
	srv, _ := fakeProgram(t, 200, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"a1","type":"function","function":{"name":"look","arguments":"{\"x\":[1,2]}"}}]},"finish_reason":"tool_calls"}]}`)
	r, err := askLocal(t, srv, "k", []chatcompleter.Message{{Role: "user", Content: "hi"}}, []chatcompleter.ToolDef{{Name: "look"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.ToolCalls) != 1 || r.ToolCalls[0].ID != "a1" || r.ToolCalls[0].Name != "look" || string(r.ToolCalls[0].Arguments) != `{"x":[1,2]}` {
		t.Fatalf("tool calls = %+v", r.ToolCalls)
	}
	if len(r.Message.ToolCalls) != 1 || r.Message.ToolCalls[0].Name != "look" {
		t.Errorf("the message must carry the calls too: %+v", r.Message)
	}
}

func TestFailuresAreLabelledAsTheLocalAIAndSayIfTheyCanBeRetried(t *testing.T) {
	for _, c := range []struct {
		status    int
		retryable bool
	}{{400, false}, {401, false}, {404, false}, {429, true}, {500, true}, {503, true}} {
		srv, _ := fakeProgram(t, c.status, `{"error":"nope"}`)
		_, err := askLocal(t, srv, "k", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
		var ce *chatcompleter.CompletionError
		if !errors.As(err, &ce) {
			t.Fatalf("status %d: %v", c.status, err)
		}
		if ce.Provider != "local AI" || ce.Status != c.status || ce.Retryable != c.retryable || !strings.Contains(ce.Message, `{"error":"nope"}`) {
			t.Errorf("status %d: %+v", c.status, ce)
		}
	}
}

// the module reads the model program's own context-overflow answer from the failure text (managed.go), so the body must stay in it
func TestTheProgramsErrorBodyReachesTheFailureMessage(t *testing.T) {
	body := `{"error":{"n_prompt_tokens":4210,"n_ctx":4096,"message":"the request exceeds the available context size"}}`
	srv, _ := fakeProgram(t, 400, body)
	_, err := askLocal(t, srv, "k", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
	n, ctx, ok := overflowNumbers(err.Error())
	if !ok || n != 4210 || ctx != 4096 {
		t.Errorf("overflowNumbers = %d %d %v for %v", n, ctx, ok, err)
	}
}

func TestAnAnswerWithoutChoicesOrWithBadJSONIsAnError(t *testing.T) {
	srv, _ := fakeProgram(t, 200, `{"choices":[]}`)
	_, err := askLocal(t, srv, "k", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
	var ce *chatcompleter.CompletionError
	if !errors.As(err, &ce) || ce.Provider != "local AI" || !ce.Retryable || !strings.Contains(ce.Message, "no choices") {
		t.Errorf("no choices: %v", err)
	}
	srv2, _ := fakeProgram(t, 200, `not json`)
	_, err = askLocal(t, srv2, "k", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil || !strings.HasPrefix(err.Error(), "local AI: parse response") {
		t.Errorf("bad json: %v", err)
	}
}

func TestAnEmptyAnswerThatEndedWithALengthStopIsAFailedCall(t *testing.T) {
	srv, _ := fakeProgram(t, 200, `{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"length"}]}`)
	_, err := askLocal(t, srv, "k", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
	var ce *chatcompleter.CompletionError
	if !errors.As(err, &ce) || ce.Provider != "local AI" || !strings.Contains(ce.Message, "length") {
		t.Errorf("err = %v", err)
	}
}

func TestAProgramThatIsGoneIsARetryableTransportFailureButACancelIsNot(t *testing.T) {
	srv, _ := fakeProgram(t, 200, plainAnswer)
	url := srv.URL
	srv.Close()
	_, err := localChat(context.Background(), &http.Client{Timeout: 2 * time.Second}, url, "k", "phi", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
	var ce *chatcompleter.CompletionError
	if !errors.As(err, &ce) || ce.Provider != "local AI" || !ce.Retryable || ce.Message != "request failed" {
		t.Errorf("gone: %v", err)
	}
	live, _ := fakeProgram(t, 200, plainAnswer)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = localChat(cancelled, live.Client(), live.URL, "k", "phi", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
	if !errors.As(err, &ce) || ce.Retryable {
		t.Errorf("cancelled: %v", err)
	}
}

func TestTheEngineUsesThisClient(t *testing.T) {
	// the real engine is wired to localChat (not to another platform's client)
	e := newManagedEngine(nil, nil, nil)
	srv, got := fakeProgram(t, 200, plainAnswer)
	r, err := e.call(context.Background(), srv.Client(), srv.URL, "tok", "phi", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil || r.Message.Content != "hello" || got.path != "/chat/completions" || got.auth != "Bearer tok" {
		t.Errorf("%v %+v %+v", err, r, got)
	}
}

func TestAnyStatusOutsideTheSuccessRangeIsAFailure(t *testing.T) {
	for _, status := range []int{300, 301, 302} {
		srv, _ := fakeProgram(t, status, plainAnswer)
		srv.Client().CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		_, err := askLocal(t, srv, "k", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
		var ce *chatcompleter.CompletionError
		if !errors.As(err, &ce) || ce.Status != status {
			t.Errorf("status %d: %v", status, err)
		}
	}
}

func TestAnAnswerCutOffHalfWayIsARetryableReadFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "500")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"choices":`))
	}))
	defer srv.Close()
	_, err := localChat(context.Background(), srv.Client(), srv.URL, "k", "phi", []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
	var ce *chatcompleter.CompletionError
	if !errors.As(err, &ce) || ce.Message != "read response" || !ce.Retryable || ce.Provider != "local AI" {
		t.Errorf("err = %v", err)
	}
}
