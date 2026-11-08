package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"testing"

	tool_handler "github.com/a-digi/cinqo/src/tool/handler"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

func storedSource(t *testing.T, e *env, slug string) sql.NullString {
	t.Helper()
	var s sql.NullString
	if err := e.di.db.QueryRow(`SELECT source_repository_id FROM tools WHERE slug = ?`, slug).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// Revision 2 of plan dependency-install: an install through a repository records which repository it came from, an update through another one
// overwrites it (the new bits come from there), so a later update can stay on its origin.
func TestAnInstallRecordsTheRepositoryItCameFromAndAnUpdateFromAnotherOverwritesIt(t *testing.T) {
	e, rs := newEnv(t), newRepoServer(t)
	v1, v2 := metaZip(t, "demo", "1.0.0", ada, "MIT"), metaZip(t, "demo", "1.1.0", ada, "MIT")
	rs.put("/a/files/demo.zip", v1)
	rs.put("/a/tools.json", repoFile(2, `"authors":[{"name":"R"}],`, ada, "MIT", v1))
	rs.put("/b/files/demo.zip", v2)
	rs.put("/b/tools.json", []byte(fmt.Sprintf(`{"formatVersion":2,"name":"Other","authors":[{"name":"R"}],"tools":[{"id":"demo","currentVersionId":"b","versions":[
	  {"versionId":"b","name":"Demo","version":"1.1.0","authors":%s,"license":"MIT",%s}]}]}`, ada, pkgEntry("files/demo.zip", v2, sha(v2)))))
	a := addRepo(t, e, rs.srv.URL+"/a/tools.json")
	b := addRepo(t, e, rs.srv.URL+"/b/tools.json")

	if code, out := e.call(InstallHandler, "POST", "/x", "", map[string]string{"id": a, "toolId": "demo"}); code != http.StatusCreated {
		t.Fatalf("%d %v", code, out)
	}
	if s := storedSource(t, e, "demo"); !s.Valid || s.String != a {
		t.Fatalf("a fresh install records its repository: %+v want %s", s, a)
	}
	// and the tool repository reads it back (what the dependency planner will use)
	if tl, err := tool_query.NewToolQueryRepo(e.di.db).FindBySlug("demo"); err != nil || tl.SourceRepositoryID != a {
		t.Fatalf("the query repo reads the source: %+v %v", tl, err)
	}
	if code, out := e.call(InstallHandler, "POST", "/x", "", map[string]string{"id": b, "toolId": "demo"}); code != http.StatusOK {
		t.Fatalf("%d %v", code, out)
	}
	if s := storedSource(t, e, "demo"); !s.Valid || s.String != b {
		t.Fatalf("an update through another repository records that one: %+v want %s", s, b)
	}
	// the installed-tools answer does not carry it (nothing in the UI needs it yet)
	_, list := e.call(tool_handler.ListHandler, "GET", "/api/v1/tools", "", nil)
	if _, has := list["message"].([]any)[0].(map[string]any)["source_repository_id"]; has {
		t.Fatalf("the source is not part of the tools answer: %v", list["message"])
	}
}
