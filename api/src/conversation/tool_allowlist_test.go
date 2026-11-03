package conversation

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/tool/mcp"
)

func offered() []chatcompleter.ToolDef {
	return []chatcompleter.ToolDef{
		{Name: "list_personas"}, {Name: "get_persona_details"}, {Name: "create_persona"},
		{Name: "update_persona_details"}, {Name: "add_career_skill"},
	}
}

func names(tools []chatcompleter.ToolDef) []string {
	out := []string{}
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

func convWith(allowed string) *conversation_entity.Conversation {
	return &conversation_entity.Conversation{AllowedTools: allowed}
}

func TestApplyToolAllowlist_FiltersWhatIsOffered(t *testing.T) {
	ctx, got := applyToolAllowlist(context.Background(), offered(), convWith(`["get_persona_details","list_personas","nonexistent"]`))
	if !reflect.DeepEqual(names(got), []string{"list_personas", "get_persona_details"}) {
		t.Fatalf("offered = %v, want only the allowed ones, in the offered order", names(got))
	}
	if refusesDisallowedTool(ctx, "list_personas") || !refusesDisallowedTool(ctx, "create_persona") || !refusesDisallowedTool(ctx, "run_plan") {
		t.Fatal("the context must refuse exactly what is not on the list")
	}
}

func TestApplyToolAllowlist_NoListChangesNothing(t *testing.T) {
	for name, conv := range map[string]*conversation_entity.Conversation{"no column": convWith(""), "nil conversation": nil} {
		ctx, got := applyToolAllowlist(context.Background(), offered(), conv)
		if len(got) != len(offered()) {
			t.Errorf("%s: %d tools offered, want all %d", name, len(got), len(offered()))
		}
		if refusesDisallowedTool(ctx, "create_persona") || refusesDisallowedTool(ctx, "anything") {
			t.Errorf("%s: nothing may be refused without an allowlist", name)
		}
	}
}

// An unreadable stored list restricts to nothing: no tool offered, every call refused.
func TestApplyToolAllowlist_UnreadableListFailsClosed(t *testing.T) {
	ctx, got := applyToolAllowlist(context.Background(), offered(), convWith("garbage"))
	if len(got) != 0 {
		t.Fatalf("offered %v, want none", names(got))
	}
	if !refusesDisallowedTool(ctx, "list_personas") {
		t.Fatal("every call must be refused")
	}
}

// The real loop: a model that calls a write tool is refused and the tool never runs, an allowed
// tool runs, and the model is told "tool unavailable" for the refused one.
func TestRunToolLoop_RefusesAToolOutsideTheAllowlist(t *testing.T) {
	ran := []string{}
	old := invokeToolFn
	invokeToolFn = func(_ context.Context, _ *sql.DB, _ []string, _ string, call chatcompleter.ToolCall, _ string, _ int, _ string) (string, []mcp.ResourceLink, *pendingDelegation) {
		ran = append(ran, call.Name)
		return "ok:" + call.Name, nil, nil
	}
	t.Cleanup(func() { invokeToolFn = old })

	ctx, tools := applyToolAllowlist(context.Background(), offered(), convWith(`["list_personas","get_persona_details"]`))
	s := &scriptedCalls{names: []string{"list_personas", "create_persona", "add_career_skill", "get_persona_details"}}
	_, err := runToolLoop(ctx, http.DefaultClient, platform.Entry{Completer: s}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, tools, nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ran, []string{"list_personas", "get_persona_details"}) {
		t.Fatalf("tools that ran = %v, want only the two allowed ones", ran)
	}
	want := []string{"ok:list_personas", "tool unavailable", "tool unavailable", "ok:get_persona_details"}
	if !reflect.DeepEqual(s.results, want) {
		t.Fatalf("what the model got back = %v, want %v", s.results, want)
	}
}

// The orchestration tools are refused too: they are handled before a normal tool call is, so a
// restricted conversation must not be able to start sub-agents or a plan.
func TestRunToolLoop_RefusesOrchestrationToolsOutsideTheAllowlist(t *testing.T) {
	withTools(t, func(string, int) string { return "ran" })
	ctx, tools := applyToolAllowlist(context.Background(), offered(), convWith(`["list_personas"]`))
	s := &scriptedCalls{names: []string{subAgentToolName, runPlanToolName, crawlURLsWithSubagentsToolName}}
	_, err := runToolLoop(ctx, http.DefaultClient, platform.Entry{Completer: s}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, tools, nil, nil, "u1", t.TempDir(), 0, "conv", nil, "turn-1", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range s.results {
		if r != "tool unavailable" {
			t.Errorf("result %d = %q, want %q", i, r, "tool unavailable")
		}
	}
	if len(s.results) != 3 {
		t.Fatalf("results = %v", s.results)
	}
}

// Without an allowlist the loop behaves exactly as before: every call goes to the tool.
func TestRunToolLoop_NoAllowlistRunsEveryTool(t *testing.T) {
	withTools(t, func(name string, _ int) string { return "ran:" + name })
	ctx, tools := applyToolAllowlist(context.Background(), offered(), convWith(""))
	s := &scriptedCalls{names: []string{"create_persona", "list_personas"}}
	if _, err := runToolLoop(ctx, http.DefaultClient, platform.Entry{Completer: s}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, tools, nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.results, []string{"ran:create_persona", "ran:list_personas"}) {
		t.Fatalf("results = %v", s.results)
	}
}

// The turn runners cannot be run here (they need a database, a key and a model), so this pins the
// wiring itself: between building the offered tools and running the loop, both entry points must
// apply the conversation's allowlist (through offeredTools, which also applies the platform's tool policy). Dropping that line would silently let a restricted
// conversation call anything.
func TestTurnEntryPointsApplyTheAllowlist(t *testing.T) {
	for file, from := range map[string]string{
		"runner.go": "offerableToolsForMode(",
		"chat.go":   "tools, err := offerableTools(mainDB, callerScopes, false)",
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		start := strings.Index(src, from)
		if start < 0 {
			t.Fatalf("%s: cannot find %q, update this test with the code", file, from)
		}
		rest := src[start:]
		loop := strings.Index(rest, "runToolLoop(ctx, ")
		if loop < 0 {
			t.Fatalf("%s: cannot find the runToolLoop call after the tools are built", file)
		}
		if !strings.Contains(rest[:loop], "offeredTools(ctx, entry, model, tools, conv)") {
			t.Errorf("%s: the allowlist is not applied between building the tools and running the loop", file)
		}
		if !strings.Contains(rest[:loop], "withToolHint(entry, tools, messages)") {
			t.Errorf("%s: the platform's tool hint is not added between building the tools and running the loop", file)
		}
	}
}

func TestOfferedToolsFollowThePlatformsToolPolicy(t *testing.T) {
	perModel := func(model string) platform.ToolPolicy {
		if model == "tools" {
			return platform.ToolsExplicit
		}
		return platform.ToolsNone
	}
	allowlist := `["list_personas","get_persona_details"]`
	for _, c := range []struct {
		name  string
		entry platform.Entry
		model string
		conv  *conversation_entity.Conversation
		want  []string
	}{
		{"a built-in platform without a list: every tool", platform.Entry{}, "m", convWith(""), []string{"list_personas", "get_persona_details", "create_persona", "update_persona_details", "add_career_skill"}},
		{"a built-in platform with a list: the list", platform.Entry{}, "m", convWith(allowlist), []string{"list_personas", "get_persona_details"}},
		{"no tools, even with a list", platform.Entry{NoTools: true}, "m", convWith(allowlist), []string{}},
		{"explicit tools, with a list: the list", platform.Entry{ToolsFor: perModel}, "tools", convWith(allowlist), []string{"list_personas", "get_persona_details"}},
		{"explicit tools, no list: none (never every tool)", platform.Entry{ToolsFor: perModel}, "tools", convWith(""), []string{}},
		{"a model without tools, even with a list", platform.Entry{ToolsFor: perModel}, "other", convWith(allowlist), []string{}},
	} {
		_, got := offeredTools(context.Background(), c.entry, c.model, offered(), c.conv)
		if !reflect.DeepEqual(names(got), c.want) {
			t.Errorf("%s: %v, want %v", c.name, names(got), c.want)
		}
	}
}

func TestTheToolHintIsAddedOnlyToATurnThatHasTools(t *testing.T) {
	msgs := []chatcompleter.Message{{Role: "user", Content: "hi"}}
	entry := platform.Entry{ToolHint: "look first"}
	got := withToolHint(entry, offered(), msgs)
	if len(got) != 2 || got[0].Role != "system" || got[0].Content != "look first" || got[1].Content != "hi" {
		t.Errorf("with tools: %+v", got)
	}
	if len(msgs) != 1 {
		t.Error("the caller's messages must not be changed")
	}
	if got := withToolHint(entry, nil, msgs); len(got) != 1 {
		t.Errorf("no tools offered: no hint, got %+v", got)
	}
	if got := withToolHint(platform.Entry{}, offered(), msgs); len(got) != 1 {
		t.Errorf("a platform without a hint adds none: %+v", got)
	}
}
