package handler

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"

	tool_handler "github.com/a-digi/cinqo/src/tool/handler"
)

func metaZip(t *testing.T, slug, version, authors, license string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("manifest.json")
	fmt.Fprintf(w, `{"slug":%q,"name":"Demo","version":%q,"authors":%s,"license":%q,"company":{"name":"Acme GmbH"},"links":{"homepage":"https://acme.example"}}`, slug, version, authors, license)
	w, _ = zw.Create("frontend/bundle.js")
	w.Write([]byte("// " + version))
	zw.Close()
	return buf.Bytes()
}

const (
	ada   = `[{"name":"Ada Lovelace","email":"ada@example.com"}]`
	grace = `[{"name":"Grace Hopper"}]`
)

// repoV2 is a format 2 repository file that lists one tool "demo", version 1.0.0, claiming the given authors and
// licence (empty claims are left out, like a repository that states nothing).
func repoFile(format int, top, claimedAuthors, claimedLicense string, zipBytes []byte) []byte {
	claim := ""
	if claimedAuthors != "" {
		claim += `"authors":` + claimedAuthors + ","
	}
	if claimedLicense != "" {
		claim += fmt.Sprintf(`"license":%q,`, claimedLicense)
	}
	return []byte(fmt.Sprintf(`{"formatVersion":%d,"name":"Test",%s"tools":[{"id":"demo","currentVersionId":"a","versions":[
	  {"versionId":"a","name":"Demo","version":"1.0.0",%s%s}]}]}`, format, top, claim, pkgEntry("files/demo.zip", zipBytes, sha(zipBytes))))
}

func storedMetadata(t *testing.T, e *env, slug string) sql.NullString {
	t.Helper()
	var m sql.NullString
	if err := e.di.db.QueryRow(`SELECT metadata FROM tools WHERE slug = ?`, slug).Scan(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func addRepo(t *testing.T, e *env, url string) string {
	t.Helper()
	code, out := e.call(AddHandler, "POST", "/api/v1/tool-repositories", fmt.Sprintf(`{"url":%q}`, url), nil)
	if code != http.StatusCreated {
		t.Fatalf("add: %d %v", code, out)
	}
	return out["message"].(map[string]any)["id"].(string)
}

func TestARepositoryAndItsToolsShowWhoMadeThem(t *testing.T) {
	e, rs := newEnv(t), newRepoServer(t)
	pkg := metaZip(t, "demo", "1.0.0", ada, "MIT")
	rs.put("/repo/files/demo.zip", pkg)
	rs.put("/repo/tools.json", repoFile(2, `"authors":[{"name":"Cinqo team","url":"https://example.com/cinqo"}],"company":{"name":"Acme GmbH"},"links":{"security":"mailto:security@acme.example"},"generatedAt":"2026-10-31T10:00:00Z",`, ada, "MIT", pkg))
	id := addRepo(t, e, rs.srv.URL+"/repo/tools.json")

	// the list carries who maintains the repository
	code, out := e.call(ListHandler, "GET", "/x", "", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	pub := out["message"].([]any)[0].(map[string]any)["publisher"].(map[string]any)
	if pub["authors"].([]any)[0].(map[string]any)["name"] != "Cinqo team" || pub["company"].(map[string]any)["name"] != "Acme GmbH" ||
		pub["links"].(map[string]any)["security"] != "mailto:security@acme.example" || pub["generatedAt"] != "2026-10-31T10:00:00Z" {
		t.Fatalf("publisher: %v", pub)
	}

	// the detail carries it too, and each tool's facts (current version) and each version's authors and licence
	code, out = e.call(GetHandler, "GET", "/x", "", map[string]string{"id": id})
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	detail := out["message"].(map[string]any)
	if detail["publisher"] == nil {
		t.Fatalf("no publisher in the detail: %v", detail)
	}
	tool := detail["tools"].([]any)[0].(map[string]any)
	meta := tool["metadata"].(map[string]any)
	if meta["authors"].([]any)[0].(map[string]any)["name"] != "Ada Lovelace" || meta["license"] != "MIT" {
		t.Fatalf("tool metadata: %v", meta)
	}
	ver := tool["versions"].([]any)[0].(map[string]any)
	if ver["authors"].([]any)[0].(map[string]any)["name"] != "Ada Lovelace" || ver["license"] != "MIT" {
		t.Fatalf("version: %v", ver)
	}
}

func TestAnOlderFormatRepositoryStatesNothingAndIsShownAsSuch(t *testing.T) {
	e, rs := newEnv(t), newRepoServer(t)
	pkg := metaZip(t, "demo", "1.0.0", ada, "MIT")
	rs.put("/repo/files/demo.zip", pkg)
	rs.put("/repo/tools.json", repoFile(1, "", "", "", pkg))
	id := addRepo(t, e, rs.srv.URL+"/repo/tools.json")

	_, out := e.call(ListHandler, "GET", "/x", "", nil)
	if out["message"].([]any)[0].(map[string]any)["publisher"] != nil {
		t.Fatalf("a repository that states nothing has no publisher: %v", out)
	}
	_, out = e.call(GetHandler, "GET", "/x", "", map[string]string{"id": id})
	tool := out["message"].(map[string]any)["tools"].([]any)[0].(map[string]any)
	if tool["metadata"] != nil {
		t.Fatalf("a tool the repository says nothing about has no metadata: %v", tool["metadata"])
	}
	ver := tool["versions"].([]any)[0].(map[string]any)
	if list, ok := ver["authors"].([]any); !ok || len(list) != 0 {
		t.Fatalf("authors must be an empty list, never null: %v", ver["authors"])
	}
	// ... and it still installs; the installed tool then records what its own package says
	if code, out := e.call(InstallHandler, "POST", "/x", "", map[string]string{"id": id, "toolId": "demo"}); code != http.StatusCreated {
		t.Fatalf("%d %v", code, out)
	}
	if m := storedMetadata(t, e, "demo"); !m.Valid || !strings.Contains(m.String, "Ada Lovelace") {
		t.Fatalf("stored: %+v", m)
	}
}

func TestInstallRecordsWhatThePackageSaysAndTheToolListShowsIt(t *testing.T) {
	e, rs := newEnv(t), newRepoServer(t)
	pkg := metaZip(t, "demo", "1.0.0", ada, "MIT")
	rs.put("/repo/files/demo.zip", pkg)
	rs.put("/repo/tools.json", repoFile(2, `"authors":[{"name":"R"}],`, ada, "MIT", pkg))
	id := addRepo(t, e, rs.srv.URL+"/repo/tools.json")

	if code, out := e.call(InstallHandler, "POST", "/x", "", map[string]string{"id": id, "toolId": "demo"}); code != http.StatusCreated {
		t.Fatalf("%d %v", code, out)
	}
	m := storedMetadata(t, e, "demo")
	for _, want := range []string{"Ada Lovelace", "ada@example.com", `"license":"MIT"`, "Acme GmbH", "https://acme.example"} {
		if !strings.Contains(m.String, want) {
			t.Errorf("stored metadata lacks %q: %s", want, m.String)
		}
	}
	// the installed-tools API carries it
	code, out := e.call(tool_handler.ListHandler, "GET", "/api/v1/tools", "", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	meta := out["message"].([]any)[0].(map[string]any)["metadata"].(map[string]any)
	if meta["authors"].([]any)[0].(map[string]any)["name"] != "Ada Lovelace" || meta["license"] != "MIT" || meta["company"].(map[string]any)["name"] != "Acme GmbH" {
		t.Fatalf("%v", meta)
	}
}

func TestAnUpdateReplacesWhatWasRecorded(t *testing.T) {
	e, rs := newEnv(t), newRepoServer(t)
	v1, v2 := metaZip(t, "demo", "1.0.0", ada, "MIT"), metaZip(t, "demo", "1.1.0", grace, "Apache-2.0")
	rs.put("/repo/files/demo.zip", v1)
	rs.put("/repo/files/demo2.zip", v2)
	rs.put("/repo/tools.json", []byte(fmt.Sprintf(`{"formatVersion":2,"authors":[{"name":"R"}],"tools":[{"id":"demo","currentVersionId":"b","versions":[
	  {"versionId":"a","name":"Demo","version":"1.0.0","authors":%s,"license":"MIT",%s},
	  {"versionId":"b","name":"Demo","version":"1.1.0","authors":%s,"license":"Apache-2.0",%s}]}]}`,
		ada, pkgEntry("files/demo.zip", v1, sha(v1)), grace, pkgEntry("files/demo2.zip", v2, sha(v2)))))
	id := addRepo(t, e, rs.srv.URL+"/repo/tools.json")
	if code, out := e.call(InstallHandler, "POST", "/x", `{"versionId":"a"}`, map[string]string{"id": id, "toolId": "demo"}); code != http.StatusCreated {
		t.Fatalf("%d %v", code, out)
	}
	if m := storedMetadata(t, e, "demo"); !strings.Contains(m.String, "Ada Lovelace") {
		t.Fatalf("%s", m.String)
	}
	if code, out := e.call(InstallHandler, "POST", "/x", `{"versionId":"b"}`, map[string]string{"id": id, "toolId": "demo"}); code != http.StatusOK {
		t.Fatalf("%d %v", code, out)
	}
	m := storedMetadata(t, e, "demo")
	if strings.Contains(m.String, "Ada Lovelace") || !strings.Contains(m.String, "Grace Hopper") || !strings.Contains(m.String, "Apache-2.0") {
		t.Fatalf("an update must record the new version's facts: %s", m.String)
	}
}

func TestAPackageThatDisagreesWithTheRepositoryIsRefusedAndNothingIsInstalled(t *testing.T) {
	cases := []struct{ name, claimedAuthors, claimedLicense, want string }{
		{"authors", grace, "MIT", "Grace Hopper but the package says Ada Lovelace"},
		{"licence", ada, "GPL-3.0-only", `"GPL-3.0-only" but the package says "MIT"`},
	}
	for _, c := range cases {
		e, rs := newEnv(t), newRepoServer(t)
		pkg := metaZip(t, "demo", "1.0.0", ada, "MIT")
		rs.put("/repo/files/demo.zip", pkg)
		rs.put("/repo/tools.json", repoFile(2, `"authors":[{"name":"R"}],`, c.claimedAuthors, c.claimedLicense, pkg))
		id := addRepo(t, e, rs.srv.URL+"/repo/tools.json")
		code, out := e.call(InstallHandler, "POST", "/x", "", map[string]string{"id": id, "toolId": "demo"})
		if code != http.StatusBadRequest || !strings.Contains(message(out), "disagrees with the repository") || !strings.Contains(message(out), c.want) {
			t.Errorf("%s: %d %v", c.name, code, out)
		}
		if installedVersion(t, e, "demo") != "" || len(leftovers(t, e)) != 0 {
			t.Errorf("%s: something was left behind: %v", c.name, leftovers(t, e))
		}
	}
}

// A tool installed before the column existed has no metadata; the listing shows null and never fails.
func TestAnOldToolRowListsWithoutMetadata(t *testing.T) {
	e := newEnv(t)
	if _, err := e.di.db.Exec(`INSERT INTO tools (id, slug, name, version, kind, enabled, status, install_path, created_at)
	  VALUES ('t1','legacy','Legacy','1.0.0','frontend_only',1,'installed','/x','2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	code, out := e.call(tool_handler.ListHandler, "GET", "/api/v1/tools", "", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	row := out["message"].([]any)[0].(map[string]any)
	if v, present := row["metadata"]; !present || v != nil {
		t.Fatalf("metadata must be present and null: %v", row)
	}
	// and a row whose stored text is damaged is treated the same
	e.di.db.Exec(`UPDATE tools SET metadata = 'not json' WHERE id = 't1'`)
	if _, out = e.call(tool_handler.ListHandler, "GET", "/api/v1/tools", "", nil); out["message"].([]any)[0].(map[string]any)["metadata"] != nil {
		t.Fatalf("%v", out)
	}
}
