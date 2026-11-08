package resolver

import (
	"fmt"
	"sort"
	"strings"

	"github.com/a-digi/cinqo/src/tool/manifest"
	"github.com/a-digi/cinqo/src/toolrepository/catalog"
	"github.com/a-digi/cinqo/src/toolrepository/status"
)

// Resolve works out what has to be done, in which order, to install or update the requested tool, or why that cannot be done. It never changes
// anything. The rules are the table S1 to S15 of the plan (plan/ai/tools/repository/dependency-install.md, revision 2); each is marked where it is applied.
//
// How it works: the chosen tool is the root of the plan. The requirements of every tool the plan installs or updates (and of an installed tool it
// enables, because enabling re-checks them) become constraints on other tools; every constrained tool is decided again under all constraints
// known so far (satisfied, enable, install, update, or blocked); this repeats until the decisions stop changing, because a tool chosen for one
// requester can be tightened by another (S9) and a tool that joins the plan stops counting as an untouched installed dependent (S6). Then the steps
// are put in dependency order, each tool once (S13 for a cycle, the depth and the step cap).
func Resolve(in Input) Plan {
	r := newRun(in)
	plan := Plan{Searched: r.searched()}

	root, probs := r.resolveRoot()
	if len(probs) > 0 {
		plan.Problems = sortProblems(probs)
		return plan
	}

	decisions := map[string]*decision{root.id: root}
	stable := false
	for round := 0; round < maxRounds; round++ {
		cons := r.constraints(decisions)
		inPlan := planSet(decisions)
		next := map[string]*decision{root.id: root}
		ids := make([]string, 0, len(cons))
		for id := range cons {
			if id != root.id {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		for _, id := range ids {
			next[id] = r.decide(id, cons[id], r.installedDependents(id, inPlan))
		}
		if countSteps(next) > MaxSteps {
			plan.Problems = []Problem{{ToolID: root.id, Kind: ProblemTooMany, Message: fmt.Sprintf("this would take more than %d steps: it is stopped here", MaxSteps)}}
			return plan
		}
		if signature(next) == signature(decisions) {
			decisions, stable = next, true
			break
		}
		decisions = next
	}
	if !stable {
		plan.Problems = []Problem{{ToolID: root.id, Kind: ProblemUnstable, Message: "the requirements of these tools keep changing each other: nothing is installed"}}
		return plan
	}

	cons := r.constraints(decisions)
	var problems []Problem
	// a tool that needs the root in a range the chosen version is not in
	for _, c := range cons[root.id] {
		if ok, _ := within(root.cand.ver.Version, c.min, c.max); !ok {
			problems = append(problems, Problem{ToolID: root.id, RequiredBy: []string{c.by}, Kind: ProblemConflictingRequirements,
				Message: fmt.Sprintf("%s needs %s %s, but %s %s would be installed", c.by, root.id, rangeText(c.min, c.max), root.id, root.cand.ver.Version)})
		}
	}
	for _, d := range decisions {
		problems = append(problems, d.problems...)
	}
	if len(problems) > 0 {
		plan.Problems = sortProblems(problems)
		return plan
	}

	steps, probs := r.order(decisions, cons)
	if len(probs) > 0 {
		plan.Problems = sortProblems(probs)
		return plan
	}
	plan.OK, plan.Steps = true, steps
	return plan
}

// ---- the run ----

type run struct {
	in        Input
	installed map[string]*InstalledTool
	byID      map[string]*Source
	chosen    *Source
	env       status.Env
}

func newRun(in Input) *run {
	r := &run{in: in, installed: map[string]*InstalledTool{}, byID: map[string]*Source{}, env: status.Env{OS: in.Env.OS, Arch: in.Env.Arch, AppVersion: in.Env.AppVersion}}
	for i := range in.Installed {
		r.installed[in.Installed[i].ID] = &in.Installed[i]
	}
	for i := range in.Sources {
		r.byID[in.Sources[i].ID] = &in.Sources[i]
	}
	r.chosen = r.byID[in.Request.RepositoryID]
	return r
}

// sourcesFor is the order a tool is looked for in (decision 8): the repository the user chose, then the one the tool was installed from, then the
// others by name. Repositories that were never loaded are left out.
func (r *run) sourcesFor(id string) []*Source {
	var out []*Source
	seen := map[string]bool{}
	add := func(s *Source) {
		if s == nil || s.Catalog == nil || seen[s.ID] {
			return
		}
		seen[s.ID] = true
		out = append(out, s)
	}
	add(r.chosen)
	if inst := r.installed[id]; inst != nil && inst.SourceRepositoryID != "" {
		add(r.byID[inst.SourceRepositoryID])
	}
	rest := make([]*Source, 0, len(r.in.Sources))
	for i := range r.in.Sources {
		rest = append(rest, &r.in.Sources[i])
	}
	sort.SliceStable(rest, func(i, j int) bool {
		if rest[i].Name != rest[j].Name {
			return rest[i].Name < rest[j].Name
		}
		return rest[i].ID < rest[j].ID
	})
	for _, s := range rest {
		add(s)
	}
	return out
}

func (r *run) searched() []Searched {
	var out []Searched
	for _, s := range r.sourcesFor("") {
		out = append(out, Searched{ID: s.ID, Name: s.Name, Stale: s.Stale, RefreshedAt: s.RefreshedAt})
	}
	return out
}

// ---- decisions ----

type cand struct {
	src  *Source
	tool *catalog.Tool
	ver  *catalog.Version
	pkg  *catalog.Package
}

type decision struct {
	id string
	// satisfied, enable, install, update or blocked
	action   string
	cand     *cand
	problems []Problem
}

func isStep(d *decision) bool {
	return d.action == "install" || d.action == "update" || d.action == "enable"
}

func planSet(ds map[string]*decision) map[string]bool {
	out := map[string]bool{}
	for id, d := range ds {
		if d.action == "install" || d.action == "update" {
			out[id] = true
		}
	}
	return out
}

func countSteps(ds map[string]*decision) int {
	n := 0
	for _, d := range ds {
		if isStep(d) {
			n++
		}
	}
	return n
}

func signature(ds map[string]*decision) string {
	ids := make([]string, 0, len(ds))
	for id := range ds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	for _, id := range ids {
		d := ds[id]
		fmt.Fprintf(&b, "%s|%s|", id, d.action)
		if d.cand != nil {
			fmt.Fprintf(&b, "%s@%s|", d.cand.src.ID, d.cand.ver.Version)
		}
		for _, p := range d.problems {
			fmt.Fprintf(&b, "%s:%s;", p.Kind, p.Message)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// requirementsOf are the requirements a decision brings in: the chosen version's (what the downloaded manifest said when it is known, else what the
// repository file says), or an installed tool's own when it is only enabled (enabling re-checks them).
func (r *run) requirementsOf(d *decision) []Requirement {
	switch d.action {
	case "install", "update":
		key := d.id + "@" + d.cand.ver.Version
		if disc, ok := r.in.Discovered[key]; ok {
			return disc
		}
		out := make([]Requirement, 0, len(d.cand.ver.Requires))
		for _, q := range d.cand.ver.Requires {
			out = append(out, Requirement{ID: q.ID, Min: q.MinVersion, Max: q.MaxVersion})
		}
		return out
	case "enable":
		if inst := r.installed[d.id]; inst != nil {
			return inst.Requires
		}
	}
	return nil
}

type constraint struct {
	by, min, max string
}

// constraints gathers, for every tool the plan needs, what each plan tool asks of it.
func (r *run) constraints(ds map[string]*decision) map[string][]constraint {
	out := map[string][]constraint{}
	for _, d := range ds {
		for _, q := range r.requirementsOf(d) {
			out[q.ID] = append(out[q.ID], constraint{by: d.id, min: q.Min, max: q.Max})
		}
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool {
			a, b := out[id][i], out[id][j]
			if a.by != b.by {
				return a.by < b.by
			}
			if a.min != b.min {
				return a.min < b.min
			}
			return a.max < b.max
		})
	}
	return out
}

// installedDependents are the installed tools outside the plan that need id: they are not changed, so a new version of id must stay inside their range (S6).
func (r *run) installedDependents(id string, inPlan map[string]bool) []constraint {
	var out []constraint
	for tid, t := range r.installed {
		if tid == id || inPlan[tid] {
			continue
		}
		for _, q := range t.Requires {
			if q.ID == id {
				out = append(out, constraint{by: tid, min: q.Min, max: q.Max})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].by != out[j].by {
			return out[i].by < out[j].by
		}
		return out[i].min < out[j].min
	})
	return out
}

// ---- versions and ranges ----

func cmpVer(a, b string) (int, bool) {
	c, err := manifest.CompareVersions(a, b)
	return c, err == nil
}

func newer(a, b string) bool {
	c, ok := cmpVer(a, b)
	return ok && c > 0
}

// within reports whether version lies between lo and hi (inclusive, either may be empty), and whether it could be compared at all.
func within(version, lo, hi string) (ok, readable bool) {
	if lo != "" {
		c, rd := cmpVer(version, lo)
		if !rd {
			return false, false
		}
		if c < 0 {
			return false, true
		}
	}
	if hi != "" {
		c, rd := cmpVer(version, hi)
		if !rd {
			return false, false
		}
		if c > 0 {
			return false, true
		}
	}
	return true, true
}

type folded struct {
	lo, loBy, hi, hiBy string
	conflict           bool
}

// fold combines the ranges several tools ask for (S9): the highest minimum and the lowest maximum; conflict when nothing can satisfy both.
func fold(cs []constraint) folded {
	var f folded
	for _, c := range cs {
		if c.min != "" && (f.lo == "" || newer(c.min, f.lo)) {
			f.lo, f.loBy = c.min, c.by
		}
		if c.max != "" && (f.hi == "" || newer(f.hi, c.max)) {
			f.hi, f.hiBy = c.max, c.by
		}
	}
	f.conflict = f.lo != "" && f.hi != "" && newer(f.lo, f.hi)
	return f
}

func rangeText(lo, hi string) string {
	switch {
	case lo != "" && hi != "" && lo == hi:
		return "exactly " + lo
	case lo != "" && hi != "":
		return lo + " to " + hi
	case lo != "":
		return lo + " or newer"
	case hi != "":
		return hi + " or older"
	}
	return "any version"
}

// ---- the root ----

func (r *run) resolveRoot() (*decision, []Problem) {
	req := r.in.Request
	fail := func(kind ProblemKind, format string, a ...any) (*decision, []Problem) {
		return nil, []Problem{{ToolID: req.ToolID, Kind: kind, Message: fmt.Sprintf(format, a...)}}
	}
	if r.chosen == nil || r.chosen.Catalog == nil {
		return fail(ProblemNotInAnyRepository, "the repository has not been loaded yet")
	}
	tool, ok := r.chosen.Catalog.Tool(req.ToolID)
	if !ok {
		return fail(ProblemNotInAnyRepository, "%s is not in this repository", req.ToolID)
	}
	vid := req.VersionID
	if vid == "" {
		vid = tool.CurrentVersionID
	}
	ver, ok := tool.Version(vid)
	if !ok {
		return fail(ProblemNoVersionInRange, "%s has no version %q in this repository", req.ToolID, vid)
	}
	if okAvail, reason := status.Availability(ver, r.env); !okAvail {
		return fail(ProblemNotInstallableHere, "%s %s cannot be installed here: %s", req.ToolID, ver.Version, reason)
	}
	pkg, _ := ver.PickPackage(r.env.OS, r.env.Arch)
	d := &decision{id: req.ToolID, action: "install", cand: &cand{src: r.chosen, tool: tool, ver: ver, pkg: pkg}}
	if inst := r.installed[req.ToolID]; inst != nil {
		c, rd := cmpVer(ver.Version, inst.Version)
		switch {
		case !rd:
			return fail(ProblemInstalledVersionUnreadble, "the installed version of %s (%s) cannot be compared", req.ToolID, inst.Version)
		case c == 0:
			return fail(ProblemAlreadyInstalled, "%s %s is already installed", req.ToolID, inst.Version)
		case c < 0:
			return fail(ProblemOlderThanInstalled, "%s %s is older than the installed %s: an update must be newer", req.ToolID, ver.Version, inst.Version)
		}
		d.action = "update"
		if p := publisherProblem(req.ToolID, inst, ver, r.chosen); p != nil {
			return nil, []Problem{*p}
		}
	}
	return d, nil
}

// ---- a dependency ----

func (r *run) decide(id string, plan, installedDeps []constraint) *decision {
	requiredBy := distinctBy(plan)
	blocked := func(kind ProblemKind, format string, a ...any) *decision {
		return &decision{id: id, action: "blocked", problems: []Problem{{ToolID: id, RequiredBy: requiredBy, Kind: kind, Message: fmt.Sprintf(format, a...)}}}
	}
	f := fold(plan)
	if f.conflict { // S8, S9: two tools ask for ranges that do not meet
		return blocked(ProblemConflictingRequirements, "%s needs %s %s or newer, while %s works with it only up to %s: no version can satisfy both", f.loBy, id, f.lo, f.hiBy, f.hi)
	}
	inst := r.installed[id]
	if inst != nil {
		ok, readable := within(inst.Version, f.lo, f.hi)
		if !readable {
			return blocked(ProblemInstalledVersionUnreadble, "the installed version of %s (%s) cannot be compared", id, inst.Version)
		}
		if ok { // S3 and S4: nothing to download
			if inst.Enabled {
				return &decision{id: id, action: "satisfied"}
			}
			return &decision{id: id, action: "enable"}
		}
		if f.hi != "" && newer(inst.Version, f.hi) { // S7: never downgraded
			return blocked(ProblemNeedsDowngrade, "%s needs %s %s but %s is installed: nothing is downgraded automatically", f.hiBy, id, rangeText(f.lo, f.hi), inst.Version)
		}
	}

	cands, diag := r.candidates(id, f, installedDeps, inst)
	if len(cands) == 0 {
		return r.noCandidate(id, f, diag, requiredBy, inst)
	}
	c := cands[0]
	// S14: a repository that could not be refreshed is only used when nothing else offers it, and is then marked, never silently trusted
	if c.src.Stale && c.src.ID != r.in.Request.RepositoryID {
		fresh := false
		for _, o := range cands {
			if !o.src.Stale {
				fresh = true
			}
		}
		if !fresh {
			return blocked(ProblemStaleOnlySource, "%s can only come from %s, which could not be refreshed (its file is from %s)", id, c.src.Name, c.src.RefreshedAt)
		}
	}
	if p := r.copiesProblem(id, c); p != nil { // S12
		p.RequiredBy = requiredBy
		return &decision{id: id, action: "blocked", problems: []Problem{*p}}
	}
	action := "install"
	if inst != nil {
		action = "update"
		if p := publisherProblem(id, inst, c.ver, c.src); p != nil { // S12
			p.RequiredBy = requiredBy
			return &decision{id: id, action: "blocked", problems: []Problem{*p}}
		}
	}
	return &decision{id: id, action: action, cand: c}
}

func distinctBy(cs []constraint) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range cs {
		if !seen[c.by] {
			seen[c.by] = true
			out = append(out, c.by)
		}
	}
	sort.Strings(out)
	return out
}

type diagnosis struct {
	foundTool   bool
	anyInRange  bool
	anyFits     bool
	unavailable string
	// newestInRange is the newest version inside the requested range, and which installed tools reject it.
	newestInRange string
	rejectedBy    []string
	newestOffered string
}

// candidates are the qualifying versions, at most one per repository (the current version if it qualifies, else the newest), in search order.
func (r *run) candidates(id string, f folded, installedDeps []constraint, inst *InstalledTool) ([]*cand, diagnosis) {
	var out []*cand
	var d diagnosis
	for _, src := range r.sourcesFor(id) {
		tool, ok := src.Catalog.Tool(id)
		if !ok {
			continue
		}
		d.foundTool = true
		var best *catalog.Version
		for i := range tool.Versions {
			v := &tool.Versions[i]
			if d.newestOffered == "" || newer(v.Version, d.newestOffered) {
				d.newestOffered = v.Version
			}
			inRange, _ := within(v.Version, f.lo, f.hi)
			// (an installed tool below the minimum is the only way to get here with an installed version, so every version in range is newer)
			if !inRange {
				continue
			}
			d.anyInRange = true
			rejected := rejecters(id, v.Version, installedDeps)
			if d.newestInRange == "" || newer(v.Version, d.newestInRange) {
				d.newestInRange, d.rejectedBy = v.Version, rejected
			}
			if len(rejected) > 0 {
				continue
			}
			d.anyFits = true
			if okAvail, reason := status.Availability(v, r.env); !okAvail {
				if d.unavailable == "" {
					d.unavailable = reason
				}
				continue
			}
			if best == nil || preferred(v, best, tool.CurrentVersionID) {
				best = v
			}
		}
		if best != nil {
			pkg, _ := best.PickPackage(r.env.OS, r.env.Arch)
			out = append(out, &cand{src: src, tool: tool, ver: best, pkg: pkg})
		}
	}
	return out, d
}

func preferred(v, best *catalog.Version, currentID string) bool {
	if best.VersionID == currentID {
		return false
	}
	if v.VersionID == currentID {
		return true
	}
	return newer(v.Version, best.Version)
}

// rejecters are the installed tools that work with id only in a range version falls outside of ("career (works with browser 1.0.0 to 1.5.0)").
func rejecters(id, version string, deps []constraint) []string {
	var out []string
	for _, c := range deps {
		if ok, _ := within(version, c.min, c.max); !ok {
			out = append(out, fmt.Sprintf("%s (works with %s %s)", c.by, id, rangeText(c.min, c.max)))
		}
	}
	return out
}

func (r *run) noCandidate(id string, f folded, d diagnosis, requiredBy []string, inst *InstalledTool) *decision {
	mk := func(kind ProblemKind, format string, a ...any) *decision {
		return &decision{id: id, action: "blocked", problems: []Problem{{ToolID: id, RequiredBy: requiredBy, Kind: kind, Message: fmt.Sprintf(format, a...)}}}
	}
	switch {
	case !d.foundTool: // S10
		var names []string
		for _, s := range r.sourcesFor(id) {
			names = append(names, s.Name)
		}
		return mk(ProblemNotInAnyRepository, "%s is not offered by any registered repository (searched: %s): install it from another source first", id, strings.Join(names, ", "))
	case !d.anyInRange: // S10
		return mk(ProblemNoVersionInRange, "no registered repository offers %s %s (the newest offered is %s)", id, rangeText(f.lo, f.hi), d.newestOffered)
	case !d.anyFits: // S6: the version that would do is outside what an installed tool works with
		action := "installing"
		if inst != nil {
			action = "updating"
		}
		return mk(ProblemWouldBreakInstalled, "%s %s to %s would leave %s: update that tool first, if a newer version supports it", action, id, d.newestInRange, strings.Join(d.rejectedBy, " and "))
	default: // S11
		return mk(ProblemNotInstallableHere, "%s cannot be installed here: %s", id, d.unavailable)
	}
}

// copiesProblem is S12's second half: another repository lists the same tool and version with a different package.
func (r *run) copiesProblem(id string, c *cand) *Problem {
	for i := range r.in.Sources {
		o := &r.in.Sources[i]
		if o.ID == c.src.ID || o.Catalog == nil {
			continue
		}
		t, ok := o.Catalog.Tool(id)
		if !ok {
			continue
		}
		for j := range t.Versions {
			v := &t.Versions[j]
			if v.Version != c.ver.Version {
				continue
			}
			if p, ok := v.PickPackage(r.env.OS, r.env.Arch); ok && c.pkg != nil && p.SHA256 != c.pkg.SHA256 {
				return &Problem{ToolID: id, Kind: ProblemConflictingCopies,
					Message: fmt.Sprintf("%s %s is offered by %s and by %s with different packages: it is not installed until that is sorted out", id, c.ver.Version, c.src.Name, o.Name)}
			}
		}
	}
	return nil
}

// publisherProblem is S12's first half: an update whose authors differ from the installed version's. Nothing is compared when either side does not say.
func publisherProblem(id string, inst *InstalledTool, ver *catalog.Version, src *Source) *Problem {
	if inst == nil || len(inst.Authors) == 0 || len(ver.Authors) == 0 {
		return nil
	}
	var theirs []string
	for _, a := range ver.Authors {
		theirs = append(theirs, a.Name)
	}
	if sameNames(inst.Authors, theirs) {
		return nil
	}
	return &Problem{ToolID: id, Kind: ProblemPublisherChanged,
		Message: fmt.Sprintf("%s %s from %s is published by %s, the installed version by %s: it is not updated automatically", id, ver.Version, src.Name, strings.Join(theirs, ", "), strings.Join(inst.Authors, ", "))}
}

func sameNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func sortProblems(ps []Problem) []Problem {
	out := append([]Problem(nil), ps...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ToolID != out[j].ToolID {
			return out[i].ToolID < out[j].ToolID
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Message < out[j].Message
	})
	return out
}

// ---- order ----

// order puts the steps after everything they need (S13 for cycles, depth and size caps), each tool once, ties by tool id so the same input gives the same plan.
func (r *run) order(ds map[string]*decision, cons map[string][]constraint) ([]Step, []Problem) {
	var ids []string
	for id, d := range ds {
		if isStep(d) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	isIn := map[string]bool{}
	for _, id := range ids {
		isIn[id] = true
	}
	needs := map[string][]string{}
	for _, id := range ids {
		seen := map[string]bool{}
		for _, q := range r.requirementsOf(ds[id]) {
			if isIn[q.ID] && q.ID != id && !seen[q.ID] {
				seen[q.ID] = true
				needs[id] = append(needs[id], q.ID)
			}
		}
		sort.Strings(needs[id])
	}

	done := map[string]bool{}
	var orderIDs []string
	for len(orderIDs) < len(ids) {
		var ready []string
		for _, id := range ids {
			if done[id] {
				continue
			}
			ok := true
			for _, n := range needs[id] {
				if !done[n] {
					ok = false
				}
			}
			if ok {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			loop := cyclePath(ids, done, needs)
			return nil, []Problem{{ToolID: loop[0], Kind: ProblemCycle, Message: "these tools need each other: " + strings.Join(loop, " -> ")}}
		}
		// one at a time keeps the order stable (the smallest id first), and every tool appears once
		done[ready[0]] = true
		orderIDs = append(orderIDs, ready[0])
	}

	depth := map[string]int{}
	maxDepth := 0
	for _, id := range orderIDs {
		d := 1
		for _, n := range needs[id] {
			if depth[n]+1 > d {
				d = depth[n] + 1
			}
		}
		depth[id] = d
		if d > maxDepth {
			maxDepth = d
		}
	}
	if maxDepth > MaxDepth {
		return nil, []Problem{{ToolID: r.in.Request.ToolID, Kind: ProblemTooDeep, Message: fmt.Sprintf("these tools need each other %d levels deep, at most %d are followed", maxDepth, MaxDepth)}}
	}

	inPlan := planSet(ds)
	steps := make([]Step, 0, len(orderIDs))
	for _, id := range orderIDs {
		steps = append(steps, r.step(ds[id], cons[id], inPlan))
	}
	return steps, nil
}

// cyclePath finds a loop among the tools that could not be ordered and returns it as ["a", "b", "a"]: the first tool is one that is in the loop.
func cyclePath(ids []string, done map[string]bool, needs map[string][]string) []string {
	var start string
	for _, id := range ids {
		if !done[id] {
			start = id
			break
		}
	}
	path := []string{start}
	at := map[string]int{start: 0}
	cur := start
	for {
		next := ""
		for _, n := range needs[cur] {
			if !done[n] {
				next = n
				break
			}
		}
		if next == "" { // cannot happen for tools left over by the ordering; keep the answer honest
			return path
		}
		if i, seen := at[next]; seen {
			return append(path[i:], next)
		}
		at[next] = len(path)
		path = append(path, next)
		cur = next
	}
}

func (r *run) step(d *decision, cs []constraint, inPlan map[string]bool) Step {
	s := Step{ToolID: d.id, RequiredBy: distinctBy(cs)}
	inst := r.installed[d.id]
	if inst != nil {
		s.InstalledVersion = inst.Version
		s.Name = inst.Name
	}
	switch d.action {
	case "enable":
		s.Action = ActionEnable
		s.Version = inst.Version
		s.Authors = append([]string(nil), inst.Authors...)
		s.Marks = append(s.Marks, MarkWasDisabled)
		return s
	case "install":
		s.Action = ActionInstall
	case "update":
		s.Action = ActionUpdate
	}
	s.Name = d.cand.ver.Name
	s.License = d.cand.ver.License
	for _, a := range d.cand.ver.Authors {
		s.Authors = append(s.Authors, a.Name)
	}
	s.VersionID, s.Version = d.cand.ver.VersionID, d.cand.ver.Version
	s.RepositoryID, s.RepositoryName = d.cand.src.ID, d.cand.src.Name
	if d.cand.pkg != nil {
		s.SizeBytes = d.cand.pkg.SizeBytes
	}
	if d.cand.src.ID != r.in.Request.RepositoryID {
		s.Marks = append(s.Marks, MarkOtherRepository) // S2
	}
	if d.cand.src.Local {
		s.Marks = append(s.Marks, MarkLocalNetwork)
	}
	if d.cand.src.Stale {
		s.Marks = append(s.Marks, MarkStaleRepository) // S14
	}
	if d.action == "update" {
		for _, dep := range r.installedDependents(d.id, inPlan) {
			if t := r.installed[dep.by]; t != nil && t.Enabled {
				s.Dependents = appendUnique(s.Dependents, dep.by)
			}
		}
		if inst != nil && inst.Running && len(s.Dependents) > 0 {
			s.Marks = append(s.Marks, MarkRestarts) // S15
		}
		// an update keeps the tool disabled, and a disabled tool does not satisfy what needs it: it is enabled right after
		if inst != nil && !inst.Enabled {
			s.EnableAfter = true
			s.Marks = append(s.Marks, MarkWasDisabled)
		}
	}
	return s
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}
