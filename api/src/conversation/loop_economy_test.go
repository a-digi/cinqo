package conversation

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/tool/mcp"
)

func bigText(n int) string { return strings.Repeat("x", n) }

func call(id, name, args string) chatcompleter.ToolCall {
	return chatcompleter.ToolCall{ID: id, Name: name, Arguments: []byte(args)}
}

// group is one round: the assistant's calls and one result per call.
func group(calls []chatcompleter.ToolCall, results ...string) []chatcompleter.Message {
	g := []chatcompleter.Message{{Role: "assistant", ToolCalls: calls}}
	for i, r := range results {
		g = append(g, chatcompleter.Message{Role: "tool", ToolCallID: calls[i].ID, Content: r})
	}
	return g
}

func TestOldBulkyResultsAreFoldedAndTheNewestStayWhole(t *testing.T) {
	groups := [][]chatcompleter.Message{
		group([]chatcompleter.ToolCall{call("a", "fetch_page_html", `{"url":"https://e.com/jobs","maxAttributeLength":200}`)}, bigText(18000)),
		group([]chatcompleter.ToolCall{call("b", "find_repeating_items", `{"url":"https://e.com/jobs"}`), call("c", "get_portal_link_crawl_instructions", `{"id":"l1"}`)}, bigText(5000), "short answer"),
		group([]chatcompleter.ToolCall{call("d", "crawl_paginated", `{"instructions":"x"}`)}, bigText(9000)),
		group([]chatcompleter.ToolCall{call("e", "fetch_page_html", `{"url":"https://e.com/2"}`)}, bigText(7000)),
	}
	before := fmt.Sprint(groups)
	out := foldOldToolResults(groups)
	if fmt.Sprint(groups) != before {
		t.Fatal("the groups were modified")
	}
	a := out[0][1].Content
	if !strings.HasPrefix(a, "[earlier result of fetch_page_html (") || !strings.Contains(a, `"maxAttributeLength":200`) || !strings.Contains(a, "18,000 characters") || !strings.Contains(a, "call it again") {
		t.Errorf("stub: %q", a)
	}
	if out[1][1].Content == bigText(5000) || !strings.Contains(out[1][1].Content, "find_repeating_items") {
		t.Errorf("second round's big result was not folded: %q", out[1][1].Content)
	}
	if out[1][2].Content != "short answer" {
		t.Errorf("a short result was folded: %q", out[1][2].Content)
	}
	if out[2][1].Content != bigText(9000) || out[3][1].Content != bigText(7000) {
		t.Error("one of the newest two rounds was folded")
	}
	if out[0][0].ToolCalls[0].Name != "fetch_page_html" {
		t.Error("the assistant message changed")
	}
}

func TestAFoldNeedsNothingWhenThereAreFewRoundsOrNoBulk(t *testing.T) {
	if got := foldOldToolResults(nil); len(got) != 0 {
		t.Error(got)
	}
	two := [][]chatcompleter.Message{
		group([]chatcompleter.ToolCall{call("a", "fetch_page_html", `{}`)}, bigText(9000)),
		group([]chatcompleter.ToolCall{call("b", "fetch_page_html", `{}`)}, bigText(9000)),
	}
	out := foldOldToolResults(two)
	if out[0][1].Content != bigText(9000) || out[1][1].Content != bigText(9000) {
		t.Error("with two rounds nothing may be folded")
	}
	// at the threshold exactly: not folded; one more character: folded
	edge := [][]chatcompleter.Message{
		group([]chatcompleter.ToolCall{call("a", "t", `{}`)}, bigText(foldMinChars)),
		group([]chatcompleter.ToolCall{call("b", "t", `{}`)}, bigText(foldMinChars+1)),
		group([]chatcompleter.ToolCall{call("c", "t", `{}`)}, "x"),
		group([]chatcompleter.ToolCall{call("d", "t", `{}`)}, "x"),
	}
	got := foldOldToolResults(edge)
	if got[0][1].Content != bigText(foldMinChars) || !strings.HasPrefix(got[1][1].Content, "[earlier result of t") {
		t.Errorf("threshold: %q / %q", got[0][1].Content[:20], got[1][1].Content[:30])
	}
}

func TestAResultWithoutItsCallStillGetsAStub(t *testing.T) {
	groups := [][]chatcompleter.Message{
		{{Role: "assistant"}, {Role: "tool", ToolCallID: "zz", Content: bigText(3000)}},
		group([]chatcompleter.ToolCall{call("b", "t", `{}`)}, "x"),
		group([]chatcompleter.ToolCall{call("c", "t", `{}`)}, "x"),
	}
	if got := foldOldToolResults(groups)[0][1].Content; !strings.HasPrefix(got, "[earlier result of a tool,") {
		t.Errorf("%q", got)
	}
}

func TestTheBudgetNoteIsAddedOnceTwoRoundsBeforeTheLast(t *testing.T) {
	msgs := []chatcompleter.Message{{Role: "user", Content: "go"}, {Role: "assistant"}, {Role: "tool", ToolCallID: "a", Content: "result"}}
	for _, limit := range []int{3, 10, 14} {
		for i := 0; i < limit; i++ {
			got := withBudgetNudge(msgs, i, limit)
			noted := strings.Contains(got[len(got)-1].Content, "Budget note")
			if noted != (i == limit-2) {
				t.Errorf("limit %d round %d: note %v", limit, i, noted)
			}
			if i == limit-2 && !strings.HasPrefix(got[len(got)-1].Content, "result") {
				t.Errorf("the tool result was replaced: %q", got[len(got)-1].Content)
			}
		}
	}
	if msgs[2].Content != "result" {
		t.Error("the caller's messages were modified")
	}
	if got := withBudgetNudge(msgs, 0, 2); strings.Contains(got[2].Content, "Budget note") {
		t.Error("a budget of 2 rounds got a note")
	}
	notTool := []chatcompleter.Message{{Role: "user", Content: "go"}}
	if got := withBudgetNudge(notTool, 8, 10); got[0].Content != "go" {
		t.Error("a request without a tool result as its last message was changed")
	}
}

func TestOnlyAnOptedInConversationTurnsTheEconomyOn(t *testing.T) {
	ctx := context.Background()
	if economyOn(withToolLoopEconomy(ctx, nil)) || economyOn(withToolLoopEconomy(ctx, &conversation_entity.Conversation{})) {
		t.Error("economy on without the opt-in")
	}
	if !economyOn(withToolLoopEconomy(ctx, &conversation_entity.Conversation{CompactToolResults: true})) {
		t.Error("economy off with the opt-in")
	}
}

// ---- the loop ----

type round struct{ name, args string }

// recordingModel answers every request with the next scripted tool call (and "done" at the end), recording every request it got.
type recordingModel struct {
	script   []round
	requests [][]chatcompleter.Message
}

func (m *recordingModel) ChatCompletion(_ context.Context, _ *http.Client, _, _, _ string, messages []chatcompleter.Message, tools []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	m.requests = append(m.requests, append([]chatcompleter.Message(nil), messages...))
	i := len(m.requests) - 1
	if i >= len(m.script) || len(tools) == 0 {
		return &chatcompleter.ChatCompletionResult{Message: chatcompleter.Message{Role: "assistant", Content: "done"}, FinishReason: "stop"}, nil
	}
	r := m.script[i]
	return &chatcompleter.ChatCompletionResult{
		Message:   chatcompleter.Message{Role: "assistant"},
		ToolCalls: []chatcompleter.ToolCall{{ID: fmt.Sprintf("c%d", i), Name: r.name, Arguments: []byte(r.args)}},
	}, nil
}

func economyTools() []chatcompleter.ToolDef {
	var out []chatcompleter.ToolDef
	for _, n := range []string{"fetch_page_html", "find_repeating_items", "get_portal_link_crawl_instructions", "crawl_paginated", "set_portal_link_crawl_instructions", "other_tool"} {
		out = append(out, chatcompleter.ToolDef{Name: n, InputSchema: []byte(`{"type":"object"}`)})
	}
	return out
}

// runEconomy runs the loop on the script; every tool answers result(n) (n = how many tool calls ran so far, 1-based) and the names
// of the calls that really ran are returned.
func runEconomy(t *testing.T, on bool, limit int, script []round, result func(name string, n int) string) (*recordingModel, []string) {
	t.Helper()
	model := &recordingModel{script: script}
	var ran []string
	old := invokeToolFn
	invokeToolFn = func(_ context.Context, _ *sql.DB, _ []string, _ string, c chatcompleter.ToolCall, _ string, _ int, _ string) (string, []mcp.ResourceLink, *pendingDelegation) {
		ran = append(ran, c.Name+" "+string(c.Arguments))
		return result(c.Name, len(ran)), nil, nil
	}
	t.Cleanup(func() { invokeToolFn = old })
	ctx := context.Background()
	if on {
		ctx = withToolLoopEconomy(ctx, &conversation_entity.Conversation{CompactToolResults: true})
	}
	_, err := runToolLoop(ctx, http.DefaultClient, platform.Entry{Completer: model}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, economyTools(), nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, limit)
	if err != nil {
		t.Fatalf("loop: %v", err)
	}
	return model, ran
}

func joined(msgs []chatcompleter.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func TestAnIdenticalRepeatedCallIsAnsweredInOneLine(t *testing.T) {
	args := `{"url":"https://e.com/jobs","maxAttributeLength":200}`
	reordered := `{ "maxAttributeLength": 200, "url": "https://e.com/jobs" }`
	model, ran := runEconomy(t, true, 14, []round{{"fetch_page_html", args}, {"fetch_page_html", reordered}}, func(string, int) string { return bigText(5000) })
	if len(ran) != 1 {
		t.Fatalf("the repeated call ran again: %v", ran)
	}
	last := model.requests[2] // the request after the second call
	if got := last[len(last)-1].Content; !strings.Contains(got, "same fetch_page_html call as in round 1") || strings.Contains(got, "xxxxx") {
		t.Errorf("the answer: %q", got)
	}
}

func TestAnyOtherToolClearsTheMemoryOfRepeatableCalls(t *testing.T) {
	args := `{"url":"https://e.com/jobs"}`
	script := []round{{"fetch_page_html", args}, {"other_tool", `{"n":1}`}, {"other_tool", `{"n":2}`}, {"other_tool", `{"n":3}`}, {"fetch_page_html", args}}
	_, ran := runEconomy(t, true, 14, script, func(name string, n int) string {
		if name == "fetch_page_html" {
			return bigText(5000)
		}
		return "ok"
	})
	// "other_tool" is not a read-only tool, so it clears the memory: the second fetch is a fresh call
	if len(ran) != 5 {
		t.Errorf("ran %v", ran)
	}
}

func TestAnotherReadOnlyCallInBetweenKeepsTheMemoryButTheWindowEnds(t *testing.T) {
	a := `{"url":"https://e.com/a"}`
	script := []round{{"fetch_page_html", a}, {"find_repeating_items", `{"url":"1"}`}, {"find_repeating_items", `{"url":"2"}`}, {"find_repeating_items", `{"url":"3"}`}, {"fetch_page_html", a}}
	_, ran := runEconomy(t, true, 14, script, func(string, int) string { return bigText(5000) })
	if len(ran) != 5 {
		t.Errorf("a call whose earlier result was folded away must run again: %v", ran)
	}
	// within the window (two rounds later) it is answered from memory
	script = []round{{"fetch_page_html", a}, {"find_repeating_items", `{"url":"1"}`}, {"fetch_page_html", a}}
	_, ran = runEconomy(t, true, 14, script, func(string, int) string { return bigText(5000) })
	if len(ran) != 2 {
		t.Errorf("a repeat inside the window should be answered: %v", ran)
	}
}

func TestAWriteToolClearsWhatTheReadOnlyToolsWouldAnswer(t *testing.T) {
	a := `{"id":"l1"}`
	script := []round{{"get_portal_link_crawl_instructions", a}, {"set_portal_link_crawl_instructions", `{"id":"l1","instructions":"y"}`}, {"get_portal_link_crawl_instructions", a}}
	_, ran := runEconomy(t, true, 14, script, func(string, int) string { return bigText(4000) })
	if len(ran) != 3 {
		t.Errorf("the second read ran from stale memory: %v", ran)
	}
}

func TestASmallResultIsNotRememberedAndRunsAgain(t *testing.T) {
	a := `{"url":"https://e.com/a"}`
	_, ran := runEconomy(t, true, 14, []round{{"fetch_page_html", a}, {"fetch_page_html", a}}, func(string, int) string { return "tiny" })
	if len(ran) != 2 {
		t.Errorf("%v", ran)
	}
}

func TestCrawlPaginatedIsRepeatableOnlyWithItsOwnUrl(t *testing.T) {
	withURL := `{"instructions":"url: https://e.com/jobs\ncontainer: li\n"}`
	without := `{"instructions":"container: li\n"}`
	_, ran := runEconomy(t, true, 14, []round{{"crawl_paginated", withURL}, {"crawl_paginated", withURL}}, func(string, int) string { return bigText(5000) })
	if len(ran) != 1 {
		t.Errorf("with a url: %v", ran)
	}
	_, ran = runEconomy(t, true, 14, []round{{"crawl_paginated", without}, {"crawl_paginated", without}}, func(string, int) string { return bigText(5000) })
	if len(ran) != 2 {
		t.Errorf("without a url the browser's page may have changed: %v", ran)
	}
}

func TestADifferentArgumentIsAnotherCall(t *testing.T) {
	_, ran := runEconomy(t, true, 14, []round{{"fetch_page_html", `{"url":"https://e.com/a","offset":0}`}, {"fetch_page_html", `{"url":"https://e.com/a","offset":18000}`}}, func(string, int) string { return bigText(5000) })
	if len(ran) != 2 {
		t.Errorf("%v", ran)
	}
}

func TestRequestsFoldOldResultsAndNudgeOnlyWhenOptedIn(t *testing.T) {
	var script []round
	for i := 0; i < 8; i++ {
		script = append(script, round{"find_repeating_items", fmt.Sprintf(`{"url":"https://e.com/%d"}`, i)})
	}
	result := func(string, int) string { return bigText(4000) }

	on, _ := runEconomy(t, true, 10, script, result)
	off, _ := runEconomy(t, false, 10, script, result)

	// round 7 request (index 6) holds six results: with the economy the four oldest are stubs, the newest two whole
	req := on.requests[6]
	stubs, whole := 0, 0
	for _, m := range req {
		if m.Role != "tool" {
			continue
		}
		if strings.HasPrefix(m.Content, "[earlier result of find_repeating_items") {
			stubs++
		} else if m.Content == bigText(4000) {
			whole++
		}
	}
	if stubs != 4 || whole != 2 {
		t.Errorf("economy on: %d stubs and %d whole results, want 4 and 2", stubs, whole)
	}
	for i, req := range on.requests {
		noted := strings.Contains(joined(req), "Budget note")
		if noted != (i == 8) { // limit 10: the request of round index 8
			t.Errorf("economy on, request %d: note %v", i, noted)
		}
	}
	// without the opt-in: every result whole, no stub, no note, requests exactly the plain history
	for i, req := range off.requests {
		j := joined(req)
		if strings.Contains(j, "earlier result of") || strings.Contains(j, "Budget note") || strings.Contains(j, "same find_repeating_items call") {
			t.Fatalf("economy off, request %d was changed", i)
		}
		tool := 0
		for _, m := range req {
			if m.Role == "tool" && m.Content == bigText(4000) {
				tool++
			}
		}
		if tool != i {
			t.Errorf("economy off, request %d holds %d whole results, want %d", i, tool, i)
		}
	}
	if len(on.requests[0]) != len(off.requests[0]) {
		t.Error("the first request differs")
	}
}

func TestTheWrapUpAfterAUsedUpBudgetAlsoShowsTheFoldedHistory(t *testing.T) {
	var script []round
	for i := 0; i < 5; i++ {
		script = append(script, round{"find_repeating_items", fmt.Sprintf(`{"url":"%d"}`, i)})
	}
	model := &recordingModel{script: script}
	old := invokeToolFn
	t.Cleanup(func() { invokeToolFn = old })
	invokeToolFn = func(_ context.Context, _ *sql.DB, _ []string, _ string, c chatcompleter.ToolCall, _ string, _ int, _ string) (string, []mcp.ResourceLink, *pendingDelegation) {
		return bigText(4000), nil, nil
	}
	ctx := withToolLoopEconomy(context.Background(), &conversation_entity.Conversation{CompactToolResults: true})
	_, err := runToolLoop(ctx, http.DefaultClient, platform.Entry{Completer: model}, "k", "m",
		[]chatcompleter.Message{{Role: "user", Content: "hi"}}, economyTools(), nil, nil, "u1", t.TempDir(), 0, "conv", nil, "", 0,
		conversation_entity.OrchestrationModeDirect, nil, nil, nil, 4)
	if err == nil || !strings.Contains(err.Error(), "without a final reply") {
		t.Fatalf("err = %v", err)
	}
	wrap := model.requests[len(model.requests)-1]
	if !strings.Contains(joined(wrap), "earlier result of find_repeating_items") || !strings.Contains(wrap[len(wrap)-1].Content, "tool-call budget") {
		t.Error("the wrap-up request did not show the folded history")
	}
}

// Both turn runners must switch the economy on for an opted-in conversation, or the flag would be stored and never used.
func TestBothTurnRunnersApplyTheOptIn(t *testing.T) {
	for _, file := range []string{"chat.go", "runner.go"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "ctx = withToolLoopEconomy(ctx, conv)") {
			t.Errorf("%s does not apply the compactToolResults opt-in", file)
		}
	}
}
