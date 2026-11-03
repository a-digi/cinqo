// Package localai is the optional local language model feature: a platform ("local") that answers conversations with a
// model Cinqo itself runs on this machine, with its settings, its API and its settings page. It is one self-contained
// module: everything it needs from the core goes through the generic seams of platform.Entry, and the core never imports
// it except at the registration points (see boundary_test.go). The local AI is managed by Cinqo only: there is no way to
// point it at a server or a model of the admin's own. See plan/ai/small-llm-local/step-181-local-ai-small-models.md for
// the design and for how to remove it again.
package localai

import (
	"context"
	"errors"
	"net/http"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// Engine is whatever answers a chat request locally. The real managed engine (a runtime and a model Cinqo downloads and
// runs itself) is built in steps 4 to 6 of the plan; until then the unavailable engine stands in and says why.
type Engine interface {
	// Ready says whether the engine can take a request right now; the error is a reason fit to show an admin.
	Ready(ctx context.Context) error
	// Chat sends one chat completion request and returns the provider-shaped result.
	Chat(ctx context.Context, client *http.Client, model string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error)
}

// ErrNotAvailable means the settings do not describe a local AI that can answer (switched off or no model chosen).
var ErrNotAvailable = errors.New("the local AI is switched off or no model is chosen (see AI, Local AI)")

var errManagedNotBuilt = errors.New("the managed runtime is not available yet")

// engine is what answers chat requests. It is a variable so the tests can put a working engine in its place, and so the
// managed engine can take over when it exists.
var engine Engine = unavailableEngine{err: errManagedNotBuilt}

// unavailableEngine stands in for an engine that does not exist: every call says why.
type unavailableEngine struct{ err error }

func (e unavailableEngine) Ready(context.Context) error { return e.err }

func (e unavailableEngine) Chat(context.Context, *http.Client, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	return nil, e.err
}

// managedRuntime tells the settings API whether the local AI can be switched on right now.
type managedRuntime struct{}

func (managedRuntime) CanEnable() error { return engine.Ready(context.Background()) }
