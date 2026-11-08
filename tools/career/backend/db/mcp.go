package db

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrResult/JSONResult/WriteJSON originally lived in persona.go/http.go
// (package main) — moved here as part of the package-split refactor
// (step XX) specifically because they're genuinely generic, used by
// EVERY domain package's own MCP tool registrations and HTTP handlers
// alike, and package main can never be imported by anything, so
// anything every other package needs has to live somewhere else. This
// is that "somewhere else," same role tools/browser/backend/shared
// plays for that sibling tool. See
// plan/ai/tools/career/step-XX-package-split.md.

func ErrResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: true}
}

func JSONResult(v any) (*mcp.CallToolResult, any, error) {
	text, err := jsonMarshalIndent(v)
	if err != nil {
		return ErrResult(fmt.Sprintf("failed to format result: %v", err)), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

func jsonMarshalIndent(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteJSON is the plain HTTP mirror's own shared response writer —
// every domain's own HTTP handler (most still in this tool's own
// package main, some now in the portal/jobs/crawl packages) uses this
// same one.
func WriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
