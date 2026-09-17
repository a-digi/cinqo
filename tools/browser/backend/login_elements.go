// login_elements.go implements the find-login-elements feature — a
// read-only, deterministic DOM search for probable login form
// elements on whatever page the shared session currently has loaded.
// Never navigates anywhere itself and never submits anything — see
// plan/ai/tools/browser/step-04-find-login-elements-feature.md.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"browser-tool-backend/crawler"
	"browser-tool-backend/shared"
)

//go:embed loginheuristic.js
var loginHeuristicJS string

// findLoginElementsTimeout bounds the DOM search itself — generous
// even though the search is fast, since it still has to wait its turn
// on shared.Mu behind whatever else the shared session is doing.
const findLoginElementsTimeout = 10 * time.Second

type loginElementCandidate struct {
	UsernameSelector string `json:"usernameSelector"`
	PasswordSelector string `json:"passwordSelector"`
	SubmitSelector   string `json:"submitSelector"`
	FormSelector     string `json:"formSelector"`
	Confidence       string `json:"confidence"`
}

type findLoginElementsResponse struct {
	Found      bool                    `json:"found"`
	Candidates []loginElementCandidate `json:"candidates"`
}

// findLoginElementsHandler handles POST /find-login-elements — the
// --mcp adapter's own real target for find_login_elements.
func findLoginElementsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result, err := findLoginElements()
	if err != nil {
		http.Error(w, fmt.Sprintf("find-login-elements failed: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// findLoginElements runs the heuristic against whatever page the
// shared session currently has loaded — never navigates, never
// submits anything. Holds shared.Mu for the whole operation, same as
// crawlPage.
func findLoginElements() (findLoginElementsResponse, error) {
	if err := shared.EnsureSharedSession(); err != nil {
		return findLoginElementsResponse{}, err
	}
	shared.Mu.Lock()
	defer shared.Mu.Unlock()

	// step 47.2/47.3 — fail fast on a tab left wedged by a previous,
	// unrelated caller instead of discovering it only after burning
	// findLoginElementsTimeout on an evaluation that was never going to
	// complete, and replace the wedged tab immediately (still holding
	// shared.Mu) so the NEXT caller gets a fresh, healthy session
	// instead of inheriting the same wedge.
	if err := shared.ProbeSessionLiveness(shared.Ctx); err != nil {
		recreateErr := shared.RecreateSharedSessionLocked()
		return findLoginElementsResponse{}, crawler.NewSessionWedgedError(err, recreateErr)
	}

	ctx, cancel := context.WithTimeout(shared.Ctx, findLoginElementsTimeout)
	defer cancel()

	var candidates []loginElementCandidate
	if err := chromedp.Run(ctx, chromedp.Evaluate(loginHeuristicJS, &candidates)); err != nil {
		return findLoginElementsResponse{}, err
	}
	if candidates == nil {
		candidates = []loginElementCandidate{}
	}

	return findLoginElementsResponse{Found: len(candidates) > 0, Candidates: candidates}, nil
}

type findLoginElementsArgs struct{}

// registerFindLoginElements adds the find_login_elements MCP tool —
// thin, like fetch_page_html: only ever calls shared.CallSibling and formats
// the result.
func registerFindLoginElements(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "find_login_elements",
		Description: "Inspect the currently loaded page (see fetch_page_html) for probable login form fields — username, password, and submit selectors. Read-only; submits nothing.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args findLoginElementsArgs) (*mcp.CallToolResult, any, error) {
		respBody, err := shared.CallSibling("find-login-elements", []byte("{}"))
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		var result findLoginElementsResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to parse find-login-elements response: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		if !result.Found {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "No login form fields found on the currently loaded page."}}}, nil, nil
		}

		text, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to format result: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}}, nil, nil
	})
}
