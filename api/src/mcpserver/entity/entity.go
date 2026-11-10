// Package entity holds the records of the MCP server module.
package entity

// Settings is the one row of mcp_settings: the server is off until a person switches it on.
type Settings struct {
	Enabled   bool
	UpdatedAt string
}

// Token is one MCP access token. The token itself is never stored: only its SHA-256 hash (for the lookup) and a short visible prefix (for the list).
type Token struct {
	ID         string
	UserID     string
	Name       string
	Hash       string
	Prefix     string
	Scopes     []string
	CreatedAt  string
	ExpiresAt  string // "" = never
	LastUsedAt string // "" = never used
	RevokedAt  string // "" = active
}
