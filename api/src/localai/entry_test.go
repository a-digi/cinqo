package localai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

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
	if e.RequiresKey() || !e.NoTools {
		t.Errorf("requiresKey %v noTools %v, want false and true", e.RequiresKey(), e.NoTools)
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

func TestAnEngineThatIsNotReadySaysWhyAndNothingIsHidden(t *testing.T) {
	withEngine(t, &fakeEngine{readyErr: errors.New("the model is still being downloaded")})
	db := openMainDB(t)
	saveSettings(t, db, true, phiID)
	e := Entry(db)
	if !e.IsAvailable() {
		t.Error("the platform stays listed: an engine that is not ready fails the turn with its reason instead")
	}
	_, _, err := e.Credentials(context.Background())
	if err == nil || !strings.Contains(err.Error(), "still being downloaded") {
		t.Errorf("err = %v, want the engine's reason", err)
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
