// helpers.go holds the small, domain-agnostic helpers every one of
// this tool's own feature packages (profile, cvimport, companies,
// portals, jobs, crawl) needs — the MCP result constructors each
// package's own tool registrations return, the JSON response writer
// each package's own HTTP handlers use, and the one shared shape
// behind every domain's own "this sentinel error means a 400, anything
// else means a 500" mapping.
//
// Originally split across util.go (jsonMarshalIndent), the bottom of
// persona_details.go (errResult/jsonResult — called from every domain
// file's own MCP tool registrations, never actually persona-specific),
// http.go (writeJSON), and crawl_runs.go (orNull). Collected here, once,
// when each of those domains became its own package: they were already
// shared by everything, so none of them belonged to any single domain.
package shared

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// JSONMarshalIndent renders v as indented JSON text — the one shape
// JSONResult below hands back to an MCP caller.
func JSONMarshalIndent(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ErrResult is the MCP failure result every tool registration in this
// tool returns for a caller mistake — a plain text message with
// IsError set, never a transport-level Go error (an MCP tool reports
// its own domain failures in-band, in its result).
func ErrResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: true}
}

// JSONResult is the MCP success result for anything with real
// structure to return — v rendered as indented JSON text.
func JSONResult(v any) (*mcp.CallToolResult, any, error) {
	text, err := JSONMarshalIndent(v)
	if err != nil {
		return ErrResult("failed to format result: " + err.Error()), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

// WriteJSON is the plain JSON response writer every HTTP handler in
// this tool uses — deliberately not the core app's own
// response.SuccessResponse envelope, matching what this tool's own
// frontend already expects.
func WriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// WriteKnownError is the one shared shape behind every domain's own
// write*AwareError function: a specific, recognized sentinel (an
// unknown persona/profile/company/recruiter/portal id) is a caller
// mistake and gets a 400 with notFoundMsg; anything else is a real
// server failure and gets a 500 naming the action that failed. Each
// domain package wraps this around its own sentinel rather than
// re-implementing the mapping — the reasoning originally written on
// persona_details.go's own personaAwareErrResult, now applied once
// here for the HTTP side.
func WriteKnownError(w http.ResponseWriter, err error, known error, notFoundMsg, action string) {
	if errors.Is(err, known) {
		http.Error(w, notFoundMsg, http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

// OrNull turns a *string into a value database/sql writes as either
// the string or NULL — explicit rather than relying on database/sql's
// own pointer-dereferencing conversion, matching this codebase's own
// established preference for explicit nil handling over implicit
// driver behavior. Used by both crawl_runs.go's own finishCrawlRun
// (crawl) and job_match.go's own updateJobMatchStatus (jobs), which is
// why it lives here rather than in either.
func OrNull(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
