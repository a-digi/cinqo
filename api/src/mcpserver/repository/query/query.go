// Package query reads the MCP module's tables.
package query

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/a-digi/cinqo/src/mcpserver/entity"
)

// Repo reads mcp_settings and mcp_tokens.
type Repo struct{ db *sql.DB }

func New(db *sql.DB) *Repo { return &Repo{db: db} }

// Settings returns the one settings row (off when the row is missing).
func (r *Repo) Settings() (entity.Settings, error) {
	var enabled int
	var updated sql.NullString
	err := r.db.QueryRow(`SELECT enabled, updated_at FROM mcp_settings WHERE id = 1`).Scan(&enabled, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return entity.Settings{}, nil
	}
	if err != nil {
		return entity.Settings{}, err
	}
	return entity.Settings{Enabled: enabled != 0, UpdatedAt: updated.String}, nil
}

const tokenColumns = `id, user_id, name, token_hash, token_prefix, scopes, created_at, expires_at, last_used_at, revoked_at`

func scan(row interface{ Scan(...any) error }) (entity.Token, error) {
	var t entity.Token
	var scopes string
	var expires, used, revoked sql.NullString
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Hash, &t.Prefix, &scopes, &t.CreatedAt, &expires, &used, &revoked); err != nil {
		return entity.Token{}, err
	}
	if err := json.Unmarshal([]byte(scopes), &t.Scopes); err != nil {
		return entity.Token{}, err
	}
	t.ExpiresAt, t.LastUsedAt, t.RevokedAt = expires.String, used.String, revoked.String
	return t, nil
}

// TokenByHash finds a token by the hash of its value; sql.ErrNoRows when there is none.
func (r *Repo) TokenByHash(hash string) (entity.Token, error) {
	return scan(r.db.QueryRow(`SELECT `+tokenColumns+` FROM mcp_tokens WHERE token_hash = ?`, hash))
}

// TokenByID finds a token by its id; sql.ErrNoRows when there is none.
func (r *Repo) TokenByID(id string) (entity.Token, error) {
	return scan(r.db.QueryRow(`SELECT `+tokenColumns+` FROM mcp_tokens WHERE id = ?`, id))
}

// Tokens lists every token, newest first (the people who may manage the server see them all).
func (r *Repo) Tokens() ([]entity.Token, error) {
	rows, err := r.db.Query(`SELECT ` + tokenColumns + ` FROM mcp_tokens ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.Token{}
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ActiveCountForUser counts the tokens of a user that are neither revoked nor expired at now (RFC 3339 UTC).
func (r *Repo) ActiveCountForUser(userID, now string) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM mcp_tokens WHERE user_id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)`, userID, now).Scan(&n)
	return n, err
}
