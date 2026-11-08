package handler

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func depZip(t *testing.T, slug, version, requires string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("manifest.json")
	if requires != "" {
		requires = `,"requires_tools":` + requires
	}
	fmt.Fprintf(w, `{"slug":%q,"name":%q,"version":%q,"authors":[{"name":"Test Author"}],"license":"MIT"%s}`, slug, slug, version, requires)
	w, _ = zw.Create("frontend/bundle.js")
	w.Write([]byte("// " + slug + version))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// careerRepo serves a repository with career (needs browser and pdf_tools), browser and pdf_tools, with real packages.
func careerRepo(t *testing.T, e *env) (repoID string, confirm string) {
	t.Helper()
	srv := newRepoServer(t)
	type tl struct{ id, version, req string }
	tools := []tl{{"career", "1.0.0", `[{"slug":"browser"},{"slug":"pdf_tools","min_version":"0.0.1"}]`}, {"browser", "2.0.0", ""}, {"pdf_tools", "0.5.0", ""}}
	var entries []string
	for _, x := range tools {
		z := depZip(t, x.id, x.version, x.req)
		srv.put("/files/"+x.id+".zip", z)
		req := ""
		if x.id == "career" {
			req = `"requires":[{"id":"browser"},{"id":"pdf_tools","minVersion":"0.0.1"}],`
		}
		entries = append(entries, fmt.Sprintf(`{"id":%q,"currentVersionId":"v1","versions":[{"versionId":"v1","name":%q,"version":%q,"authors":[{"name":"Test Author"}],"license":"MIT",%s
		 "packages":[{"os":"any","arch":"any","url":"files/%s.zip","sha256":%q,"sizeBytes":%d}]}]}`, x.id, x.id, x.version, req, x.id, sha(z), len(z)))
	}
	srv.put("/main.json", []byte(`{"formatVersion":2,"name":"Main","authors":[{"name":"R"}],"tools":[`+strings.Join(entries, ",")+`]}`))
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	return id, id
}

func (e *env) installWith(repoID, confirm string) (int, map[string]any) {
	body := `{"withDependencies":true,"confirm":[` + confirm + `]}`
	return e.call(InstallHandler, "POST", "/x", body, map[string]string{"id": repoID, "toolId": "career"})
}

func confirmFor(id string) string {
	return fmt.Sprintf(`"install:browser@2.0.0:%[1]s","install:pdf_tools@0.5.0:%[1]s","install:career@1.0.0:%[1]s"`, id)
}

func TestTheOrderedInstallDoesTheConfirmedStepsInOrder(t *testing.T) {
	e := newEnv(t)
	id, _ := careerRepo(t, e)

	// the preview hands out exactly what must be confirmed
	code, out := e.plan(id, "career", "")
	if code != http.StatusOK || strings.Join(toStrings(planOf(out)["confirm"].([]any)), ",") != strings.ReplaceAll(confirmFor(id), `"`, "") {
		t.Fatalf("%d %v", code, planOf(out)["confirm"])
	}

	// not what the person confirmed: nothing is installed and the plan is answered again
	code, out = e.installWith(id, `"install:career@1.0.0:`+id+`"`)
	res := out["message"].(map[string]any)
	if code != http.StatusConflict || res["code"] != "plan_changed" || res["plan"] == nil || installedVersion(t, e, "browser") != "" {
		t.Fatalf("%d %v", code, out)
	}
	// no confirmation at all counts as not confirmed
	if code, _ = e.installWith(id, ""); code != http.StatusConflict || installedVersion(t, e, "career") != "" {
		t.Fatalf("an empty confirmation must not install: %d", code)
	}

	code, out = e.installWith(id, confirmFor(id))
	res = out["message"].(map[string]any)
	if code != http.StatusOK || res["ok"] != true || len(res["installed"].([]any)) != 3 {
		t.Fatalf("%d %v", code, out)
	}
	for slug, v := range map[string]string{"browser": "2.0.0", "pdf_tools": "0.5.0", "career": "1.0.0"} {
		if installedVersion(t, e, slug) != v {
			t.Errorf("%s: %q", slug, installedVersion(t, e, slug))
		}
	}
	var src string
	e.di.db.QueryRow(`SELECT source_repository_id FROM tools WHERE slug='browser'`).Scan(&src)
	if src != id {
		t.Errorf("a dependency records the repository it came from: %q", src)
	}
}

func TestAStepThatFailsKeepsWhatWasInstalledAndSaysWhatIsLeft(t *testing.T) {
	e := newEnv(t)
	id, _ := careerRepo(t, e)
	if _, err := e.di.db.Exec(`CREATE TRIGGER no_career BEFORE INSERT ON tools WHEN NEW.slug = 'career' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	code, out := e.installWith(id, confirmFor(id))
	res := out["message"].(map[string]any)
	if code != http.StatusUnprocessableEntity || res["code"] != "step_failed" || !strings.HasPrefix(res["message"].(string), "career:") {
		t.Fatalf("%d %v", code, out)
	}
	if len(res["installed"].([]any)) != 2 || len(res["notDone"].([]any)) != 1 || res["notDone"].([]any)[0].(map[string]any)["toolId"] != "career" {
		t.Fatalf("%v", res)
	}
	if installedVersion(t, e, "browser") != "2.0.0" || installedVersion(t, e, "pdf_tools") != "0.5.0" || installedVersion(t, e, "career") != "" {
		t.Error("what was installed stays, what failed is not there")
	}
}

func TestOnlyOneOrderedInstallRunsAtATime(t *testing.T) {
	e := newEnv(t)
	id, _ := careerRepo(t, e)
	planLock.Lock()
	defer planLock.Unlock()
	code, out := e.installWith(id, confirmFor(id))
	if code != http.StatusConflict || !strings.Contains(message(out), "another installation is running") || installedVersion(t, e, "browser") != "" {
		t.Fatalf("%d %v", code, out)
	}
}

func TestABrokenPackageStopsTheOrderedInstallBeforeAnythingIsInstalled(t *testing.T) {
	e := newEnv(t)
	srv := newRepoServer(t)
	good := depZip(t, "browser", "2.0.0", "")
	srv.put("/files/browser.zip", good)
	srv.put("/files/career.zip", depZip(t, "career", "1.0.0", `[{"slug":"browser"}]`))
	entry := func(id, version, requires, sum string, size int) string {
		return fmt.Sprintf(`{"id":%q,"currentVersionId":"v1","versions":[{"versionId":"v1","name":%q,"version":%q,"authors":[{"name":"Test Author"}],"license":"MIT",%s
		 "packages":[{"os":"any","arch":"any","url":"files/%s.zip","sha256":%q,"sizeBytes":%d}]}]}`, id, id, version, requires, id, sum, size)
	}
	career := depZip(t, "career", "1.0.0", `[{"slug":"browser"}]`)
	srv.put("/main.json", []byte(`{"formatVersion":2,"name":"Main","authors":[{"name":"R"}],"tools":[`+
		entry("career", "1.0.0", `"requires":[{"id":"browser"}],`, sha(career), len(career))+","+
		entry("browser", "2.0.0", "", strings.Repeat("0", 64), len(good))+`]}`))
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	code, _ := e.installWith(id, fmt.Sprintf(`"install:browser@2.0.0:%[1]s","install:career@1.0.0:%[1]s"`, id))
	if code == http.StatusOK || installedVersion(t, e, "browser") != "" || installedVersion(t, e, "career") != "" {
		t.Fatalf("a package that does not match its checksum must stop everything first: %d", code)
	}
}

func toStrings(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, v.(string))
	}
	return out
}
