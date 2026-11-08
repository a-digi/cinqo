package jsonview

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// The JSON beautifier is a module that can be removed alone (plan 202): only the file reader's handler uses it, and the composition root
// (src/cinqo) builds its cache. Nothing else in the program may import it.
func TestOnlyTheHandlerAndTheCompositionRootImportJSONView(t *testing.T) {
	const self = "github.com/a-digi/cinqo/src/filereader/jsonview"
	allowed := map[string]bool{
		filepath.Join("filereader", "handler"): true,
		"cinqo":                                true,
	}
	src := filepath.Join("..", "..") // api/src
	fset := token.NewFileSet()
	found := map[string]bool{}
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(src, filepath.Dir(path))
		if rel == filepath.Join("filereader", "jsonview") {
			return nil
		}
		af, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range af.Imports {
			if strings.Trim(imp.Path.Value, `"`) == self {
				found[rel] = true
				if !allowed[rel] {
					t.Errorf("%s imports the JSON beautifier: only the file reader's handler and the composition root may", path)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found[filepath.Join("filereader", "handler")] || !found["cinqo"] {
		t.Errorf("the handler and the composition root should both use it (found in %v): the rule is checking nothing", found)
	}
}
