package conversation

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

func cost(v float64) *float64 { return &v }

func TestTheUsageDetailsAppearOnlyForWhatTheProviderReported(t *testing.T) {
	cases := []struct {
		name string
		u    chatcompleter.Usage
		want string
	}{
		{"nothing extra", chatcompleter.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110}, ""},
		{"cache read only", chatcompleter.Usage{CacheReadTokens: 8000}, " cache_read=8000 cache_write=0"},
		{"cache write only", chatcompleter.Usage{CacheCreationTokens: 300}, " cache_read=0 cache_write=300"},
		{"reasoning", chatcompleter.Usage{ReasoningTokens: 600}, " reasoning=600"},
		{"a cost", chatcompleter.Usage{Cost: cost(0.0123)}, " cost=$0.0123"},
		{"a free call", chatcompleter.Usage{Cost: cost(0)}, " cost=$0"},
		{"everything", chatcompleter.Usage{CacheReadTokens: 8000, CacheCreationTokens: 300, ReasoningTokens: 600, Cost: cost(0.000123)}, " cache_read=8000 cache_write=300 reasoning=600 cost=$0.000123"},
	}
	for _, c := range cases {
		if got := usageDetails(c.u); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestACostIsWrittenWithEveryDigitTheProviderSent(t *testing.T) {
	for in, want := range map[float64]string{0: "$0", 0.000123: "$0.000123", 1.5: "$1.5", 0.0000001: "$0.0000001", 12: "$12", 0.1234567891: "$0.1234567891"} {
		if got := formatCost(in); got != want {
			t.Errorf("formatCost(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestTheTraceLineOfAResponseCarriesTheDetailsAndStaysTheSameWithout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "turn.txt")
	plain := &chatcompleter.ChatCompletionResult{FinishReason: "stop", Message: chatcompleter.Message{Content: "yo"}, Usage: chatcompleter.Usage{PromptTokens: 24266, CompletionTokens: 112, TotalTokens: 24378}}
	rich := &chatcompleter.ChatCompletionResult{FinishReason: "stop", Message: chatcompleter.Message{Content: "yo"}, Usage: chatcompleter.Usage{
		PromptTokens: 12345, CompletionTokens: 900, TotalTokens: 13245, CacheReadTokens: 8000, ReasoningTokens: 600, Cost: cost(0.0123)}}
	msgs := []chatcompleter.Message{{Role: "user", Content: "hi"}}
	if err := appendTraceEntry(path, 1, msgs, nil, plain, nil); err != nil {
		t.Fatal(err)
	}
	if err := appendTraceEntry(path, 2, msgs, nil, rich, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	got := string(raw)
	if !strings.Contains(got, "finish_reason=stop  usage: prompt=24266 completion=112 total=24378\n") {
		t.Errorf("the line of a provider without extras must be unchanged:\n%s", got)
	}
	if !strings.Contains(got, "finish_reason=stop  usage: prompt=12345 completion=900 total=13245 cache_read=8000 cache_write=0 reasoning=600 cost=$0.0123\n") {
		t.Errorf("the line with extras:\n%s", got)
	}
}

func TestTheSummaryAddsReasoningAndCostOnlyWhenReportedAndSaysWhenTheCostIsPartial(t *testing.T) {
	// nothing reported: the line ends exactly as before
	var plain turnTotals
	plain.add(1, usageResult(100, 10, 0, 0), nil)
	if got := plain.summary(); strings.Contains(got, "reasoning") || strings.Contains(got, "cost") || !strings.HasSuffix(got, "cached_prompt=0 (cache read 0, cache write 0)\n") {
		t.Errorf("%s", got)
	}
	// every request reported a cost
	var all turnTotals
	r1 := usageResult(100, 10, 0, 0)
	r1.Usage.Cost, r1.Usage.ReasoningTokens = cost(0.25), 4
	r2 := usageResult(200, 20, 0, 0)
	r2.Usage.Cost, r2.Usage.ReasoningTokens = cost(0.5), 6
	all.add(1, r1, nil)
	all.add(2, r2, nil)
	if got := all.summary(); !strings.Contains(got, "reasoning=10") || !strings.Contains(got, "cost=$0.75") || strings.Contains(got, "reported for") {
		t.Errorf("%s", got)
	}
	// only some requests reported one: the sum says so
	var some turnTotals
	some.add(1, r1, nil)
	some.add(2, usageResult(50, 5, 0, 0), nil)
	some.add(3, nil, context.Canceled) // a failed request carries nothing
	if got := some.summary(); !strings.Contains(got, "cost=$0.25 (reported for 1 of 2 requests)") {
		t.Errorf("%s", got)
	}
	// a free call is a reported cost of 0
	var free turnTotals
	f := usageResult(10, 1, 0, 0)
	f.Usage.Cost = cost(0)
	free.add(1, f, nil)
	if got := free.summary(); !strings.Contains(got, "cost=$0\n") {
		t.Errorf("%s", got)
	}
}

type richCompleter struct{ calls int }

func (r *richCompleter) ChatCompletion(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	r.calls++
	res := &chatcompleter.ChatCompletionResult{
		Message: chatcompleter.Message{Role: "assistant", Content: map[int]string{1: "not json", 2: `{"accepted": true}`}[r.calls]},
		Usage:   chatcompleter.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110, CacheReadTokens: 60, CacheCreationTokens: 5, ReasoningTokens: 4},
	}
	if r.calls == 2 {
		res.Usage.Cost = cost(0.25)
	}
	return res, nil
}

// The acceptance check (a judge that may need two calls) accounts for everything the provider reported, not only the three counts.
func TestTheAcceptanceCheckSumsTheWholeUsageOfItsCalls(t *testing.T) {
	_, usage, err := judgeResult(context.Background(), nil, platform.Entry{Completer: &richCompleter{}}, "k", "m", "task", "crit", "res")
	if err != nil {
		t.Fatal(err)
	}
	if usage.PromptTokens != 200 || usage.CompletionTokens != 20 || usage.TotalTokens != 220 || usage.CacheReadTokens != 120 || usage.CacheCreationTokens != 10 || usage.ReasoningTokens != 8 {
		t.Errorf("usage = %+v", usage)
	}
	if usage.Cost == nil || *usage.Cost != 0.25 {
		t.Errorf("cost = %v", usage.Cost)
	}
}

func answerWith(u chatcompleter.Usage) func() (*chatcompleter.ChatCompletionResult, error) {
	return func() (*chatcompleter.ChatCompletionResult, error) {
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: "done"}, FinishReason: "stop", Usage: u}, nil
	}
}

// The run log's own line for each model request (plan/ai/conversation/step-178): the reasoning and the cost appear when the provider
// reported them, and the line of a provider that reports neither is exactly what it always was.
func TestTheRunLogLineCarriesReasoningAndCostOnlyWhenReported(t *testing.T) {
	lineFor := func(u chatcompleter.Usage) string {
		var steps []string
		var exchanges int
		c := &stepCompleter{steps: []func() (*chatcompleter.ChatCompletionResult, error){answerWith(u)}}
		if _, err := runRetryLoop(context.Background(), c, &steps, &exchanges); err != nil {
			t.Fatal(err)
		}
		for _, s := range steps {
			if strings.HasPrefix(s, "iteration 1:") && strings.Contains(s, " prompt + ") {
				return s
			}
		}
		t.Fatalf("no usage line in %v", steps)
		return ""
	}
	if got := lineFor(chatcompleter.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110}); got != "iteration 1: 100 prompt + 10 completion tokens" {
		t.Errorf("plain: %q", got)
	}
	full := chatcompleter.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110, CacheCreationTokens: 5, CacheReadTokens: 60, ReasoningTokens: 4, Cost: cost(0.25)}
	if got := lineFor(full); got != "iteration 1: 100 prompt + 10 completion tokens (5 cache write, 60 cache read) (4 reasoning) cost $0.25" {
		t.Errorf("full: %q", got)
	}
	if got := lineFor(chatcompleter.Usage{PromptTokens: 100, CompletionTokens: 10, Cost: cost(0)}); got != "iteration 1: 100 prompt + 10 completion tokens cost $0" {
		t.Errorf("free: %q", got)
	}
}
