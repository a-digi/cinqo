package localai

import (
	"context"
	"errors"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

const phiID = "phi-4-mini-instruct-q4_k_m"

// fakeEngine is a working engine for the tests: ready or not, and it answers with a fixed text.
type fakeEngine struct {
	readyErr error
	reply    string
	chatErr  error
	calls    int
	model    string
	tools    int
}

func (f *fakeEngine) Ready(context.Context) error { return f.readyErr }

func (f *fakeEngine) Chat(_ context.Context, _ *http.Client, model string, _ []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	f.calls++
	f.model, f.tools = model, len(tools)
	if f.chatErr != nil {
		return nil, f.chatErr
	}
	return &chatcompleter.ChatCompletionResult{
		Message:      chatcompleter.Message{Role: "assistant", Content: f.reply},
		FinishReason: "stop",
		Usage:        chatcompleter.Usage{PromptTokens: 12, CompletionTokens: 3, TotalTokens: 15},
	}, nil
}

func TestTheLocalPlatformIsOffUntilTheSettingsAreComplete(t *testing.T) {
	withEngine(t, &fakeEngine{})
	db := openMainDB(t)
	e := Entry(db)

	if e.Info.ID != PlatformID || e.Info.Name != "Local AI" {
		t.Errorf("info = %+v", e.Info)
	}
	if e.RequiresKey() || e.OffersAllTools() || e.ToolsFor == nil || e.NoTools {
		t.Errorf("requiresKey %v offersAllTools %v: want no key, never every tool, decided per model", e.RequiresKey(), e.OffersAllTools())
	}
	if !e.AnswerRepeatedCalls {
		t.Error("a small local model repeats calls it already has the answer to")
	}
	if !e.ShortSteps {
		t.Error("a small local model needs the work in short steps")
	}
	if e.IsAvailable() || len(e.SelectableModelList()) != 0 {
		t.Error("the default row must not be offered")
	}
	if _, _, err := e.Credentials(context.Background()); !errors.Is(err, ErrNotAvailable) {
		t.Errorf("err = %v, want ErrNotAvailable", err)
	}

	saveSettings(t, db, true, phiID)
	if !e.IsAvailable() {
		t.Error("a switched-on setting with a model must be offered, read at call time without a restart")
	}
	if got := e.SelectableModelList(); len(got) != 1 || got[0] != phiID {
		t.Errorf("models = %v, want the one chosen model", got)
	}
	if _, _, err := e.Credentials(context.Background()); err != nil {
		t.Errorf("a ready engine must let the platform answer: %v", err)
	}

	saveSettings(t, db, false, phiID)
	if e.IsAvailable() || len(e.SelectableModelList()) != 0 {
		t.Error("switching off must take effect at once")
	}
	saveSettings(t, db, true, "")
	if e.IsAvailable() {
		t.Error("switched on without a model is not offered")
	}
}

// Plan 205: "Local AI" is offered only when it can really answer. While the program or the model is still being downloaded (or
// was deleted), it is not offered anywhere, exactly as when it is off; asking it directly still says why.
func TestAnEngineThatIsNotReadyIsNotOfferedAndSaysWhy(t *testing.T) {
	eng := &fakeEngine{readyErr: errors.New("the model is still being downloaded")}
	withEngine(t, eng)
	db := openMainDB(t)
	saveSettings(t, db, true, phiID)
	e := Entry(db)
	if e.IsAvailable() || len(e.SelectableModelList()) != 0 {
		t.Error("an engine that is not ready must not be offered for a new conversation")
	}
	_, _, err := e.Credentials(context.Background())
	if err == nil || !strings.Contains(err.Error(), "still being downloaded") {
		t.Errorf("err = %v, want the engine's reason", err)
	}
	eng.readyErr = nil // the download finished
	if !e.IsAvailable() || len(e.SelectableModelList()) != 1 {
		t.Error("as soon as it is ready it must be offered, with no restart")
	}
}

func TestUntilTheManagedRuntimeExistsNothingCanAnswer(t *testing.T) {
	db := openMainDB(t)
	saveSettings(t, db, true, phiID) // a hand-edited row: validation would have refused this
	e := Entry(db)
	_, _, err := e.Credentials(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not available yet") {
		t.Errorf("err = %v, want the unavailable engine's reason", err)
	}
	if err := (managedRuntime{}).CanEnable("tiny"); err == nil {
		t.Error("the local AI cannot be switched on while the managed runtime does not exist")
	}
}

func TestAnUnreadableSettingsTableMeansUnavailable(t *testing.T) {
	withEngine(t, &fakeEngine{})
	db := openMainDB(t)
	e := Entry(db)
	if _, err := db.Exec(`DROP TABLE local_ai_settings`); err != nil {
		t.Fatal(err)
	}
	if e.IsAvailable() || len(e.SelectableModelList()) != 0 {
		t.Error("an error reading the settings must hide the platform, not offer it")
	}
	_, _, err := e.Credentials(context.Background())
	if err == nil || !strings.Contains(err.Error(), "settings") {
		t.Errorf("err = %v, want a settings read error", err)
	}
}

func TestOnlyAModelMeasuredToUseToolsGetsTheExplicitPolicy(t *testing.T) {
	withEngine(t, &fakeEngine{})
	reg := registryWith(t, "phi-json", stampDialect{})
	if err := reg.Register("qwen-json", stampDialect{}); err != nil {
		t.Fatal(err)
	}
	withDialects(t, reg)
	e := Entry(openMainDB(t))
	for model, want := range map[string]platform.ToolPolicy{
		"qwen2.5-1.5b-instruct-q4_k_m":     platform.ToolsExplicit,
		"phi-4-mini-instruct-q4_k_m":       platform.ToolsExplicit, // through its own dialect (0 of 40 natively, 50 of 50 with the dialect)
		"qwen2.5-7b-instruct-q4_k_m":       platform.ToolsNone,     // not measured yet
		"qwen2.5-coder-3b-instruct-q4_k_m": platform.ToolsExplicit, // through its text-reading dialect (plan 196: job-detail recipe 10 of 10)
		"gpt-4":                            platform.ToolsNone,
		"":                                 platform.ToolsNone,
	} {
		if got := e.ToolPolicyFor(model); got != want {
			t.Errorf("%q: policy %v, want %v", model, got, want)
		}
	}
	if !strings.Contains(e.ToolHint, "always call the matching tool first") {
		t.Errorf("the model must be told to look before it answers about the user's data: %q", e.ToolHint)
	}
}

func TestTheLocalPlatformStatesTheContextOfItsSettingsOnceConfigured(t *testing.T) {
	withEngine(t, &fakeEngine{})
	db := openMainDB(t)
	e := Entry(db)
	if e.ContextTokens == nil {
		t.Fatal("the local platform must state its context")
	}
	if got := e.ContextTokens(); got != 0 {
		t.Errorf("not configured: it states none (0), got %d", got)
	}
	saveSettings(t, db, true, phiID)
	if got := e.ContextTokens(); got != 4096 {
		t.Errorf("configured with the default: %d", got)
	}
	// Read at call time: the page's change is seen at once, nothing is cached.
	if _, err := db.Exec(`UPDATE local_ai_settings SET context_tokens = 16384`); err != nil {
		t.Fatal(err)
	}
	if got := e.ContextTokens(); got != 16384 {
		t.Errorf("after raising it: %d", got)
	}
	saveSettings(t, db, false, phiID) // switched off again: no platform, no limit to state
	if got := e.ContextTokens(); got != 0 {
		t.Errorf("switched off: %d", got)
	}
}

// --- plan 206, step 1: the platform offers every ready model ---

func TestTheLocalPlatformOffersEveryReadyModelWithTheSavedDefaultFirst(t *testing.T) {
	r := newEngineRig(t)
	r.install(t, r.other)
	withEngine(t, r.e)
	e := Entry(r.db)
	if got := e.SelectableModelList(); !reflect.DeepEqual(got, []string{"tiny", "other"}) {
		t.Fatalf("models = %v", got)
	}
	saveSettings(t, r.db, true, "other") // the default changes on the Local AI page
	if got := e.SelectableModelList(); !reflect.DeepEqual(got, []string{"other", "tiny"}) {
		t.Errorf("the new default must come first, read at call time: %v", got)
	}
	for _, m := range []struct {
		id   string
		want bool
	}{{"tiny", true}, {"other", true}, {"gpt-4", false}, {"", false}} {
		in := false
		for _, offered := range e.SelectableModelList() {
			in = in || offered == m.id
		}
		if in != m.want {
			t.Errorf("%q selectable = %v, want %v", m.id, in, m.want)
		}
	}
	saveSettings(t, r.db, false, "other")
	if e.IsAvailable() || len(e.SelectableModelList()) != 0 {
		t.Error("switched off: nothing is offered")
	}
}

func TestTheLocalPlatformIsAvailableWhileAnyModelIsReadyEvenWhenTheDefaultIsNot(t *testing.T) {
	r := newEngineRig(t)
	r.install(t, r.other)
	withEngine(t, r.e)
	e := Entry(r.db)
	os.Remove(r.in.ModelPath(r.model)) // the saved default is gone, the other one is not
	if !e.IsAvailable() {
		t.Fatal("a ready model is enough to offer the local AI")
	}
	if got := e.SelectableModelList(); !reflect.DeepEqual(got, []string{"other"}) {
		t.Errorf("models = %v", got)
	}
	if _, _, err := e.Credentials(context.Background()); err != nil {
		t.Errorf("a platform that can answer must not refuse: %v", err)
	}
	os.Remove(r.in.ModelPath(r.other)) // nothing is left
	if e.IsAvailable() || len(e.SelectableModelList()) != 0 {
		t.Error("with no model ready the local AI is not offered")
	}
	if _, _, err := e.Credentials(context.Background()); err == nil || !strings.Contains(err.Error(), "not downloaded") {
		t.Errorf("err = %v, want the reason", err)
	}
}

func TestTheLocalPlatformNamesItsModelsReadably(t *testing.T) {
	withEngine(t, &fakeEngine{})
	e := Entry(openMainDB(t))
	if e.ModelNames == nil {
		t.Fatal("the local platform must give readable names")
	}
	names := e.ModelNames()
	for id, want := range map[string]string{
		"qwen2.5-coder-3b-instruct-q4_k_m": "Qwen2.5-Coder-3B-Instruct, 2.0 GiB",
		"qwen2.5-1.5b-instruct-q4_k_m":     "Qwen2.5-1.5B-Instruct, 1.0 GiB",
		phiID:                              "Phi-4-mini-instruct, 2.3 GiB",
	} {
		if names[id] != want {
			t.Errorf("%s: %q, want %q", id, names[id], want)
		}
	}
}

func TestTheLocalPlatformFlagsTheQwen15BAsUnsuitableForTheCVImport(t *testing.T) {
	withEngine(t, &fakeEngine{})
	e := Entry(openMainDB(t))
	if e.ModelUnsuitable == nil {
		t.Fatal("the local platform must say which models cannot do which task")
	}
	got := e.ModelUnsuitable()
	if tasks := got["qwen2.5-1.5b-instruct-q4_k_m"]; len(tasks) != 1 || tasks[0] != "cv-import" {
		t.Errorf("1.5B: %v", tasks)
	}
	for _, id := range []string{"qwen2.5-coder-3b-instruct-q4_k_m", phiID} {
		if len(got[id]) != 0 {
			t.Errorf("%s must not be flagged: %v", id, got[id])
		}
	}
}
