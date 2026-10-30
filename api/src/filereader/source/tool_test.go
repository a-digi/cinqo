package source

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func toolRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "logs", "tools", "demo")
	os.MkdirAll(filepath.Join(root, "crawl", "2026-10-31"), 0o755)
	os.WriteFile(filepath.Join(root, "app.log"), []byte("hello\n"), 0o600)
	os.WriteFile(filepath.Join(root, "crawl", "2026-10-31", "run-1.log"), []byte("deep\n"), 0o600)
	os.WriteFile(filepath.Join(root, ".hidden"), []byte("x"), 0o600)
	return root
}

func TestToolSourceIDAndScope(t *testing.T) {
	s := NewToolSource("demo", "logs", "tool:demo:read", toolRoot(t))
	if s.ID() != "tool:demo:logs" || len(s.Scopes()) != 1 || s.Scopes()[0] != "tool:demo:read" {
		t.Fatalf("%q %v", s.ID(), s.Scopes())
	}
	if !Allowed(s, []string{"tool:demo:read"}) || Allowed(s, []string{"tool:other:read"}) || Allowed(s, nil) || !Allowed(s, []string{AdminScope}) {
		t.Fatal("scope check")
	}
}

func TestToolSourceResolvesFilesAndNestedPaths(t *testing.T) {
	root := toolRoot(t)
	s := NewToolSource("demo", "logs", "tool:demo:read", root)
	for _, ref := range []string{"app.log", "crawl/2026-10-31/run-1.log"} {
		got, err := s.Resolve(ref)
		if err != nil {
			t.Fatalf("%q: %v", ref, err)
		}
		real, _ := filepath.EvalSymlinks(root)
		if !strings.HasPrefix(got, real+string(os.PathSeparator)) {
			t.Fatalf("%q resolved outside the root: %s", ref, got)
		}
	}
}

func TestToolSourceRefusesEverythingElse(t *testing.T) {
	root := toolRoot(t)
	secret := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(root))), "secret.txt")
	os.WriteFile(secret, []byte("secret"), 0o600)
	s := NewToolSource("demo", "logs", "tool:demo:read", root)
	bad := []string{
		"", "/", "/app.log", "app.log/", "//app.log", "./app.log", "../app.log", "crawl/../app.log", "..", ".", "crawl/..",
		".hidden", "crawl/.hidden", "..%2fapp.log", "%2e%2e/app.log", `..\app.log`, "app.log\x00", "app log", "app.log?x=1", "a:b",
		"crawl", "crawl/2026-10-31", // folders
		"nope.log", strings.Repeat("a", 129), strings.Repeat("a/", 7) + "a",
		"../../../../secret.txt", "/etc/passwd",
	}
	for _, ref := range bad {
		if p, err := s.Resolve(ref); !errors.Is(err, ErrNotFound) {
			t.Errorf("ref %q resolved to %q (err %v)", ref, p, err)
		}
	}
}

func TestToolSourceRefusesASymlinkOut(t *testing.T) {
	root := toolRoot(t)
	outside := filepath.Join(t.TempDir(), "out.txt")
	os.WriteFile(outside, []byte("secret"), 0o600)
	if err := os.Symlink(outside, filepath.Join(root, "link.log")); err != nil {
		t.Skip("symlinks not available")
	}
	otherDir := t.TempDir()
	os.WriteFile(filepath.Join(otherDir, "in.log"), []byte("x"), 0o600)
	os.Symlink(otherDir, filepath.Join(root, "linkdir"))
	s := NewToolSource("demo", "logs", "tool:demo:read", root)
	for _, ref := range []string{"link.log", "linkdir/in.log"} {
		if _, err := s.Resolve(ref); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q resolved through a symlink: %v", ref, err)
		}
	}
}

func TestRegistryResolvers(t *testing.T) {
	r := NewRegistry()
	calls := 0
	r.AddResolver(ToolPrefix, func(id string) (Source, bool) {
		calls++
		if id == "tool:demo:logs" {
			return NewToolSource("demo", "logs", "tool:demo:read", t.TempDir()), true
		}
		return nil, false
	})
	if s, ok := r.Get("tool:demo:logs"); !ok || s.ID() != "tool:demo:logs" {
		t.Fatal("resolver source not found")
	}
	if _, ok := r.Get("tool:demo:other"); ok {
		t.Fatal("unknown tool source found")
	}
	before := calls
	if _, ok := r.Get("ai-trace"); ok || calls != before {
		t.Fatal("a resolver must only be asked about its own prefix")
	}
	// a registered source wins and never reaches a resolver
	_, src := setup(t)
	r.Register(src)
	before = calls
	if _, ok := r.Get(AITraceID); !ok || calls != before {
		t.Fatal("registered source")
	}
}
