// allowlist.go is the domain-allowlist safeguard for the login
// feature — resolved as the answer to step 5's own biggest open
// question: since steps 3-5 all share one browser session, a page
// visited earlier could influence the model into submitting real
// credentials against that same page (a real phishing-via-agent
// risk). login (login.go) only ever proceeds against a domain already
// present here.
//
// This tool's own small SQLite database, under TOOL_DB_DIR — never
// cinqo's own core database, and stores ONLY domain names a human
// approved, never credentials. Deliberately a pure-Go driver
// (modernc.org/sqlite, no cgo) so this tool's own cross-compilation
// story (step 6) is unaffected.
//
// The /allowlist HTTP route below is the ONLY way a domain is ever
// added — deliberately never registered as an MCP tool, so the model
// itself can never grant its own access to a new domain; only a
// signed-in human, calling this route directly through the ordinary
// tool proxy, can. See
// plan/ai/tools/browser/step-05-login-feature.md.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	allowlistDB *sql.DB
	allowlistMu sync.Mutex
)

// initAllowlistDB opens (creating if needed) browser.db under
// TOOL_DB_DIR. TOOL_DB_DIR itself is never pre-created by the host —
// same convention pdf_generator's own main() already established for
// TOOL_TMP_DIR/TOOL_UPLOADS_DIR — so this tool creates it itself.
func initAllowlistDB() error {
	dbDir := os.Getenv("TOOL_DB_DIR")
	if dbDir == "" {
		return fmt.Errorf("TOOL_DB_DIR is not set")
	}
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("failed to create TOOL_DB_DIR: %w", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(dbDir, "browser.db"))
	if err != nil {
		return fmt.Errorf("failed to open allowlist database: %w", err)
	}
	// This tool's own write volume is tiny (a human occasionally
	// approving/revoking a domain) — a single connection avoids any
	// "database is locked" surprise from modernc.org/sqlite's own
	// concurrent-writer behavior, which isn't worth tuning around here.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS allowed_domains (
		domain     TEXT PRIMARY KEY,
		allowed_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		db.Close()
		return fmt.Errorf("failed to prepare allowlist schema: %w", err)
	}

	allowlistDB = db
	return nil
}

// isDomainAllowed matches by exact hostname only, deliberately — no
// subdomain/suffix matching in this first pass. Allowing
// "example.com" does not also allow "login.example.com"; each needs
// its own entry. A real simplification, not a bug — flagged as an
// open question below.
func isDomainAllowed(host string) (bool, error) {
	allowlistMu.Lock()
	defer allowlistMu.Unlock()

	var exists int
	err := allowlistDB.QueryRow(`SELECT 1 FROM allowed_domains WHERE domain = ?`, host).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func addAllowedDomain(host string) error {
	allowlistMu.Lock()
	defer allowlistMu.Unlock()
	_, err := allowlistDB.Exec(`INSERT INTO allowed_domains (domain) VALUES (?) ON CONFLICT(domain) DO NOTHING`, host)
	return err
}

func removeAllowedDomain(host string) error {
	allowlistMu.Lock()
	defer allowlistMu.Unlock()
	_, err := allowlistDB.Exec(`DELETE FROM allowed_domains WHERE domain = ?`, host)
	return err
}

func listAllowedDomains() ([]string, error) {
	allowlistMu.Lock()
	defer allowlistMu.Unlock()

	rows, err := allowlistDB.Query(`SELECT domain FROM allowed_domains ORDER BY domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type allowlistRequest struct {
	Domain string `json:"domain"`
}

type allowlistResponse struct {
	Domains []string `json:"domains"`
}

// allowlistHandler handles GET/POST/DELETE /allowlist — see this
// file's own top comment for why this is deliberately HTTP-route-only,
// never an MCP tool.
func allowlistHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		domains, err := listAllowedDomains()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list allowlist: %v", err), http.StatusInternalServerError)
			return
		}
		writeAllowlistResponse(w, domains)

	case http.MethodPost:
		var body allowlistRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Domain == "" {
			http.Error(w, "domain is required", http.StatusBadRequest)
			return
		}
		if err := addAllowedDomain(body.Domain); err != nil {
			http.Error(w, fmt.Sprintf("failed to add domain: %v", err), http.StatusInternalServerError)
			return
		}
		domains, err := listAllowedDomains()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to reload allowlist: %v", err), http.StatusInternalServerError)
			return
		}
		writeAllowlistResponse(w, domains)

	case http.MethodDelete:
		// A query parameter, not a path segment — tool_routes matching
		// is an exact string match on path_suffix, so a dynamic
		// /allowlist/{domain} route could never match a real request
		// (same established reasoning as pdf_generator's own
		// files?id=... route).
		domain := r.URL.Query().Get("domain")
		if domain == "" {
			http.Error(w, "domain query parameter is required", http.StatusBadRequest)
			return
		}
		if err := removeAllowedDomain(domain); err != nil {
			http.Error(w, fmt.Sprintf("failed to remove domain: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeAllowlistResponse(w http.ResponseWriter, domains []string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(allowlistResponse{Domains: domains})
}
