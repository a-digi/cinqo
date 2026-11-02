package conversation

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/tool/mcp"
)

func usageResult(prompt, completion, cacheRead, cacheWrite int) *chatcompleter.ChatCompletionResult {
	return &chatcompleter.ChatCompletionResult{Usage: chatcompleter.Usage{
		PromptTokens: prompt, CompletionTokens: completion, TotalTokens: prompt + completion, CacheReadTokens: cacheRead, CacheCreationTokens: cacheWrite,
	}}
}

func TestTheTotalsSumTheTurnAndFindTheLargestRequest(t *testing.T) {
	var totals turnTotals
	totals.add(1, usageResult(24266, 112, 0, 0), nil)
	totals.add(2, usageResult(99000, 6198, 80000, 5000), nil)
	totals.add(3, nil, errors.New("provider error"))
	totals.add(5, usageResult(7777, 7, 0, 0), errors.New("an error that arrived together with a result")) // no usage is counted for a failed request
	totals.add(4, usageResult(30000, 100, 20000, 0), nil)
	if totals.requests != 3 || totals.failed != 2 || totals.prompt != 153266 || totals.completion != 6410 || totals.cacheRead != 100000 || totals.cacheWrite != 5000 {
		t.Fatalf("%+v", totals)
	}
	if totals.largestPrompt != 99000 || totals.largestPromptIteration != 2 {
		t.Errorf("largest %d at %d", totals.largestPrompt, totals.largestPromptIteration)
	}
	got := totals.summary()
	for _, want := range []string{"=== Turn summary ===", "requests=3 (2 failed)", "prompt=153,266", "completion=6,410", "largest_request=99,000 (iteration 2)", "cached_prompt=100,000 (cache read 100,000, cache write 5,000)"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
}

func TestASummaryWithoutFailuresSaysNothingAboutThem(t *testing.T) {
	var totals turnTotals
	totals.add(1, usageResult(100, 10, 0, 0), nil)
	if got := totals.summary(); strings.Contains(got, "failed") || !strings.Contains(got, "requests=1  prompt=100") || !strings.Contains(got, "cached_prompt=0 (cache read 0, cache write 0)") {
		t.Errorf("%s", got)
	}
}

func TestTheSummaryIsAppendedToTheTraceFileAndNothingIsWrittenForAnEmptyTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conv", "turn.txt")
	if err := appendTraceSummary(path, turnTotals{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a turn without requests created a trace file")
	}
	if err := appendTraceEntry(path, 1, []chatcompleter.Message{{Role: "user", Content: "hi"}}, nil, &chatcompleter.ChatCompletionResult{FinishReason: "stop", Message: chatcompleter.Message{Content: "yo"}}, nil); err != nil {
		t.Fatal(err)
	}
	var totals turnTotals
	totals.add(1, usageResult(10, 2, 0, 0), nil)
	if err := appendTraceSummary(path, totals); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	text := string(raw)
	if !strings.Contains(text, "=== Iteration 1 — response ===") || !strings.HasSuffix(strings.TrimSpace(text), "cached_prompt=0 (cache read 0, cache write 0)") {
		t.Errorf("trace file:\n%s", text)
	}
	if strings.Index(text, "=== Turn summary ===") < strings.Index(text, "=== Iteration 1 — response ===") {
		t.Error("the summary does not come last")
	}
}

// A real loop of five rounds with reported usages: the totals gathered through logExchange are the sum the provider reported.
func TestTheTotalsOfARealLoopAreWhatTheProviderReported(t *testing.T) {
	model := &usageModel{script: []round{{"find_repeating_items", `{"u":1}`}, {"find_repeating_items", `{"u":2}`}, {"find_repeating_items", `{"u":3}`}}}
	old := invokeToolFn
	t.Cleanup(func() { invokeToolFn = old })
	invokeToolFn = func(_ context.Context, _ *sql.DB, _ []string, _ string, _ chatcompleter.ToolCall, _ string, _ int, _ string) (string, []mcp.ResourceLink, *pendingDelegation) {
		return "ok", nil, nil
	}
	var totals turnTotals
	logExchange := func(i int, _ []chatcompleter.Message, _ []chatcompleter.ToolDef, r *chatcompleter.ChatCompletionResult, err error) {
		totals.add(i, r, err)
	}
	_, err := runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: model}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, economyTools(), nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, logExchange, 0)
	if err != nil {
		t.Fatal(err)
	}
	// 4 requests: 1000, 2000, 3000 and 4000 prompt tokens; completion 10 each
	if totals.requests != 4 || totals.prompt != 10000 || totals.completion != 40 || totals.largestPrompt != 4000 || totals.largestPromptIteration != 4 {
		t.Errorf("%+v", totals)
	}
}

type usageModel struct {
	script []round
	n      int
}

func (m *usageModel) ChatCompletion(_ context.Context, _ *http.Client, _, _, _ string, _ []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	i := m.n
	m.n++
	usage := chatcompleter.Usage{PromptTokens: 1000 * (i + 1), CompletionTokens: 10}
	if i >= len(m.script) || len(tools) == 0 {
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: "done"}, FinishReason: "stop", Usage: usage}, nil
	}
	r := m.script[i]
	return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant"}, Usage: usage,
		ToolCalls: []chatcompleter.ToolCall{{ID: "c", Name: r.name, Arguments: []byte(r.args)}}}, nil
}

// The runner must gather the totals through logExchange and append the summary after the loop, or the block would never be written.
func TestTheRunnerWritesTheSummaryAfterTheLoop(t *testing.T) {
	raw, err := os.ReadFile("runner.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	for _, want := range []string{"totals.add(iteration, result, callErr)", "appendTraceSummary(tracePath, totals)"} {
		if !strings.Contains(src, want) {
			t.Errorf("runner.go lacks %q", want)
		}
	}
	if strings.Index(src, "appendTraceSummary(tracePath, totals)") < strings.Index(src, "assistantContent, err := runToolLoop(") {
		t.Error("the summary is written before the loop has run")
	}
}
