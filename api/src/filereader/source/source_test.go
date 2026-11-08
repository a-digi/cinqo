package source

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	conv = "11111111-2222-4333-8444-555555555555"
	turn = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

func setup(t *testing.T) (root string, src Source) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "conversations")
	if err := os.MkdirAll(filepath.Join(root, conv), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, conv, turn+".txt"), []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, NewAITrace(root)
}

func TestAITraceResolvesAValidReference(t *testing.T) {
	root, src := setup(t)
	got, err := src.Resolve(conv + "/" + turn)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(root, conv, turn+".txt"))
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAITraceRefusesEverythingElse(t *testing.T) {
	root, src := setup(t)
	// a second conversation with a file outside what a reference may name
	secret := filepath.Join(filepath.Dir(root), "secret.txt")
	os.WriteFile(secret, []byte("secret"), 0o600)

	bad := []string{
		"",
		conv,
		conv + "/",
		"/" + turn,
		conv + "/" + turn + "/extra",
		"../" + turn,
		conv + "/../" + turn,
		conv + "/..",
		"..%2f..%2fsecret/" + turn,
		conv + "/" + strings.ToUpper(turn), // not the lowercase shape
		conv + "/" + turn + ".txt",         // the extension is added by the source, never sent
		"/etc/passwd",
		`..\..\secret`,
		conv + "\x00/" + turn,
		conv + "/" + strings.Repeat("a", 5000),
		"11111111-2222-4333-8444-555555555555/bbbbbbbb-bbbb-4ccc-8ddd-eeeeeeeeeeee", // well formed, no such file
	}
	for _, ref := range bad {
		if p, err := src.Resolve(ref); !errors.Is(err, ErrNotFound) {
			t.Errorf("ref %q resolved to %q (err %v)", ref, p, err)
		}
	}
}

func TestAITraceRefusesASymlinkOutOfTheRoot(t *testing.T) {
	root, src := setup(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	os.WriteFile(outside, []byte("secret"), 0o600)
	link := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	if err := os.Symlink(outside, filepath.Join(root, conv, link+".txt")); err != nil {
		t.Skip("symlinks not available:", err)
	}
	if _, err := src.Resolve(conv + "/" + link); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a symlink out of the root resolved: %v", err)
	}
	// a whole conversation folder that is a link to elsewhere
	otherDir := t.TempDir()
	os.WriteFile(filepath.Join(otherDir, turn+".txt"), []byte("x"), 0o600)
	linkedConv := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	os.Symlink(otherDir, filepath.Join(root, linkedConv))
	if _, err := src.Resolve(linkedConv + "/" + turn); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a linked folder resolved: %v", err)
	}
}

func TestAITraceRefusesADirectory(t *testing.T) {
	root, src := setup(t)
	dir := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	os.MkdirAll(filepath.Join(root, conv, dir+".txt"), 0o755)
	if _, err := src.Resolve(conv + "/" + dir); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a directory resolved: %v", err)
	}
}

func TestMissingRootIsNotFound(t *testing.T) {
	if _, err := NewAITrace(filepath.Join(t.TempDir(), "nope")).Resolve(conv + "/" + turn); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestAllowed(t *testing.T) {
	_, src := setup(t)
	cases := []struct {
		scopes []string
		want   bool
	}{
		{nil, false},
		{[]string{"cinqo:conversation:use"}, false},
		{[]string{AdminScope}, true},
		{[]string{"x", AdminScope}, true},
	}
	for _, c := range cases {
		if got := Allowed(src, c.scopes); got != c.want {
			t.Errorf("scopes %v: got %v", c.scopes, got)
		}
	}
	tool := fakeSource{scopes: []string{"tool:a:read"}}
	if !Allowed(tool, []string{"tool:a:read"}) || Allowed(tool, []string{"tool:b:read"}) {
		t.Fatal("a tool source's own scope must be honoured and no other")
	}
}

type fakeSource struct{ scopes []string }

func (fakeSource) ID() string                     { return "tool:a:x" }
func (t fakeSource) Scopes() []string             { return t.scopes }
func (fakeSource) Resolve(string) (string, error) { return "", ErrNotFound }

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	_, src := setup(t)
	if err := r.Register(src); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(src); err == nil {
		t.Fatal("a duplicate id must be refused")
	}
	if err := r.Register(fakeSource{}); err == nil {
		t.Fatal("a source without scopes must be refused")
	}
	if _, ok := r.Get(AITraceID); !ok {
		t.Fatal("registered source not found")
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("unknown source found")
	}
}
