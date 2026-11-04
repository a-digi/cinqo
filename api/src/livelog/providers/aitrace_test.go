package providers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/livelog"
)

const (
	conv = "11111111-2222-4333-8444-555555555555"
	turn = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	ref  = conv + "/" + turn
)

func setup(t *testing.T) (root, file string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "conversations")
	if err := os.MkdirAll(filepath.Join(root, conv), 0o755); err != nil {
		t.Fatal(err)
	}
	file = filepath.Join(root, conv, turn+".txt")
	if err := os.WriteFile(file, []byte("=== Iteration 1 ===\nrequest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, file
}

func TestTheProviderIsDescribedAndAdminOnly(t *testing.T) {
	p := NewAITrace(t.TempDir())
	if p.ID() != "ai-trace" || p.Title() == "" || !reflect.DeepEqual(p.Scopes(), []string{livelog.AdminScope}) {
		t.Errorf("%s %q %v", p.ID(), p.Title(), p.Scopes())
	}
	reg := livelog.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatalf("it must be registrable: %v", err)
	}
	if got := reg.List([]string{"cinqo:conversation:use"}); len(got) != 0 {
		t.Errorf("a non-admin sees it: %v", got)
	}
	if got := reg.List([]string{livelog.AdminScope}); len(got) != 1 {
		t.Errorf("admin: %v", got)
	}
}

func TestAReferenceOpensTheTraceFileAndItsGrowthIsFollowed(t *testing.T) {
	root, file := setup(t)
	f, err := NewAITrace(root).Open(ref)
	if err != nil {
		t.Fatal(err)
	}
	b, err := livelog.Poll(context.Background(), f, "", 10, livelog.DefaultLimits())
	if err != nil || len(b.Lines) != 2 || b.Lines[0].Text != "=== Iteration 1 ===" {
		t.Fatalf("%+v %v", b, err)
	}
	fh, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600) // the runner appends per model call
	fh.WriteString("=== Iteration 2 ===\n")
	fh.Close()
	next, err := livelog.Poll(context.Background(), f, b.Cursor, 10, livelog.DefaultLimits())
	if err != nil || len(next.Lines) != 1 || next.Lines[0].Text != "=== Iteration 2 ===" {
		t.Fatalf("%+v %v", next, err)
	}
}

func TestEveryWayAReferenceCanFailIsTheSameNotFound(t *testing.T) {
	root, _ := setup(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("secret\n"), 0o600)
	if err := os.Symlink(outside, filepath.Join(root, conv, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb.txt")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	os.MkdirAll(filepath.Join(root, conv, "cccccccc-cccc-4ccc-8ccc-cccccccccccc.txt"), 0o755) // a directory with a trace name
	p := NewAITrace(root)
	for name, r := range map[string]string{
		"empty":             "",
		"no slash":          conv,
		"three parts":       ref + "/x",
		"traversal":         conv + "/../../../etc/passwd",
		"dot dot":           "../" + turn,
		"upper case":        strings.ToUpper(ref),
		"not a uuid":        "conv/turn",
		"missing turn":      conv + "/bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbb99",
		"missing conv":      "99999999-2222-4333-8444-555555555555/" + turn,
		"symlink out":       conv + "/bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		"a directory":       conv + "/cccccccc-cccc-4ccc-8ccc-cccccccccccc",
		"nul byte":          conv + "/" + turn[:30] + "\x00",
		"backslash":         conv + `\` + turn,
		"trailing slash":    ref + "/",
		"with an extension": ref + ".txt",
	} {
		f, err := p.Open(r)
		if !errors.Is(err, livelog.ErrNotFound) || f != nil {
			t.Errorf("%s: %v %v", name, f, err)
		}
	}
}
