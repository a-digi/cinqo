package resolver

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/toolmeta"
	"github.com/a-digi/cinqo/src/toolrepository/catalog"
)

var machine = Env{OS: "darwin", Arch: "arm64", AppVersion: "1.0.0"}

// ver builds one version of a tool, offered for every platform.
func ver(version string, reqs ...catalog.Requirement) catalog.Version {
	return catalog.Version{VersionID: "v" + version, Name: "Tool", Version: version, Requires: reqs,
		Packages: []catalog.Package{{OS: "any", Arch: "any", URL: "https://x/" + version + ".zip", SHA256: "sha-" + version, SizeBytes: 100}}}
}

func withAuthors(v catalog.Version, names ...string) catalog.Version {
	for _, n := range names {
		v.Info = toolmeta.Info{Authors: append(v.Info.Authors, toolmeta.Author{Name: n})}
	}
	return v
}

func withSHA(v catalog.Version, sha string) catalog.Version {
	v.Packages = []catalog.Package{{OS: "any", Arch: "any", URL: "https://x/" + v.Version + ".zip", SHA256: sha}}
	return v
}

func needs(id string, bounds ...string) catalog.Requirement {
	r := catalog.Requirement{ID: id}
	if len(bounds) > 0 {
		r.MinVersion = bounds[0]
	}
	if len(bounds) > 1 {
		r.MaxVersion = bounds[1]
	}
	return r
}

// tl builds a tool; the current version is the first one given unless a version is named "current:<v>".
func tl(id string, versions ...catalog.Version) catalog.Tool {
	return catalog.Tool{ID: id, CurrentVersionID: versions[0].VersionID, Versions: versions}
}

func cat(tools ...catalog.Tool) *catalog.Catalog {
	return &catalog.Catalog{FormatVersion: 2, Tools: tools}
}

func repo(id, name string, tools ...catalog.Tool) Source {
	return Source{ID: id, Name: name, Catalog: cat(tools...)}
}

func installed(id, version string) InstalledTool {
	return InstalledTool{ID: id, Name: id, Version: version, Enabled: true}
}

func (i InstalledTool) disabled() InstalledTool { i.Enabled = false; return i }
func (i InstalledTool) running() InstalledTool  { i.Running = true; return i }
func (i InstalledTool) from(repo string) InstalledTool {
	i.SourceRepositoryID = repo
	return i
}
func (i InstalledTool) by(names ...string) InstalledTool { i.Authors = names; return i }
func (i InstalledTool) needs(reqs ...Requirement) InstalledTool {
	i.Requires = reqs
	return i
}

func req(id string, bounds ...string) Requirement {
	r := Requirement{ID: id}
	if len(bounds) > 0 {
		r.Min = bounds[0]
	}
	if len(bounds) > 1 {
		r.Max = bounds[1]
	}
	return r
}

func ask(toolID string, sources []Source, inst ...InstalledTool) Input {
	return Input{Installed: inst, Sources: sources, Env: machine, Request: Request{RepositoryID: sources[0].ID, ToolID: toolID}}
}

// summary writes a plan as "browser:install@0.0.2 career:update@1.0.0" (enable steps as "pdf:enable"), or the problem kinds as "!kind(tool)".
func summary(p Plan) string {
	if !p.OK {
		var parts []string
		for _, pr := range p.Problems {
			parts = append(parts, fmt.Sprintf("!%s(%s)", pr.Kind, pr.ToolID))
		}
		return strings.Join(parts, " ")
	}
	var parts []string
	for _, s := range p.Steps {
		if s.Action == ActionEnable {
			parts = append(parts, s.ToolID+":enable")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%s@%s", s.ToolID, s.Action, s.Version))
	}
	return strings.Join(parts, " ")
}

func mustPlan(t *testing.T, in Input, want string) Plan {
	t.Helper()
	p := Resolve(in)
	if got := summary(p); got != want {
		t.Fatalf("plan:\n got  %s\n want %s\n problems: %+v", got, want, p.Problems)
	}
	return p
}

func stepOf(p Plan, id string) Step {
	for _, s := range p.Steps {
		if s.ToolID == id {
			return s
		}
	}
	return Step{}
}

func hasMark(s Step, m Mark) bool {
	for _, x := range s.Marks {
		if x == m {
			return true
		}
	}
	return false
}

func problemOf(p Plan, id string) Problem {
	for _, pr := range p.Problems {
		if pr.ToolID == id {
			return pr
		}
	}
	return Problem{}
}

func sorted(s []string) []string { out := append([]string(nil), s...); sort.Strings(out); return out }
