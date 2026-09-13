// Package mcp is the host's own MCP client — spawning an MCP-capable
// tool's backend on demand, talking to it over stdio for exactly one
// exchange, then letting it exit. Never a long-running supervised
// process (that's manager.go's separate, unrelated job for a tool's
// HTTP surface) — see
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md.
package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

// clientImplementation identifies cinqo itself to a tool's MCP
// server during the initialize handshake — purely informational.
var clientImplementation = &mcp.Implementation{Name: "cinqo", Version: "0.0.1"}

// Discover spawns execPath in --mcp mode, completes the MCP
// initialize handshake, calls tools/list, and closes the session —
// which, per the spec's own stdio transport semantics, closes stdin
// and terminates the subprocess. Nothing stays running afterward.
//
// envVars is the same TOOL_DB_DIR/TOOL_UPLOADS_DIR/TOOL_TMP_DIR/
// CORE_API_URL contract manager.ToolEnvVars builds for the
// long-running HTTP-mode spawn — this is a separate spawn of the same
// tool code, so it needs the identical env, not a bare inherited
// environment. Missing this caused a real, reproduced failure during
// implementation: the spawned process's own main() calls
// os.MkdirAll(os.Getenv("TOOL_TMP_DIR"), ...) at startup, which fails
// immediately on an empty/unset path, so the process exited before
// ever completing the MCP initialize handshake — surfacing here only
// as "connection closed: EOF", not an obviously-env-related error.
func Discover(ctx context.Context, execPath string, envVars []string) ([]tool_entity.ToolMCPTool, error) {
	client := mcp.NewClient(clientImplementation, nil)
	cmd := exec.Command(execPath, "--mcp")
	cmd.Env = append(os.Environ(), envVars...)
	transport := &mcp.CommandTransport{Command: cmd}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	res, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}

	out := make([]tool_entity.ToolMCPTool, 0, len(res.Tools))
	for _, t := range res.Tools {
		schema, err := json.Marshal(t.InputSchema)
		if err != nil {
			return nil, err
		}
		out = append(out, tool_entity.ToolMCPTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: string(schema),
		})
	}
	return out, nil
}
