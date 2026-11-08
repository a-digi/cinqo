// Package resolver turns "install or update this tool from this repository" into an ordered list of what has to be installed, updated or enabled
// first, or into the reasons it cannot be done. It is the heart of plan/ai/tools/repository/dependency-install.md (revision 2, rule table S1 to S15).
//
// It is pure on purpose: the catalogs of every registered repository, the installed tools with their requirements, the machine and the request
// come in as values; a Plan comes out. No network, no database, no file system, no clock. Everything that has to touch the world (reading the
// repositories, downloading, installing) happens around it, so the rules can be tested exhaustively and replaced without touching the install pipeline.
package resolver

import "github.com/a-digi/cinqo/src/toolrepository/catalog"

const (
	// MaxDepth is the longest chain of tools that may have to be installed one after the other, and MaxSteps the most steps one plan may have, so a
	// broken or hostile repository cannot make the server do unbounded work.
	MaxDepth = 16
	MaxSteps = 25
	// maxRounds bounds the search for a stable plan; a plan of at most MaxSteps tools settles in far fewer.
	maxRounds = 60
)

// Action is what a step does to one tool.
type Action string

const (
	ActionInstall Action = "install"
	ActionUpdate  Action = "update"
	// ActionEnable only enables an installed tool that is disabled: no download.
	ActionEnable Action = "enable"
)

// Mark is something the user must see (and, for some, tick) before the plan runs.
type Mark string

const (
	// MarkOtherRepository: the package comes from a repository other than the chosen one or the one the tool was installed from.
	MarkOtherRepository Mark = "other_repository"
	// MarkLocalNetwork: the repository is on a local network address.
	MarkLocalNetwork Mark = "local_network"
	// MarkStaleRepository: the repository could not be refreshed; the last good file is used.
	MarkStaleRepository Mark = "stale_repository"
	// MarkRestarts: the tool is running and is updated, so it (and what uses it) restarts.
	MarkRestarts Mark = "restarts"
	// MarkWasDisabled: the tool is installed but disabled, and will be enabled.
	MarkWasDisabled Mark = "was_disabled"
)

// ProblemKind says why a plan cannot be made.
type ProblemKind string

const (
	ProblemNotInAnyRepository        ProblemKind = "not_in_any_repository"
	ProblemNoVersionInRange          ProblemKind = "no_version_in_range"
	ProblemWouldBreakInstalled       ProblemKind = "would_break_installed"
	ProblemNeedsDowngrade            ProblemKind = "needs_downgrade"
	ProblemConflictingRequirements   ProblemKind = "conflicting_requirements"
	ProblemNotInstallableHere        ProblemKind = "not_installable_here"
	ProblemPublisherChanged          ProblemKind = "publisher_changed"
	ProblemConflictingCopies         ProblemKind = "conflicting_copies"
	ProblemStaleOnlySource           ProblemKind = "stale_only_source"
	ProblemCycle                     ProblemKind = "cycle"
	ProblemTooDeep                   ProblemKind = "too_deep"
	ProblemTooMany                   ProblemKind = "too_many"
	ProblemAlreadyInstalled          ProblemKind = "already_installed"
	ProblemOlderThanInstalled        ProblemKind = "older_than_installed"
	ProblemInstalledVersionUnreadble ProblemKind = "installed_version_unreadable"
	ProblemUnstable                  ProblemKind = "unstable"
)

// Requirement is one tool another tool needs: the lowest and the highest version of it that works (both inclusive, either may be empty).
type Requirement struct {
	ID  string
	Min string
	Max string
}

// InstalledTool is an installed tool as the resolver needs to know it.
type InstalledTool struct {
	ID      string
	Name    string
	Version string
	Enabled bool
	// Running says whether its backend is running now (it restarts when the tool is updated).
	Running bool
	// SourceRepositoryID is the repository it was installed from; empty means unknown.
	SourceRepositoryID string
	// Authors are the names its package declared (empty when not stated).
	Authors []string
	// Requires is what it declared it needs.
	Requires []Requirement
}

// Source is one registered repository with its last good catalog.
type Source struct {
	ID   string
	Name string
	// Local: the repository is on a local network address.
	Local bool
	// Stale: its last refresh failed, Catalog is from an earlier refresh (RefreshedAt says when).
	Stale       bool
	RefreshedAt string
	// Catalog is nil for a repository that was never loaded: it is skipped.
	Catalog *catalog.Catalog
}

// Env is the machine and the running app.
type Env struct {
	OS         string
	Arch       string
	AppVersion string
}

// Request is what the user chose: a tool (and optionally a version, default the current one) in one repository.
type Request struct {
	RepositoryID string
	ToolID       string
	VersionID    string
}

// Input is everything the resolver looks at.
type Input struct {
	Installed []InstalledTool
	Sources   []Source
	Env       Env
	Request   Request
	// Discovered are requirements read from a downloaded package's own manifest, by "toolId@version". They replace what the repository file says for
	// that version (the manifest is the authority); empty means the file's `requires` is used.
	Discovered map[string][]Requirement
}

// Step is one thing the plan does, in the order it must be done.
type Step struct {
	ToolID           string
	Name             string
	Action           Action
	VersionID        string
	Version          string
	InstalledVersion string
	RepositoryID     string
	RepositoryName   string
	SizeBytes        int64
	// Authors and License are what the repository says about this version (for an enable step: what the installed tool declared), so the
	// confirmation can show who made everything it is about to install.
	Authors []string
	License string
	// RequiredBy are the tools of this plan that need this one.
	RequiredBy []string
	// EnableAfter: the tool is installed but disabled and an update keeps it disabled, so it is enabled after the update.
	EnableAfter bool
	Marks       []Mark
	// Dependents are the installed tools outside the plan that need this one (listed for an update, which restarts it).
	Dependents []string
}

// Problem is one reason the plan cannot be made.
type Problem struct {
	ToolID     string
	RequiredBy []string
	Kind       ProblemKind
	Message    string
}

// Searched is a repository that was looked in, in the order it was searched.
type Searched struct {
	ID          string
	Name        string
	Stale       bool
	RefreshedAt string
}

// Plan is the answer: the ordered steps, or the problems (then Steps is empty).
type Plan struct {
	OK       bool
	Steps    []Step
	Problems []Problem
	Searched []Searched
}
