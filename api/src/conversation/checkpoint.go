package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	tool_mcp "github.com/a-digi/cinqo/src/tool/mcp"
)

// maxCheckpointBytes caps what one failed turn may save (plan/ai/conversation/retry-last-step.md, step 3). The messages are already
// bounded by the loop's context trim and the 20,000-character cut of each tool result, so this only stops a runaway: a turn over it
// simply has no checkpoint (its Retry is then a Start again).
const maxCheckpointBytes = 2 << 20

// ErrCheckpointTooLarge is returned by EncodeCheckpoint for a state over maxCheckpointBytes.
var ErrCheckpointTooLarge = errors.New("conversation: checkpoint is too large")

// Checkpoint is the state a failed turn leaves behind so its last model call can be made again: the messages that call was sent (the
// history, the user's message and every tool round so far) and the resource links the tools returned, which the final answer lists.
type Checkpoint struct {
	Messages []chatcompleter.Message
	Links    []tool_mcp.ResourceLink
}

// checkpointMessage and its siblings are the stored shape. chatcompleter.Message hides its tool calls and tool-call id from JSON (they are
// not part of what any provider receives as-is), so the checkpoint has its own, complete form.
type checkpointMessage struct {
	Role       string               `json:"role"`
	Content    string               `json:"content,omitempty"`
	ToolCalls  []checkpointToolCall `json:"toolCalls,omitempty"`
	ToolCallID string               `json:"toolCallId,omitempty"`
}

type checkpointToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type checkpointLink struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

type checkpointFile struct {
	Version  int                 `json:"version"`
	Messages []checkpointMessage `json:"messages"`
	Links    []checkpointLink    `json:"links,omitempty"`
}

const checkpointVersion = 1

// EncodeCheckpoint turns the state into the text stored on the turn run. The cache breakpoint of a message is not stored: the loop sets it
// again itself.
func EncodeCheckpoint(c Checkpoint) (string, error) {
	f := checkpointFile{Version: checkpointVersion, Messages: make([]checkpointMessage, 0, len(c.Messages))}
	for _, m := range c.Messages {
		cm := checkpointMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			cm.ToolCalls = append(cm.ToolCalls, checkpointToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
		f.Messages = append(f.Messages, cm)
	}
	for _, l := range c.Links {
		f.Links = append(f.Links, checkpointLink{Name: l.Name, URI: l.URI})
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return "", fmt.Errorf("conversation: encode checkpoint: %w", err)
	}
	if len(raw) > maxCheckpointBytes {
		return "", ErrCheckpointTooLarge
	}
	return string(raw), nil
}

// DecodeCheckpoint is the inverse of EncodeCheckpoint. Text that is not a checkpoint of a version this code knows is an error, never a
// guess: a retry then falls back to Start again.
func DecodeCheckpoint(text string) (Checkpoint, error) {
	var f checkpointFile
	if err := json.Unmarshal([]byte(text), &f); err != nil {
		return Checkpoint{}, fmt.Errorf("conversation: decode checkpoint: %w", err)
	}
	if f.Version != checkpointVersion {
		return Checkpoint{}, fmt.Errorf("conversation: checkpoint version %d is not supported", f.Version)
	}
	if len(f.Messages) == 0 {
		return Checkpoint{}, errors.New("conversation: checkpoint has no messages")
	}
	var c Checkpoint
	for _, cm := range f.Messages {
		m := chatcompleter.Message{Role: cm.Role, Content: cm.Content, ToolCallID: cm.ToolCallID}
		for _, tc := range cm.ToolCalls {
			m.ToolCalls = append(m.ToolCalls, chatcompleter.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
		c.Messages = append(c.Messages, m)
	}
	for _, l := range f.Links {
		c.Links = append(c.Links, tool_mcp.ResourceLink{Name: l.Name, URI: l.URI})
	}
	return c, nil
}

// checkpointSink receives the state of a failed turn's loop (plan/ai/conversation/retry-last-step.md, step 4). It rides on the turn's
// context, so the loop's many parameters stay as they are, and a loop without one (a sub-agent, a test) saves nothing.
type checkpointSink struct {
	write func(Checkpoint)
	// links are the resource links the earlier attempt had collected.
	links []tool_mcp.ResourceLink
}

type checkpointSinkKey struct{}

func withCheckpointSink(ctx context.Context, s *checkpointSink) context.Context {
	return context.WithValue(ctx, checkpointSinkKey{}, s)
}

func checkpointSinkOf(ctx context.Context) *checkpointSink {
	s, _ := ctx.Value(checkpointSinkKey{}).(*checkpointSink)
	return s
}

// save keeps the messages of the call that failed, but only once the loop has done a tool round of its own (before that a retry would be
// a plain Start again). A resumed loop that fails with no new tool round saves nothing: the state it resumed from was carried onto its
// run when it started (StartRetryRun), and is still the right one. Safe on a nil sink.
func (s *checkpointSink) save(toolRounds int, sent []chatcompleter.Message, links []tool_mcp.ResourceLink) {
	if s == nil || s.write == nil || (toolRounds == 0) {
		return
	}
	s.write(Checkpoint{Messages: sent, Links: links})
}
