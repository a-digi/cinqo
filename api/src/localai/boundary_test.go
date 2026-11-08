package localai

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/a-digi/cinqo/src/localai"

// Only these files may import the module: the one start-up hook and the one place handlers are merged into the router.
// Everything else in the program must not know the module exists, so deleting it stays a deletion (plan 181, decision 11).
var allowedImporters = map[string]bool{
	"src/cinqo/app.go":        true,
	"config/routes/routes.go": true,
}

func TestNothingOutsideTheModuleImportsIt(t *testing.T) {
	root := filepath.Join("..", "..") // the api/ directory
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "embeddedconfig" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "src/localai/") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Errorf("%s: %v", rel, perr)
			return nil
		}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if (p == modulePath || strings.HasPrefix(p, modulePath+"/")) && !allowedImporters[rel] {
				t.Errorf("%s imports %s: only %v may import the local AI module", rel, p, keys(allowedImporters))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The frontend side of the same rule: only the menu configuration, the router and the Platforms page (which hands the
// module's slide to its slider) may import the module's folder.
var allowedFrontendImporters = map[string]bool{
	"config/menu/menu.ts":                          true,
	"config/routing/routes.tsx":                    true,
	"Components/Admin/Platforms/PlatformsPage.tsx": true,
}

const frontendModuleDir = "Components/Admin/LocalAi"

func TestNoFrontendFileOutsideTheModuleImportsIt(t *testing.T) {
	root := filepath.Join("..", "..", "..", "frontend", "src")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("no frontend directory next to the api (%v)", err)
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, frontendModuleDir+"/") {
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, line := range strings.Split(string(raw), "\n") {
			trimmed := strings.TrimSpace(line)
			isImport := strings.HasPrefix(trimmed, "import ") || strings.Contains(trimmed, "from '") || strings.Contains(trimmed, `from "`)
			if isImport && strings.Contains(trimmed, "Admin/LocalAi/") && !allowedFrontendImporters[rel] {
				t.Errorf("%s imports the local AI folder: only %v may", rel, keys(allowedFrontendImporters))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
