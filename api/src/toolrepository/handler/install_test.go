package handler

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/a-digi/coco-logger/logger"
	orm "github.com/a-digi/coco-orm/orm"
	"github.com/a-digi/coco-server/server/request"
	_ "github.com/mattn/go-sqlite3"
)

// testDI is the slice of the app's DI container these handlers use.
type testDI struct {
	db     *sql.DB
	values map[string]any
}

func (d *testDI) GetDatabaseManager() *orm.DatabaseManager {
	return &orm.DatabaseManager{Connector: &orm.Connector{DB: d.db}}
}
func (d *testDI) GetLogger() logger.Logger { return quietLogger{} }
func (d *testDI) Get(key string) (any, bool) {
	v, ok := d.values[key]
	return v, ok
}

type quietLogger struct{}

func (quietLogger) Info(string, ...interface{})    {}
func (quietLogger) Warning(string, ...interface{}) {}
func (quietLogger) Error(string, ...interface{})   {}
func (quietLogger) Close()                         {}

func openCoreDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	files, _ := filepath.Glob(filepath.Join("..", "..", "..", "config", "db", "migrations", "*.sql"))
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no migrations")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(raw), "/***Statement***/") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", filepath.Base(f), err)
			}
		}
	}
	return db
}

type env struct {
	di      *testDI
	dataDir string
}

func newEnv(t *testing.T) *env {
	dir := t.TempDir()
	return &env{
		di:      &testDI{db: openCoreDB(t), values: map[string]any{"data_dir": dir, "app_version": "0.0.100"}},
		dataDir: dir,
	}
}

// call runs one handler as the framework would and returns the status and the
// decoded "message" payload (or the error message).
func (e *env) call(h func(request.RequestContext), method, path, body string, vars map[string]string) (int, map[string]any) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := request.NewContext(rec, r, e.di)
	for k, v := range vars {
		ctx.GetURI().SetPathVariable(k, v)
	}
	h(ctx)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func message(out map[string]any) string {
	s, _ := out["message"].(string)
	return s
}

func toolZip(t *testing.T, slug, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("manifest.json")
	fmt.Fprintf(w, `{"slug":%q,"name":"Demo","version":%q,"authors":[{"name":"Test Author"}],"license":"MIT"}`, slug, version)
	w, _ = zw.Create("frontend/bundle.js")
	w.Write([]byte("// bundle " + version))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// repoServer serves a mutable repository file and package zips.
type repoServer struct {
	mu    sync.Mutex
	files map[string][]byte
	srv   *httptest.Server
}

func newRepoServer(t *testing.T) *repoServer {
	r := &repoServer{files: map[string][]byte{}}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		b, ok := r.files[req.URL.Path]
		r.mu.Unlock()
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *repoServer) put(path string, b []byte) {
	r.mu.Lock()
	r.files[path] = b
	r.mu.Unlock()
}

func pkgEntry(file string, zipBytes []byte, sum string) string {
	return fmt.Sprintf(`"packages":[{"os":"any","arch":"any","url":%q,"sha256":%q,"sizeBytes":%d}]`, file, sum, len(zipBytes))
}

func installedVersion(t *testing.T, e *env, slug string) string {
	t.Helper()
	var v string
	err := e.di.db.QueryRow(`SELECT version FROM tools WHERE slug = ?`, slug).Scan(&v)
	if err == sql.ErrNoRows {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func leftovers(t *testing.T, e *env) []string {
	t.Helper()
	var out []string
	root := filepath.Join(e.dataDir, "tools")
	entries, _ := os.ReadDir(root)
	for _, en := range entries {
		if en.Name() == ".staging" {
			sub, _ := os.ReadDir(filepath.Join(root, ".staging"))
			for _, s := range sub {
				out = append(out, ".staging/"+s.Name())
			}
			continue
		}
		out = append(out, en.Name())
	}
	return out
}

// otherOS and otherArch name a platform that is never the one running the tests.
func otherPlatform() (string, string) {
	os, arch := "windows", "amd64"
	if runtime.GOOS == "windows" {
		os = "linux"
	}
	if runtime.GOARCH == "amd64" {
		arch = "arm64"
	}
	return os, arch
}

func TestInstallFromRepositoryEndToEnd(t *testing.T) {
	otherOS, otherArch := otherPlatform()
	e := newEnv(t)
	rs := newRepoServer(t)

	v100, v110 := toolZip(t, "demo", "1.0.0"), toolZip(t, "demo", "1.1.0")
	rs.put("/repo/files/demo-1.0.0.zip", v100)
	rs.put("/repo/files/demo-1.1.0.zip", v110)
	rs.put("/repo/files/mismatch.zip", toolZip(t, "somethingelse", "1.0.0"))
	rs.put("/repo/files/badsum.zip", v100)

	file := func(current string) []byte {
		return []byte(fmt.Sprintf(`{"formatVersion":1,"name":"Test","tools":[
		 {"id":"demo","currentVersionId":%q,"versions":[
		   {"versionId":"a","name":"Demo","version":"1.0.0",%s},
		   {"versionId":"b","name":"Demo","version":"1.1.0",%s}]},
		 {"id":"mismatch","currentVersionId":"m","versions":[{"versionId":"m","name":"M","version":"1.0.0",%s}]},
		 {"id":"badsum","currentVersionId":"s","versions":[{"versionId":"s","name":"S","version":"1.0.0",
		   "packages":[{"os":"any","arch":"any","url":"files/badsum.zip","sha256":%q}]}]},
		 {"id":"gone","currentVersionId":"g","versions":[{"versionId":"g","name":"G","version":"1.0.0",
		   "packages":[{"os":"any","arch":"any","url":"files/missing.zip","sha256":%q}]}]},
		 {"id":"winonly","currentVersionId":"w","versions":[{"versionId":"w","name":"W","version":"1.0.0",
		   "packages":[{"os":"%s","arch":"%s","url":"files/x.zip","sha256":%q}]}]}]}`,
			current,
			pkgEntry("files/demo-1.0.0.zip", v100, sha(v100)),
			pkgEntry("files/demo-1.1.0.zip", v110, sha(v110)),
			pkgEntry("files/mismatch.zip", toolZip(t, "somethingelse", "1.0.0"), sha(toolZip(t, "somethingelse", "1.0.0"))),
			strings.Repeat("0", 64), strings.Repeat("1", 64), otherOS, otherArch, strings.Repeat("2", 64)))
	}
	rs.put("/repo/tools.json", file("a"))

	code, out := e.call(AddHandler, "POST", "/api/v1/tool-repositories", fmt.Sprintf(`{"url":%q}`, rs.srv.URL+"/repo/tools.json"), nil)
	if code != http.StatusCreated {
		t.Fatalf("add: %d %v", code, out)
	}
	repoID := out["message"].(map[string]any)["id"].(string)
	if out["message"].(map[string]any)["isLocal"] != true {
		t.Error("a repository on 127.0.0.1 must be marked local")
	}
	install := func(toolID, body string) (int, map[string]any) {
		return e.call(InstallHandler, "POST", "/x", body, map[string]string{"id": repoID, "toolId": toolID})
	}

	// the page's status before anything is installed
	code, out = e.call(GetHandler, "GET", "/x", "", map[string]string{"id": repoID})
	if code != 200 || out["message"].(map[string]any)["counts"].(map[string]any)["notInstalled"].(float64) < 1 {
		t.Fatalf("detail: %d %v", code, out)
	}

	// fresh install of the current version (empty body)
	code, out = install("demo", "")
	if code != http.StatusCreated || installedVersion(t, e, "demo") != "1.0.0" {
		t.Fatalf("fresh install: %d %v", code, out)
	}
	if _, err := os.Stat(filepath.Join(e.dataDir, "tools", "demo", "manifest.json")); err != nil {
		t.Errorf("package not extracted: %v", err)
	}

	// already current → refused before any download
	if code, _ = install("demo", ""); code != http.StatusBadRequest {
		t.Errorf("reinstalling the same version: %d", code)
	}

	// the repository publishes 1.1.0 as current; refresh; it becomes an update
	rs.put("/repo/tools.json", file("b"))
	if code, _ = e.call(RefreshHandler, "POST", "/x", "", map[string]string{"id": repoID}); code != 200 {
		t.Fatalf("refresh: %d", code)
	}
	code, out = e.call(GetHandler, "GET", "/x", "", map[string]string{"id": repoID})
	tools := out["message"].(map[string]any)["tools"].([]any)
	if tools[0].(map[string]any)["status"] != "update_available" {
		t.Fatalf("status: %v", tools[0])
	}
	code, out = install("demo", `{"versionId":"b"}`)
	if code != http.StatusOK || installedVersion(t, e, "demo") != "1.1.0" {
		t.Fatalf("update: %d %v", code, out)
	}

	// an older version over a newer one is refused, and nothing changes
	if code, _ = install("demo", `{"versionId":"a"}`); code != http.StatusBadRequest || installedVersion(t, e, "demo") != "1.1.0" {
		t.Errorf("downgrade: %d", code)
	}

	// wrong checksum
	code, out = install("badsum", "")
	if code != http.StatusBadGateway || !strings.Contains(message(out), "checksum") || installedVersion(t, e, "badsum") != "" {
		t.Errorf("bad checksum: %d %v", code, out)
	}
	// the package's manifest disagrees with the repository entry
	code, out = install("mismatch", "")
	if code != http.StatusBadRequest || !strings.Contains(message(out), "repository lists") || installedVersion(t, e, "somethingelse") != "" || installedVersion(t, e, "mismatch") != "" {
		t.Errorf("manifest mismatch: %d %v", code, out)
	}
	// a failing download
	code, out = install("gone", "")
	if code != http.StatusBadGateway || installedVersion(t, e, "gone") != "" {
		t.Errorf("failing download: %d %v", code, out)
	}
	// no package for this platform
	if code, out = install("winonly", ""); code != http.StatusBadRequest || !strings.Contains(message(out), "no package") {
		t.Errorf("missing platform: %d %v", code, out)
	}
	// unknown tool, unknown version, unknown repository
	if code, _ = install("nope", ""); code != http.StatusNotFound {
		t.Errorf("unknown tool: %d", code)
	}
	if code, _ = install("demo", `{"versionId":"zzz"}`); code != http.StatusNotFound {
		t.Errorf("unknown version: %d", code)
	}
	if code, _ = e.call(InstallHandler, "POST", "/x", "", map[string]string{"id": "nope", "toolId": "demo"}); code != http.StatusNotFound {
		t.Errorf("unknown repository: %d", code)
	}
	if code, _ = install("demo", "{not json"); code != http.StatusBadRequest {
		t.Errorf("bad body: %d", code)
	}

	// every failure left nothing behind: only the one installed tool, no staging
	if left := leftovers(t, e); len(left) != 1 || left[0] != "demo" {
		t.Errorf("leftovers: %v", left)
	}

	// removing the repository never uninstalls the tool
	if code, _ = e.call(DeleteHandler, "DELETE", "/x", "", map[string]string{"id": repoID}); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if installedVersion(t, e, "demo") != "1.1.0" {
		t.Error("removing a repository uninstalled a tool")
	}
}
