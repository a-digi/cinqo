// login_credentials.go is the human-only credential store the login
// feature (login.go) resolves against. A human registers a domain's
// real username/password here, through a plain HTTP route — never an
// MCP tool, so the AI's own tool-calling loop can never reach it, let
// alone add or read a credential itself. This is the structural
// guarantee the whole step exists for: "the AI agent will NEVER
// access the real login data." See
// plan/ai/tools/browser/step-07-login-profiles-and-credential-isolation.md.
//
// Supersedes allowlist.go (step 5) — a stored credential's own
// existence for a domain now IS the login approval; there is no
// separate allow/deny list to keep in sync with it.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"

	"browser-tool-backend/shared"
)

var (
	browserMu sync.Mutex
	cryptoKey []byte
)

// initCryptoKey loads (or generates, on first run) this tool's own
// login-credential encryption key file under TOOL_DB_DIR — separate
// from shared.InitDB (which owns browser.db/schema only) since key
// material is this feature's own concern, not the shared DB's. TOOL_DB_DIR
// itself is not assumed to already exist — same convention every other
// TOOL_DB_DIR consumer in this tool follows.
func initCryptoKey() error {
	dbDir := os.Getenv("TOOL_DB_DIR")
	if dbDir == "" {
		return fmt.Errorf("TOOL_DB_DIR is not set")
	}
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("failed to create TOOL_DB_DIR: %w", err)
	}

	key, err := loadOrGenerateCryptoKey(filepath.Join(dbDir, "credentials.key"))
	if err != nil {
		return err
	}
	cryptoKey = key
	return nil
}

// storedCredential is what login.go actually needs to perform a
// login — the decrypted password, resolved fresh right before use,
// never held longer than that.
type storedCredential struct {
	Username string
	Password string
}

// lookupCredential returns (nil, nil) — not an error — when no
// credential is registered for domain, so callers can distinguish
// "not configured yet" from a real lookup/decryption failure.
func lookupCredential(domain string) (*storedCredential, error) {
	browserMu.Lock()
	defer browserMu.Unlock()

	var username, encryptedPassword string
	err := shared.DB.QueryRow(`SELECT username, encrypted_password FROM login_credentials WHERE domain = ?`, domain).
		Scan(&username, &encryptedPassword)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	password, err := decryptSecret(encryptedPassword, cryptoKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt stored credential: %w", err)
	}
	return &storedCredential{Username: username, Password: password}, nil
}

func upsertCredential(domain, username, password string) error {
	encryptedPassword, err := encryptSecret(password, cryptoKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt credential: %w", err)
	}

	browserMu.Lock()
	defer browserMu.Unlock()

	_, err = shared.DB.Exec(`
		INSERT INTO login_credentials (domain, username, encrypted_password)
		VALUES (?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			username = excluded.username,
			encrypted_password = excluded.encrypted_password,
			updated_at = datetime('now')`,
		domain, username, encryptedPassword)
	return err
}

func removeCredential(domain string) error {
	browserMu.Lock()
	defer browserMu.Unlock()
	_, err := shared.DB.Exec(`DELETE FROM login_credentials WHERE domain = ?`, domain)
	return err
}

// credentialSummary is what a human sees back — the real username
// masked, the password never present in any form. See this file's own
// top comment: no route in this design ever re-returns a stored
// password, full stop.
type credentialSummary struct {
	Domain         string `json:"domain" yaml:"domain"`
	MaskedUsername string `json:"username" yaml:"username"`
}

func listCredentialSummaries() ([]credentialSummary, error) {
	browserMu.Lock()
	defer browserMu.Unlock()

	rows, err := shared.DB.Query(`SELECT domain, username FROM login_credentials ORDER BY domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []credentialSummary{}
	for rows.Next() {
		var domain, username string
		if err := rows.Scan(&domain, &username); err != nil {
			return nil, err
		}
		out = append(out, credentialSummary{Domain: domain, MaskedUsername: maskUsername(username)})
	}
	return out, rows.Err()
}

// maxCredentialBodyBytes bounds the raw YAML body before it's even
// parsed — matches the tool-install pipeline's own
// maxUploadedPackageBytes precedent (api/src/tool/handler/install_handler.go)
// of capping a body before buffering/parsing it.
const maxCredentialBodyBytes = 64 * 1024

type loginCredentialEntry struct {
	Domain   string `yaml:"domain"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type loginCredentialsYAML struct {
	LoginCredentials []loginCredentialEntry `yaml:"login_credentials"`
}

type credentialsListResponse struct {
	Credentials []credentialSummary `json:"credentials"`
}

// loginCredentialsHandler handles GET/POST/DELETE /login-credentials
// — see this file's own top comment for why this is deliberately
// HTTP-route-only, never an MCP tool.
func loginCredentialsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		summaries, err := listCredentialSummaries()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list credentials: %v", err), http.StatusInternalServerError)
			return
		}
		writeCredentialsResponse(w, summaries)

	case http.MethodPost:
		body, err := io.ReadAll(io.LimitReader(r.Body, maxCredentialBodyBytes+1))
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		if len(body) > maxCredentialBodyBytes {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		var parsed loginCredentialsYAML
		if err := yaml.Unmarshal(body, &parsed); err != nil {
			http.Error(w, fmt.Sprintf("invalid YAML: %v", err), http.StatusBadRequest)
			return
		}
		if len(parsed.LoginCredentials) == 0 {
			http.Error(w, "login_credentials must contain at least one entry", http.StatusBadRequest)
			return
		}
		for _, entry := range parsed.LoginCredentials {
			if entry.Domain == "" || entry.Username == "" || entry.Password == "" {
				http.Error(w, "each entry requires domain, username, and password", http.StatusBadRequest)
				return
			}
			if err := upsertCredential(entry.Domain, entry.Username, entry.Password); err != nil {
				http.Error(w, fmt.Sprintf("failed to store credential for %q: %v", entry.Domain, err), http.StatusInternalServerError)
				return
			}
		}

		summaries, err := listCredentialSummaries()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to reload credentials: %v", err), http.StatusInternalServerError)
			return
		}
		writeCredentialsResponse(w, summaries)

	case http.MethodDelete:
		// A query parameter, not a path segment — tool_routes matching
		// is an exact string match on path_suffix, so a dynamic
		// /login-credentials/{domain} route could never match a real
		// request (same established reasoning as pdf_generator's own
		// files?id=... route, and step 5's own /allowlist).
		domain := r.URL.Query().Get("domain")
		if domain == "" {
			http.Error(w, "domain query parameter is required", http.StatusBadRequest)
			return
		}
		if err := removeCredential(domain); err != nil {
			http.Error(w, fmt.Sprintf("failed to remove credential: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeCredentialsResponse(w http.ResponseWriter, summaries []credentialSummary) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(credentialsListResponse{Credentials: summaries})
}

// normalizeDomain reduces a realistic model failure mode — passing a
// full URL (e.g. "https://example.com/login") instead of a bare
// hostname — without adding real complexity. Falls back to the raw,
// trimmed input whenever it isn't a parseable absolute URL. See
// plan/ai/tools/browser/step-11-ai-facing-credential-existence-check.md.
func normalizeDomain(raw string) string {
	raw = strings.TrimSpace(raw)
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Hostname() != "" {
		return u.Hostname()
	}
	return raw
}

// hasCredentialRequest/Response carry exactly one bit of information
// in either direction — no credential, username, or even masked
// username ever crosses this boundary. This is the first MCP-exposed
// surface that touches login_credentials at all; see this file's own
// top comment and step 11's own "Security considerations" for why
// that's a deliberate, narrow exception, not an oversight.
type hasCredentialRequest struct {
	Domain string `json:"domain"`
}

type hasCredentialResponse struct {
	Exists bool `json:"exists"`
}

// hasCredentialHandler handles POST /has-login-credential — the
// --mcp adapter's own real target for has_login_credential. Never
// reads cred.Username/cred.Password beyond the nil check itself.
func hasCredentialHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body hasCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Domain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}

	cred, err := lookupCredential(normalizeDomain(body.Domain))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to look up credential: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(hasCredentialResponse{Exists: cred != nil})
}

type hasCredentialArgs struct {
	Domain string `json:"domain" jsonschema:"the domain to check — a bare hostname (e.g. example.com), not a full URL"`
}

// registerHasLoginCredential adds the has_login_credential MCP tool —
// the only credential-adjacent action the AI can take that isn't
// gated behind a human having already registered one. Deliberately
// scoped narrower than tool:browser:login (see manifest.json's own
// scope description for tool:browser:login:status) — this tool can
// never fill/submit anything or leak a credential, only report
// whether one exists.
func registerHasLoginCredential(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "has_login_credential",
		Description: "Check whether a login credential is already stored for a domain. Returns yes/no only — never the credential itself. If no credential is stored, ask the user to add one on the Browser tool's own \"Login Credentials\" page; the AI cannot create or see this data itself.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args hasCredentialArgs) (*mcp.CallToolResult, any, error) {
		if args.Domain == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "domain is required"}},
				IsError: true,
			}, nil, nil
		}

		reqBody, err := json.Marshal(hasCredentialRequest{Domain: args.Domain})
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to build request: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		respBody, err := shared.CallSibling("has-login-credential", reqBody)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		var result hasCredentialResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to parse response: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		domain := normalizeDomain(args.Domain)
		var text string
		if result.Exists {
			text = fmt.Sprintf(
				"A login credential is already stored for %q. Proceed with fetch_page_html → find_login_elements → login.",
				domain,
			)
		} else {
			text = fmt.Sprintf(
				"No login credential is stored for %q. Ask the user to add one on the Browser tool's own \"Login Credentials\" page (Tools → Login Credentials) — the AI cannot create or see this data itself.",
				domain,
			)
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	})
}
