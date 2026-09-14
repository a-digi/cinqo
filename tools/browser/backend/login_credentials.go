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
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	_ "modernc.org/sqlite"
)

var (
	browserDB *sql.DB
	browserMu sync.Mutex
	cryptoKey []byte
)

// initBrowserDB opens (creating if needed) browser.db and this tool's
// own encryption key file, both under TOOL_DB_DIR — never cinqo's own
// core database. TOOL_DB_DIR itself is never pre-created by the host
// — same convention pdf_generator's own main() already established
// for TOOL_TMP_DIR/TOOL_UPLOADS_DIR — so this tool creates it itself.
func initBrowserDB() error {
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

	db, err := sql.Open("sqlite", filepath.Join(dbDir, "browser.db"))
	if err != nil {
		return fmt.Errorf("failed to open browser database: %w", err)
	}
	// This tool's own write volume is tiny (a human occasionally
	// registering/revoking a credential) — a single connection avoids
	// any "database is locked" surprise from modernc.org/sqlite's own
	// concurrent-writer behavior, which isn't worth tuning around here.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS login_credentials (
		domain             TEXT PRIMARY KEY,
		username           TEXT NOT NULL,
		encrypted_password TEXT NOT NULL,
		created_at         TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at         TEXT
	)`); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare login_credentials schema: %w", err)
	}

	browserDB = db
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
	err := browserDB.QueryRow(`SELECT username, encrypted_password FROM login_credentials WHERE domain = ?`, domain).
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

	_, err = browserDB.Exec(`
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
	_, err := browserDB.Exec(`DELETE FROM login_credentials WHERE domain = ?`, domain)
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

	rows, err := browserDB.Query(`SELECT domain, username FROM login_credentials ORDER BY domain`)
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
