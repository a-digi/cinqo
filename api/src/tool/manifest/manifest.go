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
	// MCP declares that this tool's own backend executable also speaks
	// MCP over stdio when invoked with --mcp — deliberately the
	// opposite of how Kind is trusted: this is a self-declared
	// optimization gate on whether the install pipeline even attempts
	// MCP discovery, not the source of truth itself (confirming MCP
	// support means actually spawning the binary and completing a real
	// handshake — a tool that declares this but fails discovery simply
	// ends up with no cached MCP tools, non-fatal). See
	// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md.
	MCP bool `json:"mcp,omitempty"`
	// MCPTools declares which scope gates calling each of this tool's
	// own MCP tools by name — MCP's own tools/list has no concept of a
	// cinqo scope at all, so this mapping can only come from the
	// manifest, not from live discovery. A discovered MCP tool name
	// with no matching entry here is never cached/offered to a model —
	// see plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md's
	// own "Offering tools to the model" design.
	MCPTools []MCPToolDecl `json:"mcp_tools,omitempty"`
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

type MCPToolDecl struct {
	Name          string `json:"name"`
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

	if m.MCP && !in.HasBackendExecutable {
		return "", fmt.Errorf("mcp: true requires the package to include a backend executable")
	}
	if len(m.MCPTools) > 0 && !m.MCP {
		return "", fmt.Errorf("mcp_tools requires mcp: true")
	}
	declaredScopes := make(map[string]struct{}, len(m.Scopes))
	for _, s := range m.Scopes {
		declaredScopes[s.Scope] = struct{}{}
	}
	for _, mt := range m.MCPTools {
		if strings.TrimSpace(mt.Name) == "" {
			return "", fmt.Errorf("mcp_tools entries require a name")
		}
		if _, ok := declaredScopes[mt.RequiredScope]; !ok {
			return "", fmt.Errorf("mcp_tools entry %q: required_scope %q must be one of this tool's own declared scopes", mt.Name, mt.RequiredScope)
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
