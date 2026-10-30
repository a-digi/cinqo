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
	"github.com/a-digi/cinqo/src/comments"
	"github.com/a-digi/cinqo/src/dashboard"
	"regexp"
	"slices"
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
	// RequiresTools declares other tools this one cannot function
	// without — each entry's Slug must already be installed (and
	// enabled — see ValidateRequiredTools) before this manifest can
	// pass validation. A genuinely different concept from
	// RequiredScopes above (that one is an existing *cinqo* scope this
	// tool needs, informational only, never enforced) — this one is
	// enforced, at both install and enable time. See
	// plan/ai/tools/step-14-tool-dependencies.md.
	RequiresTools []RequiredToolDecl `json:"requires_tools,omitempty"`
	// EventListeners declares which Domain Events this tool wants
	// delivered to its own already-running HTTP server, and at which
	// path. Deliberately independent of Routes above: a listener path is
	// never reached through the public /api/v1/tools/{slug}/proxy/**
	// route table at all — it's invoked directly by core's own event
	// dispatcher over loopback, so there is no external caller to
	// scope-check against, which is also why EventListenerDecl carries
	// no RequiredScope field. A tool may declare a listener for ANY
	// topic, including cinqo's own core-native events or another tool's
	// own namespace — the "may only ever listen to its own events"
	// restriction does not exist; only what a tool may PUBLISH is
	// namespace-restricted (enforced at the publish endpoint, a later
	// step, not here). See
	// plan/ai/domain-events/step-03-tool-listener-manifest-and-cache.md.
	EventListeners []EventListenerDecl `json:"event_listeners,omitempty"`
	// Widgets declares Home dashboard widgets this tool provides. Each names one
	// of the tool's own declared GET routes as its data source; the widget's
	// scope is that route's required_scope. See WidgetDecl and
	// plan/ai/dashboard/home-dashboard.md.
	Widgets []WidgetDecl `json:"widgets,omitempty"`
	// Comments declares which of this tool's entities can be commented on, and
	// who may read and write those comments. Needs no backend: core stores them.
	// See CommentTypeDecl and plan/ai/comments/comments-engine.md.
	Comments []CommentTypeDecl `json:"comments,omitempty"`
	// FileSources declares text files of this tool (its own logs, say) that Cinqo's file reader may show to
	// people holding the declared scope. Needs no backend route: core reads the files. See FileSourceDecl and
	// plan/ai/file-reader/file-reader-engine.md.
	FileSources []FileSourceDecl `json:"file_sources,omitempty"`
}

type ScopeDecl struct {
	Scope       string `json:"scope"`
	Description string `json:"description"`
}

type RouteDecl struct {
	Method        string `json:"method"`
	PathSuffix    string `json:"path_suffix"`
	RequiredScope string `json:"required_scope"`
	// AllowCapabilityToken — see tool_entity.ToolRoute's own field of
	// the same name, which this maps directly onto at install/update
	// time. Defaults to false (the normal, scope-gated proxy path) when
	// omitted, so it's opt-in per route.
	AllowCapabilityToken bool `json:"allow_capability_token,omitempty"`
}

type MCPToolDecl struct {
	Name          string `json:"name"`
	RequiredScope string `json:"required_scope"`
	// MediaParam names the argument of this MCP tool (by its JSON key)
	// that accepts a Media reference — a "media:<fileId>" string
	// conversation/chat.go's own invokeToolCall resolves server-side,
	// in-process, into a real "file://<absolute path>" value before the
	// tool is ever invoked, so the tool itself never performs — and can
	// never be asked to authenticate — an HTTP fetch for that value.
	// Empty (the default) means this tool has no such argument. See
	// plan/ai/media/step-02-career-media-migration.md.
	MediaParam string `json:"media_param,omitempty"`
	// PromoteMediaParam names the argument of this MCP tool (by its
	// JSON key) that carries another tool's own local resource
	// reference (e.g. a "/api/v1/tools/<slug>/proxy/..." link one of
	// this tool's own MCP tool calls returned earlier in the same
	// turn) that needs to be promoted into permanent Media storage
	// BEFORE this tool call happens — conversation/chat.go's own
	// invokeToolCall resolves it server-side, in-process (reading the
	// referenced file directly off the producing tool's own uploads
	// directory, then api/src/media's PersistLocalFile), replacing the
	// argument's value with the resulting Media file id before the
	// tool is ever invoked. The symmetric, opposite-direction
	// counterpart to MediaParam above. Empty (the default) means this
	// tool has no such argument. See
	// plan/ai/tools/career/step-XX-cv-pdf.md.
	PromoteMediaParam string `json:"promote_media_param,omitempty"`
	// Delegates opts this MCP tool into returning a delegation
	// directive (a structuredContent {"delegate": {...}} block) that
	// the host executes by spawning sub-agents on the tool's own
	// behalf — see conversation/delegation.go. nil (the default) means
	// any directive this tool returns is ignored.
	Delegates *DelegatesDecl `json:"delegates,omitempty"`
}

// DelegatesDecl bounds what a delegating tool may hand its sub-agents:
// AllowedTools is the most any directive may grant (the host further
// intersects it with the caller's own scopes), never more.
type DelegatesDecl struct {
	AllowedTools []string `json:"allowed_tools"`
}

// EventListenerDecl is one Domain Event this tool's own already-running
// HTTP server wants delivered, and the path (on that same server) it
// should be POSTed to. See Manifest.EventListeners's own doc comment.
type EventListenerDecl struct {
	Topic      string `json:"topic"`
	PathSuffix string `json:"path_suffix"`
}

// WidgetDecl is one Home dashboard widget. Kind is one of the core-owned kinds
// (stat, list, chart): the tool supplies data matching that kind's schema, core
// draws it. DataPath is the path_suffix of a GET route declared in this
// manifest's routes[]. SchemaVersion, Width, Height and RefreshSeconds are
// optional (defaults: 1, 1, 1, 60).
type WidgetDecl struct {
	Key            string `json:"key"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	Kind           string `json:"kind"`
	SchemaVersion  int    `json:"schema_version,omitempty"`
	DataPath       string `json:"data_path"`
	Width          int    `json:"width,omitempty"`
	Height         int    `json:"height,omitempty"`
	RefreshSeconds int    `json:"refresh_seconds,omitempty"`
}

// CommentTypeDecl is one kind of entity of this tool that can have comments
// (for example Career's "job"). ReadScope may list and see the comments;
// WriteScope may add them and edit or delete its own. Both must be scopes this
// tool declares.
type CommentTypeDecl struct {
	EntityType string `json:"entity_type"`
	ReadScope  string `json:"read_scope"`
	WriteScope string `json:"write_scope"`
}

// FileSourceDecl is one place in this tool's own data that the file reader may open. ID names it (the reader's
// source becomes "tool:<slug>:<id>"); Dir is which of the tool's own data folders it lives in (one of
// FileSourceDirs; never the database folder); Scope is the scope that may read it and must be one this tool
// declares. A file is then named by a path relative to that folder, which core checks stays inside it.
type FileSourceDecl struct {
	ID    string `json:"id"`
	Dir   string `json:"dir"`
	Scope string `json:"scope"`
}

// RequiredToolDecl is one other tool this manifest's own tool depends
// on. MinVersion is optional — omitted means any installed, enabled
// version of Slug satisfies the dependency.
type RequiredToolDecl struct {
	Slug       string `json:"slug"`
	MinVersion string `json:"min_version,omitempty"`
}

// InstalledToolInfo is the small, DB-free shape ValidationInput.InstalledTools
// holds per slug — everything RequiresTools checking needs and nothing
// more, matching this whole package's own "no DB dependency" rule
// (the caller converts its own tool_entity.Tool rows into this).
type InstalledToolInfo struct {
	Version string
	Enabled bool
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
	// InstalledTools maps every currently-installed tool's own slug to
	// its current version/enabled state — what RequiresTools is
	// checked against. See plan/ai/tools/step-14-tool-dependencies.md.
	InstalledTools map[string]InstalledToolInfo
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

	if err := ValidateRequiredTools(m, in.InstalledTools); err != nil {
		return "", err
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
		if mt.Delegates != nil {
			if len(mt.Delegates.AllowedTools) == 0 {
				return "", fmt.Errorf("mcp_tools entry %q: delegates requires a non-empty allowed_tools", mt.Name)
			}
			for _, name := range mt.Delegates.AllowedTools {
				switch strings.TrimSpace(name) {
				case "":
					return "", fmt.Errorf("mcp_tools entry %q: delegates.allowed_tools entries must be non-empty", mt.Name)
				case "spawn_subagent", "check_subagent", "list_subagents", "cancel_subagent", "crawl_urls_with_subagents":
					return "", fmt.Errorf("mcp_tools entry %q: delegates.allowed_tools may not grant the built-in sub-agent tool %q", mt.Name, name)
				}
			}
		}
	}

	if len(m.EventListeners) > 0 && !in.HasBackendExecutable {
		return "", fmt.Errorf("event_listeners requires the package to include a backend executable")
	}
	for _, el := range m.EventListeners {
		if strings.TrimSpace(el.Topic) == "" {
			return "", fmt.Errorf("event_listeners entries require a topic")
		}
		if strings.TrimSpace(el.PathSuffix) == "" {
			return "", fmt.Errorf("event_listeners entry %q requires a path_suffix", el.Topic)
		}
	}

	if err := ValidateWidgets(m, in.HasBackendExecutable); err != nil {
		return "", err
	}
	if err := ValidateComments(m); err != nil {
		return "", err
	}
	if err := ValidateFileSources(m); err != nil {
		return "", err
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

// WidgetDefinition is d with its optional fields defaulted, as a dashboard
// Definition for slug — the one place the declaration is turned into the
// engine's own type, so Validate and the install cache cannot disagree.
func WidgetDefinition(slug string, d WidgetDecl) dashboard.Definition {
	def := dashboard.Definition{
		Provider:       slug,
		Key:            d.Key,
		Title:          d.Title,
		Description:    d.Description,
		Kind:           dashboard.Kind(d.Kind),
		Size:           dashboard.Size{W: d.Width, H: d.Height},
		RefreshSeconds: d.RefreshSeconds,
	}
	if def.Size.W == 0 {
		def.Size.W = dashboard.MinWidth
	}
	if def.Size.H == 0 {
		def.Size.H = dashboard.MinHeight
	}
	if def.RefreshSeconds == 0 {
		def.RefreshSeconds = dashboard.DefaultRefreshSeconds
	}
	return def
}

// ValidateWidgets checks m.Widgets: a backend must exist to serve them, every
// definition must be valid (names, kind, size and refresh ranges), keys must be
// unique, the schema version must be one the engine knows, and the data path
// must be a GET route this manifest declares, with a non-empty required_scope
// (the widget inherits it, so a route without one would show the widget to
// everyone).
func ValidateWidgets(m Manifest, hasBackend bool) error {
	if len(m.Widgets) == 0 {
		return nil
	}
	if !hasBackend {
		return fmt.Errorf("widgets requires the package to include a backend executable")
	}
	routeScope := map[string]string{}
	for _, r := range m.Routes {
		if strings.EqualFold(r.Method, "GET") {
			routeScope[r.PathSuffix] = r.RequiredScope
		}
	}
	seen := map[string]struct{}{}
	for _, w := range m.Widgets {
		if err := WidgetDefinition(m.Slug, w).Validate(); err != nil {
			return fmt.Errorf("widgets: %w", err)
		}
		if _, dup := seen[w.Key]; dup {
			return fmt.Errorf("widgets: key %q is declared twice", w.Key)
		}
		seen[w.Key] = struct{}{}
		if w.SchemaVersion != 0 && w.SchemaVersion != dashboard.SchemaVersion {
			return fmt.Errorf("widgets: widget %q uses unsupported schema_version %d", w.Key, w.SchemaVersion)
		}
		scope, ok := routeScope[w.DataPath]
		if !ok {
			return fmt.Errorf("widgets: widget %q data_path %q must be a GET route declared in routes", w.Key, w.DataPath)
		}
		if strings.TrimSpace(scope) == "" {
			return fmt.Errorf("widgets: widget %q data_path %q must name a route with a required_scope", w.Key, w.DataPath)
		}
	}
	return nil
}

// ValidateComments checks m.Comments: each entity type is a valid name (the same
// rule the comments engine applies), declared once, and both scopes are scopes this
// tool itself declares (the same rule mcp_tools has), so a tool cannot hand out
// access under a scope that belongs to someone else.
func ValidateComments(m Manifest) error {
	declared := make(map[string]struct{}, len(m.Scopes))
	for _, s := range m.Scopes {
		declared[s.Scope] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, c := range m.Comments {
		if !comments.ValidEntityType(c.EntityType) {
			return fmt.Errorf("comments: entity_type %q must start with a lower-case letter and contain only lower-case letters, digits and underscores (at most %d characters)", c.EntityType, comments.MaxEntityTypeRunes)
		}
		if _, dup := seen[c.EntityType]; dup {
			return fmt.Errorf("comments: entity_type %q is declared twice", c.EntityType)
		}
		seen[c.EntityType] = struct{}{}
		for _, f := range []struct{ name, scope string }{{"read_scope", c.ReadScope}, {"write_scope", c.WriteScope}} {
			if _, ok := declared[f.scope]; !ok {
				return fmt.Errorf("comments: entity_type %q: %s %q must be one of this tool's own declared scopes", c.EntityType, f.name, f.scope)
			}
		}
	}
	return nil
}

// FileSourceDirs are the tool data folders a file source may point at. The database folder is left out on
// purpose: its files are binary and hold everything the tool stores.
var FileSourceDirs = []string{"logs", "uploads", "cache", "tmp"}

// MaxFileSources bounds how many sources one tool may declare.
const MaxFileSources = 8

var fileSourceIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// ValidFileSourceID reports whether id may name a file source.
func ValidFileSourceID(id string) bool { return fileSourceIDPattern.MatchString(id) }

// ValidateFileSources checks m.FileSources: each id is a valid name and declared once, each dir is one of
// FileSourceDirs, and each scope is one this tool itself declares (the same rule comments and mcp_tools have),
// so a tool cannot expose its files under a scope that belongs to someone else.
func ValidateFileSources(m Manifest) error {
	if len(m.FileSources) > MaxFileSources {
		return fmt.Errorf("file_sources: at most %d sources may be declared", MaxFileSources)
	}
	declared := make(map[string]struct{}, len(m.Scopes))
	for _, s := range m.Scopes {
		declared[s.Scope] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, f := range m.FileSources {
		if !ValidFileSourceID(f.ID) {
			return fmt.Errorf("file_sources: id %q must start with a lower-case letter and contain only lower-case letters, digits, - and _ (at most 32 characters)", f.ID)
		}
		if _, dup := seen[f.ID]; dup {
			return fmt.Errorf("file_sources: id %q is declared twice", f.ID)
		}
		seen[f.ID] = struct{}{}
		if !slices.Contains(FileSourceDirs, f.Dir) {
			return fmt.Errorf("file_sources: id %q: dir %q must be one of %s", f.ID, f.Dir, strings.Join(FileSourceDirs, ", "))
		}
		if _, ok := declared[f.Scope]; !ok {
			return fmt.Errorf("file_sources: id %q: scope %q must be one of this tool's own declared scopes", f.ID, f.Scope)
		}
	}
	return nil
}

// ValidateRequiredTools checks m's own RequiresTools against
// installed — a small, standalone function (not folded silently into
// Validate's own body, even though Validate also calls it for the
// install path) so EnableHandler can re-run exactly this one check on
// its own, without re-running slug/scope/app-version validation that
// doesn't apply to re-enabling an already-installed tool. A dependency
// must be both installed AND enabled — "cannot be used" (the request's
// own wording) means a disabled dependency blocks re-enabling the
// dependent tool too, not only its first install. See
// plan/ai/tools/step-14-tool-dependencies.md.
func ValidateRequiredTools(m Manifest, installed map[string]InstalledToolInfo) error {
	for _, dep := range m.RequiresTools {
		info, ok := installed[dep.Slug]
		if !ok {
			return fmt.Errorf("requires tool %q to be installed first", dep.Slug)
		}
		if !info.Enabled {
			return fmt.Errorf("requires tool %q to be enabled", dep.Slug)
		}
		if dep.MinVersion != "" {
			cmp, err := CompareVersions(info.Version, dep.MinVersion)
			if err != nil {
				return err
			}
			if cmp < 0 {
				return fmt.Errorf("requires tool %q version >= %s, installed %s", dep.Slug, dep.MinVersion, info.Version)
			}
		}
	}
	return nil
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
