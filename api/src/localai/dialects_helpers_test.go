package localai

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/a-digi/cinqo/src/localai/dialect"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

const qwenID = "qwen2.5-1.5b-instruct-q4_k_m"

func withDialects(t *testing.T, l dialect.Lookup) {
	t.Helper()
	old := currentDialects()
	setDialects(l)
	t.Cleanup(func() { setDialects(old) })
}

func toolDefs() []chatcompleter.ToolDef {
	return []chatcompleter.ToolDef{{Name: "list_profiles", Description: "Use this to list the profiles.", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}}
}

type recordingEngine struct {
	gotMessages []chatcompleter.Message
	gotTools    []chatcompleter.ToolDef
	reply       string
}

func (e *recordingEngine) Ready(context.Context) error { return nil }

func (e *recordingEngine) Chat(_ context.Context, _ *http.Client, _ string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	e.gotMessages, e.gotTools = messages, tools
	return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: e.reply}, FinishReason: "stop"}, nil
}

// stampDialect is a dialect that is not Phi's: it marks what it was asked to do, so a test can tell it from the real one.
type stampDialect struct{}

func (stampDialect) Encode(m []chatcompleter.Message, _ []chatcompleter.ToolDef) []chatcompleter.Message {
	return append([]chatcompleter.Message{{Role: "system", Content: "STAMP"}}, m...)
}

func (stampDialect) Decode(r *chatcompleter.ChatCompletionResult, _ []chatcompleter.ToolDef) {
	r.Message.Content = "decoded: " + r.Message.Content
}

func registryWith(t *testing.T, name string, d dialect.Dialect) *dialect.Registry {
	t.Helper()
	r := dialect.NewRegistry()
	if err := r.Register(name, d); err != nil {
		t.Fatal(err)
	}
	return r
}
