// trace_log.go records, per turn, the full request/response exchange
// with the AI model across every iteration of that turn's own
// tool-calling loop — a diagnostic aid for understanding excessive
// token usage, distinct from log.go's own conversation-history
// Markdown log (which only ever stores the final user/assistant
// text). See plan/ai/conversation/step-36-ai-trace-logs.md.
//
// Content is a re-serialization of the neutral chatcompleter types
// (Message/ToolDef/ChatCompletionResult), not the literal wire bytes
// actually sent/received over HTTP — no raw bytes cross the
// ChatCompleter interface boundary at all (each platform client
// discards its own request/response []byte once parsed). This is a
// deliberate trade-off: touching all three platform clients
// (openai/anthropic/openrouter) to also return raw bytes would be a
// much larger, more invasive change for the same practical value here
// — what actually grows out of control across iterations is the
// *shape* of messages/tools, which this captures completely.
package conversation

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// TraceLogDir returns the directory holding one turn's own trace files
// for conversationID — <conversationsDir>/<conversationID>/, a new
// subdirectory alongside (never colliding with) the existing flat
// <conversationsDir>/<conversationID>_conversation.md file, since that
// file's own name always carries the "_conversation.md" suffix.
func TraceLogDir(conversationsDir, conversationID string) string {
	return filepath.Join(conversationsDir, conversationID)
}

// TraceLogPath returns one turn's own trace file path —
// <conversationsDir>/<conversationID>/<turnRunID>.txt.
func TraceLogPath(conversationsDir, conversationID, turnRunID string) string {
	return filepath.Join(TraceLogDir(conversationsDir, conversationID), turnRunID+".txt")
}

// appendTraceEntry appends one iteration's own request/response
// exchange to path (created, along with its parent directory, on
// first write for a given turn) — plain O_APPEND, matching
// log.go's/turn_run_persistent.go's own "never a read-modify-write"
// idiom elsewhere in this domain. callErr, when non-nil, is the
// ChatCompletion call's own error — logged in place of a response
// section, never silently dropped, since a failed call is exactly as
// diagnostically relevant as a successful one.
func appendTraceEntry(path string, iteration int, messages []chatcompleter.Message, tools []chatcompleter.ToolDef, result *chatcompleter.ChatCompletionResult, callErr error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("conversation: create trace log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("conversation: open trace log file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(formatTraceEntry(iteration, messages, tools, result, callErr)); err != nil {
		return fmt.Errorf("conversation: append trace log entry: %w", err)
	}
	return nil
}

func formatTraceEntry(iteration int, messages []chatcompleter.Message, tools []chatcompleter.ToolDef, result *chatcompleter.ChatCompletionResult, callErr error) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "=== Iteration %d — request (%d messages) ===\n\n", iteration, len(messages))
	for i, m := range messages {
		fmt.Fprintf(&sb, "[%d] role=%s", i, m.Role)
		if m.ToolCallID != "" {
			fmt.Fprintf(&sb, " tool_call_id=%s", m.ToolCallID)
		}
		for _, tc := range m.ToolCalls {
			fmt.Fprintf(&sb, " tool_call=%s(%s)", tc.Name, string(tc.Arguments))
		}
		sb.WriteString("\n")
		sb.WriteString(m.Content)
		sb.WriteString("\n\n")
	}

	if len(tools) > 0 {
		names := make([]string, len(tools))
		for i, t := range tools {
			names[i] = t.Name
		}
		fmt.Fprintf(&sb, "--- tools offered: %s ---\n\n", strings.Join(names, ", "))
	}

	if callErr != nil {
		fmt.Fprintf(&sb, "=== Iteration %d — error ===\n\n%s\n\n", iteration, callErr.Error())
		return sb.String()
	}

	fmt.Fprintf(&sb, "=== Iteration %d — response ===\n\n", iteration)
	fmt.Fprintf(&sb, "finish_reason=%s  usage: prompt=%d completion=%d total=%d%s\n",
		result.FinishReason, result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens, usageDetails(result.Usage))
	if result.Message.Content != "" {
		sb.WriteString(result.Message.Content)
		sb.WriteString("\n")
	}
	for _, tc := range result.ToolCalls {
		fmt.Fprintf(&sb, "tool_call: %s(%s)\n", tc.Name, string(tc.Arguments))
	}
	sb.WriteString("\n")

	return sb.String()
}

// turnTotals sums one turn's model requests (plan/ai/conversation/step-45, step 4): how many were made, the prompt and completion
// tokens the provider reported, the provider cache's share, and the largest single request, so the effect of fewer tools, folded
// results or a pinned (cacheable) model is read from the log of any turn instead of guessed.
type turnTotals struct {
	requests, failed       int
	prompt, completion     int
	cacheRead, cacheWrite  int
	largestPrompt          int
	largestPromptIteration int
	// reasoning is the hidden reasoning tokens the provider reported (inside completion); cost is the sum of the costs the provider
	// reported in US dollars over the costRequests responses that carried one (plan/ai/conversation/step-178).
	reasoning    int
	cost         float64
	costRequests int
}

// add records one exchange as logExchange sees it: callErr non-nil is a failed request (no usage), otherwise the result's usage.
func (t *turnTotals) add(iteration int, result *chatcompleter.ChatCompletionResult, callErr error) {
	if callErr != nil || result == nil {
		t.failed++
		return
	}
	t.requests++
	u := result.Usage
	t.prompt += u.PromptTokens
	t.completion += u.CompletionTokens
	t.cacheRead += u.CacheReadTokens
	t.cacheWrite += u.CacheCreationTokens
	t.reasoning += u.ReasoningTokens
	if u.Cost != nil {
		t.cost += *u.Cost
		t.costRequests++
	}
	if u.PromptTokens > t.largestPrompt {
		t.largestPrompt, t.largestPromptIteration = u.PromptTokens, iteration
	}
}

// summary is the block that ends a turn's trace file.
func (t turnTotals) summary() string {
	failed := ""
	if t.failed > 0 {
		failed = fmt.Sprintf(" (%d failed)", t.failed)
	}
	return fmt.Sprintf("=== Turn summary ===\n\nrequests=%d%s  prompt=%s  completion=%s  largest_request=%s (iteration %d)  cached_prompt=%s (cache read %s, cache write %s)%s\n",
		t.requests, failed, groupThousands(t.prompt), groupThousands(t.completion), groupThousands(t.largestPrompt), t.largestPromptIteration,
		groupThousands(t.cacheRead), groupThousands(t.cacheRead), groupThousands(t.cacheWrite), t.providerExtras())
}

// providerExtras is what the provider reported besides the token counts, for the summary line: the reasoning tokens and the cost, each
// only when something was reported, so a turn from a provider that reports neither reads exactly as it always did. The cost says how many
// of the requests carried one when not all did, because a partial sum must not look like the whole bill.
func (t turnTotals) providerExtras() string {
	out := ""
	if t.reasoning > 0 {
		out += fmt.Sprintf("  reasoning=%s", groupThousands(t.reasoning))
	}
	if t.costRequests > 0 {
		out += "  cost=" + formatCost(t.cost)
		if t.costRequests < t.requests {
			out += fmt.Sprintf(" (reported for %d of %d requests)", t.costRequests, t.requests)
		}
	}
	return out
}

// usageDetails is the part of one response's usage line that goes beyond the three main counts: the cache numbers, the reasoning
// tokens and the cost, each only when the provider reported something, so an unchanged provider's line is unchanged.
func usageDetails(u chatcompleter.Usage) string {
	out := ""
	if u.CacheReadTokens > 0 || u.CacheCreationTokens > 0 {
		out += fmt.Sprintf(" cache_read=%d cache_write=%d", u.CacheReadTokens, u.CacheCreationTokens)
	}
	if u.ReasoningTokens > 0 {
		out += fmt.Sprintf(" reasoning=%d", u.ReasoningTokens)
	}
	if u.Cost != nil {
		out += " cost=" + formatCost(*u.Cost)
	}
	return out
}

// formatCost writes US dollars with every digit the provider sent (no rounding: a call can cost a fraction of a cent).
func formatCost(dollars float64) string {
	return "$" + strconv.FormatFloat(dollars, 'f', -1, 64)
}

// appendTraceSummary appends the summary to the turn's trace file; nothing is written for a turn that made no request at all.
func appendTraceSummary(path string, t turnTotals) error {
	if t.requests == 0 && t.failed == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("conversation: open trace log file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString("\n" + t.summary()); err != nil {
		return fmt.Errorf("conversation: append trace log summary: %w", err)
	}
	return nil
}
