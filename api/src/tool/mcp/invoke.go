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

// ResourceLink is a real file reference a tool call produced,
// extracted separately from the free-text result text. Fixes a real,
// live-observed problem: a model paraphrasing a tool's result into its
// own final reply is not guaranteed to preserve a URL verbatim — in
// one live test it fabricated a plausible-looking but entirely wrong
// absolute domain instead of the real relative path. The caller
// (conversation.chat.go) appends these deterministically to the
// user-visible reply instead of trusting the model to carry them
// through unchanged. See
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md.
type ResourceLink struct {
	Name string
	URI  string
}

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
func Invoke(ctx context.Context, execPath, toolName string, args json.RawMessage, envVars []string) (resultText string, isError bool, links []ResourceLink) {
	ctx, cancel := context.WithTimeout(ctx, invokeTimeout)
	defer cancel()

	client := mcp.NewClient(clientImplementation, nil)
	cmd := exec.Command(execPath, "--mcp")
	cmd.Env = append(os.Environ(), envVars...)
	transport := &mcp.CommandTransport{Command: cmd}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Sprintf("tool failed to start: %v", err), true, nil
	}
	defer session.Close()

	var argMap map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &argMap); err != nil {
			return fmt.Sprintf("invalid tool arguments: %v", err), true, nil
		}
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: argMap})
	if err != nil {
		return fmt.Sprintf("tool call failed: %v", err), true, nil
	}
	text, links := renderContent(res.Content)
	return text, res.IsError, links
}

// renderContent turns MCP's own content items into the plain text a
// chat-completion tool-result message carries, AND separately collects
// every resource_link's real {name, uri} — the deterministic source of
// truth conversation.chat.go appends to the final reply itself, rather
// than relying on the model to reproduce the Markdown link text
// unchanged.
func renderContent(content []mcp.Content) (string, []ResourceLink) {
	parts := make([]string, 0, len(content))
	links := make([]ResourceLink, 0, len(content))
	for _, c := range content {
		switch v := c.(type) {
		case *mcp.TextContent:
			parts = append(parts, v.Text)
		case *mcp.ResourceLink:
			parts = append(parts, fmt.Sprintf("[%s](%s)", v.Name, v.URI))
			links = append(links, ResourceLink{Name: v.Name, URI: v.URI})
		}
	}
	return strings.Join(parts, "\n"), links
}
