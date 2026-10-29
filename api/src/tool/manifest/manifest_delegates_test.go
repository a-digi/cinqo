package manifest

import "testing"

func delegatingManifest(allowed []string) Manifest {
	return Manifest{
		Slug: "demo", Name: "Demo", Version: "1.0.0", MCP: true,
		Scopes:   []ScopeDecl{{Scope: "tool:demo:run"}},
		MCPTools: []MCPToolDecl{{Name: "plan", RequiredScope: "tool:demo:run", Delegates: &DelegatesDecl{AllowedTools: allowed}}},
	}
}

func TestValidateDelegates(t *testing.T) {
	in := ValidationInput{HasBackendExecutable: true, CurrentAppVersion: "1.0.0"}
	if _, err := Validate(delegatingManifest([]string{"extract_from_url"}), in); err != nil {
		t.Fatalf("valid delegates rejected: %v", err)
	}
	for name, allowed := range map[string][]string{
		"empty":   {},
		"blank":   {" "},
		"builtin": {"spawn_subagent"},
	} {
		if _, err := Validate(delegatingManifest(allowed), in); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}
