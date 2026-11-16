package manifest

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The tools shipped in this repository must pass the same validation an install
// applies, so a bad widgets[] or routes[] edit fails here, not on a user's machine.
func TestShippedToolManifestsAreValid(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "..", "..", "tools", "*", "manifest.json"))
	if len(files) == 0 {
		t.Skip("no tool manifests found (not running inside the repository)")
	}
	installed := map[string]InstalledToolInfo{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		m, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		installed[m.Slug] = InstalledToolInfo{Version: m.Version, Enabled: true, BackgroundRoutes: backgroundRoutesOf(m)}
	}
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		m, _ := Parse(raw)
		if _, err := Validate(m, ValidationInput{HasBackendExecutable: true, HasFrontendBundle: true, CurrentAppVersion: "999.0.0", InstalledTools: installed}); err != nil {
			t.Errorf("%s: %v", filepath.Base(filepath.Dir(f)), err)
		}
	}
}

// backgroundRoutesOf is what the install pipeline would cache for a tool: its routes marked allow_background, as "METHOD path_suffix".
func backgroundRoutesOf(m Manifest) map[string]struct{} {
	out := map[string]struct{}{}
	for _, r := range m.Routes {
		if r.AllowBackground {
			out[r.Method+" "+r.PathSuffix] = struct{}{}
		}
	}
	return out
}

func shipped(t *testing.T, tool string) Manifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "tools", tool, "manifest.json"))
	if err != nil {
		t.Skip("tool manifests not found (not running inside the repository)")
	}
	m, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// The routes of the Browser that may be used without a signed-in person are exactly these: the read-only crawl routes Namako's unattended monitoring needs and the
// call that stops one of its own crawls. Everything that needs a person (the login window and session, the security settings, the media download) and everything
// that writes or reads evidence stays out. Adding a route here is a decision, so it fails this test until the list is changed on purpose.
func TestTheBrowserAllowsExactlyTheFourCrawlRoutesInTheBackground(t *testing.T) {
	b := shipped(t, "browser")
	var got []string
	for _, r := range b.Routes {
		if r.AllowBackground {
			got = append(got, r.Method+" "+r.PathSuffix)
			if r.RequiredScope != "tool:browser:crawl" {
				t.Errorf("%s %s is allowed in the background but needs %s: only the crawl scope belongs here", r.Method, r.PathSuffix, r.RequiredScope)
			}
			if r.AllowCapabilityToken {
				t.Errorf("%s %s mixes both ways around a signed-in person", r.Method, r.PathSuffix)
			}
		}
	}
	sort.Strings(got)
	want := []string{"POST capture-network", "POST crawl-cancel", "POST fetch-frames", "POST list-links"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("allowed in the background: %v, want %v", got, want)
	}
	for _, r := range b.Routes {
		for _, banned := range []string{"login", "security", "media", "page-archive", "fetch-manifest", "capture-file", "capture", "katana-window", "browser-settings", "temp-files", "domains"} {
			if r.AllowBackground && strings.HasPrefix(r.PathSuffix, banned) && r.PathSuffix != "capture-network" {
				t.Errorf("%s must never be used without a signed-in person", r.PathSuffix)
			}
		}
	}
}

// Namako asks for exactly those four, nothing that writes evidence, and the install rules accept it against the real Browser manifest (and refuse it when the Browser
// stops marking a route).
func TestNamakoAsksForExactlyTheBrowserRoutesItNeedsUnattendedAndTheRulesAcceptIt(t *testing.T) {
	b, n := shipped(t, "browser"), shipped(t, "namako")
	var asked []string
	for _, d := range n.BackgroundAccess {
		if d.Tool != "browser" {
			t.Errorf("unexpected target %s", d.Tool)
		}
		for _, r := range d.Routes {
			asked = append(asked, r.Method+" "+r.PathSuffix)
		}
	}
	sort.Strings(asked)
	if strings.Join(asked, ",") != "POST capture-network,POST crawl-cancel,POST fetch-frames,POST list-links" {
		t.Errorf("%v", asked)
	}
	for _, a := range asked {
		for _, attended := range []string{"page-archive", "fetch-manifest", "capture-file", "fetch-media", "relay", "media-file", "login", "security"} {
			if strings.Contains(a, attended) {
				t.Errorf("%s is a person's action", a)
			}
		}
	}
	installed := map[string]InstalledToolInfo{"browser": {Version: b.Version, Enabled: true, BackgroundRoutes: backgroundRoutesOf(b)}}
	if err := ValidateBackground(n, installed, true); err != nil {
		t.Errorf("the real manifests must pass the real rules: %v", err)
	}
	// the Browser stops marking one of them: Namako's declaration is refused, naming the route
	stripped := map[string]struct{}{}
	for k := range installed["browser"].BackgroundRoutes {
		if k != "POST list-links" {
			stripped[k] = struct{}{}
		}
	}
	err := ValidateBackground(n, map[string]InstalledToolInfo{"browser": {Version: b.Version, Enabled: true, BackgroundRoutes: stripped}}, true)
	if err == nil || !strings.Contains(err.Error(), "POST list-links") {
		t.Errorf("%v", err)
	}
	// and the versions that make it work are required: the browser that marks the routes, the core that knows the rules
	if n.RequiresTools[0].Slug != "browser" || n.RequiresTools[0].MinVersion != "0.0.269" {
		t.Errorf("%+v", n.RequiresTools)
	}
	if cmp, _ := CompareVersions(b.Version, "0.0.269"); cmp < 0 {
		t.Errorf("the Browser that marks the routes and keeps a stand-in for a player in its page copies is 0.0.269 or newer: %s", b.Version)
	}
	if n.MinAppVersion != "0.0.4" {
		t.Errorf("Namako needs the core that has unattended calls (api/VERSION 0.0.4, which also names the organisations to work for): %q", n.MinAppVersion)
	}
}
