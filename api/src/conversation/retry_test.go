package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/platform/entity"
	tool_mcp "github.com/a-digi/cinqo/src/tool/mcp"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// scriptedPlatform answers from a script and records what every call was sent (the messages and the tool names).
type scriptedPlatform struct {
	mu       sync.Mutex
	steps    []func() (*chatcompleter.ChatCompletionResult, error)
	sent     [][]chatcompleter.Message
	toolsOf  [][]string
	callsNum int
}

func (p *scriptedPlatform) ChatCompletion(_ context.Context, _ *http.Client, _, _, _ string, msgs []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := p.callsNum
	p.callsNum++
	p.sent = append(p.sent, append([]chatcompleter.Message(nil), msgs...))
	p.toolsOf = append(p.toolsOf, toolNames(tools))
	if i >= len(p.steps) {
		return nil, errors.New("script exhausted")
	}
	return p.steps[i]()
}

func (p *scriptedPlatform) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callsNum
}

func toolCallStep(name string) func() (*chatcompleter.ChatCompletionResult, error) {
	return func() (*chatcompleter.ChatCompletionResult, error) {
		return &chatcompleter.ChatCompletionResult{
			Message:   chatcompleter.Message{Role: "assistant"},
			ToolCalls: []chatcompleter.ToolCall{{ID: "call-1", Name: name, Arguments: json.RawMessage(`{}`)}},
		}, nil
	}
}

func emptyStep() (*chatcompleter.ChatCompletionResult, error) { return emptyAnswer() }

type retryEnv struct {
	mainDB, convDB *sql.DB
	conv           string
	platform       *scriptedPlatform
}

// newRetryEnv registers a scripted platform (the registry cannot be emptied, so the id is unique per test), seeds one tool the caller may
// use and a conversation on that platform.
func newRetryEnv(t *testing.T, id string, steps ...func() (*chatcompleter.ChatCompletionResult, error)) *retryEnv {
	t.Helper()
	fastRetries(t)
	p := &scriptedPlatform{steps: steps}
	if err := platform.Register(platform.Entry{
		Info: entity.Platform{ID: id, Name: id}, DefaultBaseURL: "http://must-not-be-used.invalid", Completer: p, Credentials: fixedCredentials,
		ToolHint: "HINT: use the tools",
	}); err != nil {
		t.Fatal(err)
	}
	e := &retryEnv{mainDB: openMainDB(t), convDB: openConversationDB(t), platform: p}
	seedDomainTool(t, e.mainDB, "list_jobs", "tool:demo:read")
	e.conv = keylessConversation(t, e.convDB, id)
	return e
}

var readScope = []string{"tool:demo:read"}

func (e *retryEnv) start(t *testing.T, content string) *conversation_entity.TurnRun {
	t.Helper()
	run, err := StartTurnRun(http.DefaultClient, e.mainDB, e.convDB, testEncryptionKey, e.conv, content, readScope, t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func (e *retryEnv) retry(t *testing.T, failedID string, confirmed bool, scopes []string) (*conversation_entity.TurnRun, error) {
	t.Helper()
	return StartRetryRun(http.DefaultClient, e.mainDB, e.convDB, testEncryptionKey, e.conv, failedID, confirmed, scopes, t.TempDir(), 0)
}

func (e *retryEnv) checkpoint(t *testing.T, id string) string {
	t.Helper()
	text, err := conversation_query.NewTurnRunQueryRepo(e.convDB).FindCheckpoint(id)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func (e *retryEnv) run(t *testing.T, id string) *conversation_entity.TurnRun {
	t.Helper()
	r, err := conversation_query.NewTurnRunQueryRepo(e.convDB).FindByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// failedAfterAToolRound: the first call asks for a tool, then the model answers nothing three times (the call and its two retries).
func failedAfterAToolRound() []func() (*chatcompleter.ChatCompletionResult, error) {
	return []func() (*chatcompleter.ChatCompletionResult, error){toolCallStep("list_jobs"), emptyStep, emptyStep, emptyStep}
}

func TestAFailedTurnKeepsACheckpointAndARetryRedoesOnlyTheLastCall(t *testing.T) {
	steps := append(failedAfterAToolRound(), okAnswer("recovered"))
	e := newRetryEnv(t, "retry-ok", steps...)
	run := e.start(t, "make a persona")
	if status := waitForTurn(t, e.convDB, run.ID); status != "failed" {
		t.Fatalf("first turn = %q, want failed", status)
	}
	if got := e.platform.calls(); got != 4 {
		t.Fatalf("calls before the retry = %d, want 4 (the tool call, the empty reply and its two retries)", got)
	}
	cp, err := DecodeCheckpoint(e.checkpoint(t, run.ID))
	if err != nil {
		t.Fatalf("the failed turn must keep a decodable checkpoint: %v", err)
	}
	last := cp.Messages[len(cp.Messages)-1]
	if last.Role != "tool" || last.ToolCallID != "call-1" {
		t.Fatalf("the saved state must end with the tool result the failed call was sent: %+v", last)
	}

	retry, err := e.retry(t, run.ID, false, readScope)
	if err != nil {
		t.Fatal(err)
	}
	if retry.RetryOf == nil || *retry.RetryOf != run.ID || retry.RetryCount != 1 {
		t.Errorf("the retry must record its chain: %+v", retry)
	}
	if status := waitForTurn(t, e.convDB, retry.ID); status != "completed" {
		t.Fatalf("retry = %q, want completed", status)
	}
	if got := e.platform.calls(); got != 5 {
		t.Errorf("the retry made %d more calls, want exactly 1 (the tool round must not be redone)", got-4)
	}
	if sent := e.platform.sent[4]; len(sent) != len(cp.Messages) || sent[len(sent)-1].Content != last.Content {
		t.Errorf("the retried call must be sent the saved messages (%d), got %d", len(cp.Messages), len(sent))
	}
	if e.checkpoint(t, run.ID) != "" || e.checkpoint(t, retry.ID) != "" {
		t.Error("after a successful retry no checkpoint is left")
	}
	hints := 0
	for _, m := range e.platform.sent[4] {
		if m.Role == "system" && strings.HasPrefix(m.Content, "HINT") {
			hints++
		}
	}
	if hints != 1 {
		t.Errorf("the retried call carries the tool hint %d times, want once (the saved messages already hold it)", hints)
	}
	if !strings.Contains(strings.Join(e.platform.toolsOf[4], ","), "list_jobs") {
		t.Errorf("the retry must be offered the tool the caller may use: %v", e.platform.toolsOf[4])
	}
	if got := e.run(t, retry.ID).UserContent; got != "make a persona" {
		t.Errorf("the retry run keeps the user's text: %q", got)
	}
	if r := e.run(t, retry.ID); r.RetryOf == nil || *r.RetryOf != run.ID || r.RetryCount != 1 {
		t.Errorf("stored retry fields: %+v", r)
	}
	conv, _ := conversation_query.NewConversationQueryRepo(e.convDB).FindByID(e.conv)
	turns, err := ReadRecentTurns(conv.FilePath, 10)
	if err != nil || len(turns) == 0 {
		t.Fatalf("turns: %v %v", turns, err)
	}
	if final := turns[len(turns)-1]; final.Failed || final.AssistantContent != "recovered" || final.UserContent != "make a persona" {
		t.Errorf("the log must end with the recovered turn for the same message: %+v", final)
	}
}

func TestATurnThatFailedBeforeAnyToolRoundHasNothingToRetry(t *testing.T) {
	e := newRetryEnv(t, "retry-early", emptyStep, emptyStep, emptyStep)
	run := e.start(t, "hi")
	if status := waitForTurn(t, e.convDB, run.ID); status != "failed" {
		t.Fatalf("status = %q", status)
	}
	if e.checkpoint(t, run.ID) != "" {
		t.Error("a turn with no tool round keeps no checkpoint (a retry would equal Start again)")
	}
	if _, err := e.retry(t, run.ID, false, readScope); !errors.Is(err, ErrTurnNotRetryable) {
		t.Errorf("err = %v, want ErrTurnNotRetryable", err)
	}
}

func TestARetryThatFailsAgainIsRetryableAgainWithTheCountRising(t *testing.T) {
	steps := append(failedAfterAToolRound(), emptyStep, emptyStep, emptyStep, okAnswer("third time lucky"))
	e := newRetryEnv(t, "retry-chain", steps...)
	run := e.start(t, "x")
	waitForTurn(t, e.convDB, run.ID)

	r1, err := e.retry(t, run.ID, false, readScope)
	if err != nil {
		t.Fatal(err)
	}
	if status := waitForTurn(t, e.convDB, r1.ID); status != "failed" {
		t.Fatalf("retry 1 = %q, want failed", status)
	}
	if e.checkpoint(t, r1.ID) == "" || e.checkpoint(t, run.ID) != "" {
		t.Error("the state moves on to the newest failed run, and only it keeps one")
	}
	r2, err := e.retry(t, r1.ID, false, readScope)
	if err != nil {
		t.Fatal(err)
	}
	if r2.RetryCount != 2 || r2.RetryOf == nil || *r2.RetryOf != r1.ID {
		t.Errorf("the second retry: %+v", r2)
	}
	if status := waitForTurn(t, e.convDB, r2.ID); status != "completed" {
		t.Fatalf("retry 2 = %q", status)
	}
	if _, err := e.retry(t, run.ID, false, readScope); !errors.Is(err, ErrTurnNotRetryable) {
		t.Errorf("an older run cannot be retried: %v", err)
	}
}

func TestARetryThatCannotStartTheModelKeepsTheStateForTheNextRetry(t *testing.T) {
	steps := append(failedAfterAToolRound(), okAnswer("never asked"))
	e := newRetryEnv(t, "retry-carry", steps...)
	run := e.start(t, "x")
	waitForTurn(t, e.convDB, run.ID)
	// Retry as a caller whose conversation is on a platform that has gone: the run fails before calling the model.
	if _, err := e.convDB.Exec(`UPDATE conversations SET platform_id = 'gone-platform' WHERE id = ?`, e.conv); err != nil {
		t.Fatal(err)
	}
	r1, err := e.retry(t, run.ID, false, readScope)
	if err != nil {
		t.Fatal(err)
	}
	if status := waitForTurn(t, e.convDB, r1.ID); status != "failed" {
		t.Fatalf("status = %q", status)
	}
	if e.checkpoint(t, r1.ID) == "" {
		t.Error("a retry that failed before calling the model must still be retryable (the state was carried onto it)")
	}
}

func TestAfterThreeFailedRetriesAnotherNeedsConfirmation(t *testing.T) {
	steps := append(failedAfterAToolRound(), okAnswer("done"))
	e := newRetryEnv(t, "retry-confirm", steps...)
	run := e.start(t, "x")
	waitForTurn(t, e.convDB, run.ID)
	runs := conversation_persistent.NewTurnRunPersistentRepo(e.convDB)
	count := func() int {
		var n int
		if err := e.convDB.QueryRow(`SELECT COUNT(*) FROM turn_runs`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	for retries := 0; retries < 3; retries++ { // 0, 1 and 2 failed retries need no confirmation: the 3rd retry is allowed
		if err := runs.SetRetry(run.ID, "earlier", retries); err != nil {
			t.Fatal(err)
		}
		if _, err := e.convDB.Exec(`UPDATE turn_runs SET status = 'failed' WHERE id = ?`, run.ID); err != nil {
			t.Fatal(err)
		}
		before := count()
		r, err := e.retry(t, run.ID, false, readScope)
		if err != nil {
			t.Fatalf("with %d failed retries: %v", retries, err)
		}
		waitForTurn(t, e.convDB, r.ID)
		// put the state back for the next round of this loop: the retried run took it
		if err := runs.SaveCheckpoint(run.ID, savedStateText(t)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.convDB.Exec(`DELETE FROM turn_runs WHERE id = ?`, r.ID); err != nil {
			t.Fatal(err)
		}
		if count() != before {
			t.Fatalf("rows = %d, want %d", count(), before)
		}
	}

	if err := runs.SetRetry(run.ID, "earlier", 3); err != nil {
		t.Fatal(err)
	}
	before, callsBefore := count(), e.platform.calls()
	_, err := e.retry(t, run.ID, false, readScope)
	var confirm *RetryConfirmationRequiredError
	if !errors.As(err, &confirm) || confirm.RetryCount != 3 {
		t.Fatalf("err = %v, want a confirmation request for 3", err)
	}
	if count() != before || e.platform.calls() != callsBefore {
		t.Errorf("an unconfirmed retry must start nothing: rows %d (was %d), calls %d", count(), before, e.platform.calls())
	}
	r, err := e.retry(t, run.ID, true, readScope)
	if err != nil {
		t.Fatalf("confirmed: %v", err)
	}
	if r.RetryCount != 4 {
		t.Errorf("RetryCount = %d, want 4", r.RetryCount)
	}
	waitForTurn(t, e.convDB, r.ID)
}

func savedStateText(t *testing.T) string {
	t.Helper()
	text, err := EncodeCheckpoint(Checkpoint{Messages: []chatcompleter.Message{{Role: "user", Content: "x"}, {Role: "tool", Content: "r", ToolCallID: "call-1"}}})
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestARetryIsRefusedForTheWrongRun(t *testing.T) {
	e := newRetryEnv(t, "retry-wrong", failedAfterAToolRound()...)
	run := e.start(t, "x")
	waitForTurn(t, e.convDB, run.ID)

	if _, err := e.retry(t, "no-such-run", false, readScope); !errors.Is(err, ErrTurnRunNotFound) {
		t.Errorf("unknown run: %v", err)
	}
	// a run of another conversation reads as not found, never as retryable
	if err := conversation_persistent.NewConversationPersistentRepo(e.convDB).Insert(&conversation_entity.Conversation{ID: "other", UserID: "u1", Title: "t", FilePath: "x", PlatformID: "retry-wrong", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if _, err := StartRetryRun(http.DefaultClient, e.mainDB, e.convDB, testEncryptionKey, "other", run.ID, false, readScope, t.TempDir(), 0); !errors.Is(err, ErrTurnRunNotFound) {
		t.Errorf("another conversation's run: %v", err)
	}
	// a completed run is not retryable
	if _, err := e.convDB.Exec(`UPDATE turn_runs SET status = 'completed' WHERE id = ?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.retry(t, run.ID, false, readScope); !errors.Is(err, ErrTurnNotRetryable) {
		t.Errorf("a completed run: %v", err)
	}
}

func TestARetryIsRefusedWhileATurnIsRunning(t *testing.T) {
	e := newRetryEnv(t, "retry-busy", failedAfterAToolRound()...)
	run := e.start(t, "x")
	waitForTurn(t, e.convDB, run.ID)
	if err := conversation_persistent.NewTurnRunPersistentRepo(e.convDB).Insert(&conversation_entity.TurnRun{ID: "busy", ConversationID: e.conv, UserContent: "y", StartedAt: "2999-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.retry(t, run.ID, false, readScope); !errors.Is(err, ErrTurnAlreadyRunning) {
		t.Errorf("err = %v, want ErrTurnAlreadyRunning", err)
	}
}

func TestAnewTurnEndsTheChanceToRetryAnEarlierFailedOne(t *testing.T) {
	steps := append(failedAfterAToolRound(), okAnswer("fresh"))
	e := newRetryEnv(t, "retry-fresh", steps...)
	run := e.start(t, "x")
	waitForTurn(t, e.convDB, run.ID)
	if e.checkpoint(t, run.ID) == "" {
		t.Fatal("precondition: the failed turn has a checkpoint")
	}
	fresh := e.start(t, "start again")
	waitForTurn(t, e.convDB, fresh.ID)
	if e.checkpoint(t, run.ID) != "" {
		t.Error("starting a new turn must clear the older checkpoint")
	}
	if _, err := e.retry(t, run.ID, false, readScope); !errors.Is(err, ErrTurnNotRetryable) {
		t.Errorf("err = %v", err)
	}
}

func TestARetryOffersOnlyWhatTheCallerMayUseNow(t *testing.T) {
	steps := append(failedAfterAToolRound(), okAnswer("ok"))
	e := newRetryEnv(t, "retry-scopes", steps...)
	run := e.start(t, "x")
	waitForTurn(t, e.convDB, run.ID)
	if !strings.Contains(strings.Join(e.platform.toolsOf[0], ","), "list_jobs") {
		t.Fatalf("precondition: the first call was offered the tool: %v", e.platform.toolsOf[0])
	}
	r, err := e.retry(t, run.ID, false, nil) // the caller has lost the scope since
	if err != nil {
		t.Fatal(err)
	}
	waitForTurn(t, e.convDB, r.ID)
	if got := strings.Join(e.platform.toolsOf[4], ","); strings.Contains(got, "list_jobs") {
		t.Errorf("the retry was offered %v: the tool needs a scope the caller no longer has (scopes are the caller's now, not the failed run's)", got)
	}
}

func TestACheckpointIsNeverPartOfAnyTurnRunJSON(t *testing.T) {
	e := newRetryEnv(t, "retry-json", failedAfterAToolRound()...)
	run := e.start(t, "x")
	waitForTurn(t, e.convDB, run.ID)
	raw, err := json.Marshal(e.run(t, run.ID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "checkpoint") || strings.Contains(string(raw), "call-1") {
		t.Errorf("the checkpoint leaked into the turn run: %s", raw)
	}
}

func TestAPlainProviderErrorAfterAToolRoundAlsoKeepsACheckpoint(t *testing.T) {
	e := newRetryEnv(t, "retry-plain-error", toolCallStep("list_jobs"), func() (*chatcompleter.ChatCompletionResult, error) {
		return nil, errors.New("the provider said no")
	})
	run := e.start(t, "x")
	if status := waitForTurn(t, e.convDB, run.ID); status != "failed" {
		t.Fatalf("status = %q", status)
	}
	if e.checkpoint(t, run.ID) == "" {
		t.Error("a failure of the model call itself (not only an empty reply) keeps the state")
	}
}

func TestTheLinksASavedStateCollectedAreListedInTheRetriedAnswer(t *testing.T) {
	e := newRetryEnv(t, "retry-links", okAnswer("here you go"))
	runs := conversation_persistent.NewTurnRunPersistentRepo(e.convDB)
	if err := runs.Insert(&conversation_entity.TurnRun{ID: "failed-run", ConversationID: e.conv, UserContent: "make the pdf", StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.SetTerminalStatus("failed-run", "failed", "2026-01-01T00:00:01Z"); err != nil {
		t.Fatal(err)
	}
	text, err := EncodeCheckpoint(Checkpoint{
		Messages: []chatcompleter.Message{{Role: "user", Content: "make the pdf"}, {Role: "tool", Content: "done", ToolCallID: "c"}},
		Links:    []tool_mcp.ResourceLink{{Name: "cv.pdf", URI: "file:///x/cv.pdf"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runs.SaveCheckpoint("failed-run", text); err != nil {
		t.Fatal(err)
	}
	r, err := e.retry(t, "failed-run", false, readScope)
	if err != nil {
		t.Fatal(err)
	}
	if status := waitForTurn(t, e.convDB, r.ID); status != "completed" {
		t.Fatalf("status = %q", status)
	}
	conv, _ := conversation_query.NewConversationQueryRepo(e.convDB).FindByID(e.conv)
	turns, err := ReadRecentTurns(conv.FilePath, 5)
	if err != nil || len(turns) != 1 || !strings.Contains(turns[0].AssistantContent, "[cv.pdf](file:///x/cv.pdf)") {
		t.Errorf("the answer must list the link the failed attempt had collected: %+v %v", turns, err)
	}
}
