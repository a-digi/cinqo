package shared

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// Plan 183: the conversation id reaches the running tool only from core's environment (TOOL_CONVERSATION_ID), never from the model's arguments.

func TestWithAIContextSetsTheIdFromTheEnvironmentAndKeepsTheRest(t *testing.T) {
	out := withAIContext([]byte(`{"url":"https://a.example/","fields":[{"label":"t"}]}`), "conv-1")
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["aiContextId"] != "conv-1" || got["url"] != "https://a.example/" || got["fields"] == nil {
		t.Errorf("%s", out)
	}
}

func TestWithAIContextOverwritesWhatTheModelSent(t *testing.T) {
	out := withAIContext([]byte(`{"url":"https://a.example/","aiContextId":"someone-elses-conversation"}`), "conv-1")
	var got map[string]any
	_ = json.Unmarshal(out, &got)
	if got["aiContextId"] != "conv-1" {
		t.Errorf("the model's value survived: %s", out)
	}
}

func TestWithAIContextRemovesWhatTheModelSentWhenThereIsNoConversation(t *testing.T) {
	out := withAIContext([]byte(`{"url":"https://a.example/","aiContextId":"forged"}`), "")
	var got map[string]any
	_ = json.Unmarshal(out, &got)
	if _, has := got["aiContextId"]; has || got["url"] != "https://a.example/" {
		t.Errorf("%s", out)
	}
	// nothing to remove: the very same bytes
	in := []byte(`{"url":"https://a.example/"}`)
	if string(withAIContext(in, "")) != string(in) {
		t.Error("a body without the field changed")
	}
}

func TestWithAIContextLeavesAnythingThatIsNotAnObjectAlone(t *testing.T) {
	for _, in := range []string{`[1,2]`, `"x"`, `null`, `not json`, ``} {
		if got := withAIContext([]byte(in), "conv-1"); string(got) != in {
			t.Errorf("%q became %q", in, got)
		}
	}
}

func TestWithAIContextQuotesAnythingInTheIdSafely(t *testing.T) {
	out := withAIContext([]byte(`{}`), `a"b\c`+"\n")
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil || got["aiContextId"] != `a"b\c`+"\n" {
		t.Errorf("%s (%v)", out, err)
	}
}

// End to end through CallSibling: what the running tool receives.
func TestCallSiblingSendsTheConversationIdFromTheEnvironmentOnly(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received = map[string]any{}
		_ = json.Unmarshal(b, &received)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	t.Setenv("TOOL_OWN_PORT", u.Port())

	t.Setenv("TOOL_CONVERSATION_ID", "conv-77")
	if _, err := CallSibling("crawl", []byte(`{"url":"https://a.example/","aiContextId":"forged"}`)); err != nil {
		t.Fatal(err)
	}
	if received["aiContextId"] != "conv-77" {
		t.Errorf("received %v", received)
	}

	t.Setenv("TOOL_CONVERSATION_ID", "")
	if _, err := CallSibling("crawl", []byte(`{"url":"https://a.example/","aiContextId":"forged"}`)); err != nil {
		t.Fatal(err)
	}
	if _, has := received["aiContextId"]; has {
		t.Errorf("a forged id reached the tool without a conversation: %v", received)
	}
}
