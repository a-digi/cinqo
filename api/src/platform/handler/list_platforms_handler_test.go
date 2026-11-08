package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/platform/entity"
)

type noopCompleter struct{}

func (noopCompleter) ChatCompletion(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	return nil, errors.New("noop")
}

func find(list []platformResponse, id string) (platformResponse, bool) {
	for _, p := range list {
		if p.ID == id {
			return p, true
		}
	}
	return platformResponse{}, false
}

func TestThePlatformListSaysWhichPlatformsNeedAKeyAndHidesUnavailableOnes(t *testing.T) {
	available := false
	if err := platform.Register(platform.Entry{
		Info:        entity.Platform{ID: "list-keyless", Name: "Keyless"},
		Completer:   noopCompleter{},
		Credentials: func(context.Context) (string, string, error) { return "http://127.0.0.1:1", "", nil },
		Models:      func() []string { return []string{"only"} },
		Available:   func() bool { return available },
		NoTools:     true,
	}); err != nil {
		t.Fatal(err)
	}

	list := platformResponses()
	if _, ok := find(list, "list-keyless"); ok {
		t.Error("an unavailable platform must not be listed")
	}
	for _, id := range []string{"openai", "anthropic", "openrouter"} {
		p, ok := find(list, id)
		if !ok || !p.RequiresKey || !p.OffersTools || len(p.Models) == 0 {
			t.Errorf("%s: %+v (listed %v), want listed, requiresKey, offersTools, with models", id, p, ok)
		}
	}

	available = true
	p, ok := find(platformResponses(), "list-keyless")
	if !ok {
		t.Fatal("an available platform must be listed")
	}
	if p.RequiresKey || p.OffersTools || len(p.Models) != 1 || p.Models[0] != "only" {
		t.Errorf("got %+v, want requiresKey false, offersTools false and the dynamic model list", p)
	}
}

func TestAPlatformWithPerModelToolsIsNotOfferedTheToolModes(t *testing.T) {
	if err := platform.Register(platform.Entry{
		Info:        entity.Platform{ID: "list-permodel", Name: "PerModel"},
		Completer:   noopCompleter{},
		Credentials: func(context.Context) (string, string, error) { return "http://127.0.0.1:1", "", nil },
		Models:      func() []string { return []string{"a"} },
		ToolsFor:    func(string) platform.ToolPolicy { return platform.ToolsExplicit },
	}); err != nil {
		t.Fatal(err)
	}
	p, ok := find(platformResponses(), "list-permodel")
	if !ok || p.OffersTools {
		t.Errorf("%+v (listed %v): a form that cannot name an allowlist must not offer tool modes", p, ok)
	}
}

func TestThePlatformListSaysWhichPlatformsNeedShortSteps(t *testing.T) {
	if err := platform.Register(platform.Entry{
		Info:        entity.Platform{ID: "list-shortsteps", Name: "Short"},
		Completer:   noopCompleter{},
		Credentials: func(context.Context) (string, string, error) { return "http://127.0.0.1:1", "", nil },
		Models:      func() []string { return []string{"a"} },
		ShortSteps:  true,
	}); err != nil {
		t.Fatal(err)
	}
	list := platformResponses()
	if p, ok := find(list, "list-shortsteps"); !ok || !p.NeedsShortSteps {
		t.Errorf("%+v (listed %v): a platform that asked for short steps must say so", p, ok)
	}
	for _, id := range []string{"openai", "anthropic", "openrouter"} {
		if p, ok := find(list, id); !ok || p.NeedsShortSteps {
			t.Errorf("%s: %+v, the cloud platforms take the long prompt", id, p)
		}
	}
}

func TestThePlatformListStatesTheContextOnlyOfAPlatformThatHasOne(t *testing.T) {
	limit := 16384
	if err := platform.Register(platform.Entry{
		Info:          entity.Platform{ID: "list-context", Name: "Context"},
		Completer:     noopCompleter{},
		Credentials:   func(context.Context) (string, string, error) { return "http://127.0.0.1:1", "", nil },
		Models:        func() []string { return []string{"a"} },
		ContextTokens: func() int { return limit },
	}); err != nil {
		t.Fatal(err)
	}
	if err := platform.Register(platform.Entry{
		Info:          entity.Platform{ID: "list-nocontext", Name: "None"},
		Completer:     noopCompleter{},
		Credentials:   func(context.Context) (string, string, error) { return "http://127.0.0.1:1", "", nil },
		Models:        func() []string { return []string{"a"} },
		ContextTokens: func() int { return 0 }, // set but states nothing (for example not configured yet)
	}); err != nil {
		t.Fatal(err)
	}
	list := platformResponses()
	if p, ok := find(list, "list-context"); !ok || p.ContextTokens == nil || *p.ContextTokens != 16384 {
		t.Errorf("%+v (listed %v): the context must be stated", p, ok)
	}
	limit = 8192 // read at call time, not cached
	if p, _ := find(platformResponses(), "list-context"); p.ContextTokens == nil || *p.ContextTokens != 8192 {
		t.Errorf("the context is read at call time: %+v", p)
	}
	if p, ok := find(list, "list-nocontext"); !ok || p.ContextTokens != nil {
		t.Errorf("%+v: a platform that states 0 states none", p)
	}
	for _, id := range []string{"openai", "anthropic", "openrouter"} {
		if p, ok := find(list, id); !ok || p.ContextTokens != nil {
			t.Errorf("%s: %+v, the cloud platforms state no context", id, p)
		}
	}
	raw, _ := json.Marshal(platformResponse{ID: "x"})
	if strings.Contains(string(raw), "contextTokens") {
		t.Errorf("an absent context must not be in the JSON: %s", raw)
	}
}
