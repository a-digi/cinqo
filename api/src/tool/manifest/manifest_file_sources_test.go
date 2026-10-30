package manifest

import (
	"strings"
	"testing"
)

func withSources(sources ...FileSourceDecl) Manifest {
	return Manifest{
		Slug:        "demo",
		Scopes:      []ScopeDecl{{Scope: "tool:demo:read", Description: "read"}},
		FileSources: sources,
	}
}

func TestValidateFileSourcesAcceptsGoodOnes(t *testing.T) {
	m := withSources(
		FileSourceDecl{ID: "crawl-logs", Dir: "logs", Scope: "tool:demo:read"},
		FileSourceDecl{ID: "exports", Dir: "uploads", Scope: "tool:demo:read"},
	)
	if err := ValidateFileSources(m); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFileSources(Manifest{}); err != nil {
		t.Fatalf("a manifest without file sources: %v", err)
	}
}

func TestValidateFileSourcesRefusesBadOnes(t *testing.T) {
	good := FileSourceDecl{ID: "logs", Dir: "logs", Scope: "tool:demo:read"}
	cases := map[string]Manifest{
		"id upper case":      withSources(FileSourceDecl{ID: "Logs", Dir: "logs", Scope: "tool:demo:read"}),
		"id starts digit":    withSources(FileSourceDecl{ID: "1logs", Dir: "logs", Scope: "tool:demo:read"}),
		"id with colon":      withSources(FileSourceDecl{ID: "a:b", Dir: "logs", Scope: "tool:demo:read"}),
		"id with slash":      withSources(FileSourceDecl{ID: "a/b", Dir: "logs", Scope: "tool:demo:read"}),
		"id empty":           withSources(FileSourceDecl{ID: "", Dir: "logs", Scope: "tool:demo:read"}),
		"id too long":        withSources(FileSourceDecl{ID: "a" + strings.Repeat("b", 32), Dir: "logs", Scope: "tool:demo:read"}),
		"dir db":             withSources(FileSourceDecl{ID: "x", Dir: "db", Scope: "tool:demo:read"}),
		"dir traversal":      withSources(FileSourceDecl{ID: "x", Dir: "../etc", Scope: "tool:demo:read"}),
		"dir absolute":       withSources(FileSourceDecl{ID: "x", Dir: "/etc", Scope: "tool:demo:read"}),
		"dir empty":          withSources(FileSourceDecl{ID: "x", Dir: "", Scope: "tool:demo:read"}),
		"scope not declared": withSources(FileSourceDecl{ID: "x", Dir: "logs", Scope: "tool:other:read"}),
		"scope empty":        withSources(FileSourceDecl{ID: "x", Dir: "logs", Scope: ""}),
		"scope of core":      withSources(FileSourceDecl{ID: "x", Dir: "logs", Scope: "cinqo:super:admin"}),
		"duplicate id":       withSources(good, good),
	}
	many := make([]FileSourceDecl, 0, MaxFileSources+1)
	for i := 0; i <= MaxFileSources; i++ {
		many = append(many, FileSourceDecl{ID: "s" + string(rune('a'+i)), Dir: "logs", Scope: "tool:demo:read"})
	}
	cases["too many"] = withSources(many...)
	for name, m := range cases {
		if err := ValidateFileSources(m); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestValidateRunsFileSourceChecks(t *testing.T) {
	// A frontend-only tool can expose files too: core reads them, no backend route is involved.
	frontendOnly := ValidationInput{HasFrontendBundle: true, CurrentAppVersion: "1.0.0"}
	ok := withSources(FileSourceDecl{ID: "logs", Dir: "logs", Scope: "tool:demo:read"})
	ok.Name, ok.Version = "Demo", "1.0.0"
	if _, err := Validate(ok, frontendOnly); err != nil {
		t.Fatalf("a valid declaration was rejected by Validate: %v", err)
	}
	bad := withSources(FileSourceDecl{ID: "logs", Dir: "db", Scope: "tool:demo:read"})
	bad.Name, bad.Version = "Demo", "1.0.0"
	if _, err := Validate(bad, frontendOnly); err == nil {
		t.Fatal("Validate accepted the database folder")
	}
}

func TestParseReadsFileSources(t *testing.T) {
	raw := `{"slug":"demo","name":"Demo","version":"1.0.0","scopes":[{"scope":"tool:demo:read","description":"r"}],
	"file_sources":[{"id":"logs","dir":"logs","scope":"tool:demo:read"}]}`
	m, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.FileSources) != 1 || m.FileSources[0] != (FileSourceDecl{ID: "logs", Dir: "logs", Scope: "tool:demo:read"}) {
		t.Fatalf("%+v", m.FileSources)
	}
}
