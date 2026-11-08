package instruction_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const module = "career-tool-backend/"

// imports returns every import path of the non-test and test Go files under dir (recursively), keyed by file.
func imports(t *testing.T, dir string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, im := range f.Imports {
			out[path] = append(out[path], strings.Trim(im.Path.Value, `"`))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Engines never import each other, and the neutral packages never import an engine or the domain code (plan 192, decoupling rules).
func TestEnginesAreDecoupled(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	engineDirs := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() && e.Name() != "enginetest" {
			engineDirs[e.Name()] = true
		}
	}
	for dir := range engineDirs {
		for file, imps := range imports(t, dir) {
			for _, imp := range imps {
				if strings.HasPrefix(imp, module+"instruction/") {
					other := strings.SplitN(strings.TrimPrefix(imp, module+"instruction/"), "/", 2)[0]
					if other != dir && other != "enginetest" {
						t.Errorf("%s imports the engine package %s", file, imp)
					}
				}
			}
		}
	}
	forbidden := []string{"portal", "crawl", "jobs", "autodiscovery"}
	check := func(file string, imps []string) {
		for _, imp := range imps {
			if strings.HasPrefix(imp, module+"instruction/") && !strings.HasSuffix(file, "_test.go") && !strings.Contains(file, "enginetest") {
				t.Errorf("%s (neutral) imports the engine package %s", file, imp)
			}
			for _, f := range forbidden {
				if imp == module+f || strings.HasPrefix(imp, module+f+"/") {
					t.Errorf("%s (neutral) imports the domain package %s", file, imp)
				}
			}
		}
	}
	for file, imps := range imports(t, "enginetest") {
		for _, imp := range imps {
			if strings.HasPrefix(imp, module+"instruction/") {
				t.Errorf("%s imports the engine package %s", file, imp)
			}
		}
		check(file, imps)
	}
	top, _ := filepath.Glob("*.go")
	for _, file := range top {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		var imps []string
		for _, im := range f.Imports {
			imps = append(imps, strings.Trim(im.Path.Value, `"`))
		}
		check(file, imps)
	}
}
