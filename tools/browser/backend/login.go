// login.go implements the login feature — fills and submits a login
// form in the shared browser session. The one credential-submitting
// action in this tool, gated both by scope (tool:browser:login, step
// 1) and by allowlist.go's own domain allowlist. See
// plan/ai/tools/browser/step-05-login-feature.md.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// loginTimeout bounds one fill+submit+settle cycle.
const loginTimeout = 20 * time.Second

type loginRequest struct {
	UsernameSelector string `json:"usernameSelector"`
	PasswordSelector string `json:"passwordSelector"`
	SubmitSelector   string `json:"submitSelector"`
	Username         string `json:"username"`
	Password         string `json:"password"`
}

// loginResponse's Success only ever means "the form was mechanically
// filled and submitted" — NOT that the login itself succeeded on the
// target site (a failed login often just re-renders the same form
// with an error message, still a normal page load). That judgment is
// left to whoever reads FinalURL/HTML — see registerLogin's own
// prompt text below. Reason is set instead of Success/FinalURL/HTML
// when the allowlist check itself is what stopped this from
// proceeding at all.
type loginResponse struct {
	Success  bool   `json:"success"`
	Reason   string `json:"reason,omitempty"`
	FinalURL string `json:"finalUrl,omitempty"`
	HTML     string `json:"html,omitempty"`
}

// loginHandler handles POST /login — the --mcp adapter's own real
// target for the login MCP tool.
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body loginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.UsernameSelector == "" || body.PasswordSelector == "" || body.SubmitSelector == "" || body.Username == "" || body.Password == "" {
		http.Error(w, "usernameSelector, passwordSelector, submitSelector, username, and password are all required", http.StatusBadRequest)
		return
	}

	result, err := performLogin(body)
	if err != nil {
		// Never include body (credentials) in the error — this branch
		// only ever reaches a chromedp/plumbing error, never anything
		// derived from the request payload itself.
		http.Error(w, fmt.Sprintf("login failed: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// performLogin checks the shared session's CURRENT page origin against
// the allowlist before touching anything — it never navigates itself,
// so "current page" is exactly whatever fetch_page_html (step 3) last
// loaded. Holds sessionMu for the whole operation, same as
// crawlPage/findLoginElements.
func performLogin(req loginRequest) (loginResponse, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	ctx, cancel := context.WithTimeout(sessionCtx, loginTimeout)
	defer cancel()

	var currentURL string
	if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err != nil {
		return loginResponse{}, err
	}

	u, err := url.Parse(currentURL)
	if err != nil {
		return loginResponse{}, fmt.Errorf("could not parse the current page's URL: %w", err)
	}

	allowed, err := isDomainAllowed(u.Hostname())
	if err != nil {
		return loginResponse{}, fmt.Errorf("could not check the domain allowlist: %w", err)
	}
	if !allowed {
		return loginResponse{
			Success: false,
			Reason: fmt.Sprintf(
				"%q is not in the approved domains list — ask the user to add it first (POST /allowlist on this tool, outside of any AI tool call), then retry",
				u.Hostname(),
			),
		}, nil
	}

	if err := chromedp.Run(ctx,
		chromedp.SendKeys(req.UsernameSelector, req.Username),
		chromedp.SendKeys(req.PasswordSelector, req.Password),
	); err != nil {
		return loginResponse{Success: false, Reason: fmt.Sprintf("failed to fill the form: %v", err)}, nil
	}

	// Prefer a real form submit() over a synthetic coordinate-based
	// click — verified directly with an isolated diagnostic against a
	// real site (quotes.toscrape.com/login): chromedp.Click on the
	// submit control did NOT reliably register as a real,
	// server-accepted form submission (the page just silently
	// re-rendered /login), while chromedp.Submit (which calls the
	// enclosing form's own .submit()) correctly redirected and showed
	// the logged-in state. Falls back to a click only when there's no
	// enclosing <form> to call .submit() on — a JS-driven login widget
	// relying on its own click handler instead of native form
	// submission (the "low confidence" case from find_login_elements).
	// See plan/ai/tools/browser/step-05-login-feature.md's own
	// "Amendment".
	if err := chromedp.Run(ctx, chromedp.Submit(req.SubmitSelector)); err != nil {
		if err := chromedp.Run(ctx, chromedp.Click(req.SubmitSelector)); err != nil {
			return loginResponse{Success: false, Reason: fmt.Sprintf("failed to submit the form: %v", err)}, nil
		}
	}

	// Give the submit's own navigation/response a moment to settle —
	// same reasoning as crawlPage's own settleDelay (step 3's own
	// amendment).
	if err := chromedp.Run(ctx, chromedp.Sleep(settleDelay)); err != nil {
		return loginResponse{}, err
	}

	var html, finalURL string
	if err := chromedp.Run(ctx,
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		return loginResponse{}, err
	}
	if len(html) > maxHTMLBytes {
		html = html[:maxHTMLBytes]
	}

	return loginResponse{Success: true, FinalURL: finalURL, HTML: html}, nil
}

type loginArgs struct {
	UsernameSelector string `json:"usernameSelector" jsonschema:"CSS selector for the username/email field, from find_login_elements"`
	PasswordSelector string `json:"passwordSelector" jsonschema:"CSS selector for the password field, from find_login_elements"`
	SubmitSelector   string `json:"submitSelector" jsonschema:"CSS selector for the submit control, from find_login_elements"`
	Username         string `json:"username" jsonschema:"the username/email to submit"`
	Password         string `json:"password" jsonschema:"the password to submit"`
}

// registerLogin adds the login MCP tool — the only credential-
// submitting action in this whole tool. Deliberately does not itself
// run find_login_elements or navigate anywhere; the model is expected
// to have already done both. See this file's own top comment for the
// allowlist gate this sits behind.
func registerLogin(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "login",
		Description: "Fill and submit a login form in the shared browser session, using selectors from find_login_elements. Only proceeds if the current page's domain has already been approved by the user.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args loginArgs) (*mcp.CallToolResult, any, error) {
		if args.UsernameSelector == "" || args.PasswordSelector == "" || args.SubmitSelector == "" || args.Username == "" || args.Password == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "usernameSelector, passwordSelector, submitSelector, username, and password are all required"}},
				IsError: true,
			}, nil, nil
		}

		reqBody, err := json.Marshal(loginRequest{
			UsernameSelector: args.UsernameSelector,
			PasswordSelector: args.PasswordSelector,
			SubmitSelector:   args.SubmitSelector,
			Username:         args.Username,
			Password:         args.Password,
		})
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to build request: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		respBody, err := callSibling("login", reqBody)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		var result loginResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to parse login response: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		if !result.Success {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: result.Reason}}, IsError: true}, nil, nil
		}

		text := fmt.Sprintf(
			"The form was submitted — this does not by itself mean login succeeded. Judge that from the resulting page below (e.g. a re-rendered login form usually means it failed).\n\nURL: %s\n\n%s",
			result.FinalURL, result.HTML,
		)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	})
}
