package conversation

import (
	"context"
	"database/sql"
	"net/http"
	"reflect"
	"strings"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/tool/mcp"
)

// plan/ai/conversation/step-45, step 5: a provider's prompt cache reuses the START of a request, so what comes first must be
// byte-identical from round to round: the history before the first tool round (system text, earlier turns, the new user message) and
// the tool list in its order. Only the tool rounds at the end may grow.

type prefixModel struct {
	requests [][]chatcompleter.Message
	tools    [][]chatcompleter.ToolDef
	n        int
}

func (m *prefixModel) ChatCompletion(_ context.Context, _ *http.Client, _, _, _ string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	m.requests = append(m.requests, append([]chatcompleter.Message(nil), messages...))
	m.tools = append(m.tools, append([]chatcompleter.ToolDef(nil), tools...))
	i := m.n
	m.n++
	if i >= 8 || len(tools) == 0 {
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: "done"}, FinishReason: "stop"}, nil
	}
	return &chatcompleter.ChatCompletionResult{
		Message:   chatcompleter.Message{Role: "assistant"},
		ToolCalls: []chatcompleter.ToolCall{{ID: "c", Name: "find_repeating_items", Arguments: []byte(`{"url":"u` + string(rune('0'+i)) + `"}`)}},
	}, nil
}

func runPrefix(t *testing.T, economy bool, prefix []chatcompleter.Message, limit int) *prefixModel {
	t.Helper()
	model := &prefixModel{}
	old := invokeToolFn
	t.Cleanup(func() { invokeToolFn = old })
	invokeToolFn = func(_ context.Context, _ *sql.DB, _ []string, _ string, _ chatcompleter.ToolCall, _ string, _ int, _ string) (string, []mcp.ResourceLink, *pendingDelegation) {
		return bigText(4000), nil, nil
	}
	ctx := context.Background()
	if economy {
		ctx = withToolLoopEconomy(ctx, &conversation_entity.Conversation{CompactToolResults: true})
	}
	if _, err := runToolLoop(ctx, http.DefaultClient, platform.Entry{Completer: model}, "k", "m", prefix, economyTools(), nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, limit); err != nil {
		t.Fatal(err)
	}
	return model
}

func TestTheStartOfEveryRequestIsIdenticalFromRoundToRound(t *testing.T) {
	prefix := []chatcompleter.Message{
		{Role: "system", Content: "You are a careful assistant."},
		{Role: "user", Content: "an earlier question"},
		{Role: "assistant", Content: "an earlier answer"},
		{Role: "user", Content: "the new task"},
	}
	for _, economy := range []bool{false, true} {
		model := runPrefix(t, economy, prefix, 10)
		if len(model.requests) < 6 {
			t.Fatalf("economy=%v: only %d requests", economy, len(model.requests))
		}
		first := model.requests[0]
		for i, req := range model.requests {
			if !reflect.DeepEqual(req[:len(prefix)], first[:len(prefix)]) {
				t.Errorf("economy=%v: request %d starts differently from request 0:\n%+v\n%+v", economy, i, req[:len(prefix)], first[:len(prefix)])
			}
			if !reflect.DeepEqual(model.tools[i], model.tools[0]) {
				t.Errorf("economy=%v: request %d offers other tools than request 0", economy, i)
			}
			if i > 0 && len(req) <= len(model.requests[i-1])-1 {
				t.Errorf("economy=%v: request %d is not longer than the one before", economy, i)
			}
		}
		// the original messages the caller passed are never touched (the cache breakpoint is set on a copy)
		if prefix[len(prefix)-1].CacheBreakpoint {
			t.Errorf("economy=%v: the caller's last message was marked in place", economy)
		}
		// the breakpoint sits on the last message of the prefix in every request, and only there before the tool rounds
		for i, req := range model.requests {
			if !req[len(prefix)-1].CacheBreakpoint {
				t.Errorf("economy=%v: request %d has no cache breakpoint at the end of the prefix", economy, i)
			}
			for j := 0; j < len(prefix)-1; j++ {
				if req[j].CacheBreakpoint {
					t.Errorf("economy=%v: request %d marks message %d", economy, i, j)
				}
			}
		}
	}
}

// What has already been sent stays as it was: a round's own messages are never rewritten by a later request, except by the one-time
// fold of an old bulky result when the economy is on (a stub, then unchanged).
func TestEarlierRoundsStayAsTheyWereWithoutTheEconomy(t *testing.T) {
	prefix := []chatcompleter.Message{{Role: "user", Content: "task"}}
	model := runPrefix(t, false, prefix, 10)
	for i := 1; i < len(model.requests); i++ {
		if !reflect.DeepEqual(model.requests[i][:len(model.requests[i-1])], model.requests[i-1]) {
			t.Fatalf("request %d rewrote what request %d already contained", i, i-1)
		}
	}
}

func TestFoldedResultsKeepTheirStubOnceFolded(t *testing.T) {
	prefix := []chatcompleter.Message{{Role: "user", Content: "task"}}
	model := runPrefix(t, true, prefix, 12)
	stubs := 0
	// a result folded in request i has the very same stub in every later request: the cache is broken once per folded round, not again
	for i := 1; i < len(model.requests); i++ {
		prev, cur := model.requests[i-1], model.requests[i]
		for j := range prev {
			if strings.HasPrefix(prev[j].Content, "[earlier result of") {
				stubs++
				if cur[j].Content != prev[j].Content {
					t.Errorf("request %d changed the stub at message %d: %q -> %q", i, j, prev[j].Content, cur[j].Content)
				}
			}
		}
	}
	if stubs == 0 {
		t.Fatal("no result was folded: the test checks nothing")
	}
}

// The tool list is part of every request: it comes in a fixed order (by tool, then name), whatever order the rows were stored in, so a
// provider cache sees the same list in every turn.
func TestTheOfferedToolsComeInAFixedOrder(t *testing.T) {
	db := openMigratedDB(t, "migrations")
	for _, tool := range []struct{ id, slug string }{{"t2", "zeta"}, {"t1", "alpha"}} {
		if _, err := db.Exec(`INSERT INTO tools (id, slug, name, version, kind, enabled, install_path, backend_executable_relpath) VALUES (?, ?, ?, '1.0.0', 'backend_only', 1, '/nonexistent', 'tool')`, tool.id, tool.slug, tool.slug); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range []struct{ tool, name string }{{"t2", "z_last"}, {"t1", "b_second"}, {"t2", "a_first_of_zeta"}, {"t1", "a_first"}} {
		if _, err := db.Exec(`INSERT INTO tool_mcp_tools (tool_id, name, description, input_schema, required_scope) VALUES (?, ?, 'd', '{}', 's')`, m.tool, m.name); err != nil {
			t.Fatal(err)
		}
	}
	got, err := offerableTools(db, []string{"s"}, false)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range got {
		names = append(names, tool.Name)
	}
	want := []string{"a_first", "b_second", "a_first_of_zeta", "z_last"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("order %v, want %v", names, want)
	}
}
