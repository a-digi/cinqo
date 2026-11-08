package main

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// This tool stores no login (plan 182): no route, scope, MCP tool or wiring of the old credential store may come back.
func TestTheBrowserToolHasNoStoredCredentialFeature(t *testing.T) {
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Scopes []struct {
			Scope string `json:"scope"`
		} `json:"scopes"`
		Routes []struct {
			PathSuffix string `json:"path_suffix"`
		} `json:"routes"`
		MCPTools []struct {
			Name string `json:"name"`
		} `json:"mcp_tools"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, r := range m.Routes {
		for _, banned := range []string{"login-credentials", "has-login-credential"} {
			if r.PathSuffix == banned {
				t.Errorf("manifest.json declares the route %s", banned)
			}
		}
		if r.PathSuffix == "login" {
			t.Error("manifest.json declares the stored-credential login route")
		}
	}
	for _, s := range m.Scopes {
		if s.Scope == "tool:browser:login:status" {
			t.Error("manifest.json declares the credential-existence scope")
		}
	}
	for _, tool := range m.MCPTools {
		if tool.Name == "login" || tool.Name == "has_login_credential" {
			t.Errorf("manifest.json declares the MCP tool %s", tool.Name)
		}
	}

	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(string(src), "")
	for _, needle := range []string{`"/login"`, `"/login-credentials"`, `"/has-login-credential"`, "RegisterLogin(", "RegisterHasLoginCredential", "InitCryptoKey"} {
		if strings.Contains(code, needle) {
			t.Errorf("main.go still wires %s", needle)
		}
	}
	// the files of the old feature are gone
	for _, f := range []string{"auth/login.go", "auth/login_credentials.go", "auth/crypto.go"} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s must not exist (stat error = %v)", f, err)
		}
	}
	// the page of the old feature is gone from the frontend source
	for _, f := range []string{"ListPage.tsx", "AddPage.tsx", "VaultCard.tsx", "WillHappenCard.tsx", "credentialRules.ts"} {
		if _, err := os.Stat("../frontend-app/src/" + f); !os.IsNotExist(err) {
			t.Errorf("frontend-app/src/%s must not exist (stat error = %v)", f, err)
		}
	}
	entries, err := os.ReadDir("../frontend-app/src")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !(strings.HasSuffix(e.Name(), ".ts") || strings.HasSuffix(e.Name(), ".tsx")) {
			continue
		}
		b, err := os.ReadFile("../frontend-app/src/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, needle := range []string{"login-credentials", "credentials/new", "/tools/browser/credentials"} {
			if strings.Contains(string(b), needle) {
				t.Errorf("%s still mentions %s", e.Name(), needle)
			}
		}
	}
}
