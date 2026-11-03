package conversation

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strings"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/tool/mcp"
)

// runCounting runs a scripted turn with a reporter in the context and returns how many tool failures it counted. answer is what the tool
// says for each call that really runs.
func runCounting(t *testing.T, ctx context.Context, entry platform.Entry, answer func(call chatcompleter.ToolCall) string, calls ...chatcompleter.ToolCall) int {
	t.Helper()
	counted := 0
	ctx = withToolErrorReporter(ctx, func() { counted++ })
	old := invokeToolFn
	invokeToolFn = func(_ context.Context, _ *sql.DB, _ []string, _ string, call chatcompleter.ToolCall, _ string, _ int, _ string) (string, []mcp.ResourceLink, *pendingDelegation) {
		return answer(call), nil, nil
	}
	t.Cleanup(func() { invokeToolFn = old })
	entry.Completer = &argScript{calls: calls}
	if _, err := runToolLoop(ctx, http.DefaultClient, entry, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, offered(), nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, 0); err != nil {
		var rejected *ArgumentRejectionError
		if !asRejection(err, &rejected) {
			t.Fatal(err)
		}
	}
	return counted
}

func asRejection(err error, target **ArgumentRejectionError) bool {
	e, ok := err.(*ArgumentRejectionError)
	if ok {
		*target = e
	}
	return ok
}

func ok(chatcompleter.ToolCall) string { return "fine" }

func TestRefusedAndRejectedCallsAreCountedAndNothingElse(t *testing.T) {
	// In a direct conversation a tool name that was not offered still goes to the tool layer, which answers for it: not counted here.
	if n := runCounting(t, context.Background(), platform.Entry{}, ok, callOf("not_offered", `{}`), callOf("list_personas", `{}`)); n != 0 {
		t.Errorf("no framework refusal happened: counted %d", n)
	}
	// A tool outside the conversation's allowlist is refused too.
	ctx, _ := applyToolAllowlist(context.Background(), offered(), convWith(`["list_personas"]`))
	if n := runCounting(t, ctx, platform.Entry{}, ok, callOf("create_persona", `{}`), callOf("add_career_skill", `{}`)); n != 2 {
		t.Errorf("two calls outside the allowlist: counted %d", n)
	}
	// A call whose arguments failed the schema is rejected.
	reject := func(call chatcompleter.ToolCall) string {
		return `validating "arguments": validating root: missing property "id"`
	}
	if n := runCounting(t, context.Background(), platform.Entry{}, reject, callOf("get_persona_details", `{}`)); n != 1 {
		t.Errorf("one rejected call: counted %d", n)
	}
	// The framework's own "tool unavailable" from the tool layer (an id that cannot be resolved) counts as well.
	unavailable := func(chatcompleter.ToolCall) string { return "tool unavailable" }
	if n := runCounting(t, context.Background(), platform.Entry{}, unavailable, callOf("list_personas", `{}`)); n != 1 {
		t.Errorf("a refused call from the tool layer: counted %d", n)
	}
	// A tool that ran and reported a problem of its own, and a call that worked, are not counted.
	own := func(chatcompleter.ToolCall) string { return "no profile with that id" }
	if n := runCounting(t, context.Background(), platform.Entry{}, own, callOf("list_personas", `{}`), callOf("get_persona_details", `{"a":1}`)); n != 0 {
		t.Errorf("a tool's own problem is not a framework refusal: counted %d", n)
	}
	if n := runCounting(t, context.Background(), platform.Entry{}, ok, callOf("list_personas", `{}`)); n != 0 {
		t.Errorf("a good call: counted %d", n)
	}
}

func TestACallAnsweredFromTheEarlierResultIsNotCountedAsAFailure(t *testing.T) {
	same := callOf("list_personas", `{"id":"1"}`)
	if n := runCounting(t, context.Background(), platform.Entry{AnswerRepeatedCalls: true}, ok, same, same, same); n != 0 {
		t.Errorf("the short repeat answer is not a refusal: counted %d", n)
	}
}

func TestATurnWithoutAReporterCountsNothingAndDoesNotFail(t *testing.T) {
	old := invokeToolFn
	invokeToolFn = func(_ context.Context, _ *sql.DB, _ []string, _ string, call chatcompleter.ToolCall, _ string, _ int, _ string) (string, []mcp.ResourceLink, *pendingDelegation) {
		return "tool unavailable", nil, nil
	}
	t.Cleanup(func() { invokeToolFn = old })
	entry := platform.Entry{Completer: &argScript{calls: []chatcompleter.ToolCall{callOf("list_personas", `{}`)}}}
	if _, err := runToolLoop(context.Background(), http.DefaultClient, entry, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, offered(), nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	reportToolError(context.Background()) // and the helper is safe on a plain context
	reportToolError(withToolErrorReporter(context.Background(), nil))
}

func TestIsToolFailureIsExactlyTheFrameworksRefusals(t *testing.T) {
	for text, want := range map[string]bool{
		"tool unavailable":                     true,
		`validating "arguments": x`:            true,
		"invalid tool arguments: x":            true,
		"Tool unavailable":                     false,
		"tool unavailable for this user":       false,
		"":                                     false,
		"a result mentioning tool unavailable": false,
		"ok":                                   false,
	} {
		if got := isToolFailure(text); got != want {
			t.Errorf("%q: %v, want %v", text, got, want)
		}
	}
}

// The detached turn (runner.go) is where a turn's row is known: it must hand the loop the reporter that writes to that row.
func TestTheRunnerCountsToolErrorsOnTheTurnRow(t *testing.T) {
	raw, err := os.ReadFile("runner.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	i := strings.Index(src, "withToolErrorReporter(ctx")
	if i < 0 || !strings.Contains(src[i:i+120], "runs.AddToolError(turnRunID)") {
		t.Error("runner.go does not give the turn a reporter that counts on the turn row")
	}
	if j := strings.Index(src, "runToolLoop("); i > j && j > 0 {
		t.Error("the reporter must be put in the context before the loop starts")
	}
}
