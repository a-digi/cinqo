package filesource

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/a-digi/cinqo/src/filereader/source"
	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

type fakeFinder map[string]*tool_entity.Tool

func (f fakeFinder) FindBySlug(slug string) (*tool_entity.Tool, error) {
	if t, ok := f[slug]; ok {
		return t, nil
	}
	return nil, errors.New("no rows")
}

const goodManifest = `{"slug":"demo","name":"Demo","version":"1.0.0",
 "scopes":[{"scope":"tool:demo:read","description":"read"}],
 "file_sources":[{"id":"logs","dir":"logs","scope":"tool:demo:read"},{"id":"bad","dir":"db","scope":"tool:demo:read"},{"id":"foreign","dir":"logs","scope":"cinqo:super:admin"}]}`

type env struct {
	dataDir string
	install string
	finder  fakeFinder
	resolve source.Resolver
}

func newEnv(t *testing.T, manifestJSON string, enabled bool) *env {
	t.Helper()
	dataDir := t.TempDir()
	install := filepath.Join(dataDir, "tools", "demo")
	os.MkdirAll(install, 0o755)
	os.WriteFile(filepath.Join(install, "manifest.json"), []byte(manifestJSON), 0o600)
	logs := filepath.Join(dataDir, "logs", "tools", "demo")
	os.MkdirAll(logs, 0o755)
	os.WriteFile(filepath.Join(logs, "app.log"), []byte("line one\n"), 0o600)
	e := &env{dataDir: dataDir, install: install}
	e.finder = fakeFinder{"demo": {Slug: "demo", Enabled: enabled, InstallPath: install}}
	e.resolve = NewResolver(e.finder, dataDir)
	return e
}

func TestResolvesADeclaredSource(t *testing.T) {
	e := newEnv(t, goodManifest, true)
	s, ok := e.resolve("tool:demo:logs")
	if !ok || s.ID() != "tool:demo:logs" || len(s.Scopes()) != 1 || s.Scopes()[0] != "tool:demo:read" {
		t.Fatalf("%v %v", s, ok)
	}
	p, err := s.Resolve("app.log")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(e.dataDir, "logs", "tools", "demo", "app.log"))
	if p != want {
		t.Fatalf("got %s want %s", p, want)
	}
	if _, err := s.Resolve("../../db/x"); err == nil {
		t.Fatal("escaped the tool's folder")
	}
}

func TestRefusesWhatWasNotDeclaredOrIsNotValid(t *testing.T) {
	e := newEnv(t, goodManifest, true)
	for _, id := range []string{
		"tool:demo:missing",    // not declared
		"tool:demo:bad",        // dir "db" is never allowed, even if the file says so
		"tool:demo:foreign",    // a scope the tool does not declare
		"tool:other:logs",      // no such tool
		"tool:demo",            // malformed
		"tool:demo:logs:extra", // malformed
		"tool::logs", "tool:Demo:logs", "tool:demo:Logs", "tool:../x:logs", "tool:demo:../logs", "ai-trace", "", "demo:logs",
	} {
		if s, ok := e.resolve(id); ok {
			t.Errorf("%q resolved to %v", id, s)
		}
	}
}

func TestADisabledToolHasNoSources(t *testing.T) {
	e := newEnv(t, goodManifest, false)
	if _, ok := e.resolve("tool:demo:logs"); ok {
		t.Fatal("a disabled tool's source resolved")
	}
	e.finder["demo"].Enabled = true
	if _, ok := e.resolve("tool:demo:logs"); !ok {
		t.Fatal("re-enabled tool's source did not resolve")
	}
}

func TestAnUpdatedManifestTakesEffectAtOnce(t *testing.T) {
	e := newEnv(t, goodManifest, true)
	if _, ok := e.resolve("tool:demo:logs"); !ok {
		t.Fatal("setup")
	}
	updated := `{"slug":"demo","name":"Demo","version":"1.0.1","scopes":[{"scope":"tool:demo:read","description":"read"}],"file_sources":[]}`
	os.WriteFile(filepath.Join(e.install, "manifest.json"), []byte(updated), 0o600)
	if _, ok := e.resolve("tool:demo:logs"); ok {
		t.Fatal("a source removed by an update still resolves")
	}
}

func TestAManifestOfAnotherSlugOrGarbageIsRefused(t *testing.T) {
	other := `{"slug":"other","name":"X","version":"1.0.0","scopes":[{"scope":"tool:other:read","description":"r"}],"file_sources":[{"id":"logs","dir":"logs","scope":"tool:other:read"}]}`
	if _, ok := newEnv(t, other, true).resolve("tool:demo:logs"); ok {
		t.Fatal("a manifest whose slug differs resolved")
	}
	if _, ok := newEnv(t, "not json", true).resolve("tool:demo:logs"); ok {
		t.Fatal("garbage resolved")
	}
	e := newEnv(t, goodManifest, true)
	os.Remove(filepath.Join(e.install, "manifest.json"))
	if _, ok := e.resolve("tool:demo:logs"); ok {
		t.Fatal("a missing manifest resolved")
	}
}

func TestIntegratesWithTheRegistry(t *testing.T) {
	e := newEnv(t, goodManifest, true)
	reg := source.NewRegistry()
	reg.AddResolver(source.ToolPrefix, e.resolve)
	s, ok := reg.Get("tool:demo:logs")
	if !ok {
		t.Fatal("not found through the registry")
	}
	if source.Allowed(s, []string{"cinqo:conversation:use"}) || !source.Allowed(s, []string{"tool:demo:read"}) || !source.Allowed(s, []string{source.AdminScope}) {
		t.Fatal("scope check through the registry")
	}
}
