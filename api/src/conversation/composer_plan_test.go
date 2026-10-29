package conversation

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	conversation_persistent "github.com/a-digi/cinqo/src/conversation/repository/persistent"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

func init() { planPollInterval = 5 * time.Millisecond }

var jobsTools = map[string]bool{"list_jobs": true, "get_job": true}

func step(id, task string, deps ...string) planStepSpec {
	return planStepSpec{ID: id, Task: task, AllowedTools: []string{"list_jobs"}, DependsOn: deps}
}

func TestValidatePlan(t *testing.T) {
	ok := []planStepSpec{step("a", "x"), step("b", "y", "a")}
	if err := validatePlan(ok, jobsTools); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}

	tooMany := make([]planStepSpec, maxPlanSteps+1)
	for i := range tooMany {
		tooMany[i] = step(fmt.Sprintf("s%d", i), "x")
	}
	noTools := step("a", "x")
	noTools.AllowedTools = nil
	builtin := step("a", "x")
	builtin.AllowedTools = []string{subAgentToolName}

	cases := map[string]struct {
		steps []planStepSpec
		want  string
	}{
		"empty":           {nil, "at least one"},
		"too many":        {tooMany, "at most"},
		"bad id":          {[]planStepSpec{step("a b", "x")}, "invalid"},
		"duplicate id":    {[]planStepSpec{step("a", "x"), step("a", "y")}, "more than once"},
		"empty task":      {[]planStepSpec{step("a", "  ")}, "empty task"},
		"no allowedTools": {[]planStepSpec{noTools}, "needs allowedTools"},
		"unknown tool":    {[]planStepSpec{{ID: "a", Task: "x", AllowedTools: []string{"delete_everything"}}}, "not available"},
		"builtin tool":    {[]planStepSpec{builtin}, "not available"},
		"unknown dep":     {[]planStepSpec{step("a", "x", "zzz")}, "unknown step"},
		"self dep":        {[]planStepSpec{step("a", "x", "a")}, "itself"},
		"cycle":           {[]planStepSpec{step("a", "x", "b"), step("b", "y", "a")}, "cycle"},
	}
	for name, c := range cases {
		err := validatePlan(c.steps, jobsTools)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want error containing %q", name, err, c.want)
		}
	}
}

// fakeWorkers completes each worker after a scripted number of polls.
type fakeWorkers struct {
	mu       sync.Mutex
	next     int
	tasks    map[string]string // worker id -> task it received
	outcomes map[string]workerOutcome
	pollsFor map[string]int // how many polls a worker stays running for
	polls    map[string]int
	refuse   int // refuse this many Spawn calls first
	spawned  []string
	cancels  []string
	maxLive  int
	live     int
}

func newFakeWorkers() *fakeWorkers {
	return &fakeWorkers{tasks: map[string]string{}, outcomes: map[string]workerOutcome{}, pollsFor: map[string]int{}, polls: map[string]int{}}
}

func (f *fakeWorkers) Spawn(task string, tools []string, _ *acceptanceSpec) (string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse > 0 {
		f.refuse--
		return "", "all worker slots are busy"
	}
	f.next++
	id := fmt.Sprintf("w%d", f.next)
	f.tasks[id] = task
	f.spawned = append(f.spawned, id)
	f.live++
	if f.live > f.maxLive {
		f.maxLive = f.live
	}
	return id, ""
}

func (f *fakeWorkers) Poll(id string) (workerOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls[id]++
	if f.polls[id] <= f.pollsFor[id] {
		return workerOutcome{State: "running"}, nil
	}
	if f.polls[id] == f.pollsFor[id]+1 {
		f.live--
	}
	if out, ok := f.outcomes[id]; ok {
		return out, nil
	}
	return workerOutcome{State: "completed", Result: "result of " + id}, nil
}

func (f *fakeWorkers) Cancel(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, id)
}

func resultByID(results []planStepResult) map[string]planStepResult {
	m := map[string]planStepResult{}
	for _, r := range results {
		m[r.ID] = r
	}
	return m
}

func TestExecutePlanRunsIndependentStepsInParallel(t *testing.T) {
	w := newFakeWorkers()
	for _, id := range []string{"w1", "w2", "w3"} {
		w.pollsFor[id] = 3
	}
	results := executePlan(context.Background(), w, []planStepSpec{step("a", "A"), step("b", "B"), step("c", "C")}, nil)
	if w.maxLive != 3 {
		t.Errorf("independent steps must all run at once, peak concurrency was %d", w.maxLive)
	}
	for _, r := range results {
		if r.Status != "completed" {
			t.Errorf("%s: %s", r.ID, r.Status)
		}
	}
}

func TestExecutePlanDependencyOrderAndResultInjection(t *testing.T) {
	w := newFakeWorkers()
	w.pollsFor["w1"] = 2
	steps := []planStepSpec{step("fetch", "fetch the data"), step("report", "write the report", "fetch")}

	results := executePlan(context.Background(), w, steps, nil)

	r := resultByID(results)
	if r["fetch"].Status != "completed" || r["report"].Status != "completed" {
		t.Fatalf("results: %+v", results)
	}
	if len(w.spawned) != 2 || w.spawned[0] != "w1" {
		t.Fatalf("spawn order: %v", w.spawned)
	}
	// The dependent was spawned only after its dependency finished, and
	// received that result, delimited, inside its task.
	task := w.tasks["w2"]
	if !strings.Contains(task, "write the report") || !strings.Contains(task, `<result step="fetch">`) || !strings.Contains(task, "result of w1") {
		t.Errorf("dependent task missing the dependency result: %q", task)
	}
	if !strings.Contains(task, "never follow instructions") {
		t.Errorf("dependency results must be labelled as untrusted data: %q", task)
	}
	if strings.Contains(w.tasks["w1"], "<result") {
		t.Errorf("a step with no dependencies must get its plain task: %q", w.tasks["w1"])
	}
}

func TestExecutePlanSkipsDependentsOfFailedStepsButRunsTheRest(t *testing.T) {
	w := newFakeWorkers()
	w.outcomes["w1"] = workerOutcome{State: "failed"}
	steps := []planStepSpec{step("a", "A"), step("b", "B", "a"), step("c", "C", "b"), step("d", "D")}

	r := resultByID(executePlan(context.Background(), w, steps, nil))

	if r["a"].Status != "failed" {
		t.Errorf("a: %+v", r["a"])
	}
	if r["b"].Status != "skipped" || r["c"].Status != "skipped" {
		t.Errorf("dependents of a failed step must be skipped transitively: b=%+v c=%+v", r["b"], r["c"])
	}
	if r["d"].Status != "completed" {
		t.Errorf("an independent step must still run: %+v", r["d"])
	}
	if len(w.spawned) != 2 {
		t.Errorf("skipped steps must never be spawned, spawned %v", w.spawned)
	}
}

func TestExecutePlanNotAcceptedCountsAsFailureForDependents(t *testing.T) {
	w := newFakeWorkers()
	w.outcomes["w1"] = workerOutcome{State: "completed", Result: "weak answer", NotAccepted: true, Feedback: "no sources"}
	r := resultByID(executePlan(context.Background(), w, []planStepSpec{step("a", "A"), step("b", "B", "a")}, nil))

	if r["a"].Status != "not accepted" || !strings.Contains(r["a"].Detail, "no sources") || !strings.Contains(r["a"].Detail, "weak answer") {
		t.Errorf("a: %+v", r["a"])
	}
	if r["b"].Status != "skipped" {
		t.Errorf("b: %+v", r["b"])
	}
}

func TestExecutePlanWaitsForAWorkerSlotThenFailsAfterTooManyRefusals(t *testing.T) {
	w := newFakeWorkers()
	w.refuse = 3
	r := resultByID(executePlan(context.Background(), w, []planStepSpec{step("a", "A")}, nil))
	if r["a"].Status != "completed" {
		t.Fatalf("a refused slot must be retried, got %+v", r["a"])
	}

	w = newFakeWorkers()
	w.refuse = maxPlanSpawnRefusals + 5
	r = resultByID(executePlan(context.Background(), w, []planStepSpec{step("a", "A"), step("b", "B", "a")}, nil))
	if r["a"].Status != "failed" || !strings.Contains(r["a"].Detail, "slots are busy") {
		t.Errorf("a: %+v", r["a"])
	}
	if r["b"].Status != "skipped" {
		t.Errorf("b: %+v", r["b"])
	}
}

func TestExecutePlanCancelsEverythingWhenTheTurnIsStopped(t *testing.T) {
	w := newFakeWorkers()
	w.pollsFor["w1"] = 1_000_000 // never finishes on its own
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()

	r := resultByID(executePlan(ctx, w, []planStepSpec{step("a", "A"), step("b", "B", "a")}, nil))

	if r["a"].Status != "failed" || !strings.Contains(r["a"].Detail, "Cancelled") {
		t.Errorf("a: %+v", r["a"])
	}
	if r["b"].Status != "skipped" {
		t.Errorf("b: %+v", r["b"])
	}
	if len(w.cancels) != 1 || w.cancels[0] != "w1" {
		t.Errorf("the running worker must be cancelled, cancels=%v", w.cancels)
	}
}

func TestExecutePlanReportsEveryStatusTransition(t *testing.T) {
	w := newFakeWorkers()
	type ev struct{ id, status string }
	var events []ev
	record := func(id, status string, worker *string) {
		if status == conversation_entity.PlanStepRunning && worker == nil {
			t.Errorf("running must carry the worker id")
		}
		events = append(events, ev{id, status})
	}
	executePlan(context.Background(), w, []planStepSpec{step("a", "A"), step("b", "B", "a")}, record)
	want := []ev{{"a", "running"}, {"a", "completed"}, {"b", "running"}, {"b", "completed"}}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Errorf("events %v, want %v", events, want)
	}
}

func TestFormatPlanResultsGivesEveryStepAShare(t *testing.T) {
	long := strings.Repeat("x", 50_000)
	results := make([]planStepResult, 10)
	for i := range results {
		results[i] = planStepResult{ID: fmt.Sprintf("s%d", i), Status: "completed", Detail: long}
	}
	out := formatPlanResults(2, results)
	if len(out) > maxToolResultContentLength {
		t.Fatalf("output %d chars would be cut by the loop's own truncation (limit %d)", len(out), maxToolResultContentLength)
	}
	if !strings.Contains(out, "Plan 2 finished.") || !strings.Contains(out, "## s9 — completed") {
		t.Errorf("the last step must be present: %.200s", out)
	}
}

// --- end to end: a composer turn through the real loop and real workers ---

// plannerCompleter plays both roles: the orchestrator (offered run_plan)
// and the workers (offered domain tools only).
type plannerCompleter struct {
	mu          sync.Mutex
	plan        string
	workerTasks []string
	finalSaw    string
	// workerFirstMessages records the first message each worker call saw.
	workerFirstMessages []chatcompleter.Message
	judgeCalls          int
	rejectFirst         bool
	// plans, when set, replaces plan: the orchestrator issues plans[n]
	// after n tool results, and answers once it has run out.
	plans []string
	// failTasksContaining makes a worker whose task contains it error out.
	failTasksContaining string
}

const (
	orchestratorPromptTokens, orchestratorCompletionTokens = 50, 5
	workerPromptTokens, workerCompletionTokens             = 100, 10
)

func usageOf(prompt, completion int) chatcompleter.Usage {
	return chatcompleter.Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: prompt + completion}
}

func (p *plannerCompleter) ChatCompletion(_ context.Context, _ *http.Client, _, _, _ string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	isOrchestrator := containsName(tools, runPlanToolName)
	if isOrchestrator {
		plans := p.plans
		if len(plans) == 0 {
			plans = []string{p.plan}
		}
		toolResults := 0
		for _, m := range messages {
			if m.Role == "tool" {
				toolResults++
				p.finalSaw = m.Content
			}
		}
		usage := usageOf(orchestratorPromptTokens, orchestratorCompletionTokens)
		if toolResults >= len(plans) {
			return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: "composed answer"}, Usage: usage}, nil
		}
		return &chatcompleter.ChatCompletionResult{
			Message:   chatcompleter.Message{Role: "assistant"},
			ToolCalls: []chatcompleter.ToolCall{{ID: fmt.Sprintf("p%d", toolResults+1), Name: runPlanToolName, Arguments: []byte(plans[toolResults])}},
			Usage:     usage,
		}, nil
	}
	if len(tools) == 0 && len(messages) > 0 && messages[0].Content == judgeSystemPrompt {
		p.judgeCalls++
		if p.rejectFirst && p.judgeCalls == 1 {
			return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: `{"accepted": false, "feedback": "add more detail"}`}, Usage: usageOf(workerPromptTokens, workerCompletionTokens)}, nil
		}
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: `{"accepted": true}`}, Usage: usageOf(workerPromptTokens, workerCompletionTokens)}, nil
	}
	p.workerFirstMessages = append(p.workerFirstMessages, messages[0])
	task := messages[len(messages)-1].Content
	if len(messages) > 3 { // a retry: [system, task, previous answer, feedback]
		task = messages[1].Content + " (retry)"
	}
	p.workerTasks = append(p.workerTasks, task)
	if p.failTasksContaining != "" && strings.Contains(task, p.failTasksContaining) {
		return nil, fmt.Errorf("simulated provider failure")
	}
	return &chatcompleter.ChatCompletionResult{
		Message: chatcompleter.Message{Role: "assistant", Content: "DONE(" + strings.SplitN(task, "\n", 2)[0] + ")"},
		Usage:   usageOf(workerPromptTokens, workerCompletionTokens),
	}, nil
}

func TestComposerTurnEndToEnd(t *testing.T) {
	mainDB := openMainDB(t)
	seedDomainTool(t, mainDB, "list_jobs", "tool:demo:read")
	convDB := openConversationDB(t)
	scopes := []string{"tool:demo:read"}

	if err := conversation_persistent.NewConversationPersistentRepo(convDB).Insert(&conversation_entity.Conversation{
		ID: "conv", UserID: "u1", Title: "t", FilePath: "/tmp/conv", PlatformID: "p", Model: "m",
		OrchestrationMode: conversation_entity.OrchestrationModeComposer,
	}); err != nil {
		t.Fatal(err)
	}
	if err := conversation_persistent.NewTurnRunPersistentRepo(convDB).Insert(&conversation_entity.TurnRun{ID: "turn1", ConversationID: "conv", UserContent: "go", StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	completer := &plannerCompleter{plan: `{"steps":[
		{"id":"a","task":"count jobs","allowedTools":["list_jobs"]},
		{"id":"b","task":"count remote jobs","allowedTools":["list_jobs"]},
		{"id":"c","task":"summarise both","allowedTools":["list_jobs"],"dependsOn":["a","b"]}]}`}

	tools, _ := offerableToolsForMode(mainDB, scopes, conversation_entity.OrchestrationModeComposer)
	got, err := runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: completer}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "how many jobs?"}}, tools, mainDB, scopes, "u1", t.TempDir(), 0, "conv", convDB, "turn1", 0,
		conversation_entity.OrchestrationModeComposer, nil, nil, nil)
	if err != nil || got != "composed answer" {
		t.Fatalf("loop: %q, %v", got, err)
	}

	for _, want := range []string{"Plan 1 finished.", "## a — completed", "## b — completed", "## c — completed", "DONE(count jobs)"} {
		if !strings.Contains(completer.finalSaw, want) {
			t.Errorf("the orchestrator's tool result is missing %q:\n%s", want, completer.finalSaw)
		}
	}
	// The dependent worker got both dependencies' results.
	var summary string
	for _, task := range completer.workerTasks {
		if strings.HasPrefix(task, "summarise both") {
			summary = task
		}
	}
	if !strings.Contains(summary, `<result step="a">`) || !strings.Contains(summary, "DONE(count jobs)") || !strings.Contains(summary, "DONE(count remote jobs)") {
		t.Errorf("dependent worker task: %q", summary)
	}

	// Everything was persisted: three completed steps, each linked to a worker.
	steps, err := conversation_query.NewTurnPlanStepQueryRepo(convDB).FindByTurnRunID("turn1")
	if err != nil || len(steps) != 3 {
		t.Fatalf("plan steps: %d, %v", len(steps), err)
	}
	for _, s := range steps {
		if s.PlanNo != 1 || s.Status != "completed" || s.SubAgentRunID == nil {
			t.Errorf("step %s: %+v", s.StepID, s)
		}
	}
	if n, _ := conversation_query.NewSubAgentRunQueryRepo(convDB).CountByParentTurnRunID("turn1"); n != 3 {
		t.Errorf("sub-agent runs: %d, want 3", n)
	}
	if next, _ := conversation_query.NewTurnPlanStepQueryRepo(convDB).NextPlanNo("turn1"); next != 2 {
		t.Errorf("next plan number = %d, want 2", next)
	}
}

func TestRunPlanRejectsAnInvalidPlanWithoutSpawningAnything(t *testing.T) {
	mainDB := openMainDB(t)
	seedDomainTool(t, mainDB, "list_jobs", "tool:demo:read")
	convDB := openConversationDB(t)

	cases := map[string]string{
		"unknown tool": `{"steps":[{"id":"a","task":"x","allowedTools":["nope"]}]}`,
		"cycle":        `{"steps":[{"id":"a","task":"x","allowedTools":["list_jobs"],"dependsOn":["b"]},{"id":"b","task":"y","allowedTools":["list_jobs"],"dependsOn":["a"]}]}`,
		"not json":     `nope`,
	}
	for name, plan := range cases {
		out := invokeRunPlanCall(context.Background(), http.DefaultClient, platform.Entry{}, "k", "m", mainDB, convDB, []string{"tool:demo:read"}, "u1", t.TempDir(), 0,
			"conv", "turn1", chatcompleter.ToolCall{Name: runPlanToolName, Arguments: []byte(plan)})
		if !strings.HasPrefix(out, "invalid plan:") {
			t.Errorf("%s: got %q", name, out)
		}
	}
	if n, _ := conversation_query.NewSubAgentRunQueryRepo(convDB).CountByParentTurnRunID("turn1"); n != 0 {
		t.Errorf("an invalid plan must spawn nothing, found %d workers", n)
	}
}

func TestRunPlanRefusesAPlanThatWouldExceedThePerTurnLimit(t *testing.T) {
	mainDB := openMainDB(t)
	seedDomainTool(t, mainDB, "list_jobs", "tool:demo:read")
	convDB := openConversationDB(t)

	steps := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		steps = append(steps, fmt.Sprintf(`{"id":"s%d","task":"x","allowedTools":["list_jobs"]}`, i))
	}
	// Pretend this turn already used up most of its worker budget.
	for i := 0; i < maxSubAgentsPerTurn-2; i++ {
		if _, err := convDB.Exec(`INSERT INTO sub_agent_runs (id, parent_turn_run_id, conversation_id, task, started_at) VALUES (?, 'turn1', 'conv', 't', 'now')`, fmt.Sprintf("old%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	out := invokeRunPlanCall(context.Background(), http.DefaultClient, platform.Entry{}, "k", "m", mainDB, convDB, []string{"tool:demo:read"}, "u1", t.TempDir(), 0,
		"conv", "turn1", chatcompleter.ToolCall{Name: runPlanToolName, Arguments: []byte(`{"steps":[` + strings.Join(steps, ",") + `]}`)})
	if !strings.HasPrefix(out, "Plan refused") {
		t.Fatalf("got %q", out)
	}
}

func TestPlanWorkersGetTheWorkerPromptOnEveryAttempt(t *testing.T) {
	mainDB := openMainDB(t)
	seedDomainTool(t, mainDB, "list_jobs", "tool:demo:read")
	convDB := openConversationDB(t)
	scopes := []string{"tool:demo:read"}
	if err := conversation_persistent.NewConversationPersistentRepo(convDB).Insert(&conversation_entity.Conversation{
		ID: "conv", UserID: "u1", Title: "t", FilePath: "/tmp/conv", PlatformID: "p", Model: "m",
		OrchestrationMode: conversation_entity.OrchestrationModeComposer,
	}); err != nil {
		t.Fatal(err)
	}
	if err := conversation_persistent.NewTurnRunPersistentRepo(convDB).Insert(&conversation_entity.TurnRun{ID: "turn1", ConversationID: "conv", UserContent: "go", StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	// One step with acceptance criteria whose first result is rejected,
	// so the worker runs twice: the prompt must survive the retry.
	completer := &plannerCompleter{rejectFirst: true, plan: `{"steps":[{"id":"a","task":"describe the jobs","allowedTools":["list_jobs"],"acceptanceCriteria":"must be detailed","maxAttempts":2}]}`}
	tools, _ := offerableToolsForMode(mainDB, scopes, conversation_entity.OrchestrationModeComposer)
	if _, err := runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: completer}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "go"}}, tools, mainDB, scopes, "u1", t.TempDir(), 0, "conv", convDB, "turn1", 0,
		conversation_entity.OrchestrationModeComposer, nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	if len(completer.workerFirstMessages) != 2 {
		t.Fatalf("expected two worker attempts, got %d", len(completer.workerFirstMessages))
	}
	for i, m := range completer.workerFirstMessages {
		if m.Role != "system" || m.Content != workerSystemPrompt {
			t.Errorf("attempt %d must start with the worker system prompt, got %q: %.60s", i+1, m.Role, m.Content)
		}
	}
}

func TestSubAgentsOutsideAPlanGetNoSystemPrompt(t *testing.T) {
	// spawn_subagent, crawl fan-out and delegation pass "" — pinned here
	// because a stray prompt would silently change every direct flow.
	if got := withSystemPrompt("", []chatcompleter.Message{{Role: "user", Content: "t"}}); got[0].Role != "user" {
		t.Fatalf("no prompt expected, got %+v", got)
	}
}

// composerTurnFixture creates the databases, conversation and turn row a
// composer turn needs.
func composerTurnFixture(t *testing.T) (mainDB, convDB *sql.DB, scopes []string) {
	t.Helper()
	mainDB = openMainDB(t)
	seedDomainTool(t, mainDB, "list_jobs", "tool:demo:read")
	convDB = openConversationDB(t)
	if err := conversation_persistent.NewConversationPersistentRepo(convDB).Insert(&conversation_entity.Conversation{
		ID: "conv", UserID: "u1", Title: "t", FilePath: "/tmp/conv", PlatformID: "p", Model: "m",
		OrchestrationMode: conversation_entity.OrchestrationModeComposer,
	}); err != nil {
		t.Fatal(err)
	}
	if err := conversation_persistent.NewTurnRunPersistentRepo(convDB).Insert(&conversation_entity.TurnRun{ID: "turn1", ConversationID: "conv", UserContent: "go", StartedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	return mainDB, convDB, []string{"tool:demo:read"}
}

func runComposer(t *testing.T, completer *plannerCompleter, mainDB, convDB *sql.DB, scopes []string, reportUsage func(int, int)) string {
	t.Helper()
	tools, _ := offerableToolsForMode(mainDB, scopes, conversation_entity.OrchestrationModeComposer)
	got, err := runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: completer}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "go"}}, tools, mainDB, scopes, "u1", t.TempDir(), 0, "conv", convDB, "turn1", 0,
		conversation_entity.OrchestrationModeComposer, nil, reportUsage, nil)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func onePlan(id, task string) string {
	return fmt.Sprintf(`{"steps":[{"id":%q,"task":%q,"allowedTools":["list_jobs"]}]}`, id, task)
}

func TestReplanAfterAFailedStep(t *testing.T) {
	mainDB, convDB, scopes := composerTurnFixture(t)
	completer := &plannerCompleter{
		failTasksContaining: "flaky",
		plans: []string{
			`{"steps":[{"id":"a","task":"flaky job","allowedTools":["list_jobs"]},{"id":"b","task":"steady job","allowedTools":["list_jobs"]}]}`,
			onePlan("a2", "retry of the job"),
		},
	}
	if got := runComposer(t, completer, mainDB, convDB, scopes, nil); got != "composed answer" {
		t.Fatalf("got %q", got)
	}

	steps, err := conversation_query.NewTurnPlanStepQueryRepo(convDB).FindByTurnRunID("turn1")
	if err != nil || len(steps) != 3 {
		t.Fatalf("plan steps: %d, %v", len(steps), err)
	}
	got := map[string]string{}
	for _, s := range steps {
		got[fmt.Sprintf("%d/%s", s.PlanNo, s.StepID)] = s.Status
	}
	want := map[string]string{"1/a": "failed", "1/b": "completed", "2/a2": "completed"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("steps %v, want %v", got, want)
	}
	if !strings.Contains(completer.finalSaw, "Plan 2 finished.") {
		t.Errorf("the orchestrator must see the re-plan's results: %s", completer.finalSaw)
	}
}

func TestReplanIsCappedPerTurn(t *testing.T) {
	mainDB, convDB, scopes := composerTurnFixture(t)
	completer := &plannerCompleter{plans: []string{onePlan("p1", "one"), onePlan("p2", "two"), onePlan("p3", "three"), onePlan("p4", "four")}}
	runComposer(t, completer, mainDB, convDB, scopes, nil)

	if n, _ := conversation_query.NewSubAgentRunQueryRepo(convDB).CountByParentTurnRunID("turn1"); n != maxPlansPerTurn {
		t.Errorf("only %d plans may run, but %d workers ran", maxPlansPerTurn, n)
	}
	if !strings.Contains(completer.finalSaw, "already used its 3 plans") {
		t.Errorf("the 4th plan must be refused with guidance, got: %s", completer.finalSaw)
	}
	if next, _ := conversation_query.NewTurnPlanStepQueryRepo(convDB).NextPlanNo("turn1"); next != maxPlansPerTurn+1 {
		t.Errorf("a refused plan must not be recorded, next plan no = %d", next)
	}
}

func TestAnInvalidPlanDoesNotUseUpAPlan(t *testing.T) {
	mainDB, convDB, _ := composerTurnFixture(t)
	for i := 0; i < 5; i++ {
		out := invokeRunPlanCall(context.Background(), http.DefaultClient, platform.Entry{}, "k", "m", mainDB, convDB, []string{"tool:demo:read"}, "u1", t.TempDir(), 0,
			"conv", "turn1", chatcompleter.ToolCall{Name: runPlanToolName, Arguments: []byte(`{"steps":[{"id":"a","task":"x","allowedTools":["nope"]}]}`)})
		if !strings.HasPrefix(out, "invalid plan:") {
			t.Fatalf("attempt %d: %q", i, out)
		}
	}
	if next, _ := conversation_query.NewTurnPlanStepQueryRepo(convDB).NextPlanNo("turn1"); next != 1 {
		t.Errorf("invalid plans must not consume plan numbers, next = %d", next)
	}
}

func TestDependencyResultsAreCappedInTotal(t *testing.T) {
	long := strings.Repeat("x", 50_000)
	deps := make([]string, 8)
	byID := map[string]*planStepState{}
	for i := range deps {
		deps[i] = fmt.Sprintf("d%d", i)
		byID[deps[i]] = &planStepState{detail: long}
	}
	task := buildStepTask(planStepSpec{ID: "t", Task: "summarise", DependsOn: deps}, byID)
	if len(task) > maxDependencyResultsTotal+2000 {
		t.Errorf("a step with 8 dependencies got a %d-character task", len(task))
	}
	for _, d := range deps {
		if !strings.Contains(task, fmt.Sprintf("<result step=%q>", d)) {
			t.Errorf("dependency %s is missing", d)
		}
	}

	one := buildStepTask(planStepSpec{ID: "t", Task: "s", DependsOn: []string{"d0"}}, byID)
	if len(one) > maxDependencyResultLength+1000 {
		t.Errorf("a single dependency must be capped at %d, task is %d", maxDependencyResultLength, len(one))
	}
}

// Worker tokens live on the worker's own run (including the acceptance
// judge's calls); the orchestrator's turn total counts only the
// orchestrator's own model calls.
func TestWorkerTokensStayOnTheWorkerNotTheTurnTotal(t *testing.T) {
	mainDB, convDB, scopes := composerTurnFixture(t)
	completer := &plannerCompleter{
		rejectFirst: true,
		plan:        `{"steps":[{"id":"a","task":"describe","allowedTools":["list_jobs"],"acceptanceCriteria":"detailed","maxAttempts":2}]}`,
	}
	var turnPrompt, turnCompletion int
	runComposer(t, completer, mainDB, convDB, scopes, func(p, c int) { turnPrompt += p; turnCompletion += c })

	// Two orchestrator model calls (the plan, then the answer).
	if turnPrompt != 2*orchestratorPromptTokens || turnCompletion != 2*orchestratorCompletionTokens {
		t.Errorf("turn total = %d/%d, want only the orchestrator's own 2 calls", turnPrompt, turnCompletion)
	}
	// Worker: two attempts + two judge calls, each 100/10.
	runs, err := conversation_query.NewSubAgentRunQueryRepo(convDB).FindByParentTurnRunID("turn1")
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs: %d, %v", len(runs), err)
	}
	wantTotal := 4 * (workerPromptTokens + workerCompletionTokens)
	if runs[0].PromptTokens != 4*workerPromptTokens || runs[0].TotalTokens != wantTotal {
		t.Errorf("worker tokens = %d prompt / %d total, want %d / %d (attempts and judge calls included)",
			runs[0].PromptTokens, runs[0].TotalTokens, 4*workerPromptTokens, wantTotal)
	}
}
