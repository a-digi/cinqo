package conversation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	platform_crypto "github.com/a-digi/cinqo/src/platform/crypto"
	"github.com/a-digi/cinqo/src/platform/entity"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

var testEncryptionKey = []byte("0123456789abcdef0123456789abcdef")

// recordingCompleter remembers what every call was sent.
type recordingCompleter struct {
	mu      sync.Mutex
	baseURL []string
	token   []string
	tools   [][]string
}

func (r *recordingCompleter) ChatCompletion(_ context.Context, _ *http.Client, baseURL, apiKey, _ string, _ []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.baseURL = append(r.baseURL, baseURL)
	r.token = append(r.token, apiKey)
	r.tools = append(r.tools, toolNames(tools))
	return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: "answer"}, FinishReason: "stop"}, nil
}

func (r *recordingCompleter) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.baseURL)
}

// registerKeylessPlatform registers a platform that supplies its own credentials. Ids are unique per test because the
// registry cannot be emptied.
func registerKeylessPlatform(t *testing.T, id string, noTools bool, credentials func(context.Context) (string, string, error)) *recordingCompleter {
	t.Helper()
	rec := &recordingCompleter{}
	if err := platform.Register(platform.Entry{
		Info:           entity.Platform{ID: id, Name: id},
		DefaultBaseURL: "http://must-not-be-used.invalid",
		Completer:      rec,
		Credentials:    credentials,
		NoTools:        noTools,
	}); err != nil {
		t.Fatal(err)
	}
	return rec
}

func fixedCredentials(context.Context) (string, string, error) {
	return "http://127.0.0.1:9999/v1", "local-token", nil
}

func TestResolveEntryUsesThePlatformsOwnCredentialsAndLeavesTheRegistryAlone(t *testing.T) {
	registerKeylessPlatform(t, "cred-own", false, fixedCredentials)
	registered, _ := platform.Lookup("cred-own")

	got, token, err := resolveEntry(context.Background(), nil, nil, registered, "cred-own")
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultBaseURL != "http://127.0.0.1:9999/v1" || token != "local-token" {
		t.Errorf("base URL %q token %q, want the reported ones", got.DefaultBaseURL, token)
	}
	if again, _ := platform.Lookup("cred-own"); again.DefaultBaseURL != "http://must-not-be-used.invalid" {
		t.Errorf("the registry entry was changed: %q", again.DefaultBaseURL)
	}
}

func TestResolveEntryReportsWhyAPlatformIsNotReady(t *testing.T) {
	registerKeylessPlatform(t, "cred-down", false, func(context.Context) (string, string, error) {
		return "", "", errors.New("the model is still loading")
	})
	registered, _ := platform.Lookup("cred-down")
	_, _, err := resolveEntry(context.Background(), nil, nil, registered, "cred-down")
	if !errors.Is(err, ErrPlatformNotReady) || !strings.Contains(err.Error(), "still loading") {
		t.Errorf("err = %v, want ErrPlatformNotReady with the reason", err)
	}
}

func TestACloudPlatformStillNeedsAStoredKey(t *testing.T) {
	db := openMainDB(t)
	entry, _ := platform.Lookup("openai")

	_, _, err := resolveEntry(context.Background(), db, testEncryptionKey, entry, "openai")
	if !errors.Is(err, ErrNoKeyForPlatform) {
		t.Fatalf("err = %v, want ErrNoKeyForPlatform", err)
	}

	enc, err := platform_crypto.Encrypt("sk-secret", testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO platform_keys (id, label, platform, encrypted_key, created_by, created_at) VALUES ('k1', 'l', 'openai', ?, 'u', '2026-01-01T00:00:00Z')`, enc); err != nil {
		t.Fatal(err)
	}
	got, token, err := resolveEntry(context.Background(), db, testEncryptionKey, entry, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if token != "sk-secret" || got.DefaultBaseURL != entry.DefaultBaseURL {
		t.Errorf("token %q base %q, want the decrypted key and the platform's own URL", token, got.DefaultBaseURL)
	}
}

// keylessConversation inserts a conversation on platformID and returns its id.
func keylessConversation(t *testing.T, convDB *sql.DB, platformID string) string {
	t.Helper()
	id := "conv-" + platformID
	c := &conversation_entity.Conversation{ID: id, UserID: "u1", Title: "t", FilePath: filepath.Join(t.TempDir(), id+".md"), PlatformID: platformID, Model: "m"}
	if err := conversation_persistent.NewConversationPersistentRepo(convDB).Insert(c); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAKeylessNoToolsPlatformAnswersThroughSendMessage(t *testing.T) {
	rec := registerKeylessPlatform(t, "send-notools", true, fixedCredentials)
	mainDB, convDB := openMainDB(t), openConversationDB(t)
	seedDomainTool(t, mainDB, "list_jobs", "tool:demo:read")
	id := keylessConversation(t, convDB, "send-notools")

	turn, err := SendMessage(context.Background(), http.DefaultClient, mainDB, convDB, testEncryptionKey, id, "hello", []string{"tool:demo:read"}, t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if turn.AssistantContent != "answer" {
		t.Errorf("answer = %q", turn.AssistantContent)
	}
	if rec.baseURL[0] != "http://127.0.0.1:9999/v1" || rec.token[0] != "local-token" {
		t.Errorf("sent to %q with token %q", rec.baseURL[0], rec.token[0])
	}
	if len(rec.tools[0]) != 0 {
		t.Errorf("a no-tools platform was offered %v", rec.tools[0])
	}
}

func TestAPlatformWithToolsIsStillOfferedThem(t *testing.T) {
	rec := registerKeylessPlatform(t, "send-tools", false, fixedCredentials)
	mainDB, convDB := openMainDB(t), openConversationDB(t)
	seedDomainTool(t, mainDB, "list_jobs", "tool:demo:read")
	id := keylessConversation(t, convDB, "send-tools")

	if _, err := SendMessage(context.Background(), http.DefaultClient, mainDB, convDB, testEncryptionKey, id, "hello", []string{"tool:demo:read"}, t.TempDir(), 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(rec.tools[0], ","), "list_jobs") {
		t.Errorf("tools = %v, want list_jobs offered (proves the no-tools test can fail)", rec.tools[0])
	}
}

func TestSendMessageOnANotReadyPlatformFailsClearly(t *testing.T) {
	rec := registerKeylessPlatform(t, "send-down", true, func(context.Context) (string, string, error) { return "", "", errors.New("stopped") })
	mainDB, convDB := openMainDB(t), openConversationDB(t)
	id := keylessConversation(t, convDB, "send-down")

	_, err := SendMessage(context.Background(), http.DefaultClient, mainDB, convDB, testEncryptionKey, id, "hello", nil, t.TempDir(), 0)
	if !errors.Is(err, ErrPlatformNotReady) {
		t.Fatalf("err = %v, want ErrPlatformNotReady", err)
	}
	if rec.calls() != 0 {
		t.Error("the model must not be called")
	}
}

func TestAKeylessNoToolsPlatformAnswersThroughTheDetachedRunner(t *testing.T) {
	rec := registerKeylessPlatform(t, "run-notools", true, fixedCredentials)
	mainDB, convDB := openMainDB(t), openConversationDB(t)
	seedDomainTool(t, mainDB, "list_jobs", "tool:demo:read")
	id := keylessConversation(t, convDB, "run-notools")

	run, err := StartTurnRun(http.DefaultClient, mainDB, convDB, testEncryptionKey, id, "hello", []string{"tool:demo:read"}, t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	status := waitForTurn(t, convDB, run.ID)
	if status != "completed" {
		t.Fatalf("turn status = %q, want completed", status)
	}
	if rec.baseURL[0] != "http://127.0.0.1:9999/v1" || rec.token[0] != "local-token" || len(rec.tools[0]) != 0 {
		t.Errorf("sent to %q token %q tools %v", rec.baseURL[0], rec.token[0], rec.tools[0])
	}
}

func TestTheDetachedRunnerFailsTheTurnWhenThePlatformIsNotReady(t *testing.T) {
	rec := registerKeylessPlatform(t, "run-down", true, func(context.Context) (string, string, error) { return "", "", errors.New("stopped") })
	mainDB, convDB := openMainDB(t), openConversationDB(t)
	id := keylessConversation(t, convDB, "run-down")

	run, err := StartTurnRun(http.DefaultClient, mainDB, convDB, testEncryptionKey, id, "hello", nil, t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if status := waitForTurn(t, convDB, run.ID); status != "failed" {
		t.Fatalf("turn status = %q, want failed", status)
	}
	if rec.calls() != 0 {
		t.Error("the model must not be called")
	}
	failed := onlyFailedTurn(t, convDB, id, run.ID)
	if !strings.Contains(failed.ErrorMessage, "cannot answer right now") || !strings.Contains(failed.ErrorMessage, "stopped") {
		t.Errorf("the thread must say why: %q", failed.ErrorMessage)
	}
	if failed.UserContent != "hello" {
		t.Errorf("the user's own message must be kept: %q", failed.UserContent)
	}
}

func TestATurnOnAPlatformThatNoLongerExistsSaysSoInTheThread(t *testing.T) {
	mainDB, convDB := openMainDB(t), openConversationDB(t)
	id := keylessConversation(t, convDB, "removed-platform-xyz") // never registered, like a removed module's platform

	run, err := StartTurnRun(http.DefaultClient, mainDB, convDB, testEncryptionKey, id, "hello", nil, t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if status := waitForTurn(t, convDB, run.ID); status != "failed" {
		t.Fatalf("turn status = %q, want failed", status)
	}
	if got := onlyFailedTurn(t, convDB, id, run.ID).ErrorMessage; !strings.Contains(got, "no longer available") {
		t.Errorf("message = %q", got)
	}
}

func TestATurnOnACloudPlatformWithoutAKeySaysSoInTheThread(t *testing.T) {
	mainDB, convDB := openMainDB(t), openConversationDB(t)
	id := keylessConversation(t, convDB, "openai") // a real cloud platform, and no key stored

	run, err := StartTurnRun(http.DefaultClient, mainDB, convDB, testEncryptionKey, id, "hello", nil, t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if status := waitForTurn(t, convDB, run.ID); status != "failed" {
		t.Fatalf("turn status = %q, want failed", status)
	}
	if got := onlyFailedTurn(t, convDB, id, run.ID).ErrorMessage; !strings.Contains(got, "No API key") {
		t.Errorf("message = %q", got)
	}
}

func TestThePlatformFailureMessagesAreInPlainWords(t *testing.T) {
	cases := map[string]error{
		"No API key is registered":         ErrNoKeyForPlatform,
		"cannot answer right now: because": fmt.Errorf("%w: because", ErrPlatformNotReady),
		"could not be reached":             errors.New("anything else, possibly with internals"),
	}
	for want, err := range cases {
		got := platformFailureMessage(err)
		if !strings.Contains(got, want) {
			t.Errorf("%v: %q does not contain %q", err, got, want)
		}
		if strings.Contains(got, "internals") || strings.Contains(got, "conversation:") {
			t.Errorf("%q leaks internals", got)
		}
	}
}

func TestTheFailedTurnIsAlsoInTheTurnLog(t *testing.T) {
	registerKeylessPlatform(t, "run-down-log", true, func(context.Context) (string, string, error) { return "", "", errors.New("stopped") })
	mainDB, convDB := openMainDB(t), openConversationDB(t)
	id := keylessConversation(t, convDB, "run-down-log")
	run, err := StartTurnRun(http.DefaultClient, mainDB, convDB, testEncryptionKey, id, "hello", nil, t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	waitForTurn(t, convDB, run.ID)
	var log string
	if err := convDB.QueryRow(`SELECT log FROM turn_runs WHERE id = ?`, run.ID).Scan(&log); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log, "failed before calling the model") || !strings.Contains(log, "stopped") {
		t.Errorf("log = %q", log)
	}
}

// onlyFailedTurn reads the conversation's own log file and returns its single turn, which must be a failed one that
// belongs to the run.
func onlyFailedTurn(t *testing.T, convDB *sql.DB, conversationID, runID string) Turn {
	t.Helper()
	conv, err := conversation_query.NewConversationQueryRepo(convDB).FindByID(conversationID)
	if err != nil {
		t.Fatal(err)
	}
	turns, err := ReadRecentTurns(conv.FilePath, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || !turns[0].Failed || turns[0].TurnRunID != runID {
		t.Fatalf("turns = %+v, want exactly one failed turn of run %s", turns, runID)
	}
	return turns[0]
}

func waitForTurn(t *testing.T, convDB *sql.DB, runID string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		if err := convDB.QueryRow(`SELECT status FROM turn_runs WHERE id = ?`, runID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "running" {
			return status
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the turn did not finish")
	return ""
}
