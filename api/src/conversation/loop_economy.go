package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	conversation_entity "github.com/a-digi/cinqo/src/conversation/entity"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// loop_economy.go (plan/ai/conversation/step-45, step 3): three rules of the tool loop, on only for a conversation created with
// compactToolResults. They change what the model is SHOWN, never what is stored or what a first call does:
//
//   - old bulky tool results are folded into one-line stubs (the loop re-sends the whole history with every request, and two page
//     windows plus a few crawl results made a request 99,000 tokens long at round 10);
//   - an identical repeated read-only call, whose earlier result is still shown whole, is answered with one line instead of a second
//     copy of the result;
//   - a short note warns two rounds before the tool-call budget ends (the only message used to come after it was gone).
//
// A model that needs a folded result simply calls the tool again; the stub says so.

type economyKey struct{}

// withToolLoopEconomy marks the turn's context when the conversation opted in.
func withToolLoopEconomy(ctx context.Context, conv *conversation_entity.Conversation) context.Context {
	if conv == nil || !conv.CompactToolResults {
		return ctx
	}
	return context.WithValue(ctx, economyKey{}, true)
}

func economyOn(ctx context.Context) bool {
	on, _ := ctx.Value(economyKey{}).(bool)
	return on
}

const (
	// foldKeepGroups: the newest rounds whose results always stay whole (the model has not yet acted on them).
	foldKeepGroups = 2
	// foldMinChars: only a result longer than this is folded; shorter ones cost less than their stub's explanation.
	foldMinChars = 2000
	// stubArgsRunes bounds the argument summary in a stub.
	stubArgsRunes = 100
	// nudgeRoundsLeft: the budget note goes into the request of the round this many rounds before the last one.
	nudgeRoundsLeft = 2
)

// foldOldToolResults returns a copy of groups in which every bulky tool result outside the newest foldKeepGroups rounds is replaced
// by a one-line stub naming the tool, a short form of its arguments and the size. groups and its messages are never modified.
func foldOldToolResults(groups [][]chatcompleter.Message) [][]chatcompleter.Message {
	out := make([][]chatcompleter.Message, len(groups))
	for gi, g := range groups {
		if gi >= len(groups)-foldKeepGroups || len(g) == 0 {
			out[gi] = g
			continue
		}
		calls := make(map[string]chatcompleter.ToolCall, len(g[0].ToolCalls))
		for _, c := range g[0].ToolCalls {
			calls[c.ID] = c
		}
		folded := make([]chatcompleter.Message, len(g))
		copy(folded, g)
		for mi, m := range folded {
			if m.Role != "tool" {
				continue
			}
			if n := utf8.RuneCountInString(m.Content); n > foldMinChars {
				folded[mi].Content = foldedStub(calls[m.ToolCallID], n)
			}
		}
		out[gi] = folded
	}
	return out
}

func foldedStub(call chatcompleter.ToolCall, chars int) string {
	name := call.Name
	if name == "" {
		name = "a tool"
	}
	args := ""
	if short := shortArguments(call.Arguments); short != "" {
		args = " (" + short + ")"
	}
	return fmt.Sprintf("[earlier result of %s%s, %s characters, left out to save tokens; call it again if you need it]", name, args, groupThousands(chars))
}

func shortArguments(raw []byte) string {
	canon := canonicalArguments(raw)
	if canon == "" || canon == "{}" {
		return ""
	}
	if r := []rune(canon); len(r) > stubArgsRunes {
		return string(r[:stubArgsRunes]) + "…"
	}
	return canon
}

// canonicalArguments is the arguments as one JSON text with sorted keys, so two calls that differ only in key order or spacing are the
// same call; text that is not JSON is compared as it is.
func canonicalArguments(raw []byte) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return strings.TrimSpace(string(raw))
	}
	out, err := json.Marshal(v)
	if err != nil {
		return strings.TrimSpace(string(raw))
	}
	return string(out)
}

func groupThousands(n int) string {
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// withBudgetNudge adds a one-sentence budget note to the last tool result of the request of the round that is nudgeRoundsLeft before
// the last one. i is the 0-based round of this request. A copy is made; with a budget under 3 rounds, or when the last message is not a
// tool result, nothing changes.
func withBudgetNudge(messages []chatcompleter.Message, i, limit int) []chatcompleter.Message {
	if limit < nudgeRoundsLeft+1 || i != limit-nudgeRoundsLeft || len(messages) == 0 {
		return messages
	}
	last := messages[len(messages)-1]
	if last.Role != "tool" {
		return messages
	}
	last.Content += "\n\n[Budget note: you have two tool-call rounds left in this task. If you can already finish it (for example save a result that checks out), do it now; otherwise use them well and then say plainly what blocks you.]"
	out := append([]chatcompleter.Message{}, messages...)
	out[len(out)-1] = last
	return out
}

// repeatableTools are the read-only tools whose identical repeated call may be answered from what the model already has.
var repeatableTools = map[string]bool{
	"fetch_page_html":                    true,
	"find_repeating_items":               true,
	"get_portal_link_crawl_instructions": true,
	"crawl_paginated":                    true,
}

// repeatableCall says whether this very call may be answered from memory: a read-only tool, and for crawl_paginated only an
// instruction that carries its own url (without one it crawls whatever page the browser happens to show, which a call in between can change).
func repeatableCall(call chatcompleter.ToolCall) bool {
	if !repeatableTools[call.Name] {
		return false
	}
	if call.Name != "crawl_paginated" {
		return true
	}
	var args struct {
		Instructions string `json:"instructions"`
	}
	if json.Unmarshal(call.Arguments, &args) != nil {
		return false
	}
	for _, line := range strings.Split(args.Instructions, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "url:") {
			return true
		}
	}
	return false
}

type seenCall struct {
	group int
	chars int
}

// callMemory remembers the bulky results of repeatable calls of one turn.
type callMemory struct {
	seen map[string]seenCall
}

func callKey(call chatcompleter.ToolCall) string {
	return call.Name + "\x00" + canonicalArguments(call.Arguments)
}

// answer returns the one-line reply for a call identical to an earlier one whose result is still shown whole in the request of round
// i (0-based): it was in one of the newest foldKeepGroups rounds, so it has not been folded away. Otherwise the call runs again.
func (m *callMemory) answer(call chatcompleter.ToolCall, i int) (string, bool) {
	if m == nil || !repeatableCall(call) {
		return "", false
	}
	prev, ok := m.seen[callKey(call)]
	if !ok || i-prev.group > foldKeepGroups {
		return "", false
	}
	return fmt.Sprintf("This is the same %s call as in round %d; its result is unchanged and is still above: use it instead of calling again.", call.Name, prev.group+1), true
}

// after is told the result of a call that ran in round i: a bulky result of a repeatable call is remembered, any other tool may change
// what the read-only tools would answer, so it clears the memory.
func (m *callMemory) after(call chatcompleter.ToolCall, i int, result string) {
	if !repeatableTools[call.Name] {
		m.seen = nil
		return
	}
	if !repeatableCall(call) || utf8.RuneCountInString(result) <= foldMinChars {
		return
	}
	if m.seen == nil {
		m.seen = map[string]seenCall{}
	}
	m.seen[callKey(call)] = seenCall{group: i, chars: utf8.RuneCountInString(result)}
}

// wrapUpGroups is what the tools-free wrap-up request after a used-up budget shows: the folded history for an opted-in conversation.
func wrapUpGroups(economy bool, groups [][]chatcompleter.Message) [][]chatcompleter.Message {
	if economy {
		return foldOldToolResults(groups)
	}
	return groups
}
