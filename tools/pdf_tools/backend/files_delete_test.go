package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A generated file is <uploads>/<uuid>.pdf. These tests use real files in a temporary folder: the route must remove exactly one
// generated file and nothing else, whatever it is asked.

func newUploads(t *testing.T) (dir string, outside string) {
	t.Helper()
	root := t.TempDir()
	dir = filepath.Join(root, "uploads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside = filepath.Join(root, "outside.pdf") // what a path that leaves the folder would hit
	if err := os.WriteFile(outside, []byte("not ours"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, outside
}

func newFile(t *testing.T, dir string) string {
	t.Helper()
	id := uuid.NewString()
	if err := os.WriteFile(filepath.Join(dir, id+".pdf"), []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatal(err)
	}
	return id
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func call(method, id string, dir string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	filesHandler(dir)(rec, httptest.NewRequest(method, "/files?id="+url.QueryEscape(id), nil))
	return rec
}

func TestDeletingAGeneratedFileRemovesItAndOnlyIt(t *testing.T) {
	dir, outside := newUploads(t)
	a, b := newFile(t, dir), newFile(t, dir)
	if rec := call(http.MethodDelete, a, dir); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
	if exists(filepath.Join(dir, a+".pdf")) {
		t.Error("the file must be gone")
	}
	if !exists(filepath.Join(dir, b+".pdf")) || !exists(outside) {
		t.Error("no other file may be touched")
	}
	// the other one can still be read back
	if rec := call(http.MethodGet, b, dir); rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Errorf("reading the other file: %d", rec.Code)
	}
	// and the deleted one is not found any more
	if rec := call(http.MethodGet, a, dir); rec.Code != http.StatusNotFound {
		t.Errorf("reading the deleted file: %d", rec.Code)
	}
}

func TestDeletingTwiceOrAFileThatNeverExistedIsNotAnError(t *testing.T) {
	dir, _ := newUploads(t)
	id := newFile(t, dir)
	for i := 0; i < 2; i++ {
		if rec := call(http.MethodDelete, id, dir); rec.Code != http.StatusNoContent {
			t.Errorf("delete %d: %d", i+1, rec.Code)
		}
	}
	if rec := call(http.MethodDelete, uuid.NewString(), dir); rec.Code != http.StatusNoContent {
		t.Errorf("a file that never existed: %d", rec.Code)
	}
}

func TestOnlyTheExactShapeOfAGeneratedIdIsAccepted(t *testing.T) {
	dir, outside := newUploads(t)
	id := newFile(t, dir)
	upper := strings.ToUpper(id)
	noHyphens := strings.ReplaceAll(id, "-", "")
	for name, bad := range map[string]string{
		"nothing":                "",
		"a word":                 "abc",
		"dot dot":                "..",
		"dot":                    ".",
		"a traversal":            "../outside",
		"a traversal with an id": "../" + id,
		"an absolute path":       outside,
		"a separator":            "a/b",
		"a backslash":            `a\b`,
		"the id with .pdf":       id + ".pdf",
		"the id with a slash":    id + "/",
		"the id with a space":    id + " ",
		"a space before the id":  " " + id,
		"the id twice":           id + id,
		"capitals":               upper,
		"no hyphens":             noHyphens,
		"in braces":              "{" + id + "}",
		"a urn":                  "urn:uuid:" + id,
		"a null byte":            id + "\x00",
		"an encoded dot dot":     "%2e%2e%2foutside",
		"a glob":                 "*",
		"a star pdf":             "*.pdf",
	} {
		rec := call(http.MethodDelete, bad, dir)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s (%q): status %d, want 400", name, bad, rec.Code)
		}
	}
	if !exists(filepath.Join(dir, id+".pdf")) || !exists(outside) {
		t.Error("a refused id must delete nothing")
	}
	// no id at all (no query parameter)
	rec := httptest.NewRecorder()
	filesHandler(dir)(rec, httptest.NewRequest(http.MethodDelete, "/files", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("no id: %d", rec.Code)
	}
}

func TestADirectoryOrALinkWithTheNameOfAGeneratedFileIsLeftAlone(t *testing.T) {
	dir, outside := newUploads(t)
	asDir, asLink := uuid.NewString(), uuid.NewString()
	if err := os.Mkdir(filepath.Join(dir, asDir+".pdf"), 0o755); err != nil { // an EMPTY directory: os.Remove would delete it
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, asLink+".pdf")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	for name, id := range map[string]string{"directory": asDir, "link": asLink} {
		if rec := call(http.MethodDelete, id, dir); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	if !exists(filepath.Join(dir, asDir+".pdf")) || !exists(filepath.Join(dir, asLink+".pdf")) || !exists(outside) {
		t.Error("neither the directory, nor the link, nor the link's target may be touched")
	}
}

func TestADeleteNeverLeavesTheUploadsFolder(t *testing.T) {
	dir, outside := newUploads(t)
	// a file with the generated name in the PARENT folder, which a path that climbs out would reach
	parentID := uuid.NewString()
	parentFile := filepath.Join(filepath.Dir(dir), parentID+".pdf")
	if err := os.WriteFile(parentFile, []byte("parent"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../" + parentID, "..%2f" + parentID, "..\\" + parentID} {
		call(http.MethodDelete, id, dir)
	}
	if !exists(parentFile) || !exists(outside) {
		t.Error("nothing outside the uploads folder may be deleted")
	}
}

func TestOtherMethodsStillBehaveAsBefore(t *testing.T) {
	dir, _ := newUploads(t)
	id := newFile(t, dir)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		if rec := call(m, id, dir); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
	if !exists(filepath.Join(dir, id+".pdf")) {
		t.Error("only DELETE removes")
	}
	if rec := call(http.MethodGet, id, dir); rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" {
		t.Errorf("GET: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := call(http.MethodGet, "not-a-uuid", dir); rec.Code != http.StatusNotFound {
		t.Errorf("GET of a bad id is still a 404: %d", rec.Code)
	}
}

func TestAFileThatCannotBeRemovedIsReportedNotHidden(t *testing.T) {
	dir, _ := newUploads(t)
	id := newFile(t, dir)
	if err := os.Chmod(dir, 0o555); err != nil { // a folder nothing can be removed from
		t.Skip("cannot make the folder read-only")
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if os.Geteuid() == 0 {
		t.Skip("root can remove from a read-only folder")
	}
	rec := call(http.MethodDelete, id, dir)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), dir) {
		t.Errorf("%d %q: a failure is a 500 that does not name the folder", rec.Code, rec.Body.String())
	}
	if !exists(filepath.Join(dir, id+".pdf")) {
		t.Error("the file is still there")
	}
}

// The route is declared in the manifest with the scope that creates files: the core's proxy only forwards what the manifest declares.
func TestTheManifestDeclaresTheDeleteRouteWithTheGenerateScope(t *testing.T) {
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []struct {
			Method        string `json:"method"`
			PathSuffix    string `json:"path_suffix"`
			RequiredScope string `json:"required_scope"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, r := range m.Routes {
		found[r.Method+" "+r.PathSuffix] = r.RequiredScope
	}
	if found["DELETE files"] != "tool:pdf_tools:generate" {
		t.Errorf("DELETE files = %q", found["DELETE files"])
	}
	if found["GET files"] != "tool:pdf_tools:read" || found["POST generate"] != "tool:pdf_tools:generate" {
		t.Errorf("the existing routes changed: %v", found)
	}
}
