package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// The tools shipped in this repository must pass the same validation an install
// applies, so a bad widgets[] or routes[] edit fails here, not on a user's machine.
func TestShippedToolManifestsAreValid(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "..", "..", "tools", "*", "manifest.json"))
	if len(files) == 0 {
		t.Skip("no tool manifests found (not running inside the repository)")
	}
	installed := map[string]InstalledToolInfo{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		m, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		installed[m.Slug] = InstalledToolInfo{Version: m.Version, Enabled: true}
	}
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		m, _ := Parse(raw)
		if _, err := Validate(m, ValidationInput{HasBackendExecutable: true, HasFrontendBundle: true, CurrentAppVersion: "999.0.0", InstalledTools: installed}); err != nil {
			t.Errorf("%s: %v", filepath.Base(filepath.Dir(f)), err)
		}
	}
}
