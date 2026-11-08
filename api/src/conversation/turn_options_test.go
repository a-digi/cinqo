package conversation

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// A second tool the caller may use, next to list_jobs (seedDomainTool seeds the first and the tool row they belong to).
func seedSecondTool(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO tool_mcp_tools (tool_id, name, description, input_schema, required_scope) VALUES ('t1', 'list_companies', 'd', '{}', 'tool:demo:read')`); err != nil {
		t.Fatal(err)
	}
}

func (e *retryEnv) startWith(t *testing.T, content string, opts TurnOptions) *conversation_entity.TurnRun {
	t.Helper()
	run, err := StartTurnRunWith(http.DefaultClient, e.mainDB, e.convDB, testEncryptionKey, e.conv, content, readScope, t.TempDir(), 0, opts)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func (e *retryEnv) finish(t *testing.T, run *conversation_entity.TurnRun, want string) {
	t.Helper()
	if status := waitForTurn(t, e.convDB, run.ID); status != want {
		t.Fatalf("turn = %q, want %q", status, want)
	}
}

// contents lists the texts of the user and assistant messages of one call to the model (the hint, which is a system message, is left out).
func contents(msgs []chatcompleter.Message) []string {
	var out []string
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "assistant" {
			out = append(out, m.Role+": "+m.Content)
		}
	}
	return out
}

func TestAFreshTurnIsSentTheNewMessageOnlyAndStaysInTheConversation(t *testing.T) {
	e := newRetryEnv(t, "opts-fresh", okAnswer("first answer"), okAnswer("second answer"), okAnswer("third answer"))
	e.finish(t, e.start(t, "first question"), "completed")
	e.finish(t, e.startWith(t, "second question", TurnOptions{Fresh: true}), "completed")
	e.finish(t, e.start(t, "third question"), "completed")

	if got, want := contents(e.platform.sent[1]), []string{"user: second question"}; !reflect.DeepEqual(got, want) {
		t.Errorf("a fresh turn is sent %v, want only %v", got, want)
	}
	// The same conversation, the turn after it: the history holds all three turns again, the fresh one included.
	want := []string{"user: first question", "assistant: first answer", "user: second question", "assistant: second answer", "user: third question"}
	if got := contents(e.platform.sent[2]); !reflect.DeepEqual(got, want) {
		t.Errorf("a normal turn after a fresh one is sent %v, want %v", got, want)
	}
	conv, _ := conversation_query.NewConversationQueryRepo(e.convDB).FindByID(e.conv)
	turns, err := ReadRecentTurns(conv.FilePath, 10)
	if err != nil || len(turns) != 3 || turns[1].UserContent != "second question" || turns[1].AssistantContent != "second answer" {
		t.Errorf("the fresh turn must be stored like any other: %v %+v", err, turns)
	}
}

func TestATurnThatIsNotFreshIsSentTheRecentTurnsAsBefore(t *testing.T) {
	e := newRetryEnv(t, "opts-normal", okAnswer("a1"), okAnswer("a2"))
	e.finish(t, e.start(t, "q1"), "completed")
	e.finish(t, e.start(t, "q2"), "completed")
	if got, want := contents(e.platform.sent[1]), []string{"user: q1", "assistant: a1", "user: q2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
}

func TestATurnsOwnToolListReplacesTheConversationsForThatTurnOnly(t *testing.T) {
	e := newRetryEnv(t, "opts-tools", okAnswer("a"), okAnswer("b"), okAnswer("c"))
	seedSecondTool(t, e.mainDB)
	e.finish(t, e.startWith(t, "q1", TurnOptions{AllowedTools: []string{"list_jobs"}}), "completed")
	e.finish(t, e.start(t, "q2"), "completed")
	e.finish(t, e.startWith(t, "q3", TurnOptions{AllowedTools: []string{"list_companies"}}), "completed")

	if got := e.platform.toolsOf[0]; !reflect.DeepEqual(got, []string{"list_jobs"}) {
		t.Errorf("turn 1 was offered %v, want only list_jobs", got)
	}
	if got := strings.Join(e.platform.toolsOf[1], ","); !strings.Contains(got, "list_jobs") || !strings.Contains(got, "list_companies") {
		t.Errorf("a turn without a list is offered the conversation's tools (every tool of a built-in platform): %v", got)
	}
	if got := e.platform.toolsOf[2]; !reflect.DeepEqual(got, []string{"list_companies"}) {
		t.Errorf("turn 3 was offered %v, want only list_companies", got)
	}
	// the conversation row itself is never changed
	conv, _ := conversation_query.NewConversationQueryRepo(e.convDB).FindByID(e.conv)
	if conv.AllowedTools != "" {
		t.Errorf("the conversation's own list changed: %q", conv.AllowedTools)
	}
}

func TestATurnsToolListNarrowsTheConversationsOwnListForThatTurn(t *testing.T) {
	e := newRetryEnv(t, "opts-own-list", okAnswer("a"), okAnswer("b"))
	seedSecondTool(t, e.mainDB)
	if _, err := e.convDB.Exec(`UPDATE conversations SET allowed_tools = ? WHERE id = ?`, `["list_jobs","list_companies"]`, e.conv); err != nil {
		t.Fatal(err)
	}
	e.finish(t, e.startWith(t, "q1", TurnOptions{AllowedTools: []string{"list_companies"}}), "completed")
	e.finish(t, e.start(t, "q2"), "completed")
	if got := e.platform.toolsOf[0]; !reflect.DeepEqual(got, []string{"list_companies"}) {
		t.Errorf("turn 1 was offered %v, want only list_companies", got)
	}
	if got := e.platform.toolsOf[1]; len(got) != 2 {
		t.Errorf("turn 2 is offered the conversation's own list (both): %v", got)
	}
}

func TestATurnsOptionsAreKeptForItsRetry(t *testing.T) {
	steps := append(failedAfterAToolRound(), okAnswer("recovered"))
	e := newRetryEnv(t, "opts-retry", steps...)
	seedSecondTool(t, e.mainDB)
	run := e.startWith(t, "make a persona", TurnOptions{Fresh: true, AllowedTools: []string{"list_jobs"}})
	e.finish(t, run, "failed")
	retry, err := e.retry(t, run.ID, false, readScope)
	if err != nil {
		t.Fatal(err)
	}
	e.finish(t, retry, "completed")

	q := conversation_query.NewTurnRunQueryRepo(e.convDB)
	for name, id := range map[string]string{"the failed run": run.ID, "the retry": retry.ID} {
		fresh, tools, err := q.FindOptions(id)
		if err != nil || !fresh || tools != `["list_jobs"]` {
			t.Errorf("%s: options = %v %q %v", name, fresh, tools, err)
		}
	}
	// the retried call (the fifth) is offered the turn's list, not both tools
	if got := e.platform.toolsOf[4]; !reflect.DeepEqual(got, []string{"list_jobs"}) {
		t.Errorf("the retry was offered %v, want only list_jobs", got)
	}
}

func TestATurnWithNoOptionsStoresNone(t *testing.T) {
	e := newRetryEnv(t, "opts-none", okAnswer("a"))
	run := e.start(t, "q")
	e.finish(t, run, "completed")
	fresh, tools, err := conversation_query.NewTurnRunQueryRepo(e.convDB).FindOptions(run.ID)
	if err != nil || fresh || tools != "" {
		t.Errorf("options = %v %q %v, want none", fresh, tools, err)
	}
}

func TestTurnOptionsRoundTripThroughTheRepositories(t *testing.T) {
	db := openConversationDB(t)
	conv := keylessConversation(t, db, "opts-repo")
	runs := conversation_persistent.NewTurnRunPersistentRepo(db)
	q := conversation_query.NewTurnRunQueryRepo(db)
	mk := func(id string) {
		t.Helper()
		if err := runs.Insert(&conversation_entity.TurnRun{ID: id, ConversationID: conv, UserContent: "x", StartedAt: "2026-11-08T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		if err := runs.SetTerminalStatus(id, "completed", "2026-11-08T00:00:01Z"); err != nil {
			t.Fatal(err)
		}
	}
	mk("a")
	mk("b")
	mk("c")
	if f, tools, err := q.FindOptions("a"); err != nil || f || tools != "" {
		t.Errorf("a new run has no options: %v %q %v", f, tools, err)
	}
	if err := runs.SetOptions("a", true, `["x","y"]`); err != nil {
		t.Fatal(err)
	}
	if f, tools, err := q.FindOptions("a"); err != nil || !f || tools != `["x","y"]` {
		t.Errorf("stored options: %v %q %v", f, tools, err)
	}
	if err := runs.SetOptions("b", false, ""); err != nil {
		t.Fatal(err)
	}
	if f, tools, err := q.FindOptions("b"); err != nil || f || tools != "" {
		t.Errorf("an empty list is stored as none: %v %q %v", f, tools, err)
	}
	var isNull bool
	if err := db.QueryRow(`SELECT allowed_tools IS NULL FROM turn_runs WHERE id = 'b'`).Scan(&isNull); err != nil || !isNull {
		t.Errorf("an empty list must be NULL in the column: %v %v", isNull, err)
	}
	if err := runs.CopyOptions("a", "c"); err != nil {
		t.Fatal(err)
	}
	if f, tools, err := q.FindOptions("c"); err != nil || !f || tools != `["x","y"]` {
		t.Errorf("copied options: %v %q %v", f, tools, err)
	}
	if f, tools, err := q.FindOptions("a"); err != nil || !f || tools != `["x","y"]` {
		t.Errorf("copying must leave the source alone: %v %q %v", f, tools, err)
	}
	if _, _, err := q.FindOptions("missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("an unknown run: %v", err)
	}
}

func TestTheRunnerReadsTheTurnOptionsBeforeItOffersTools(t *testing.T) {
	data, err := os.ReadFile("runner.go")
	if err != nil {
		t.Fatal(err)
	}
	raw := string(data)
	opts := strings.Index(raw, "FindOptions(turnRunID)")
	offer := strings.Index(raw, "ctx, tools = offeredTools(ctx, entry, model, tools, conv)")
	history := strings.Index(raw, "ReadRecentTurns(conv.FilePath, ContextTurns)")
	if opts < 0 || offer < 0 || history < 0 || !(opts < history && opts < offer) {
		t.Errorf("the turn options must be read (%d) before the history is built (%d) and the tools are offered (%d)", opts, history, offer)
	}
}

func TestATurnCanNeverWidenTheConversationsOwnList(t *testing.T) {
	e := newRetryEnv(t, "opts-never-widen", okAnswer("a"))
	seedSecondTool(t, e.mainDB)
	if _, err := e.convDB.Exec(`UPDATE conversations SET allowed_tools = ? WHERE id = ?`, `["list_jobs"]`, e.conv); err != nil {
		t.Fatal(err)
	}
	e.finish(t, e.startWith(t, "q1", TurnOptions{AllowedTools: []string{"list_companies", "list_jobs"}}), "completed")
	if got := e.platform.toolsOf[0]; !reflect.DeepEqual(got, []string{"list_jobs"}) {
		t.Errorf("the turn asked for a tool the conversation does not have and was offered %v, want only list_jobs", got)
	}
}

func TestNarrowToolList(t *testing.T) {
	for _, c := range []struct {
		name         string
		conversation []string
		turn         string
		want         string
	}{
		{"no conversation list: the turn's list", nil, `["a","b"]`, `["a","b"]`},
		{"both: the tools in both, in the turn's order", []string{"b", "a", "c"}, `["c","a"]`, `["c","a"]`},
		{"a tool the conversation lacks is dropped", []string{"a"}, `["a","x"]`, `["a"]`},
		{"nothing in common: an empty list, never everything", []string{"a"}, `["x"]`, `[]`},
		{"an empty conversation list allows nothing", []string{}, `["a"]`, `[]`},
		{"an unreadable turn list fails closed", []string{"a"}, `not json`, `[]`},
		{"an unreadable turn list fails closed without a conversation list too", nil, `not json`, `[]`},
		{"a null turn list fails closed", []string{"a"}, `null`, `[]`},
	} {
		if got := narrowToolList(c.conversation, c.turn); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}
