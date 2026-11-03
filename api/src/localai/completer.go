package localai

import (
	"context"
	"errors"
	"net/http"

	"github.com/a-digi/cinqo/src/localai/repository/query"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// providerName is how a failure from the local model is labelled (instead of the name of the wire protocol).
const providerName = "local AI"

// completer is the platform's chat completer: it reads the current settings and sends the request to the engine. Local usage is free by definition, so the result carries a known cost of 0 (never an estimate):
// the conversation UI shows it as "free".
type completer struct {
	settings *query.SettingsQueryRepo
}

func (c completer) ChatCompletion(ctx context.Context, client *http.Client, _ /* baseURL */, _ /* apiKey */, model string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	s, err := c.settings.Load()
	if err != nil {
		return nil, errors.New("local AI: could not read the settings")
	}
	if !s.Configured() {
		return nil, ErrNotAvailable
	}
	// A model whose chat template cannot take tools is taught them in its own dialect and its text answer is read back.
	d := dialectOf(model)
	sent := tools
	if d != nil {
		messages = d.Encode(messages, tools)
		sent = nil
	}
	result, err := engine.Chat(ctx, client, model, messages, sent)
	if err != nil {
		return nil, relabel(err)
	}
	if d != nil {
		d.Decode(result, tools)
	}
	free := 0.0
	result.Usage.Cost = &free
	return result, nil
}

// relabel gives a completion failure the local AI's own name, so an admin does not read "openai: ..." about a model
// running on their own machine.
func relabel(err error) error {
	var ce *chatcompleter.CompletionError
	if errors.As(err, &ce) {
		copied := *ce
		copied.Provider = providerName
		return &copied
	}
	return err
}
