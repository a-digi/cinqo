package platform

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/platform/entity"
)

type stubCompleter struct{}

func (stubCompleter) ChatCompletion(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	return nil, errors.New("stub")
}

func TestTheBuiltInPlatformsKeepTheirBehaviour(t *testing.T) {
	for _, id := range []string{"openai", "anthropic", "openrouter"} {
		e, ok := Lookup(id)
		if !ok {
			t.Fatalf("%s is not registered", id)
		}
		if !e.RequiresKey() || !e.IsAvailable() || e.NoTools {
			t.Errorf("%s: requiresKey %v available %v noTools %v, want true true false", id, e.RequiresKey(), e.IsAvailable(), e.NoTools)
		}
		if e.Credentials != nil || e.Models != nil || e.Available != nil {
			t.Errorf("%s must not set any seam", id)
		}
		if got := e.SelectableModelList(); len(got) == 0 || len(got) != len(e.SelectableModels) {
			t.Errorf("%s: model list %v, want the fixed list %v", id, got, e.SelectableModels)
		}
		if !IsSelectableModel(id, e.DefaultModel) && id != "openrouter" {
			t.Errorf("%s: the default model must stay selectable", id)
		}
	}
}

func TestRegisterRefusesAnEntryThatCannotWork(t *testing.T) {
	for name, e := range map[string]Entry{
		"no id":        {Completer: stubCompleter{}},
		"no completer": {Info: entity.Platform{ID: "seam-no-completer"}},
		"duplicate":    {Info: entity.Platform{ID: "openai"}, Completer: stubCompleter{}},
	} {
		if err := Register(e); !errors.Is(err, ErrInvalidEntry) {
			t.Errorf("%s: err = %v, want ErrInvalidEntry", name, err)
		}
	}
	if _, ok := Lookup("seam-no-completer"); ok {
		t.Error("a refused entry must not be registered")
	}
}

func TestARegisteredEntryReadsItsSeamsAtCallTime(t *testing.T) {
	models := []string{"m1"}
	available := false
	err := Register(Entry{
		Info:        entity.Platform{ID: "seam-dynamic", Name: "Dynamic"},
		Completer:   stubCompleter{},
		Credentials: func(context.Context) (string, string, error) { return "http://127.0.0.1:1/v1", "tok", nil },
		Models:      func() []string { return models },
		Available:   func() bool { return available },
		NoTools:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	e, ok := Lookup("seam-dynamic")
	if !ok {
		t.Fatal("not registered")
	}
	if e.RequiresKey() {
		t.Error("an entry with Credentials needs no key")
	}
	if e.IsAvailable() {
		t.Error("not available yet")
	}
	available = true
	if !e.IsAvailable() {
		t.Error("the availability must be read at call time")
	}
	if !IsSelectableModel("seam-dynamic", "m1") || IsSelectableModel("seam-dynamic", "m2") {
		t.Error("model check must use Models()")
	}
	models = []string{"m2"}
	if !IsSelectableModel("seam-dynamic", "m2") || IsSelectableModel("seam-dynamic", "m1") {
		t.Error("a changed model list must be seen at once")
	}
	found := false
	for _, p := range List() {
		if p.ID == "seam-dynamic" {
			found = true
		}
	}
	if !found {
		t.Error("List must include a registered entry")
	}
}

func TestAnEmptyOrNilModelListIsNeverNil(t *testing.T) {
	if got := (Entry{}).SelectableModelList(); got == nil || len(got) != 0 {
		t.Errorf("got %#v, want an empty non-nil slice", got)
	}
	if got := (Entry{Models: func() []string { return nil }}).SelectableModelList(); got == nil {
		t.Error("a nil result of Models must become an empty slice (the JSON must say [], not null)")
	}
}
