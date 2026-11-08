package handler

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/a-digi/coco-logger/logger"
	orm "github.com/a-digi/coco-orm/orm"
	"github.com/a-digi/coco-server/server/request"
	_ "github.com/mattn/go-sqlite3"
)

type uploadDI struct {
	db     *sql.DB
	values map[string]any
}

func (d *uploadDI) GetDatabaseManager() *orm.DatabaseManager {
	return &orm.DatabaseManager{Connector: &orm.Connector{DB: d.db}}
}
func (d *uploadDI) GetLogger() logger.Logger { return quietLogger{} }
func (d *uploadDI) Get(key string) (any, bool) {
	v, ok := d.values[key]
	return v, ok
}

type quietLogger struct{}

func (quietLogger) Info(string, ...interface{})    {}
func (quietLogger) Warning(string, ...interface{}) {}
func (quietLogger) Error(string, ...interface{})   {}
func (quietLogger) Close()                         {}

func newUploadDI(t *testing.T) (*uploadDI, string) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	files, _ := filepath.Glob(filepath.Join("..", "..", "..", "config", "db", "migrations", "*.sql"))
	sort.Strings(files)
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
	dir := t.TempDir()
	return &uploadDI{db: db, values: map[string]any{"data_dir": dir, "app_version": "0.0.100"}}, dir
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, _ := zw.Create(name)
		w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func upload(t *testing.T, di *uploadDI, pkg []byte) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("package", "tool.zip")
	fw.Write(pkg)
	mw.Close()
	r := httptest.NewRequest("POST", "/api/v1/tools/install", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	InstallHandler(request.NewContext(rec, r, di))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func demoPackage(t *testing.T, version string) []byte {
	return zipOf(t, map[string]string{
		"manifest.json":      fmt.Sprintf(`{"slug":"demo","name":"Demo","version":%q,"authors":[{"name":"Test Author"}],"license":"MIT"}`, version),
		"frontend/bundle.js": "// " + version,
	})
}

// The upload route keeps its behavior now that its pipeline is the shared
// InstallPackage: 201 for a fresh install, 200 for a strictly newer update, 400
// for everything the pipeline rejects.
func TestUploadInstallBehaviorIsUnchanged(t *testing.T) {
	di, dir := newUploadDI(t)

	code, out := upload(t, di, demoPackage(t, "1.0.0"))
	if code != http.StatusCreated {
		t.Fatalf("fresh: %d %v", code, out)
	}
	tool := out["message"].(map[string]any)
	if tool["slug"] != "demo" || tool["version"] != "1.0.0" || tool["enabled"] != true {
		t.Errorf("%v", tool)
	}

	if code, _ = upload(t, di, demoPackage(t, "1.0.0")); code != http.StatusBadRequest {
		t.Errorf("same version: %d", code)
	}
	if code, _ = upload(t, di, demoPackage(t, "0.9.0")); code != http.StatusBadRequest {
		t.Errorf("older version: %d", code)
	}
	code, out = upload(t, di, demoPackage(t, "1.2.0"))
	if code != http.StatusOK || out["message"].(map[string]any)["version"] != "1.2.0" {
		t.Fatalf("update: %d %v", code, out)
	}

	if code, _ = upload(t, di, []byte("not a zip")); code != http.StatusBadRequest {
		t.Errorf("not a zip: %d", code)
	}
	if code, out = upload(t, di, zipOf(t, map[string]string{"frontend/bundle.js": "x"})); code != http.StatusBadRequest || out["message"] != "package is missing manifest.json" {
		t.Errorf("no manifest: %d %v", code, out)
	}
	bad := zipOf(t, map[string]string{"manifest.json": `{"slug":"Bad Slug","name":"X","version":"1.0.0"}`, "frontend/bundle.js": "x"})
	if code, _ = upload(t, di, bad); code != http.StatusBadRequest {
		t.Errorf("invalid manifest: %d", code)
	}

	entries, _ := os.ReadDir(filepath.Join(dir, "tools"))
	var names []string
	for _, e := range entries {
		if e.Name() == ".staging" {
			if sub, _ := os.ReadDir(filepath.Join(dir, "tools", ".staging")); len(sub) != 0 {
				t.Errorf("staging not cleaned: %d entries", len(sub))
			}
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "demo" {
		t.Errorf("tools dir: %v", names)
	}
}

func TestInstallPackageChecksExpectation(t *testing.T) {
	di, _ := newUploadDI(t)
	req := httptest.NewRequest("POST", "/x", nil)
	ctx := request.NewContext(httptest.NewRecorder(), req, di)

	_, err := InstallPackage(ctx, demoPackage(t, "1.0.0"), &PackageExpectation{Slug: "demo", Version: "9.9.9"})
	if err == nil || err.Status != http.StatusBadRequest || !strings.Contains(err.Message, "repository lists") {
		t.Fatalf("version mismatch: %v", err)
	}
	_, err = InstallPackage(ctx, demoPackage(t, "1.0.0"), &PackageExpectation{Slug: "other", Version: "1.0.0"})
	if err == nil || err.Status != http.StatusBadRequest {
		t.Fatalf("slug mismatch: %v", err)
	}
	res, err := InstallPackage(ctx, demoPackage(t, "1.0.0"), &PackageExpectation{Slug: "demo", Version: "1.0.0"})
	if err != nil || !res.Created {
		t.Fatalf("%v %+v", err, res)
	}
}
