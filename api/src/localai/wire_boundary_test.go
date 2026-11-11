package localai

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The local AI and the remote platforms do not share code (plan 253, step 1): no file of this module (tests aside) imports another platform's package. Only the
// neutral contract, platform/chatcompleter (and the platform registry the module registers itself in), may be imported.
var forbiddenPlatformImports = []string{
	"github.com/a-digi/cinqo/src/platform/openai",
	"github.com/a-digi/cinqo/src/platform/openrouter",
	"github.com/a-digi/cinqo/src/platform/anthropic",
}

func TestTheLocalAIImportsNoRemotePlatform(t *testing.T) {
	var checked int
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		checked++
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Errorf("%s: %v", path, perr)
			return nil
		}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbiddenPlatformImports {
				if p == bad || strings.HasPrefix(p, bad+"/") {
					t.Errorf("%s imports %s: the local AI has its own client and must not use a remote platform's code", filepath.ToSlash(path), p)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 20 {
		t.Errorf("only %d files were checked: the rule is not looking at the module", checked)
	}
}
