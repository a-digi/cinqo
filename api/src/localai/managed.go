package localai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/hardware"
	"github.com/a-digi/cinqo/src/localai/repository/query"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/platform/openai"
)

// chatFunc sends one chat request to an OpenAI-compatible server; the real one is the application's OpenAI client.
type chatFunc func(ctx context.Context, client *http.Client, baseURL, token, model string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error)

// managedEngine is the local AI's engine: it checks that everything needed is there, starts the model program (through the
// supervisor) when a conversation needs it, and sends the request to it. One request runs at a time: the program has one slot,
// and a second conversation's turn waits its turn (decision 6 of the plan).
type managedEngine struct {
	in       *installer
	sup      *supervisor
	settings *query.SettingsQueryRepo
	system   func() hardware.Info
	call     chatFunc
	slot     chan struct{}
}

func newManagedEngine(in *installer, sup *supervisor, settings *query.SettingsQueryRepo) *managedEngine {
	return &managedEngine{
		in:       in,
		sup:      sup,
		settings: settings,
		system:   hardware.Detect,
		call: func(ctx context.Context, client *http.Client, baseURL, token, model string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
			return openai.Client{}.ChatCompletion(ctx, client, baseURL, token, model, messages, tools)
		},
		slot: make(chan struct{}, 1),
	}
}

func gib(bytes uint64) string { return fmt.Sprintf("%.1f GiB", float64(bytes)/(1<<30)) }

// prerequisites checks that a model can be run now: the system is supported, the runtime and the model are downloaded, and the
// computer has the memory for it (unknown memory is not held against it). The messages are fit to show on the page.
func (e *managedEngine) prerequisites(modelID string) (catalog.Model, catalog.Runtime, error) {
	m, ok := e.in.catalog.Find(modelID)
	if !ok {
		return catalog.Model{}, catalog.Runtime{}, entity.ErrUnknownModel
	}
	info := e.system()
	rt, ok := e.in.catalog.RuntimeFor(info.OS, info.Arch)
	if !ok {
		return m, catalog.Runtime{}, entity.ErrUnsupportedSystem
	}
	if e.in.RuntimeState(rt).State != entity.InstallReady {
		return m, rt, errors.New("the program that runs the models is not downloaded yet")
	}
	if e.in.ModelState(m).State != entity.InstallReady {
		return m, rt, errors.New("the model is not downloaded yet")
	}
	if fits := info.Fits(m.SizeBytes); fits != nil && !*fits {
		return m, rt, fmt.Errorf("this computer has %s of memory and the model needs about %s", gib(info.RAMBytes), gib(hardware.MinRAMBytes(m.SizeBytes)))
	}
	return m, rt, nil
}

// CanEnable says whether the local AI can be switched on with this model, and if not, why.
func (e *managedEngine) CanEnable(modelID string) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return errors.New("choose a model first")
	}
	_, _, err := e.prerequisites(modelID)
	return err
}

// Ready says whether the model of the settings can answer: everything is downloaded and fits. It does not start the program.
func (e *managedEngine) Ready(context.Context) error {
	s, err := e.settings.Load()
	if err != nil {
		return errors.New("could not read the local AI settings")
	}
	_, _, err = e.prerequisites(s.ModelID)
	return err
}

// acquire waits for the one slot, or for the request to be cancelled.
func (e *managedEngine) acquire(ctx context.Context) error {
	select {
	case e.slot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *managedEngine) free() { <-e.slot }

// Chat runs one request on the model of the conversation, starting the program first when it is not running.
func (e *managedEngine) Chat(ctx context.Context, client *http.Client, model string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	s, err := e.settings.Load()
	if err != nil {
		return nil, errors.New("could not read the local AI settings")
	}
	m, rt, err := e.prerequisites(model)
	if err != nil {
		return nil, err
	}
	if err := e.acquire(ctx); err != nil {
		return nil, err
	}
	defer e.free()

	base, token, release, err := e.sup.ensure(ctx, e.launchFor(m, rt, s), s.IdleMinutes)
	if err != nil {
		return nil, err
	}
	defer release()
	res, err := e.call(ctx, client, base, token, model, messages, tools)
	if err != nil {
		return nil, explainFailure(err, s.ContextTokens)
	}
	return res, nil
}

func (e *managedEngine) launchFor(m catalog.Model, rt catalog.Runtime, s entity.Settings) launch {
	return launch{Binary: e.in.RuntimeBinary(rt), ModelPath: e.in.ModelPath(m), ModelID: m.ID, ContextTokens: s.ContextTokens, Dir: e.in.root()}
}

// explainFailure turns the model program's "this request does not fit in the context" answer into plain words with the way out.
func explainFailure(err error, contextTokens int) error {
	var ce *chatcompleter.CompletionError
	if errors.As(err, &ce) && ce.Status == http.StatusBadRequest {
		low := strings.ToLower(ce.Error())
		if strings.Contains(low, "exceed") && strings.Contains(low, "context") {
			return &chatcompleter.CompletionError{
				Provider: ce.Provider,
				Status:   ce.Status,
				Message:  fmt.Sprintf("the conversation is longer than the model can read at once (a context of %d tokens): start a new conversation, or raise the context size in AI, Local AI", contextTokens),
			}
		}
	}
	return err
}

// Status is the state of the model program.
func (e *managedEngine) Status() entity.EngineStatus { return e.sup.Status() }

// Stop stops the model program (the application is shutting down).
func (e *managedEngine) Stop() { e.sup.Stop() }

const testPrompt = "Reply with exactly one word: OK"

// Test sends one tiny real request to the model, starting the program when needed, and reports how long that took. Failures
// of the model are part of the result (OK false with the reason), not an error: the page shows them.
func (e *managedEngine) Test(ctx context.Context, modelID string) (entity.TestResult, error) {
	m, rt, err := e.prerequisites(modelID)
	if errors.Is(err, entity.ErrUnknownModel) {
		return entity.TestResult{}, err
	}
	if err != nil {
		return entity.TestResult{Error: err.Error()}, nil
	}
	s, err := e.settings.Load()
	if err != nil {
		return entity.TestResult{Error: "could not read the local AI settings"}, nil
	}
	if err := e.acquire(ctx); err != nil {
		return entity.TestResult{Error: "the test was cancelled"}, nil
	}
	defer e.free()

	t0 := time.Now()
	base, token, release, err := e.sup.ensure(ctx, e.launchFor(m, rt, s), s.IdleMinutes)
	startSeconds := time.Since(t0).Seconds()
	if err != nil {
		return entity.TestResult{Error: err.Error(), StartSeconds: startSeconds}, nil
	}
	defer release()

	t1 := time.Now()
	res, err := e.call(ctx, &http.Client{Timeout: 3 * time.Minute}, base, token, m.ID, []chatcompleter.Message{{Role: "user", Content: testPrompt}}, nil)
	seconds := time.Since(t1).Seconds()
	if err != nil {
		return entity.TestResult{Error: explainFailure(err, s.ContextTokens).Error(), StartSeconds: startSeconds, Seconds: seconds}, nil
	}
	out := entity.TestResult{
		OK:               true,
		StartSeconds:     startSeconds,
		Seconds:          seconds,
		PromptTokens:     res.Usage.PromptTokens,
		CompletionTokens: res.Usage.CompletionTokens,
		Reply:            strings.TrimSpace(res.Message.Content),
	}
	if len([]rune(out.Reply)) > 200 {
		out.Reply = string([]rune(out.Reply)[:200]) + "…"
	}
	if seconds > 0 {
		out.TokensPerSecond = float64(out.CompletionTokens) / seconds
	}
	return out, nil
}
