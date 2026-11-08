package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/a-digi/cinqo/src/tool/manifest"
	"github.com/a-digi/cinqo/src/toolmeta"
	"github.com/a-digi/cinqo/src/toolrepository/resolver"
)

// MaxPlanDownloadBytes bounds what one plan may download (and hold in memory) before anything is installed: a little more than two of the largest
// single packages the format allows. A variable only so a test can lower it.
var MaxPlanDownloadBytes int64 = 500 << 20

var (
	ErrPackageMismatch = errorKind("the package is not what the repository listed")
	ErrTooLarge        = errorKind("the download is too large")
)

// VerifiedPackage is one package of a plan after it was downloaded and checked: the checksum and size the repository listed, and the package's own
// manifest against what the repository said about that version.
type VerifiedPackage struct {
	ToolID       string
	Version      string
	VersionID    string
	RepositoryID string
	Data         []byte
	Manifest     manifest.Manifest
	// Install is what the install pipeline is told to expect.
	Install *InstallPlan
}

// Verified is the result of the verify-first phase: the plan as it stands once every package was read, and the packages (in memory, nothing installed).
type Verified struct {
	// First is the plan made from the repository files alone; Plan is the final one (equal unless a package's manifest said something else).
	First resolver.Plan
	Plan  resolver.Plan
	// Packages are the verified zips of the final plan's install and update steps, by tool id.
	Packages map[string]*VerifiedPackage
	// Changed: the final plan is not the plan the repository files alone gave (a manifest needed more, or less, than the file said, or a different version
	// had to be chosen). The caller must show it again before installing.
	Changed bool
	// Notes say where a package and its repository file disagreed about what the tool needs ("the package wins").
	Notes []string
}

// VerifyPlan is the verify-first phase of an install together with its dependencies (plan plan/ai/tools/repository/dependency-install.md, step 5). It
// makes the plan, downloads every package it needs (and only those), checks each one against the checksum and size the repository lists, reads each one's own
// manifest and checks that it is the tool and the version the repository listed and says who made it as the repository claimed, and runs the resolver again
// with what the manifests say they need (the manifest is the authority; a repository file may be older or hand written, or say nothing). Round after round,
// until every package of the plan has been read and nothing changed. Nothing is installed in here.
//
// A plan that cannot be made is returned as it is (with its problems) and nothing is downloaded. A package that is not what the repository promised stops it.
func (s *Service) VerifyPlan(ctx context.Context, id, toolID, versionID string, installed []resolver.InstalledTool, env resolver.Env) (*Verified, error) {
	in, err := s.planInput(id, toolID, versionID, installed, env)
	if err != nil {
		return nil, err
	}
	first := resolver.Resolve(in)
	out := &Verified{First: first, Plan: first}
	if !first.OK {
		return out, nil
	}

	got := map[string]*VerifiedPackage{} // by tool@version, every package read so far (some may stop being needed)
	in.Discovered = map[string][]resolver.Requirement{}
	var downloaded int64
	settled := false
	for round := 0; round <= resolver.MaxSteps+1; round++ {
		plan := resolver.Resolve(in)
		out.Plan = plan
		if !plan.OK {
			settled = true
			break
		}
		var pending []resolver.Step
		var listed int64
		for _, st := range plan.Steps {
			if st.Action == resolver.ActionEnable {
				continue
			}
			listed += st.SizeBytes
			if got[key(st.ToolID, st.Version)] == nil {
				pending = append(pending, st)
			}
		}
		if listed > MaxPlanDownloadBytes {
			return nil, &Error{ErrTooLarge, fmt.Sprintf("this plan would download %d MB, at most %d MB at once", listed>>20, MaxPlanDownloadBytes>>20)}
		}
		if len(pending) == 0 {
			settled = true
			break
		}
		for _, st := range pending {
			vp, err := s.fetchAndCheck(ctx, in, st)
			if err != nil {
				return nil, err
			}
			downloaded += int64(len(vp.Data))
			if downloaded > MaxPlanDownloadBytes {
				return nil, &Error{ErrTooLarge, fmt.Sprintf("this plan downloaded more than %d MB: it is stopped", MaxPlanDownloadBytes>>20)}
			}
			got[key(st.ToolID, st.Version)] = vp
			in.Discovered[key(st.ToolID, st.Version)] = requirementsOfManifest(vp.Manifest)
			if note := s.requirementNote(in, st, vp.Manifest); note != "" {
				out.Notes = append(out.Notes, note)
			}
		}
	}
	if !settled {
		return nil, &Error{ErrNotInstallable, "what these tools need keeps changing as their packages are read: nothing was installed"}
	}

	out.Packages = map[string]*VerifiedPackage{}
	if out.Plan.OK {
		for _, st := range out.Plan.Steps {
			if vp := got[key(st.ToolID, st.Version)]; vp != nil && st.Action != resolver.ActionEnable {
				out.Packages[st.ToolID] = vp
			}
		}
	}
	out.Changed = stepsSignature(first) != stepsSignature(out.Plan)
	return out, nil
}

func key(toolID, version string) string { return toolID + "@" + version }

// fetchAndCheck downloads the package of one step, checks its size and checksum, reads its manifest and checks it against what the repository listed.
func (s *Service) fetchAndCheck(ctx context.Context, in resolver.Input, st resolver.Step) (*VerifiedPackage, error) {
	var src *resolver.Source
	for i := range in.Sources {
		if in.Sources[i].ID == st.RepositoryID {
			src = &in.Sources[i]
		}
	}
	if src == nil || src.Catalog == nil {
		return nil, &Error{ErrToolNotFound, "the repository of " + st.ToolID + " is not available"}
	}
	tool, ok := src.Catalog.Tool(st.ToolID)
	if !ok {
		return nil, &Error{ErrToolNotFound, st.ToolID + " is no longer in " + src.Name}
	}
	ver, ok := tool.Version(st.VersionID)
	if !ok {
		return nil, &Error{ErrVersionMissing, st.ToolID + " " + st.Version + " is no longer in " + src.Name}
	}
	pkg, ok := ver.PickPackage(in.Env.OS, in.Env.Arch)
	if !ok {
		return nil, &Error{ErrNotInstallable, fmt.Sprintf("%s %s has no package for %s/%s", st.ToolID, st.Version, in.Env.OS, in.Env.Arch)}
	}
	plan := &InstallPlan{
		URL: pkg.URL, SHA256: pkg.SHA256, SizeBytes: pkg.SizeBytes,
		ToolID: st.ToolID, Version: ver.Version, VersionID: ver.VersionID, Local: src.Local, Info: ver.Info,
	}
	data, err := s.Download(ctx, plan)
	if err != nil {
		return nil, err
	}
	m, err := manifest.ReadFromZip(data)
	if err != nil {
		return nil, &Error{ErrPackageMismatch, fmt.Sprintf("the package of %s %s cannot be read: %v", st.ToolID, st.Version, err)}
	}
	switch {
	case m.Slug != st.ToolID:
		return nil, &Error{ErrPackageMismatch, fmt.Sprintf("the package listed as %s is the tool %q", st.ToolID, m.Slug)}
	case m.Version != ver.Version:
		return nil, &Error{ErrPackageMismatch, fmt.Sprintf("the package listed as %s %s is version %s", st.ToolID, ver.Version, m.Version)}
	}
	if msg := toolmeta.Mismatch(ver.Info, m.Info); msg != "" {
		return nil, &Error{ErrPackageMismatch, fmt.Sprintf("%s %s: %s", st.ToolID, st.Version, msg)}
	}
	if err := manifest.ValidateRequires(m); err != nil {
		return nil, &Error{ErrPackageMismatch, fmt.Sprintf("the package of %s %s declares what it needs in a way that cannot be used: %v", st.ToolID, st.Version, err)}
	}
	return &VerifiedPackage{ToolID: st.ToolID, Version: ver.Version, VersionID: ver.VersionID, RepositoryID: st.RepositoryID, Data: data, Manifest: m, Install: plan}, nil
}

func requirementsOfManifest(m manifest.Manifest) []resolver.Requirement {
	out := make([]resolver.Requirement, 0, len(m.RequiresTools))
	for _, r := range m.RequiresTools {
		out = append(out, resolver.Requirement{ID: r.Slug, Min: r.MinVersion, Max: r.MaxVersion})
	}
	return out
}

// requirementNote says so when the package and the repository file disagree about what a version needs (the package wins).
func (s *Service) requirementNote(in resolver.Input, st resolver.Step, m manifest.Manifest) string {
	var listed []resolver.Requirement
	for i := range in.Sources {
		if in.Sources[i].ID != st.RepositoryID || in.Sources[i].Catalog == nil {
			continue
		}
		if t, ok := in.Sources[i].Catalog.Tool(st.ToolID); ok {
			if v, ok := t.Version(st.VersionID); ok {
				for _, r := range v.Requires {
					listed = append(listed, resolver.Requirement{ID: r.ID, Min: r.MinVersion, Max: r.MaxVersion})
				}
			}
		}
	}
	actual := requirementsOfManifest(m)
	if sameRequirements(listed, actual) {
		return ""
	}
	return fmt.Sprintf("%s %s: the repository lists it as needing %s, the package says %s (the package is used)", st.ToolID, st.Version, describeRequirements(listed), describeRequirements(actual))
}

func describeRequirements(rs []resolver.Requirement) string {
	if len(rs) == 0 {
		return "nothing"
	}
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		p := r.ID
		switch {
		case r.Min != "" && r.Max != "":
			p += " " + r.Min + " to " + r.Max
		case r.Min != "":
			p += " " + r.Min + " or newer"
		case r.Max != "":
			p += " " + r.Max + " or older"
		}
		parts = append(parts, p)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func sameRequirements(a, b []resolver.Requirement) bool {
	return describeRequirements(a) == describeRequirements(b)
}

// stepsSignature identifies a plan by what it does: "tool@version:action" in order.
func stepsSignature(p resolver.Plan) string {
	var b strings.Builder
	for _, st := range p.Steps {
		fmt.Fprintf(&b, "%s@%s:%s;", st.ToolID, st.Version, st.Action)
	}
	return b.String()
}
