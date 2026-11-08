package manifest

import (
	"encoding/json"
	"strings"
	"testing"
)

func requiresManifest(deps ...RequiredToolDecl) Manifest {
	return Manifest{Info: testInfo(), Slug: "career", Name: "Career", Version: "1.0.0", RequiresTools: deps}
}

func installed(version string, enabled bool) map[string]InstalledToolInfo {
	return map[string]InstalledToolInfo{"browser": {Version: version, Enabled: enabled}}
}

// The upper bound of a requirement (revision 2 of plan dependency-install).
func TestRequiredToolsHonourTheMaximumVersion(t *testing.T) {
	m := requiresManifest(RequiredToolDecl{Slug: "browser", MinVersion: "1.0.0", MaxVersion: "2.0.0"})
	for _, tc := range []struct {
		name, installed string
		ok              bool
	}{
		{"inside the range", "1.5.0", true},
		{"exactly the minimum", "1.0.0", true},
		{"exactly the maximum", "2.0.0", true},
		{"above the maximum", "2.0.1", false},
		{"far above the maximum", "10.0.0", false}, // numbers, not text: "10" is above "2"
		{"below the minimum", "0.9.9", false},
	} {
		err := ValidateRequiredTools(m, installed(tc.installed, true))
		if (err == nil) != tc.ok {
			t.Errorf("%s: installed %s: err = %v, want ok=%v", tc.name, tc.installed, err, tc.ok)
		}
	}
	err := ValidateRequiredTools(m, installed("3.0.0", true))
	if err == nil || !strings.Contains(err.Error(), "version <= 2.0.0") || !strings.Contains(err.Error(), "installed 3.0.0") {
		t.Errorf("the message names the bound and the installed version: %v", err)
	}
	// a maximum alone, and a maximum on a disabled tool (disabled is reported first, as before)
	maxOnly := requiresManifest(RequiredToolDecl{Slug: "browser", MaxVersion: "1.2"})
	if ValidateRequiredTools(maxOnly, installed("1.2.0", true)) != nil || ValidateRequiredTools(maxOnly, installed("1.3.0", true)) == nil {
		t.Error("a maximum alone is enforced (1.2 equals 1.2.0)")
	}
	if err := ValidateRequiredTools(m, installed("9.0.0", false)); err == nil || !strings.Contains(err.Error(), "to be enabled") {
		t.Errorf("disabled is reported before the version: %v", err)
	}
	// no maximum: any newer version still satisfies, as before
	if ValidateRequiredTools(requiresManifest(RequiredToolDecl{Slug: "browser", MinVersion: "1.0.0"}), installed("99.0.0", true)) != nil {
		t.Error("without a maximum a newer version satisfies")
	}
}

func TestValidateRequiresChecksTheDeclaration(t *testing.T) {
	good := []RequiredToolDecl{
		{Slug: "browser"},
		{Slug: "pdf_tools", MinVersion: "0.0.1"},
		{Slug: "notes", MinVersion: "1.0.0", MaxVersion: "1.0.0"}, // min equal to max: exactly one version
		{Slug: "other", MaxVersion: "2"},
	}
	if err := ValidateRequires(requiresManifest(good...)); err != nil {
		t.Fatalf("a good declaration was refused: %v", err)
	}
	if err := ValidateRequires(requiresManifest()); err != nil {
		t.Fatalf("no requirements: %v", err)
	}
	for name, tc := range map[string]struct {
		decl []RequiredToolDecl
		want string
	}{
		"bad slug":            {[]RequiredToolDecl{{Slug: "Bad-Slug"}}, "slug"},
		"empty slug":          {[]RequiredToolDecl{{Slug: ""}}, "slug"},
		"itself":              {[]RequiredToolDecl{{Slug: "career"}}, "itself"},
		"twice":               {[]RequiredToolDecl{{Slug: "browser"}, {Slug: "browser", MinVersion: "1.0.0"}}, "twice"},
		"unreadable minimum":  {[]RequiredToolDecl{{Slug: "browser", MinVersion: "1.x"}}, "min_version"},
		"unreadable maximum":  {[]RequiredToolDecl{{Slug: "browser", MaxVersion: "beta"}}, "max_version"},
		"maximum below min":   {[]RequiredToolDecl{{Slug: "browser", MinVersion: "2.0.0", MaxVersion: "1.9.9"}}, "below"},
		"numbers not text":    {[]RequiredToolDecl{{Slug: "browser", MinVersion: "10.0.0", MaxVersion: "9.0.0"}}, "below"},
		"second one is wrong": {[]RequiredToolDecl{{Slug: "browser"}, {Slug: "pdf_tools", MinVersion: "x"}}, "pdf_tools"},
	} {
		err := ValidateRequires(requiresManifest(tc.decl...))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
}

// Validate runs the declaration check before the comparison with what is installed, so a broken declaration is named as such.
func TestValidateRefusesABrokenRequiresBeforeLookingAtInstalledTools(t *testing.T) {
	in := ValidationInput{HasFrontendBundle: true, CurrentAppVersion: "1.0.0", InstalledTools: installed("1.0.0", true)}
	_, err := Validate(requiresManifest(RequiredToolDecl{Slug: "browser", MinVersion: "2.0.0", MaxVersion: "1.0.0"}), in)
	if err == nil || !strings.Contains(err.Error(), "no version could satisfy") {
		t.Fatalf("%v", err)
	}
	if _, err := Validate(requiresManifest(RequiredToolDecl{Slug: "browser", MinVersion: "1.0.0", MaxVersion: "1.5.0"}), in); err != nil {
		t.Fatalf("a satisfied range was refused: %v", err)
	}
	if _, err := Validate(requiresManifest(RequiredToolDecl{Slug: "browser", MaxVersion: "0.5.0"}), in); err == nil || !strings.Contains(err.Error(), "version <= 0.5.0") {
		t.Fatalf("an installed version above the maximum is refused at install: %v", err)
	}
}

func TestRequiredToolDeclReadsMaxVersionFromTheManifest(t *testing.T) {
	var m Manifest
	raw := `{"slug":"career","name":"Career","version":"1.0.0","requires_tools":[{"slug":"browser","min_version":"1.0.0","max_version":"2.0.0"},{"slug":"pdf_tools"}]}`
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.RequiresTools) != 2 || m.RequiresTools[0].MinVersion != "1.0.0" || m.RequiresTools[0].MaxVersion != "2.0.0" || m.RequiresTools[1].MaxVersion != "" {
		t.Fatalf("%+v", m.RequiresTools)
	}
	out, _ := json.Marshal(RequiredToolDecl{Slug: "x", MinVersion: "1.0.0"})
	if strings.Contains(string(out), "max_version") {
		t.Fatalf("an empty maximum is not written: %s", out)
	}
}
