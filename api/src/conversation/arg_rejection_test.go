package conversation

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/tool/mcp"
)

const rejectionText = `validating "arguments": validating root: unexpected additional properties ["maxPages"]`

func TestIsArgumentRejection(t *testing.T) {
	for text, want := range map[string]bool{
		rejectionText: true,
		`validating "arguments": validating /properties/pages: type: [] has type "string"`: true,
		"invalid tool arguments: invalid character 'x'":                                    true,
		"tool unavailable":                      false,
		"Saved 3 jobs":                          false,
		`the page said: validating "arguments"`: false, // only a leading marker counts
		"":                                      false,
	} {
		if got := isArgumentRejection(text); got != want {
			t.Errorf("isArgumentRejection(%q) = %v, want %v", text, got, want)
		}
	}
}

func crawlTools() []chatcompleter.ToolDef {
	return []chatcompleter.ToolDef{
		{Name: "crawl_paginated", InputSchema: []byte(`{"type":"object","properties":{"instructions":{"type":"string"}},"required":["instructions"]}`)},
		{Name: "check_crawl_extraction_result", InputSchema: []byte(`{"type":"object","properties":{"pages":{"type":["null","array"]}}}`)},
		{Name: "opaque", InputSchema: []byte(`not json`)},
	}
}

func TestArgumentHintComesFromTheToolsOwnSchema(t *testing.T) {
	tools := crawlTools()
	if got := argumentHint(tools, "crawl_paginated"); !strings.Contains(got, "instructions (string, required)") || strings.Contains(got, "maxPages") {
		t.Errorf("crawl_paginated hint: %q", got)
	}
	if got := argumentHint(tools, "check_crawl_extraction_result"); !strings.Contains(got, "pages (array)") {
		t.Errorf("a nullable array must read as array: %q", got)
	}
	if argumentHint(tools, "nope") != "" || argumentHint(tools, "opaque") != "" {
		t.Error("an unknown tool or an unreadable schema gives no hint")
	}
}

// scriptedCalls calls the named tools in order, one per model call, and
// records every tool-result message the model got back.
type scriptedCalls struct {
	names   []string
	calls   int
	results []string
}

func (s *scriptedCalls) ChatCompletion(_ context.Context, _ *http.Client, _, _, _ string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	if m := messages[len(messages)-1]; m.Role == "tool" {
		s.results = append(s.results, m.Content)
	}
	if s.calls >= len(s.names) || len(tools) == 0 {
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: "done"}, FinishReason: "stop"}, nil
	}
	name := s.names[s.calls]
	s.calls++
	return &chatcompleter.ChatCompletionResult{
		Message:   chatcompleter.Message{Role: "assistant"},
		ToolCalls: []chatcompleter.ToolCall{{ID: "c", Name: name, Arguments: []byte(`{}`)}},
	}, nil
}

// withTools makes every call to a tool answer from answers[name] (a func so a
// test can alternate), instead of running a real tool.
func withTools(t *testing.T, answer func(name string, n int) string) {
	t.Helper()
	old := invokeToolFn
	n := 0
	invokeToolFn = func(_ context.Context, _ *sql.DB, _ []string, _ string, call chatcompleter.ToolCall, _ string, _ int, _ string) (string, []mcp.ResourceLink, *pendingDelegation) {
		n++
		return answer(call.Name, n), nil, nil
	}
	t.Cleanup(func() { invokeToolFn = old })
}

func runCalls(t *testing.T, s *scriptedCalls, limit int) (string, error) {
	t.Helper()
	return runToolLoop(context.Background(), http.DefaultClient, platform.Entry{Completer: s}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, crawlTools(), nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, limit)
}

func TestTheSecondRejectionOfAToolAddsItsAllowedArguments(t *testing.T) {
	withTools(t, func(string, int) string { return rejectionText })
	s := &scriptedCalls{names: []string{"crawl_paginated", "crawl_paginated"}}
	if _, err := runCalls(t, s, 0); err != nil {
		t.Fatalf("err = %v (two rejections must not stop the turn)", err)
	}
	if len(s.results) != 2 {
		t.Fatalf("results = %v", s.results)
	}
	if strings.Contains(s.results[0], "Allowed arguments") {
		t.Errorf("the first rejection is returned as is: %q", s.results[0])
	}
	if !strings.HasPrefix(s.results[1], rejectionText) || !strings.Contains(s.results[1], "Allowed arguments for crawl_paginated: instructions (string, required)") {
		t.Errorf("the second rejection must carry the hint: %q", s.results[1])
	}
}

func TestHintsAreCountedPerTool(t *testing.T) {
	withTools(t, func(string, int) string { return rejectionText })
	s := &scriptedCalls{names: []string{"crawl_paginated", "check_crawl_extraction_result"}}
	if _, err := runCalls(t, s, 0); err != nil {
		t.Fatal(err)
	}
	for i, r := range s.results {
		if strings.Contains(r, "Allowed arguments") {
			t.Errorf("result %d: one rejection of each tool must not get a hint: %q", i, r)
		}
	}
}

func TestFourRejectionsEndTheTurnNamingTheTool(t *testing.T) {
	withTools(t, func(string, int) string { return rejectionText })
	s := &scriptedCalls{names: []string{"crawl_paginated", "check_crawl_extraction_result", "crawl_paginated", "check_crawl_extraction_result", "crawl_paginated"}}
	got, err := runCalls(t, s, 0)
	if !errors.Is(err, ErrToolArgumentsRejected) || got != "" {
		t.Fatalf("got %q, %v", got, err)
	}
	if s.calls != maxArgumentRejectionsPerTurn {
		t.Errorf("calls = %d, want the turn to stop at the %dth rejection", s.calls, maxArgumentRejectionsPerTurn)
	}
	for _, want := range []string{"4 tool calls were rejected", "check_crawl_extraction_result", "unexpected additional properties"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err.Error(), want)
		}
	}
}

func TestAValidCallInBetweenDoesNotResetTheCount(t *testing.T) {
	withTools(t, func(_ string, n int) string {
		if n%2 == 1 {
			return rejectionText
		}
		return "ok"
	})
	s := &scriptedCalls{names: []string{"crawl_paginated", "crawl_paginated", "crawl_paginated", "crawl_paginated", "crawl_paginated", "crawl_paginated", "crawl_paginated", "crawl_paginated"}}
	_, err := runCalls(t, s, 0)
	if !errors.Is(err, ErrToolArgumentsRejected) || s.calls != 7 {
		t.Fatalf("err = %v after %d calls, want the 4th rejection (call 7) to end the turn", err, s.calls)
	}
}

func TestAToolThatRanAndReportedAProblemIsNotARejection(t *testing.T) {
	withTools(t, func(string, int) string { return "no jobs found on this page" })
	names := make([]string, 8)
	for i := range names {
		names[i] = "crawl_paginated"
	}
	s := &scriptedCalls{names: names}
	_, err := runCalls(t, s, 6)
	if errors.Is(err, ErrToolArgumentsRejected) {
		t.Fatalf("a tool's own error text must not count as a rejected call: %v", err)
	}
	if !errors.Is(err, ErrToolIterationLimitReached) {
		t.Errorf("err = %v, want the ordinary budget error", err)
	}
}
