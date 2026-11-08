// Package entity holds the plain data shapes the scope registry works
// with. Not persisted — see plan/ai/security/security.md's "Data
// model" section for why: these are static, developer-authored
// declarations (the same nature as a route's own declared scope), not
// user data, so there is no table and no migration for them.
package entity

// Scope is a single permission identifier this application (or a
// plugin) defines, with a human-readable description for whoever is
// assigning it later (an admin in coco-iam, a developer reading this
// registry's own admin page).
type Scope struct {
	ID          string
	Description string
}

// ScopeGroup clusters related scopes under one namespace (e.g.
// "cinqo:ping"). Mirrors the shape api/config/scopes/*.json already
// used before this package replaced it as the source of truth.
type ScopeGroup struct {
	ID          string
	Description string
	Scopes      []Scope
}
