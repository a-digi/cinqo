// Package manifest parses and validates a tool package's manifest.json
// — see plan/ai/tools/step-02-manifest-and-safe-zip-extraction.md.
// Deliberately free of any DB or filesystem dependency: everything this
// package can't determine on its own (existing scopes owned by other
// tools, which on-disk files the package actually contains, the
// currently running app's own version) is passed in by the caller
// (step 3's install handler) via ValidationInput.
package manifest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Manifest struct {
	Slug           string      `json:"slug"`
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	MinAppVersion  string      `json:"min_app_version,omitempty"`
	MaxAppVersion  string      `json:"max_app_version,omitempty"`
	Scopes         []ScopeDecl `json:"scopes,omitempty"`
	Routes         []RouteDecl `json:"routes,omitempty"`
	RequiredScopes []string    `json:"required_scopes,omitempty"`
}

type ScopeDecl struct {
	Scope       string `json:"scope"`
	Description string `json:"description"`
}

type RouteDecl struct {
	Method        string `json:"method"`
	PathSuffix    string `json:"path_suffix"`
	RequiredScope string `json:"required_scope"`
}

var (
	slugPattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// Parse reads manifest.json's raw bytes. Kind is deliberately not a
// field on Manifest at all — it's derived in Validate from which
// on-disk files the package actually contains, never trusted from the
// package's own self-description.
func Parse(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest.json: %w", err)
	}
	return m, nil
}

// ValidationInput bundles everything Validate needs beyond the
// manifest's own bytes.
type ValidationInput struct {
	HasFrontendBundle    bool
	HasBackendExecutable bool
	// ExistingScopes holds every scope string already owned by a
	// different, already-installed tool — the pre-write half of the
	// global-uniqueness guarantee the migration's unique index
	// backstops (see step 1).
	ExistingScopes map[string]struct{}
	// CurrentAppVersion is whichever version file the binary running
	// this check actually reads — api/VERSION for the plain backend,
	// app/VERSION for cinqo-app — the caller's choice, not this
	// package's (see step 2's "Open questions").
	CurrentAppVersion string
}

// Validate checks every rule in step 2's "Validation, in order" list
// (points 5-9 — points 1-4 are sandbox.ExtractZip's and the caller's
// concern) and returns the derived kind on success.
func Validate(m Manifest, in ValidationInput) (kind string, err error) {
	if !slugPattern.MatchString(m.Slug) {
		return "", fmt.Errorf("slug %q must match %s", m.Slug, slugPattern.String())
	}
	if strings.TrimSpace(m.Name) == "" {
		return "", fmt.Errorf("name is required")
	}
	if !semverPattern.MatchString(m.Version) {
		return "", fmt.Errorf("version %q must be a valid semver (e.g. 1.0.0)", m.Version)
	}

	scopePrefix := fmt.Sprintf("tool:%s:", m.Slug)
	for _, s := range m.Scopes {
		if !strings.HasPrefix(s.Scope, scopePrefix) {
			return "", fmt.Errorf("scope %q must be prefixed %q", s.Scope, scopePrefix)
		}
		if _, exists := in.ExistingScopes[s.Scope]; exists {
			return "", fmt.Errorf("scope %q is already declared by another installed tool", s.Scope)
		}
	}

	if m.MinAppVersion != "" {
		cmp, err := CompareVersions(in.CurrentAppVersion, m.MinAppVersion)
		if err != nil {
			return "", err
		}
		if cmp < 0 {
			return "", fmt.Errorf("requires app version >= %s, running %s", m.MinAppVersion, in.CurrentAppVersion)
		}
	}
	if m.MaxAppVersion != "" {
		cmp, err := CompareVersions(in.CurrentAppVersion, m.MaxAppVersion)
		if err != nil {
			return "", err
		}
		if cmp > 0 {
			return "", fmt.Errorf("requires app version <= %s, running %s", m.MaxAppVersion, in.CurrentAppVersion)
		}
	}

	switch {
	case in.HasFrontendBundle && in.HasBackendExecutable:
		return "frontend_and_backend", nil
	case in.HasFrontendBundle:
		return "frontend_only", nil
	case in.HasBackendExecutable:
		return "backend_only", nil
	default:
		return "", fmt.Errorf("package must contain frontend/bundle.js and/or backend/tool")
	}
}

// CompareVersions compares two dot-separated, all-numeric version
// strings (e.g. "0.0.12"), returning -1/0/1. Missing trailing
// components compare as 0 (e.g. "1.2" == "1.2.0").
func CompareVersions(a, b string) (int, error) {
	as, err := splitVersion(a)
	if err != nil {
		return 0, err
	}
	bs, err := splitVersion(b)
	if err != nil {
		return 0, err
	}

	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv int
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av != bv {
			if av < bv {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

func splitVersion(v string) ([]int, error) {
	parts := strings.Split(v, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid version segment %q in %q", p, v)
		}
		out[i] = n
	}
	return out, nil
}
