package localai

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/a-digi/cinqo/src/localai/repository/query"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

func newCompleter(t *testing.T, enabled bool, model string) completer {
	t.Helper()
	db := openMainDB(t)
	saveSettings(t, db, enabled, model)
	return completer{settings: query.NewSettingsQueryRepo(db)}
}

func chat(c completer, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	return c.ChatCompletion(context.Background(), http.DefaultClient, "ignored", "ignored", phiID, []chatcompleter.Message{{Role: "user", Content: "hi"}}, tools)
}

func TestTheCompleterAsksTheEngineAndMarksTheCallFree(t *testing.T) {
	f := &fakeEngine{reply: "hello"}
	withEngine(t, f)

	res, err := chat(newCompleter(t, true, phiID), nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || f.model != phiID || f.tools != 0 {
		t.Errorf("engine calls %d model %q tools %d", f.calls, f.model, f.tools)
	}
	if res.Message.Content != "hello" || res.Usage.PromptTokens != 12 || res.Usage.CompletionTokens != 3 {
		t.Errorf("result = %+v", res)
	}
	if res.Usage.Cost == nil || *res.Usage.Cost != 0 {
		t.Errorf("cost = %v, want a known 0 (free), not unknown", res.Usage.Cost)
	}
}

func TestAnEngineFailureIsLabelledAsTheLocalAIAndKeepsItsRetryability(t *testing.T) {
	withEngine(t, &fakeEngine{chatErr: &chatcompleter.CompletionError{Provider: "llama", Status: 503, Retryable: true, Message: "model is still loading"}})
	_, err := chat(newCompleter(t, true, phiID), nil)
	var ce *chatcompleter.CompletionError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a CompletionError", err)
	}
	if ce.Provider != providerName || ce.Status != 503 || !ce.Retryable {
		t.Errorf("got provider %q status %d retryable %v, want %q 503 true", ce.Provider, ce.Status, ce.Retryable, providerName)
	}
}

func TestAPlainEngineErrorPassesThrough(t *testing.T) {
	plain := errors.New("out of memory")
	withEngine(t, &fakeEngine{chatErr: plain})
	if _, err := chat(newCompleter(t, true, phiID), nil); !errors.Is(err, plain) {
		t.Errorf("err = %v, want the engine's own error", err)
	}
}

func TestTheCompleterRefusesWhenSwitchedOffOrWithoutAModel(t *testing.T) {
	f := &fakeEngine{reply: "never"}
	withEngine(t, f)
	for name, c := range map[string]completer{"off": newCompleter(t, false, phiID), "no model": newCompleter(t, true, "")} {
		if _, err := chat(c, nil); !errors.Is(err, ErrNotAvailable) {
			t.Errorf("%s: err = %v, want ErrNotAvailable", name, err)
		}
	}
	if f.calls != 0 {
		t.Error("the engine must not be called")
	}
}

func TestUntilTheManagedRuntimeExistsTheCompleterSaysSo(t *testing.T) {
	_, err := chat(newCompleter(t, true, phiID), nil)
	if err == nil || err.Error() != errManagedNotBuilt.Error() {
		t.Errorf("err = %v, want %v", err, errManagedNotBuilt)
	}
}
