// Package entity holds the organisation types. One organisation per user (decision 2026-11-15); see plan/ai/tools/namako/saas/s1-core-organisations.md.
package entity

// DefaultID is the organisation every existing install is, created by the migration.
const DefaultID = "default"

// Statuses of an organisation. A suspended organisation's members get no tool access.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
)

// MaxNameLength is the longest organisation name.
const MaxNameLength = 120

type Organisation struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Member is one user of an organisation (the identity provider's id; core stores no other user data).
type Member struct {
	UserID    string `json:"userId"`
	OrgID     string `json:"orgId"`
	CreatedAt string `json:"createdAt"`
}
