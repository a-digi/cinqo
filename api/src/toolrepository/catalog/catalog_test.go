package catalog

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const sum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var validFile = `{
  "formatVersion": 1, "name": "Repo", "futureField": {"x": 1},
  "tools": [{
    "id": "browser", "currentVersionId": "v2",
    "versions": [
      {"versionId":"v1","name":"Browser","version":"0.0.1","packages":[{"os":"any","arch":"any","url":"a.zip","sha256":"` + sum + `"}]},
      {"versionId":"v2","name":"Browser","description":"d","version":"0.0.2","releasedAt":"2026-10-30","minAppVersion":"0.0.100",
       "packages":[
         {"os":"darwin","arch":"arm64","url":"files/b.zip","sha256":"` + strings.ToUpper(sum) + `","sizeBytes":10},
         {"os":"linux","arch":"any","url":"https://example.com/l.zip","sha256":"` + sum + `"}]}
    ]}]}`

func problems(t *testing.T, data, base string) []Problem {
	t.Helper()
	_, err := Parse([]byte(data), base)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	return ve.Problems
}

func hasProblem(ps []Problem, path, contains string) bool {
	for _, p := range ps {
		if p.Path == path && strings.Contains(p.Message, contains) {
			return true
		}
	}
	return false
}

func TestParseValidFile(t *testing.T) {
	c, err := Parse([]byte(validFile), "https://example.com/cinqo/tools.json")
	if err != nil {
		t.Fatal(err)
	}
	tool, ok := c.Tool("browser")
	if !ok || len(tool.Versions) != 2 {
		t.Fatalf("tool: %+v", tool)
	}
	cur, _ := tool.Current()
	if cur.Version != "0.0.2" {
		t.Errorf("current = %s", cur.Version)
	}
	p := cur.Packages[0]
	if p.URL != "https://example.com/cinqo/files/b.zip" {
		t.Errorf("relative URL not resolved: %s", p.URL)
	}
	if p.SHA256 != sum {
		t.Errorf("sha256 not lower-cased: %s", p.SHA256)
	}
}

func TestParseRejectsBrokenJSON(t *testing.T) {
	for name, data := range map[string]string{
		"empty": "", "syntax": "{", "array": "[]", "trailing": validFile + "{}",
		"wrongType": `{"formatVersion":"1"}`,
	} {
		if ps := problems(t, data, "https://e.com/r.json"); len(ps) != 1 {
			t.Errorf("%s: %v", name, ps)
		}
	}
}

func TestParseOversizedFile(t *testing.T) {
	big := validFile + strings.Repeat(" ", MaxFileBytes)
	if ps := problems(t, big, "https://e.com/r.json"); !strings.Contains(ps[0].Message, "larger") {
		t.Errorf("%v", ps)
	}
}

func pkg(extra string) string {
	return `{"os":"any","arch":"any","url":"a.zip","sha256":"` + sum + `"` + extra + `}`
}

func file(tool string) string {
	return `{"formatVersion":1,"tools":[` + tool + `]}`
}

func oneVersion(id, ver, pkgs string) string {
	return `{"versionId":"` + id + `","name":"N","version":"` + ver + `","packages":[` + pkgs + `]}`
}

func TestRules(t *testing.T) {
	base := "https://e.com/r.json"
	cases := []struct{ name, data, path, msg string }{
		{"format version", `{"formatVersion":2,"tools":[]}`, "formatVersion", "must be 1"},
		{"bad tool id", file(`{"id":"Bad-Id","currentVersionId":"a","versions":[` + oneVersion("a", "1.0.0", pkg("")) + `]}`), "tools[0].id", "must match"},
		{"duplicate tool id", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", pkg("")) + `]},{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", pkg("")) + `]}`), "tools[1].id", "duplicate"},
		{"no versions", file(`{"id":"a","currentVersionId":"v","versions":[]}`), "tools[0].versions", "at least one"},
		{"current missing", file(`{"id":"a","versions":[` + oneVersion("v", "1.0.0", pkg("")) + `]}`), "tools[0].currentVersionId", "required"},
		{"current unknown", file(`{"id":"a","currentVersionId":"zz","versions":[` + oneVersion("v", "1.0.0", pkg("")) + `]}`), "tools[0].currentVersionId", "not one of"},
		{"duplicate versionId", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", pkg("")) + `,` + oneVersion("v", "1.0.1", pkg("")) + `]}`), "tools[0].versions[1].versionId", "duplicate"},
		{"duplicate version", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", pkg("")) + `,` + oneVersion("w", "1.0.0", pkg("")) + `]}`), "tools[0].versions[1].version", "duplicate"},
		{"bad versionId", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("has space", "1.0.0", pkg("")) + `]}`), "tools[0].versions[0].versionId", "must be"},
		{"bad semver", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0", pkg("")) + `]}`), "tools[0].versions[0].version", "MAJOR.MINOR.PATCH"},
		{"no packages", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", "") + `]}`), "tools[0].versions[0].packages", "at least one"},
		{"bad checksum", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", `{"os":"any","arch":"any","url":"a.zip","sha256":"abc"}`) + `]}`), "tools[0].versions[0].packages[0].sha256", "64"},
		{"bad os", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", `{"os":"plan9","arch":"any","url":"a.zip","sha256":"`+sum+`"}`) + `]}`), "tools[0].versions[0].packages[0].os", "darwin"},
		{"bad arch", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", `{"os":"any","arch":"mips","url":"a.zip","sha256":"`+sum+`"}`) + `]}`), "tools[0].versions[0].packages[0].arch", "arm64"},
		{"duplicate platform", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", pkg("")+","+pkg("")) + `]}`), "tools[0].versions[0].packages[1]", "duplicate package"},
		{"http public", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", `{"os":"any","arch":"any","url":"http://example.com/a.zip","sha256":"`+sum+`"}`) + `]}`), "tools[0].versions[0].packages[0].url", "https"},
		{"ftp", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", `{"os":"any","arch":"any","url":"ftp://x.com/a.zip","sha256":"`+sum+`"}`) + `]}`), "tools[0].versions[0].packages[0].url", "https"},
		{"credentials", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", `{"os":"any","arch":"any","url":"https://u:p@x.com/a.zip","sha256":"`+sum+`"}`) + `]}`), "tools[0].versions[0].packages[0].url", "credentials"},
		{"negative size", file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0", pkg(`,"sizeBytes":-1`)) + `]}`), "tools[0].versions[0].packages[0].sizeBytes", "between"},
		{"bad minAppVersion", file(`{"id":"a","currentVersionId":"v","versions":[{"versionId":"v","name":"N","version":"1.0.0","minAppVersion":"x","packages":[` + pkg("") + `]}]}`), "tools[0].versions[0].minAppVersion", "MAJOR"},
		{"bad releasedAt", file(`{"id":"a","currentVersionId":"v","versions":[{"versionId":"v","name":"N","version":"1.0.0","releasedAt":"yesterday","packages":[` + pkg("") + `]}]}`), "tools[0].versions[0].releasedAt", "date"},
		{"missing name", file(`{"id":"a","currentVersionId":"v","versions":[{"versionId":"v","name":" ","version":"1.0.0","packages":[` + pkg("") + `]}]}`), "tools[0].versions[0].name", "required"},
		{"long description", file(`{"id":"a","currentVersionId":"v","versions":[{"versionId":"v","name":"N","description":"` + strings.Repeat("x", MaxDescriptionLen+1) + `","version":"1.0.0","packages":[` + pkg("") + `]}]}`), "tools[0].versions[0].description", "longer"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if ps := problems(t, c.data, base); !hasProblem(ps, c.path, c.msg) {
				t.Errorf("want %s: %q, got %v", c.path, c.msg, ps)
			}
		})
	}
}

func TestAllProblemsCollected(t *testing.T) {
	data := `{"formatVersion":3,"tools":[{"id":"BAD","currentVersionId":"x","versions":[
		{"versionId":"v","name":"","version":"1","packages":[{"os":"x","arch":"y","url":"","sha256":"z"}]}]}]}`
	if ps := problems(t, data, "https://e.com/r.json"); len(ps) < 8 {
		t.Errorf("expected many problems, got %d: %v", len(ps), ps)
	}
}

func TestOversizedLists(t *testing.T) {
	var tools []string
	for i := 0; i <= MaxTools; i++ {
		tools = append(tools, fmt.Sprintf(`{"id":"t%d"}`, i))
	}
	if ps := problems(t, file(strings.Join(tools, ",")), "https://e.com/r.json"); !hasProblem(ps, "tools", "at most") {
		t.Errorf("tools: %v", ps)
	}
	var vs []string
	for i := 0; i <= MaxVersionsPerTool; i++ {
		vs = append(vs, `{}`)
	}
	if ps := problems(t, file(`{"id":"a","currentVersionId":"v","versions":[`+strings.Join(vs, ",")+`]}`), "https://e.com/r.json"); !hasProblem(ps, "tools[0].versions", "at most") {
		t.Errorf("versions: %v", ps)
	}
	var pk []string
	for i := 0; i <= MaxPackagesPerVer; i++ {
		pk = append(pk, pkg(""))
	}
	if ps := problems(t, file(`{"id":"a","currentVersionId":"v","versions":[`+oneVersion("v", "1.0.0", strings.Join(pk, ","))+`]}`), "https://e.com/r.json"); !hasProblem(ps, "tools[0].versions[0].packages", "at most") {
		t.Errorf("packages: %v", ps)
	}
}

func TestResolvePackageURL(t *testing.T) {
	base := "https://example.com/cinqo/tools.json"
	ok := map[string]string{
		"a.zip":                       "https://example.com/cinqo/a.zip",
		"/root/a.zip":                 "https://example.com/root/a.zip",
		"../a.zip#frag":               "https://example.com/a.zip",
		"https://other.org/a.zip":     "https://other.org/a.zip",
		"http://localhost:8080/a.zip": "http://localhost:8080/a.zip",
		"http://192.168.1.5/a.zip":    "http://192.168.1.5/a.zip",
		"http://nas.local/a.zip":      "http://nas.local/a.zip",
		"http://127.0.0.1/a.zip":      "http://127.0.0.1/a.zip",
	}
	for in, want := range ok {
		got, err := ResolvePackageURL(base, in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "http://example.com/a.zip", "javascript:alert(1)", "file:///etc/passwd", "//", "https://u@x.com/a", strings.Repeat("a", MaxURLLen+1)} {
		if _, err := ResolvePackageURL(base, in); err == nil {
			t.Errorf("%q should be refused", in)
		}
	}
	if _, err := ResolvePackageURL("", "a.zip"); err == nil {
		t.Error("relative URL without a base should be refused")
	}
	if _, err := ResolvePackageURL("", "https://x.com/a.zip"); err != nil {
		t.Errorf("absolute URL needs no base: %v", err)
	}
}

func TestIsLocalHost(t *testing.T) {
	for h, want := range map[string]bool{
		"localhost": true, "foo.localhost": true, "127.0.0.1": true, "::1": true, "10.0.0.1": true,
		"192.168.0.2": true, "172.16.0.1": true, "169.254.169.254": true, "0.0.0.0": true,
		"nas.local": true, "nas": true, "example.com": false, "8.8.8.8": false, "": true,
	} {
		if got := IsLocalHost(h); got != want {
			t.Errorf("IsLocalHost(%q) = %v", h, got)
		}
	}
}

func TestPickPackage(t *testing.T) {
	v := Version{Packages: []Package{
		{OS: "any", Arch: "any", URL: "anyany"},
		{OS: "any", Arch: "arm64", URL: "anyarm"},
		{OS: "darwin", Arch: "any", URL: "darwinany"},
		{OS: "darwin", Arch: "arm64", URL: "darwinarm"},
	}}
	for _, c := range []struct{ os, arch, want string }{
		{"darwin", "arm64", "darwinarm"}, {"darwin", "amd64", "darwinany"},
		{"linux", "arm64", "anyarm"}, {"linux", "amd64", "anyany"},
	} {
		p, ok := v.PickPackage(c.os, c.arch)
		if !ok || p.URL != c.want {
			t.Errorf("%s/%s: got %+v", c.os, c.arch, p)
		}
	}
	only := Version{Packages: []Package{{OS: "windows", Arch: "amd64", URL: "w"}}}
	if _, ok := only.PickPackage("darwin", "arm64"); ok {
		t.Error("no package should match")
	}
}

func TestLocalPackageProblems(t *testing.T) {
	data := file(`{"id":"a","currentVersionId":"v","versions":[` + oneVersion("v", "1.0.0",
		`{"os":"darwin","arch":"any","url":"http://192.168.1.5/a.zip","sha256":"`+sum+`"},`+
			`{"os":"linux","arch":"any","url":"https://example.com/a.zip","sha256":"`+sum+`"}`) + `]}`)
	c, err := Parse([]byte(data), "https://e.com/r.json")
	if err != nil {
		t.Fatal(err)
	}
	ps := LocalPackageProblems(c)
	if len(ps) != 1 || ps[0].Path != "tools[0].versions[0].packages[0].url" {
		t.Errorf("%v", ps)
	}
	if len(LocalPackageProblems(&Catalog{})) != 0 {
		t.Error("empty catalog has no problems")
	}
}
