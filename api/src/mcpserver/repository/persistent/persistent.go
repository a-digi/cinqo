// Package persistent writes the MCP module's tables.
package persistent

import (
	"database/sql"
	"encoding/json"

	"github.com/a-digi/cinqo/src/mcpserver/entity"
)

// Repo writes mcp_settings and mcp_tokens.
type Repo struct{ db *sql.DB }

func New(db *sql.DB) *Repo { return &Repo{db: db} }

// SetEnabled switches the server on or off.
func (r *Repo) SetEnabled(enabled bool, now string) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := r.db.Exec(`INSERT INTO mcp_settings (id, enabled, updated_at) VALUES (1, ?, ?) ON CONFLICT(id) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`, v, now)
	return err
}

// InsertToken stores a token (its hash and prefix, never its value).
func (r *Repo) InsertToken(t entity.Token) error {
	scopes, err := json.Marshal(t.Scopes)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(
		`INSERT INTO mcp_tokens (id, user_id, name, token_hash, token_prefix, scopes, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.UserID, t.Name, t.Hash, t.Prefix, string(scopes), t.CreatedAt, nullable(t.ExpiresAt),
	)
	return err
}

// Revoke marks a token revoked (it is kept, for the list and the log); sql.ErrNoRows when there is no such token. Revoking twice keeps the first time.
func (r *Repo) Revoke(id, now string) error {
	res, err := r.db.Exec(`UPDATE mcp_tokens SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Touch records that a token was used.
func (r *Repo) Touch(id, now string) error {
	_, err := r.db.Exec(`UPDATE mcp_tokens SET last_used_at = ? WHERE id = ?`, now, id)
	return err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
