package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// invokeTimeout bounds one call's whole spawn+handshake+call round
// trip — real headroom above a tool's own internal budget (pdf_generator's
// own 30s render timeout) for process spawn + MCP handshake overhead.
const invokeTimeout = 45 * time.Second

// Invoke spawns execPath in --mcp mode, calls toolName with args, and
// closes the session (subprocess terminates) — one spawn per call.
//
// Deliberately never returns a bare error for a call/execution
// failure — MCP's own spec draws two error kinds (protocol errors and
// tool-reported isError:true results), and both are collapsed here
// into the same contract: always produce result text the caller can
// feed back to the model as a normal tool-result message, so a failed
// call never strands an unanswered tool_call in conversation history.
// A returned error here means something so broken the caller cannot
// proceed at all (e.g. args weren't valid JSON).
// envVars is the same manager.ToolEnvVars contract Discover requires
// — see that function's own doc comment for the real failure this
// fixes.
func Invoke(ctx context.Context, execPath, toolName string, args json.RawMessage, envVars []string) (resultText string, isError bool) {
	ctx, cancel := context.WithTimeout(ctx, invokeTimeout)
	defer cancel()

	client := mcp.NewClient(clientImplementation, nil)
	cmd := exec.Command(execPath, "--mcp")
	cmd.Env = append(os.Environ(), envVars...)
	transport := &mcp.CommandTransport{Command: cmd}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Sprintf("tool failed to start: %v", err), true
	}
	defer session.Close()

	var argMap map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &argMap); err != nil {
			return fmt.Sprintf("invalid tool arguments: %v", err), true
		}
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: argMap})
	if err != nil {
		return fmt.Sprintf("tool call failed: %v", err), true
	}
	return renderContent(res.Content), res.IsError
}

// renderContent turns MCP's own content items into the plain text a
// chat-completion tool-result message carries. A resource_link
// becomes a Markdown link deliberately: if the model quotes or
// paraphrases this text back to the user,
// plan/ai/frontend/frontend/step-12's already-shipped Markdown
// renderer turns it into a real, clickable link with zero extra
// plumbing.
func renderContent(content []mcp.Content) string {
	parts := make([]string, 0, len(content))
	for _, c := range content {
		switch v := c.(type) {
		case *mcp.TextContent:
			parts = append(parts, v.Text)
		case *mcp.ResourceLink:
			parts = append(parts, fmt.Sprintf("[%s](%s)", v.Name, v.URI))
		}
	}
	return strings.Join(parts, "\n")
}
