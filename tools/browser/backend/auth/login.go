// login.go implements the login feature — fills and submits a login
// form in the shared browser session. The one credential-submitting
// action in this tool, gated by scope (tool:browser:login, step 1)
// and by the mere existence of a stored credential for the caller-
// supplied domain (step 7's login_credentials store). As of step 8,
// the MCP args carry no credential at all — the AI only ever
// instructs WHICH elements matter (via find_login_elements), never
// what to fill them with; the tool resolves the real
// username/password itself. As of the domain-normalization/YAML-
// instructions follow-up, the AI's own instruction to this tool is a
// literal YAML document (parsed server-side, never sent as raw HTML-
// form fields) — the same "AI instructs via YAML how the tool can
// login" shape from the original request, now applied to the MCP
// wire args themselves, not just the human-facing credential store.
// See plan/ai/tools/browser/step-08-ai-instructed-login.md.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"

	"browser-tool-backend/crawler"
	"browser-tool-backend/shared"
)

// loginTimeout bounds one fill+submit+settle cycle.
const loginTimeout = 20 * time.Second

type loginRequest struct {
	Domain           string `json:"domain"`
	UsernameSelector string `json:"usernameSelector"`
	PasswordSelector string `json:"passwordSelector"`
	SubmitSelector   string `json:"submitSelector"`
}

// loginResponse's Success only ever means "the form was mechanically
// filled and submitted" — NOT that the login itself succeeded on the
// target site (a failed login often just re-renders the same form
// with an error message, still a normal page load). That judgment is
// left to whoever reads FinalURL/HTML — see RegisterLogin's own
// prompt text below. Reason is set instead of Success/FinalURL/HTML
// when the credential lookup itself is what stopped this from
// proceeding at all (no stored credential for the given domain).
type loginResponse struct {
	Success  bool   `json:"success"`
	Reason   string `json:"reason,omitempty"`
	FinalURL string `json:"finalUrl,omitempty"`
	HTML     string `json:"html,omitempty"`
}

// LoginHandler handles POST /login — the --mcp adapter's own real
// target for the login MCP tool.
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body loginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Domain == "" || body.UsernameSelector == "" || body.PasswordSelector == "" || body.SubmitSelector == "" {
		http.Error(w, "domain, usernameSelector, passwordSelector, and submitSelector are all required", http.StatusBadRequest)
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

// performLogin never navigates itself — it fills/submits whatever
// login page the shared session already has loaded (via
// fetch_page_html, step 3) — and looks up the credential to use by
// the caller-supplied Domain rather than parsing the current page's
// own URL: the two are expected to agree, but keeping the lookup
// explicit removes any ambiguity if they don't (e.g. a www. prefix
// difference from how the human typed the domain when registering
// it) and makes a lookup failure's own error message unambiguous
// about which domain it checked. See this step's own "Open
// questions". Holds shared.Mu for the whole operation, same as
// crawlPage/findLoginElements.
func performLogin(req loginRequest) (loginResponse, error) {
	if err := shared.EnsureSharedSession(); err != nil {
		return loginResponse{}, err
	}
	shared.Mu.Lock()
	defer shared.Mu.Unlock()

	// step 47.2/47.3 — fail fast on a tab left wedged by a previous,
	// unrelated caller instead of discovering it only after burning
	// loginTimeout on a fill/submit that was never going to complete,
	// and replace the wedged tab immediately (still holding shared.Mu)
	// so the NEXT caller gets a fresh, healthy session instead of
	// inheriting the same wedge.
	if err := shared.ProbeSessionLiveness(shared.Ctx); err != nil {
		recreateErr := shared.RecreateSharedSessionLocked()
		return loginResponse{}, crawler.NewSessionWedgedError(err, recreateErr)
	}

	ctx, cancel := context.WithTimeout(shared.Ctx, loginTimeout)
	defer cancel()

	// normalizeDomain (login_credentials.go, built for
	// has_login_credential) applied here too — the retrofit flagged in
	// step 11's own open question 3: reduces the same realistic model
	// failure mode (a full URL instead of a bare hostname) for the
	// login path itself, not just the existence check.
	domain := normalizeDomain(req.Domain)

	// A stored credential's own existence for this domain IS the login
	// approval — no separate allowlist (step 7 retired allowlist.go
	// entirely). The MCP schema itself carries no username/password
	// field at all as of this step — cred.Username/cred.Password below
	// are the ONLY values ever used to fill the form.
	cred, err := lookupCredential(domain)
	if err != nil {
		return loginResponse{}, fmt.Errorf("could not look up the stored credential: %w", err)
	}
	if cred == nil {
		return loginResponse{
			Success: false,
			Reason: fmt.Sprintf(
				"no stored credential for %q — ask the user to add one via POST /login-credentials on this tool (outside of any AI tool call), then retry",
				domain,
			),
		}, nil
	}

	if err := chromedp.Run(ctx,
		chromedp.SendKeys(req.UsernameSelector, cred.Username),
		chromedp.SendKeys(req.PasswordSelector, cred.Password),
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
	// same reasoning as crawlPage's own crawler.SettleDelay (step 3's own
	// amendment).
	if err := chromedp.Run(ctx, chromedp.Sleep(crawler.SettleDelay)); err != nil {
		return loginResponse{}, err
	}

	// Same unconditional <head>/<script>/<style> removal as crawl.go's
	// own readCrawlResponse (step 44) — this HTML reaches the AI model
	// exactly like fetch_page_html's own does (see RegisterLogin's own
	// handler, below), so it gets the same token-reduction treatment.
	// No caller-configurable removeSelectors here — nothing asked for
	// one, and loginRequest carries no such field.
	removeScript, err := crawler.RemoveElementsJS(crawler.DefaultRemoveSelectors)
	if err != nil {
		return loginResponse{}, err
	}

	var html, finalURL string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(removeScript, nil),
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		return loginResponse{}, err
	}
	if len(html) > crawler.MaxHTMLBytes {
		html = html[:crawler.MaxHTMLBytes]
	}

	return loginResponse{Success: true, FinalURL: finalURL, HTML: html}, nil
}

// loginInstructions is the YAML shape the AI itself writes as the
// login tool's own single argument — literally "instruct via YAML how
// the tool can login," the same wording the whole browser-tool plan
// started from, now applied directly to this tool's own MCP wire
// shape rather than only the human-facing credential store (step 7).
// Deliberately has no username/password field to bind to at all: even
// if a model's own YAML text includes one (nothing stops it from
// trying), gopkg.in/yaml.v3 silently ignores unknown keys by default
// — the exact same structural guarantee step 8 already established
// for the old typed-JSON-fields shape, carried forward unchanged for
// this one.
type loginInstructions struct {
	Domain           string `yaml:"domain"`
	UsernameSelector string `yaml:"usernameSelector"`
	PasswordSelector string `yaml:"passwordSelector"`
	SubmitSelector   string `yaml:"submitSelector"`
}

type loginArgs struct {
	Instructions string `json:"instructions" jsonschema:"a YAML document describing how to log in — domain, usernameSelector, passwordSelector, submitSelector (the selectors come from find_login_elements). Example:\ndomain: example.com\nusernameSelector: \"#username\"\npasswordSelector: \"#password\"\nsubmitSelector: \"#submit\"\nNever include a username or password here — this tool resolves the real credential itself; any such fields would be silently ignored."`
}

// RegisterLogin adds the login MCP tool — the only credential-
// submitting action in this whole tool, and, as of step 8, one whose
// args have no field capable of carrying a credential at all: the
// model only ever instructs WHICH elements matter, never what to fill
// them with. Deliberately does not itself run find_login_elements or
// navigate anywhere; the model is expected to have already done both.
func RegisterLogin(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "login",
		Description: "Log into the currently loaded page, instructed via a YAML document (domain + selectors from find_login_elements). Only proceeds if a credential has already been registered for the given domain (via /login-credentials, outside any AI tool call) — the tool fills and submits it itself; the AI never sees the credential.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args loginArgs) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(args.Instructions) == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "instructions is required — a YAML document with domain, usernameSelector, passwordSelector, and submitSelector"}},
				IsError: true,
			}, nil, nil
		}

		var parsed loginInstructions
		if err := yaml.Unmarshal([]byte(args.Instructions), &parsed); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid YAML instructions: %v", err)}},
				IsError: true,
			}, nil, nil
		}
		if parsed.Domain == "" || parsed.UsernameSelector == "" || parsed.PasswordSelector == "" || parsed.SubmitSelector == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "instructions must include domain, usernameSelector, passwordSelector, and submitSelector"}},
				IsError: true,
			}, nil, nil
		}

		reqBody, err := json.Marshal(loginRequest{
			Domain:           parsed.Domain,
			UsernameSelector: parsed.UsernameSelector,
			PasswordSelector: parsed.PasswordSelector,
			SubmitSelector:   parsed.SubmitSelector,
		})
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to build request: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		respBody, err := shared.CallSibling("login", reqBody)
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
