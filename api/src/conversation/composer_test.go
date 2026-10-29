package conversation

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
)

// openMainDB applies every real main-database migration, in file-name
// order, to a fresh in-memory database.
func openMainDB(t *testing.T) *sql.DB { return openMigratedDB(t, "migrations") }

// openConversationDB does the same for the conversation database.
func openConversationDB(t *testing.T) *sql.DB {
	return openMigratedDB(t, "migrations-conversation")
}

func openMigratedDB(t *testing.T, migrationsDir string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	dir := filepath.Join("..", "..", "config", "db", migrationsDir)
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations in %s (err %v)", dir, err)
	}
	sort.Strings(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(raw), "/***Statement***/") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", filepath.Base(f), err)
			}
		}
	}
	return db
}

// seedDomainTool installs one enabled tool with one MCP tool, so the
// direct path has something real to offer.
func seedDomainTool(t *testing.T, db *sql.DB, mcpName, scope string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO tools (id, slug, name, version, kind, enabled, install_path, backend_executable_relpath) VALUES ('t1', 'demo', 'Demo', '1.0.0', 'backend_only', 1, '/nonexistent', 'tool')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tool_mcp_tools (tool_id, name, description, input_schema, required_scope) VALUES ('t1', ?, 'd', '{}', ?)`, mcpName, scope); err != nil {
		t.Fatal(err)
	}
}

func toolNames(tools []chatcompleter.ToolDef) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	sort.Strings(out)
	return out
}

func TestOfferableToolsForMode(t *testing.T) {
	db := openMainDB(t)
	seedDomainTool(t, db, "list_jobs", "tool:demo:read")
	scopes := []string{"tool:demo:read"}

	direct, err := offerableToolsForMode(db, scopes, conversation_entity.OrchestrationModeDirect)
	if err != nil {
		t.Fatal(err)
	}
	want, err := offerableTools(db, scopes, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := toolNames(direct); strings.Join(got, ",") != strings.Join(toolNames(want), ",") {
		t.Fatalf("direct must equal today's offerableTools: got %v want %v", got, toolNames(want))
	}
	if !containsName(direct, "list_jobs") || !containsName(direct, subAgentToolName) {
		t.Fatalf("direct must still offer domain and sub-agent tools: %v", toolNames(direct))
	}

	// The composer never reads the tool tables: a nil DB proves it.
	composer, err := offerableToolsForMode(nil, scopes, conversation_entity.OrchestrationModeComposer)
	if err != nil || strings.Join(toolNames(composer), ",") != runPlanToolName {
		t.Fatalf("composer must offer only run_plan: %v (err %v)", toolNames(composer), err)
	}

	// An empty/unknown mode behaves as direct (never as the restricted one).
	if got, _ := offerableToolsForMode(db, scopes, ""); !containsName(got, "list_jobs") {
		t.Fatalf("empty mode must behave as direct, got %v", toolNames(got))
	}
}

func containsName(tools []chatcompleter.ToolDef, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func TestRefusesUnofferedCall(t *testing.T) {
	offered := []chatcompleter.ToolDef{{Name: "run_plan"}}
	cases := []struct {
		name  string
		mode  string
		depth int
		tool  string
		want  bool
	}{
		{"composer: a domain tool is refused", conversation_entity.OrchestrationModeComposer, 0, "list_jobs", true},
		{"composer: spawn_subagent is refused", conversation_entity.OrchestrationModeComposer, 0, subAgentToolName, true},
		{"composer: crawl fan-out is refused", conversation_entity.OrchestrationModeComposer, 0, crawlURLsWithSubagentsToolName, true},
		{"composer: an offered tool is allowed", conversation_entity.OrchestrationModeComposer, 0, "run_plan", false},
		{"a sub-agent (depth>0) cannot call an unoffered tool", conversation_entity.OrchestrationModeDirect, 1, "list_jobs", true},
		{"a sub-agent cannot spawn past its depth limit by name", conversation_entity.OrchestrationModeDirect, 2, subAgentToolName, true},
		{"a sub-agent can call a tool it was offered", conversation_entity.OrchestrationModeDirect, 1, "run_plan", false},
		{"direct orchestrator (depth 0): never refuses here", conversation_entity.OrchestrationModeDirect, 0, "list_jobs", false},
		{"empty mode at depth 0: never refuses here", "", 0, "list_jobs", false},
	}
	for _, c := range cases {
		if got := refusesUnofferedCall(c.mode, c.depth, c.tool, offered); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// scriptedCompleter replies with a tool call on its first call and a
// plain answer on its second, recording what the model was offered and
// what the tool message said.
type scriptedCompleter struct {
	callName     string
	calls        int
	offered      [][]string
	toolMessages []string
}

func (s *scriptedCompleter) ChatCompletion(_ context.Context, _ *http.Client, _, _, _ string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	s.calls++
	s.offered = append(s.offered, toolNames(tools))
	for _, m := range messages {
		if m.Role == "tool" {
			s.toolMessages = append(s.toolMessages, m.Content)
		}
	}
	if s.calls == 1 {
		return &chatcompleter.ChatCompletionResult{
			Message:   chatcompleter.Message{Role: "assistant"},
			ToolCalls: []chatcompleter.ToolCall{{ID: "c1", Name: s.callName, Arguments: []byte(`{}`)}},
		}, nil
	}
	return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: "final answer"}}, nil
}

func runScripted(t *testing.T, mode string) *scriptedCompleter {
	t.Helper()
	db := openMainDB(t)
	seedDomainTool(t, db, "list_jobs", "tool:demo:read")
	scopes := []string{"tool:demo:read"}

	tools, err := offerableToolsForMode(db, scopes, mode)
	if err != nil {
		t.Fatal(err)
	}
	completer := &scriptedCompleter{callName: "list_jobs"}
	got, err := runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: completer}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, tools, db, scopes, "u1", t.TempDir(), 0, "conv", nil, "", 0, mode, nil, nil, nil, 0)
	if err != nil || got != "final answer" {
		t.Fatalf("loop: %q, %v", got, err)
	}
	return completer
}

func TestComposerLoopNeverReachesADomainTool(t *testing.T) {
	c := runScripted(t, conversation_entity.OrchestrationModeComposer)
	if strings.Join(c.offered[0], ",") != runPlanToolName {
		t.Errorf("the composer model must be offered only run_plan, got %v", c.offered[0])
	}
	if len(c.toolMessages) == 0 || c.toolMessages[0] != "tool unavailable" {
		t.Fatalf("a domain tool call must be refused with \"tool unavailable\", got %v", c.toolMessages)
	}
}

func TestDirectLoopStillReachesTheToolPath(t *testing.T) {
	c := runScripted(t, conversation_entity.OrchestrationModeDirect)
	if !containsString(c.offered[0], "list_jobs") {
		t.Errorf("direct must still offer the domain tool, got %v", c.offered[0])
	}
	// The call went through invokeToolCall (which fails on the fake
	// install path) — i.e. it was NOT short-circuited by the composer guard.
	if len(c.toolMessages) == 0 || c.toolMessages[0] == "tool unavailable" {
		t.Fatalf("direct must reach invokeToolCall, got %v", c.toolMessages)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
