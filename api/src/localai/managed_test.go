package localai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/download"
	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/hardware"
	"github.com/a-digi/cinqo/src/localai/repository/query"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

type engineRig struct {
	e      *managedEngine
	db     *sql.DB
	in     *installer
	starts *atomic.Int32
	model  catalog.Model
	other  catalog.Model
	rt     catalog.Runtime
}

var macSystem = hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8}

func newEngineRig(t *testing.T, env ...string) *engineRig {
	t.Helper()
	db := openMainDB(t)
	r := &engineRig{db: db, starts: &atomic.Int32{}}
	r.model = catalog.Model{ID: "tiny", Name: "Tiny", SizeBytes: 1000}
	r.other = catalog.Model{ID: "other", Name: "Other", SizeBytes: 2000}
	r.rt = catalog.Runtime{ID: "rt-mac", Name: "llama.cpp b1", OS: "darwin", Arch: "arm64", Binary: "llama-b1/llama-server"}
	r.in = &installer{downloads: download.New(download.Options{}), dataDir: t.TempDir(), catalog: catalog.Catalog{Models: []catalog.Model{r.model, r.other}, Runtimes: []catalog.Runtime{r.rt}}}
	r.install(t, r.model)
	if err := os.MkdirAll(filepath.Dir(r.in.RuntimeBinary(r.rt)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.in.RuntimeBinary(r.rt), []byte("#!fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	o := supervisorOptions{Command: fakeCommand(r.starts, env...)}
	quick(&o)
	sup := newSupervisor(o)
	t.Cleanup(sup.Stop)
	r.e = newManagedEngine(r.in, sup, query.NewSettingsQueryRepo(db))
	r.e.system = func() hardware.Info { return macSystem }
	saveSettings(t, db, true, "tiny")
	return r
}

// install puts the model file in place, as a finished download would.
func (r *engineRig) install(t *testing.T, m catalog.Model) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(r.in.ModelPath(m)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.in.ModelPath(m), make([]byte, m.SizeBytes), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (r *engineRig) chat(model string) (*chatcompleter.ChatCompletionResult, error) {
	return r.e.Chat(context.Background(), http.DefaultClient, model, []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
}

func TestWhatIsNeededToRunAModelIsCheckedWithMessagesFitForThePage(t *testing.T) {
	r := newEngineRig(t)
	if err := r.e.CanEnable("tiny"); err != nil {
		t.Errorf("everything is there: %v", err)
	}
	if err := r.e.CanEnable("  tiny "); err != nil {
		t.Errorf("a padded id: %v", err)
	}
	for _, id := range []string{"", "   "} {
		if err := r.e.CanEnable(id); err == nil || err.Error() != "choose a model first" {
			t.Errorf("%q: err = %v", id, err)
		}
	}
	if err := r.e.CanEnable("gpt-4"); !errors.Is(err, entity.ErrUnknownModel) {
		t.Errorf("unknown model: err = %v", err)
	}
	if err := r.e.CanEnable("other"); err == nil || err.Error() != "the model is not downloaded yet" {
		t.Errorf("a model that is not downloaded: err = %v", err)
	}
	// A model file of the wrong size (a download that was cut short) is not downloaded.
	if err := os.MkdirAll(filepath.Dir(r.in.ModelPath(r.other)), 0o755); err == nil {
		os.WriteFile(r.in.ModelPath(r.other), make([]byte, 5), 0o644)
	}
	if err := r.e.CanEnable("other"); err == nil || err.Error() != "the model is not downloaded yet" {
		t.Errorf("a model file of the wrong size: err = %v", err)
	}
	os.Remove(r.in.RuntimeBinary(r.rt))
	if err := r.e.CanEnable("tiny"); err == nil || err.Error() != "the program that runs the models is not downloaded yet" {
		t.Errorf("no runtime: err = %v", err)
	}
	r.e.system = func() hardware.Info { return hardware.Info{OS: "windows", Arch: "amd64", RAMBytes: 16 << 30} }
	if err := r.e.CanEnable("tiny"); !errors.Is(err, entity.ErrUnsupportedSystem) {
		t.Errorf("an unsupported system: err = %v", err)
	}
}

func TestTheMemoryCheckIsExactAndAnUnknownMemoryIsNotHeldAgainstTheModel(t *testing.T) {
	r := newEngineRig(t)
	need := hardware.MinRAMBytes(r.model.SizeBytes)
	for _, c := range []struct {
		ram  uint64
		want bool
		name string
	}{{need, true, "exactly enough"}, {need + 1, true, "one byte more"}, {need - 1, false, "one byte short"}, {0, true, "unknown memory"}} {
		r.e.system = func() hardware.Info { return hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: c.ram} }
		err := r.e.CanEnable("tiny")
		if (err == nil) != c.want {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.want)
		}
		if err != nil && (!strings.Contains(err.Error(), "this computer has") || !strings.Contains(err.Error(), "GiB of memory and the model needs about")) {
			t.Errorf("%s: the message must say how much there is and how much is needed: %v", c.name, err)
		}
	}
}

func TestReadyIsAboutTheModelOfTheSettingsAndDoesNotStartAnything(t *testing.T) {
	r := newEngineRig(t)
	if err := r.e.Ready(context.Background()); err != nil {
		t.Errorf("ready: %v", err)
	}
	saveSettings(t, r.db, true, "other") // not downloaded
	if err := r.e.Ready(context.Background()); err == nil || !strings.Contains(err.Error(), "not downloaded") {
		t.Errorf("ready for a model that is not there: %v", err)
	}
	saveSettings(t, r.db, true, "")
	if err := r.e.Ready(context.Background()); err == nil {
		t.Error("no model chosen: not ready")
	}
	if r.starts.Load() != 0 {
		t.Error("Ready must never start the program")
	}
}

func TestAConversationIsAnsweredByTheStartedProgramThroughTheRealClient(t *testing.T) {
	r := newEngineRig(t)
	res, err := r.chat("tiny")
	if err != nil {
		t.Fatal(err)
	}
	if res.Message.Content != "OK" || res.Usage.PromptTokens != 12 || res.Usage.CompletionTokens != 4 {
		t.Errorf("result = %+v", res)
	}
	if st := r.e.Status(); st.State != EngineReady || st.ModelID != "tiny" {
		t.Errorf("status = %+v", st)
	}
	if _, err := r.chat("tiny"); err != nil {
		t.Fatal(err)
	}
	if r.starts.Load() != 1 {
		t.Errorf("%d starts for two turns, want 1: the program stays up between turns", r.starts.Load())
	}
}

func TestTurnsOfDifferentConversationsRunOneAtATime(t *testing.T) {
	r := newEngineRig(t, "FAKE_LLAMA_CHAT_MS=120")
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.chat("tiny"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	base, token, release, err := r.e.sup.ensure(context.Background(), r.e.launchFor(r.model, r.rt, entity.Settings{ContextTokens: 4096, IdleMinutes: 10}), 10)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	st := stats(t, base, token)
	if st["requests"] != 4 || st["maxInFlight"] != 1 {
		t.Errorf("stats = %v, want 4 requests and never more than one at a time", st)
	}
}

func TestAWaitingTurnGivesUpWhenItIsCancelled(t *testing.T) {
	r := newEngineRig(t, "FAKE_LLAMA_CHAT_MS=500")
	done := make(chan struct{})
	go func() {
		r.chat("tiny")
		close(done)
	}()
	waitUntil(t, "the first turn to hold the slot", func() bool { return len(r.e.slot) == 1 })
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err := r.e.Chat(ctx, http.DefaultClient, "tiny", nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the cancelled turn to stop waiting", err)
	}
	<-done
	if len(r.e.slot) != 0 {
		t.Error("the slot must be free again")
	}
}

func TestNothingIsStartedWhenSomethingIsMissing(t *testing.T) {
	r := newEngineRig(t)
	if _, err := r.chat("other"); err == nil || !strings.Contains(err.Error(), "not downloaded") {
		t.Errorf("a model that is not downloaded: err = %v", err)
	}
	if _, err := r.chat("gpt-4"); !errors.Is(err, entity.ErrUnknownModel) {
		t.Errorf("an unknown model: err = %v", err)
	}
	if r.starts.Load() != 0 {
		t.Error("the program must not be started for a request that cannot be answered")
	}
}

func TestTheConversationsOwnModelIsRunNotJustTheOneInTheSettings(t *testing.T) {
	r := newEngineRig(t)
	r.install(t, r.other)
	if _, err := r.chat("tiny"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.chat("other"); err != nil {
		t.Fatal(err)
	}
	if st := r.e.Status(); st.ModelID != "other" || r.starts.Load() != 2 {
		t.Errorf("status %+v after %d starts: an older conversation's model must be run for it", st, r.starts.Load())
	}
}

func TestAChangedContextSizeInTheSettingsRestartsTheProgramOnTheNextTurn(t *testing.T) {
	r := newEngineRig(t)
	if _, err := r.chat("tiny"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`UPDATE local_ai_settings SET context_tokens = 8192 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.chat("tiny"); err != nil {
		t.Fatal(err)
	}
	if r.starts.Load() != 2 {
		t.Errorf("%d starts, want 2", r.starts.Load())
	}
}

func TestAProgramThatCannotStartFailsTheTurnWithItsReason(t *testing.T) {
	r := newEngineRig(t, "FAKE_LLAMA_MODE=exit")
	_, err := r.chat("tiny")
	if err == nil || !strings.Contains(err.Error(), "model is corrupted or incomplete") {
		t.Errorf("err = %v, want the program's own reason", err)
	}
	if len(r.e.slot) != 0 {
		t.Error("the slot must be free after a failed start")
	}
}

func TestExplainFailureOnlyRewritesTheContextOverflowAnswer(t *testing.T) {
	overflow := &chatcompleter.CompletionError{Provider: "local AI", Status: 400, Message: `unexpected status 400: {"error":{"message":"the request exceeds the available context size, try increasing it","type":"exceed_context_size_error"}}`}
	got := explainFailure(overflow, 4096, 32768, false)
	var ce *chatcompleter.CompletionError
	if !errors.As(got, &ce) || ce.Status != 400 || ce.Retryable || !strings.Contains(ce.Message, "a context of 4096 tokens") || !strings.Contains(ce.Message, "raise the context size in AI, Local AI") {
		t.Errorf("rewritten = %v", got)
	}
	if strings.Contains(got.Error(), "exceed_context_size_error") {
		t.Errorf("the program's raw answer must not be shown: %v", got)
	}
	for name, err := range map[string]error{
		"another 400":                           &chatcompleter.CompletionError{Status: 400, Message: "unexpected status 400: bad json"},
		"a 500 that says context":               &chatcompleter.CompletionError{Status: 500, Retryable: true, Message: "exceeds the context"},
		"a plain error":                         errors.New("exceeds the context"),
		"a 400 about exceeding something else":  &chatcompleter.CompletionError{Status: 400, Message: "unexpected status 400: the value exceeds the maximum"},
		"a 400 about context but not exceeding": &chatcompleter.CompletionError{Status: 400, Message: "unexpected status 400: invalid context field"},
	} {
		if got := explainFailure(err, 4096, 32768, false); got != err {
			t.Errorf("%s: must pass through unchanged, got %v", name, got)
		}
	}
}

func TestAContextOverflowFromTheProgramReachesTheTurnAsPlainWords(t *testing.T) {
	r := newEngineRig(t)
	r.e.call = func(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
		return nil, &chatcompleter.CompletionError{Provider: "openai", Status: 400, Message: "unexpected status 400: the request exceeds the available context size"}
	}
	_, err := r.chat("tiny")
	if err == nil || !strings.Contains(err.Error(), "longer than the model can read at once") || !strings.Contains(err.Error(), "4096") {
		t.Errorf("err = %v", err)
	}
}

func TestTheTestReportsStartTimeSpeedAndTheReply(t *testing.T) {
	r := newEngineRig(t, "FAKE_LLAMA_DELAY_MS=120", "FAKE_LLAMA_CHAT_MS=80")
	res, err := r.e.Test(context.Background(), "tiny")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Error != "" || res.Reply != "OK" || res.PromptTokens != 12 || res.CompletionTokens != 4 {
		t.Fatalf("result = %+v", res)
	}
	if res.StartSeconds < 0.1 || res.StartSeconds > 5 {
		t.Errorf("start seconds = %v, want about the 120 ms the program needed", res.StartSeconds)
	}
	if res.Seconds < 0.08 || res.Seconds > 5 {
		t.Errorf("seconds = %v, want about the 80 ms the answer took", res.Seconds)
	}
	if want := 4 / res.Seconds; res.TokensPerSecond < want*0.99 || res.TokensPerSecond > want*1.01 {
		t.Errorf("tokens per second = %v, want completion tokens over seconds = %v", res.TokensPerSecond, want)
	}
	// The second test finds the program running: no start time to speak of.
	second, _ := r.e.Test(context.Background(), "tiny")
	if !second.OK || second.StartSeconds > 0.05 {
		t.Errorf("second test = %+v, want a start time near zero", second)
	}
}

func TestTheTestReportsFailuresInsideTheResultNotAsErrors(t *testing.T) {
	r := newEngineRig(t)
	if res, err := r.e.Test(context.Background(), "other"); err != nil || res.OK || !strings.Contains(res.Error, "not downloaded") {
		t.Errorf("a model that is not there: %+v %v", res, err)
	}
	if _, err := r.e.Test(context.Background(), "gpt-4"); !errors.Is(err, entity.ErrUnknownModel) {
		t.Errorf("an unknown model must be an error: %v", err)
	}
	r.e.call = func(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
		return nil, errors.New("the answer broke")
	}
	if res, err := r.e.Test(context.Background(), "tiny"); err != nil || res.OK || res.Error != "the answer broke" {
		t.Errorf("a failing answer: %+v %v", res, err)
	}
	long := strings.Repeat("ä", 500)
	r.e.call = func(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Content: "  " + long + "  "}, Usage: chatcompleter.Usage{CompletionTokens: 3}}, nil
	}
	res, _ := r.e.Test(context.Background(), "tiny")
	if !res.OK || len([]rune(res.Reply)) != 201 || !strings.HasSuffix(res.Reply, "…") {
		t.Errorf("a long reply must be cut to 200 characters (counted as characters, not bytes): %d runes", len([]rune(res.Reply)))
	}

	for _, c := range []struct {
		n    int
		want int
	}{{200, 200}, {201, 201}} {
		reply := strings.Repeat("x", c.n)
		r.e.call = func(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
			return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Content: reply}}, nil
		}
		got, _ := r.e.Test(context.Background(), "tiny")
		if len([]rune(got.Reply)) != c.want || strings.HasSuffix(got.Reply, "…") != (c.n > 200) {
			t.Errorf("a reply of %d characters: kept %d, cut=%v", c.n, len([]rune(got.Reply)), strings.HasSuffix(got.Reply, "…"))
		}
	}

	bad := newEngineRig(t, "FAKE_LLAMA_MODE=exit")
	res, err := bad.e.Test(context.Background(), "tiny")
	if err != nil || res.OK || !strings.Contains(res.Error, "model is corrupted or incomplete") || res.StartSeconds <= 0 {
		t.Errorf("a program that cannot start: %+v %v", res, err)
	}
}

func TestTheTestSendsTheFixedPromptWithNoTools(t *testing.T) {
	r := newEngineRig(t)
	var got []chatcompleter.Message
	var tools []chatcompleter.ToolDef
	var model string
	r.e.call = func(_ context.Context, _ *http.Client, _, _ string, m string, msgs []chatcompleter.Message, td []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
		got, tools, model = msgs, td, m
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Content: "OK"}}, nil
	}
	if res, _ := r.e.Test(context.Background(), "tiny"); !res.OK {
		t.Fatal(res)
	}
	if len(got) != 1 || got[0].Role != "user" || got[0].Content != "Reply with exactly one word: OK" || len(tools) != 0 || model != "tiny" {
		t.Errorf("sent %+v tools %v model %q", got, tools, model)
	}
}

func TestACancelledTestSaysSoAndFreesTheSlot(t *testing.T) {
	r := newEngineRig(t, "FAKE_LLAMA_CHAT_MS=400")
	go r.chat("tiny")
	waitUntil(t, "the slot", func() bool { return len(r.e.slot) == 1 })
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	res, err := r.e.Test(ctx, "tiny")
	if err != nil || res.OK || !strings.Contains(res.Error, "cancelled") {
		t.Errorf("result = %+v %v", res, err)
	}
}

func TestStoppingTheEngineStopsTheProgram(t *testing.T) {
	r := newEngineRig(t)
	if _, err := r.chat("tiny"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(pidPath(r.in.root()))
	if len(raw) == 0 {
		t.Fatal("a running program has a pid file")
	}
	r.e.Stop()
	if st := r.e.Status(); st.State != EngineStopped {
		t.Errorf("status = %+v", st)
	}
	waitUntil(t, "the pid file to go", func() bool { _, err := os.Stat(pidPath(r.in.root())); return err != nil })
}

func TestRegisterBuildsTheEngineStopsLeftoversAndStopsTheProgramWithTheApplication(t *testing.T) {
	t.Cleanup(func() {
		managed = nil
		engine = unavailableEngine{err: errManagedNotBuilt}
		setInstaller(nil)
		setDialects(nil)
	})
	setDialects(nil)
	db := openMainDB(t)
	data := t.TempDir()
	root := filepath.Join(data, "local-ai")

	// A model program left behind by an application that crashed: a real process, named in the pid file.
	var leftStarts atomic.Int32
	cmd := fakeCommand(&leftStarts)(os.Args[0], []string{"--port", itoa(mustPort(t))})
	cmd.Env = append(cmd.Env, "LLAMA_API_KEY=x")
	setProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go cmd.Wait()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writePID(root, cmd.Process.Pid, os.Args[0])

	if err := Register(db, Deps{Downloads: download.New(download.Options{}), DataDir: data}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the leftover to be stopped at start", func() bool { return gone(cmd.Process.Pid) })
	if managed == nil || engine != Engine(managed) {
		t.Fatal("Register must make the managed engine and use it")
	}
	// Register also puts the tool dialects in place (without them a model that needs one would have no tools), and the
	// runtime the handlers use reports it.
	if currentDialects() == nil || !(managedRuntime{}).CanUseTools(phiID) || !(managedRuntime{}).CanUseTools(qwenID) || (managedRuntime{}).CanUseTools("gpt-4") {
		t.Error("Register must register the dialects, and the runtime must report which models can use tools")
	}
	if err := (managedRuntime{}).CanEnable("tiny"); errors.Is(err, errManagedNotBuilt) {
		t.Error("the runtime delegate must now ask the real engine")
	}
	if st := (managedRuntime{}).Status(); st.State != EngineStopped {
		t.Errorf("status = %+v", st)
	}
}

func mustPort(t *testing.T) int {
	t.Helper()
	p, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// The whole path a conversation takes: the platform's completer, the engine, the supervisor, a real child process and the real
// client. The runner calls exactly this.
func TestTheCompleterDrivesTheManagedEngineEndToEnd(t *testing.T) {
	r := newEngineRig(t)
	withEngine(t, r.e)
	c := completer{settings: query.NewSettingsQueryRepo(r.db)}

	res, err := chat2(c, "tiny")
	if err != nil {
		t.Fatal(err)
	}
	if res.Message.Content != "OK" || res.Usage.PromptTokens != 12 || res.Usage.CompletionTokens != 4 {
		t.Errorf("result = %+v", res)
	}
	if res.Usage.Cost == nil || *res.Usage.Cost != 0 {
		t.Errorf("cost = %v, want a known 0: local answers are free", res.Usage.Cost)
	}

	// A turn on a platform that is switched off is refused before anything starts.
	saveSettings(t, r.db, false, "tiny")
	before := r.starts.Load()
	if _, err := chat2(c, "tiny"); !errors.Is(err, ErrNotAvailable) {
		t.Errorf("switched off: err = %v", err)
	}
	if r.starts.Load() != before {
		t.Error("a switched-off platform must not start the program")
	}
}

func TestAContextOverflowIsLabelledAsTheLocalAIOnTheWayOut(t *testing.T) {
	r := newEngineRig(t)
	withEngine(t, r.e)
	r.e.call = func(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
		return nil, &chatcompleter.CompletionError{Provider: "openai", Status: 400, Message: "unexpected status 400: the request exceeds the available context size"}
	}
	c := completer{settings: query.NewSettingsQueryRepo(r.db)}
	_, err := chat2(c, "tiny")
	var ce *chatcompleter.CompletionError
	if !errors.As(err, &ce) || ce.Provider != providerName || ce.Retryable || !strings.Contains(ce.Message, "longer than the model can read") {
		t.Errorf("err = %v, want the friendly message labelled %q and not retried", err, providerName)
	}
}

func chat2(c completer, model string) (*chatcompleter.ChatCompletionResult, error) {
	return c.ChatCompletion(context.Background(), http.DefaultClient, "ignored", "ignored", model, []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil)
}

// The model program's real answer to a request that does not fit (verified on a temporary server, 2026-11-03).
func overflowBody(needed, window int) string {
	return fmt.Sprintf(`unexpected status 400: {"error":{"code":400,"message":"request (%d tokens) exceeds the available context size (%d tokens), try increasing it","type":"exceed_context_size_error","n_prompt_tokens":%d,"n_ctx":%d}}`, needed, window, needed, window)
}

func TestTheOverflowAnswerGivesTheNumbersAndTheSizeToSet(t *testing.T) {
	for _, c := range []struct {
		name                string
		needed, window, max int
		want                string
	}{
		{"the user's import: 6,232 tokens in 4,096", 6232, 4096, 131072, "the conversation needs 6,232 tokens but the model reads 4,096 at once (the context size): set the context size in AI, Local AI to at least 8,192"},
		{"room for the answer moves it up", 7500, 4096, 131072, "to at least 16,384"},
		{"exactly the room boundary stays", 7168, 4096, 131072, "to at least 8,192"},
		{"one token over the room boundary moves up", 7169, 4096, 131072, "to at least 16,384"},
		{"limited by the model's maximum", 20000, 4096, 32768, "to at least 32,768"},
		{"the maximum when only that still holds it", 32000, 4096, 32768, "to at least 32,768"},
		{"longer than the model can read", 40000, 4096, 32768, "needs 40,000 tokens, more than this model can read (at most 32,768): start a new conversation"},
		{"the maximum is not enough for the request", 32768, 4096, 32768, "more than this model can read (at most 32,768)"},
		{"unknown maximum uses the largest size", 140000, 4096, 0, "more than this model can read (at most 131,072)"},
		{"unknown maximum, a size that fits", 6232, 4096, 0, "to at least 8,192"},
	} {
		err := &chatcompleter.CompletionError{Provider: "local AI", Status: 400, Message: overflowBody(c.needed, c.window)}
		got := explainFailure(err, c.window, c.max, false)
		var ce *chatcompleter.CompletionError
		if !errors.As(got, &ce) || ce.Retryable || ce.Status != 400 || !strings.Contains(ce.Message, c.want) {
			t.Errorf("%s: %v, want it to contain %q", c.name, got, c.want)
		}
		if strings.Contains(got.Error(), "exceed_context_size_error") || strings.Contains(got.Error(), "n_prompt_tokens") {
			t.Errorf("%s: the raw answer must not be shown: %v", c.name, got)
		}
	}
}

func TestWithoutUsableNumbersTheGeneralSentenceStays(t *testing.T) {
	for name, msg := range map[string]string{
		"no body":                 "unexpected status 400: the request exceeds the available context size",
		"numbers missing":         `unexpected status 400: {"error":{"message":"exceeds the available context size","type":"exceed_context_size_error"}}`,
		"zero needed":             `unexpected status 400: {"error":{"message":"exceeds context","n_prompt_tokens":0,"n_ctx":4096}}`,
		"zero window":             `unexpected status 400: {"error":{"message":"exceeds context","n_prompt_tokens":5000,"n_ctx":0}}`,
		"needed not above window": `unexpected status 400: {"error":{"message":"exceeds context","n_prompt_tokens":4096,"n_ctx":4096}}`,
		"needed below window":     `unexpected status 400: {"error":{"message":"exceeds context","n_prompt_tokens":100,"n_ctx":4096}}`,
		"text instead of numbers": `unexpected status 400: {"error":{"message":"exceeds context","n_prompt_tokens":"many","n_ctx":"few"}}`,
		"negative":                `unexpected status 400: {"error":{"message":"exceeds context","n_prompt_tokens":-5,"n_ctx":-9}}`,
		"broken json":             `unexpected status 400: {"error":{"message":"exceeds context","n_prompt_tokens":5000,`,
	} {
		got := explainFailure(&chatcompleter.CompletionError{Provider: "local AI", Status: 400, Message: msg}, 4096, 32768, false)
		if !strings.Contains(got.Error(), "a context of 4096 tokens") || !strings.Contains(got.Error(), "raise the context size in AI, Local AI") {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestTheSizesToSetAreExactlyTheOnesThePageOffers(t *testing.T) {
	// The Local AI page's own list (frontend localAiRules.ts contextOptions); a change of one must change the other.
	want := []int{2048, 4096, 8192, 16384, 32768, 65536, 131072}
	if !reflect.DeepEqual(contextSizes, want) {
		t.Errorf("contextSizes = %v", contextSizes)
	}
}

func TestAnOverflowThroughTheEngineSaysWhatToSet(t *testing.T) {
	r := newEngineRig(t)
	r.e.call = func(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
		return nil, &chatcompleter.CompletionError{Provider: "openai", Status: 400, Message: overflowBody(6232, 4096)}
	}
	_, err := r.chat("tiny")
	if err == nil || !strings.Contains(err.Error(), "needs 6,232 tokens") || !strings.Contains(err.Error(), "at least 8,192") {
		t.Errorf("err = %v", err)
	}
	res, terr := r.e.Test(context.Background(), "tiny")
	if terr != nil || res.OK || !strings.Contains(res.Error, "needs 6,232 tokens") {
		t.Errorf("the test must say it too: %+v %v", res, terr)
	}
}

func TestTheModelsOwnMaximumLimitsWhatIsSuggested(t *testing.T) {
	r := newEngineRig(t)
	r.in.catalog.Models[0].ContextTokens = 8000 // this model cannot read 8,192: the suggestion must be its own maximum
	r.e.call = func(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
		return nil, &chatcompleter.CompletionError{Provider: "openai", Status: 400, Message: overflowBody(6232, 4096)}
	}
	_, err := r.chat("tiny")
	if err == nil || !strings.Contains(err.Error(), "at least 8,000") {
		t.Errorf("the maximum of the model in the catalog must reach the message: %v", err)
	}
	res, terr := r.e.Test(context.Background(), "tiny")
	if terr != nil || !strings.Contains(res.Error, "at least 8,000") {
		t.Errorf("and the test's message: %+v %v", res, terr)
	}
}

func TestAToolCapableModelIsToldWhatATaskLikeAnImportNeeds(t *testing.T) {
	// The user's real message: 6,215 tokens at 4,096 needs 8,192 for this request, but an import needs 16,384.
	err := &chatcompleter.CompletionError{Provider: "local AI", Status: 400, Message: overflowBody(6215, 4096)}
	got := explainFailure(err, 4096, 131072, true).Error()
	want := "the conversation needs 6,215 tokens but the model reads 4,096 at once (the context size): set the context size in AI, Local AI to at least 8,192; tool tasks such as importing a CV need at least 16,384"
	if !strings.HasSuffix(got, want) {
		t.Errorf("got %q", got)
	}
	// A model without tools is not told about tool tasks.
	if got := explainFailure(err, 4096, 131072, false).Error(); strings.Contains(got, "tool tasks") {
		t.Errorf("no tools, no hint: %q", got)
	}
	for name, c := range map[string]struct {
		needed, max int
		tools       bool
		hint        bool
	}{
		"the size already holds a tool task":     {20000, 131072, true, false},
		"exactly the tool size":                  {15360, 131072, true, false}, // 15,360 + 1,024 = 16,384
		"one token more than the tool size":      {15361, 131072, true, false}, // moves to 32,768: above the recommendation
		"just below the tool size":               {7000, 131072, true, true},
		"a model that reads less than 16,384":    {5000, 8192, true, false}, // its own maximum is the best there is
		"a model that reads less, size is below": {2000, 8192, true, false},
	} {
		e := &chatcompleter.CompletionError{Provider: "local AI", Status: 400, Message: overflowBody(c.needed, 4096)}
		msg := explainFailure(e, 4096, c.max, c.tools).Error()
		if has := strings.Contains(msg, "tool tasks such as importing a CV need at least"); has != c.hint {
			t.Errorf("%s: hint %v, want %v: %q", name, has, c.hint, msg)
		}
	}
	if toolTaskSize(0) != 16384 || toolTaskSize(131072) != 16384 || toolTaskSize(8192) != 8192 || toolTaskSize(16384) != 16384 {
		t.Error("the tool size is the recommendation, never above what the model reads")
	}
}
