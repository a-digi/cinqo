package livelog

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const modulePath = "github.com/a-digi/cinqo/src/livelog"

// Only these files may import the module, and each only the packages it needs: the composition root names the concrete providers and
// installs the handler's registry; the router merge takes the two handler functions and nothing else. Everything else in the program
// must not know the module exists, so deleting it stays a deletion (plan/ai/file-reader/live-follow.md, the decoupling contract).
var allowedImporters = map[string][]string{
	"src/cinqo/livelog.go":    {modulePath, modulePath + "/handler", modulePath + "/providers"},
	"config/routes/routes.go": {modulePath + "/handler"},
}

func apiRoot() string { return filepath.Join("..", "..") } // the api/ directory

func importsOf(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	out := make([]string, 0, len(file.Imports))
	for _, imp := range file.Imports {
		out = append(out, strings.Trim(imp.Path.Value, `"`))
	}
	return out
}

func TestNothingOutsideTheModuleImportsIt(t *testing.T) {
	root := apiRoot()
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
		if strings.HasPrefix(rel, "src/livelog/") {
			return nil
		}
		for _, p := range importsOf(t, path) {
			if p == modulePath || strings.HasPrefix(p, modulePath+"/") {
				if !contains(allowedImporters[rel], p) {
					t.Errorf("%s imports %s: only %v may import the live log module, and each only %v", rel, p, sortedKeys(allowedImporters), allowedImporters[rel])
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The core (this folder's own files) and the test fake need nothing of the program: standard library only, the fake also this
// package. That is what lets the module be lifted out or deleted without a thread to pull. A provider or the handler (their own
// sub-folders) may import what they wrap or serve; the core may not.
func TestTheCoreImportsNothingOfTheProgram(t *testing.T) {
	for _, dir := range []string{".", "livelogtest"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no Go files in %s (err %v)", dir, err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue // the tests use the fake, on purpose
			}
			for _, p := range importsOf(t, f) {
				first := strings.SplitN(p, "/", 2)[0]
				isStd := !strings.Contains(first, ".")
				isThisModule := p == modulePath
				if !isStd && !isThisModule {
					t.Errorf("%s imports %s: the live log core may import only the standard library%s", f, p, map[bool]string{true: "", false: " (and, for the fake, the core)"}[dir == "."])
				}
			}
		}
	}
	// the core may not even import its own sub-folders: it must not depend on a concrete provider or on the transport
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		for _, p := range importsOf(t, f) {
			if strings.HasPrefix(p, modulePath+"/") {
				t.Errorf("%s imports %s: the core must not depend on its sub-packages", f, p)
			}
		}
	}
}

// The file follower knows nothing of the program either: the standard library and the core, nothing else (no sources, no scopes, no
// HTTP, and above all not the file reader), so a provider that wraps some other kind of log can sit beside it and the file reader can
// be removed or rewritten without it noticing.
func TestTheFileFollowerImportsOnlyTheStandardLibraryAndTheCore(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("filetail", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no Go files in filetail (err %v)", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		for _, p := range importsOf(t, f) {
			first := strings.SplitN(p, "/", 2)[0]
			if strings.Contains(first, ".") && p != modulePath {
				t.Errorf("%s imports %s: the file follower may import only the standard library and the core", f, p)
			}
			if strings.HasPrefix(p, "net/") {
				t.Errorf("%s imports %s: the file follower is transport-free", f, p)
			}
		}
	}
}

// The transport-free rule: the core never mentions HTTP, so a provider or a different transport can sit on top of it.
func TestTheCoreIsFreeOfTransport(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		for _, p := range importsOf(t, f) {
			if strings.HasPrefix(p, "net/") {
				t.Errorf("%s imports %s: the core is transport-free", f, p)
			}
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- the frontend side of the same rules (plan/ai/file-reader/live-follow.md, the decoupling contract) ----

const frontendModuleDir = "Components/LiveLogs"

// Only these frontend files may import the module's folder, and each only the files listed: the AI Logs viewer modal takes the one component that
// offers a live view (LiveLogButton). Everything else must not know the folder exists, so deleting it stays a deletion: remove this file's import
// line and the folder, and the modal shows the file reader's view alone.
var allowedFrontendImporters = map[string][]string{
	"Components/Admin/Conversation/AiLogViewerModal.tsx": {frontendModuleDir + "/LiveLogButton"},
}

// What the module itself may import: React, its own files, and the shared fetch client. Nothing else of the application (not the file
// reader's client, not the Terminal viewer's internals), so the folder can be lifted out or deleted without a thread to pull.
func frontendImportAllowedInside(spec, resolved string) bool {
	switch {
	case spec == "react":
		return true
	case strings.HasPrefix(spec, "./"):
		return true
	case resolved == "api/client":
		return true
	}
	return false
}

var importSpec = regexp.MustCompile(`(?m)(?:from|import)\s*\(?\s*['"]([^'"]+)['"]`)

func frontendRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "frontend", "src")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("no frontend directory next to the api (%v)", err)
	}
	return root
}

func TestNoFrontendFileOutsideTheModuleImportsIt(t *testing.T) {
	root := frontendRoot(t)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || (!strings.HasSuffix(p, ".ts") && !strings.HasSuffix(p, ".tsx")) {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, frontendModuleDir+"/") {
			return nil
		}
		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for _, m := range importSpec.FindAllStringSubmatch(string(raw), -1) {
			spec := m[1]
			if !strings.HasPrefix(spec, ".") {
				continue
			}
			resolved := path.Clean(path.Join(path.Dir(rel), spec))
			if resolved == frontendModuleDir || strings.HasPrefix(resolved, frontendModuleDir+"/") {
				if !contains(allowedFrontendImporters[rel], resolved) {
					t.Errorf("%s imports %s: only %v may import the live log folder, and each only %v", rel, spec, sortedKeys(allowedFrontendImporters), allowedFrontendImporters[rel])
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The frontend half is optional: with its folder removed the backend module stands alone, and these two tests, which are about that folder,
// have nothing to check (the removal drill, step 8 of the plan, depends on it).
func frontendModule(t *testing.T) (root, dir string) {
	t.Helper()
	root = frontendRoot(t)
	dir = filepath.Join(root, filepath.FromSlash(frontendModuleDir))
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("the frontend half of the module is not there (%v)", err)
	}
	return root, dir
}

func TestTheFrontendModuleImportsOnlyReactItsOwnFilesAndTheFetchClient(t *testing.T) {
	root, dir := frontendModule(t)
	files, err := filepath.Glob(filepath.Join(dir, "*.ts*"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no files in %s (err %v)", dir, err)
	}
	for _, f := range files {
		raw, rerr := os.ReadFile(f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		rel, _ := filepath.Rel(root, f)
		rel = filepath.ToSlash(rel)
		for _, m := range importSpec.FindAllStringSubmatch(string(raw), -1) {
			spec := m[1]
			resolved := ""
			if strings.HasPrefix(spec, ".") {
				resolved = path.Clean(path.Join(path.Dir(rel), spec))
			}
			if !frontendImportAllowedInside(spec, resolved) {
				t.Errorf("%s imports %q: the live log folder may import only react, its own files and the shared fetch client (api/client)", rel, spec)
			}
		}
	}
}

// The pure files stay free of React and of the network: only the hook and the API client know them.
func TestThePureFilesImportNeitherReactNorTheClient(t *testing.T) {
	_, dir := frontendModule(t)
	for _, name := range []string{"tailRules.ts", "followLoop.ts", "types.ts", "scrollRules.ts", "highlight.ts"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range importSpec.FindAllStringSubmatch(string(raw), -1) {
			if m[1] == "react" || strings.Contains(m[1], "api/") {
				t.Errorf("%s imports %q: the pure files import neither React nor the network client", name, m[1])
			}
		}
	}
}
