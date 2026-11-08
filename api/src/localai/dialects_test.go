package localai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/localai/dialect"
	"github.com/a-digi/cinqo/src/localai/repository/query"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

func TestAModelHasToolsOnlyWhileItsDialectIsThere(t *testing.T) {
	for _, c := range []struct {
		name string
		l    dialect.Lookup
		want map[string]bool
	}{
		{"a registry with the dialect", registryWith(t, "phi-json", stampDialect{}), map[string]bool{phiID: true, qwenID: true, "qwen2.5-7b-instruct-q4_k_m": false, "gpt-4": false, "": false}},
		{"a dialect removed", dialect.NewRegistry(), map[string]bool{phiID: false, qwenID: true}},
		{"no registry at all", nil, map[string]bool{phiID: false, qwenID: true}},
		{"an unrelated dialect", registryWith(t, "other", stampDialect{}), map[string]bool{phiID: false, qwenID: true}},
	} {
		withDialects(t, c.l)
		for model, want := range c.want {
			if got := toolsUsable(model); got != want {
				t.Errorf("%s: toolsUsable(%q) = %v, want %v", c.name, model, got, want)
			}
		}
	}
}

func TestThePlatformOffersToolsOnlyToModelsWhoseToolsWork(t *testing.T) {
	withEngine(t, &fakeEngine{})
	e := Entry(openMainDB(t))
	withDialects(t, registryWith(t, "phi-json", stampDialect{}))
	if e.ToolPolicyFor(phiID) != platform.ToolsExplicit || e.ToolPolicyFor(qwenID) != platform.ToolsExplicit {
		t.Error("both measured models get explicit tools with the dialect registered")
	}
	withDialects(t, dialect.NewRegistry()) // the Phi dialect is removed: Phi is answer-only at once, nothing else changes
	if e.ToolPolicyFor(phiID) != platform.ToolsNone || e.ToolPolicyFor(qwenID) != platform.ToolsExplicit {
		t.Errorf("phi %v qwen %v", e.ToolPolicyFor(phiID), e.ToolPolicyFor(qwenID))
	}
}

func TestTheCompleterUsesTheRegisteredDialectOfTheModel(t *testing.T) {
	db := openMainDB(t)
	saveSettings(t, db, true, phiID)
	c := completer{settings: query.NewSettingsQueryRepo(db)}
	ask := func(model string) (*recordingEngine, *chatcompleter.ChatCompletionResult) {
		e := &recordingEngine{reply: `[{"name": "list_profiles", "arguments": {}}]`}
		withEngine(t, e)
		res, err := c.ChatCompletion(context.Background(), http.DefaultClient, "", "", model, []chatcompleter.Message{{Role: "user", Content: "list"}}, toolDefs())
		if err != nil {
			t.Fatal(err)
		}
		return e, res
	}

	// A model without a dialect (Qwen): the tools go as a field, the messages are untouched, a text that looks like a call stays text.
	withDialects(t, registryWith(t, "phi-json", stampDialect{}))
	e, res := ask(qwenID)
	if len(e.gotTools) != 1 || len(e.gotMessages) != 1 || len(res.ToolCalls) != 0 {
		t.Errorf("qwen: tools %d messages %d calls %d", len(e.gotTools), len(e.gotMessages), len(res.ToolCalls))
	}

	// A model whose dialect is registered: that dialect is used, whatever it is (replaceability).
	e, res = ask(phiID)
	if len(e.gotMessages) != 2 || e.gotMessages[0].Content != "STAMP" || len(e.gotTools) != 0 || !strings.HasPrefix(res.Message.Content, "decoded: ") {
		t.Errorf("replaced dialect: %+v / %q", e.gotMessages, res.Message.Content)
	}

	// The dialect removed: the request goes out as it is, with no special handling.
	withDialects(t, dialect.NewRegistry())
	e, res = ask(phiID)
	if len(e.gotMessages) != 1 || len(e.gotTools) != 1 || len(res.ToolCalls) != 0 {
		t.Errorf("removed dialect: tools %d messages %d calls %d", len(e.gotTools), len(e.gotMessages), len(res.ToolCalls))
	}
}

func TestTheRuntimeReportsWhichModelsCanUseToolsFromTheRegisteredDialects(t *testing.T) {
	withDialects(t, dialect.NewRegistry())
	rt := managedRuntime{}
	if rt.CanUseTools(phiID) || !rt.CanUseTools(qwenID) || rt.CanUseTools("gpt-4") {
		t.Errorf("without the dialect: phi %v qwen %v", rt.CanUseTools(phiID), rt.CanUseTools(qwenID))
	}
	withDialects(t, registryWith(t, "phi-json", stampDialect{}))
	if !rt.CanUseTools(phiID) {
		t.Error("with the dialect phi can use tools")
	}
}

// A dialect that only reads the answer keeps the tools in the request (the model's template renders them) and still decodes the text call.
type nativeReadDialect struct{ stampDialect }

func (nativeReadDialect) NativeTools() bool { return true }

func (nativeReadDialect) Encode(m []chatcompleter.Message, _ []chatcompleter.ToolDef) []chatcompleter.Message {
	return m
}

func (nativeReadDialect) Decode(r *chatcompleter.ChatCompletionResult, tools []chatcompleter.ToolDef) {
	r.ToolCalls = []chatcompleter.ToolCall{{ID: "call_1", Name: tools[0].Name, Arguments: json.RawMessage(`{}`)}}
	r.Message.ToolCalls = r.ToolCalls
}

func TestADialectThatOnlyReadsTheAnswerKeepsTheToolsInTheRequest(t *testing.T) {
	db := openMainDB(t)
	saveSettings(t, db, true, phiID)
	c := completer{settings: query.NewSettingsQueryRepo(db)}
	e := &recordingEngine{reply: "```json\n{\"name\": \"list_profiles\", \"arguments\": {}}\n```"}
	withEngine(t, e)
	withDialects(t, registryWith(t, "phi-json", nativeReadDialect{}))
	res, err := c.ChatCompletion(context.Background(), http.DefaultClient, "", "", phiID, []chatcompleter.Message{{Role: "user", Content: "list"}}, toolDefs())
	if err != nil {
		t.Fatal(err)
	}
	if len(e.gotTools) != 1 || len(e.gotMessages) != 1 {
		t.Errorf("the tools must stay in the request: tools %d messages %d", len(e.gotTools), len(e.gotMessages))
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Name != "list_profiles" {
		t.Errorf("the text call must be read back: %+v", res.ToolCalls)
	}
}
