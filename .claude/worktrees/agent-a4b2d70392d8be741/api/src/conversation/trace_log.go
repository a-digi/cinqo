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
	fmt.Fprintf(&sb, "finish_reason=%s  usage: prompt=%d completion=%d total=%d\n",
		result.FinishReason, result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens)
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
