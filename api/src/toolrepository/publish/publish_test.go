package publish

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/toolrepository/catalog"
	"github.com/a-digi/cinqo/src/toolrepository/fetch"
)

func makeZip(t *testing.T, manifest string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if manifest != "" {
		w, _ := zw.Create("manifest.json")
		w.Write([]byte(manifest))
	}
	w, _ := zw.Create("frontend/bundle.js")
	w.Write([]byte("// js"))
	zw.Close()
	return buf.Bytes()
}

func put(t *testing.T, toolsDir, tool, file string, data []byte) string {
	t.Helper()
	dir := filepath.Join(toolsDir, tool, "versions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, file)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func manifestJSON(slug, name, version, minApp string) string {
	extra := ""
	if minApp != "" {
		extra = fmt.Sprintf(`,"min_app_version":%q`, minApp)
	}
	return fmt.Sprintf(`{"slug":%q,"name":%q,"version":%q%s}`, slug, name, version, extra)
}

func fixture(t *testing.T) string {
	t.Helper()
	tools := t.TempDir()
	for _, plat := range []string{"darwin-arm64", "linux-amd64"} {
		put(t, tools, "browser", "tool-browser-"+plat+".zip", makeZip(t, manifestJSON("browser", "Browser", "0.0.190", "0.0.100")))
	}
	// career ships two versions, the older one for one platform only
	put(t, tools, "career", "tool-career-darwin-arm64.zip", makeZip(t, manifestJSON("career", "Career", "0.0.10", "")))
	put(t, tools, "career", "tool-career-linux-amd64.zip", makeZip(t, manifestJSON("career", "Career", "0.0.9", "")))
	// a tool with no versions folder, and a stray file, are ignored
	if err := os.MkdirAll(filepath.Join(tools, "example"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tools, "README.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(tools, "browser", "versions", "notes.txt"), []byte("x"), 0o644)
	return tools
}

func TestGenerateBuildsAValidRepository(t *testing.T) {
	tools, out := fixture(t), t.TempDir()
	res, err := Generate(Options{ToolsDir: tools, OutDir: out, Name: "Mine", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Packages != 4 || len(res.Catalog.Tools) != 2 {
		t.Fatalf("%+v", res)
	}

	data, _ := os.ReadFile(filepath.Join(out, "repository.json"))
	c, err := catalog.Parse(data, "https://example.com/repo/repository.json")
	if err != nil {
		t.Fatalf("the generated file must be valid: %v", err)
	}
	if c.Name != "Mine" || c.Description != "d" || c.Tools[0].ID != "browser" || c.Tools[1].ID != "career" {
		t.Errorf("%+v", c)
	}

	browser, _ := c.Tool("browser")
	cur, _ := browser.Current()
	if cur.Version != "0.0.190" || cur.MinAppVersion != "0.0.100" || len(cur.Packages) != 2 || cur.Name != "Browser" {
		t.Errorf("%+v", cur)
	}
	// numeric, not text, ordering: career 0.0.10 is newer than 0.0.9
	career, _ := c.Tool("career")
	cc, _ := career.Current()
	if cc.Version != "0.0.10" || len(career.Versions) != 2 || career.Versions[0].Version != "0.0.9" {
		t.Errorf("current/order: %+v", career)
	}
	if len(cc.Packages) != 1 || cc.Packages[0].OS != "darwin" {
		t.Errorf("only the platforms that have the current version: %+v", cc.Packages)
	}
	if cur.Packages[0].URL != "https://example.com/repo/files/tool-browser-darwin-arm64.zip" {
		t.Errorf("relative url: %s", cur.Packages[0].URL)
	}

	// checksums and sizes come from the zip, and the copies are byte-identical
	for _, p := range cur.Packages {
		raw, err := os.ReadFile(filepath.Join(out, "files", filepath.Base(p.URL)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != p.SHA256 || int64(len(raw)) != p.SizeBytes {
			t.Errorf("checksum or size mismatch for %s", p.URL)
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	tools := fixture(t)
	a, b := t.TempDir(), t.TempDir()
	if _, err := Generate(Options{ToolsDir: tools, OutDir: a}); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(Options{ToolsDir: tools, OutDir: b}); err != nil {
		t.Fatal(err)
	}
	x, _ := os.ReadFile(filepath.Join(a, "repository.json"))
	y, _ := os.ReadFile(filepath.Join(b, "repository.json"))
	if !bytes.Equal(x, y) {
		t.Error("two runs differ")
	}
}

func TestBaseURLMakesAbsoluteLinks(t *testing.T) {
	out := t.TempDir()
	res, err := Generate(Options{ToolsDir: fixture(t), OutDir: out, BaseURL: "https://cdn.example.com/cinqo/", FileName: "tools.json"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(res.File) != "tools.json" {
		t.Errorf("file name: %s", res.File)
	}
	if u := res.Catalog.Tools[0].Versions[0].Packages[0].URL; u != "https://cdn.example.com/cinqo/files/tool-browser-darwin-arm64.zip" {
		t.Errorf("%s", u)
	}
}

func TestGenerateRefusesBadInput(t *testing.T) {
	cases := map[string]func(t *testing.T, tools string){
		"no packages": func(t *testing.T, tools string) {},
		"unparseable name": func(t *testing.T, tools string) {
			put(t, tools, "x", "tool-x.zip", makeZip(t, manifestJSON("x", "X", "1.0.0", "")))
		},
		"slug differs from file name": func(t *testing.T, tools string) {
			put(t, tools, "x", "tool-x-linux-amd64.zip", makeZip(t, manifestJSON("y", "Y", "1.0.0", "")))
		},
		"no manifest": func(t *testing.T, tools string) { put(t, tools, "x", "tool-x-linux-amd64.zip", makeZip(t, "")) },
		"not a zip":   func(t *testing.T, tools string) { put(t, tools, "x", "tool-x-linux-amd64.zip", []byte("nope")) },
		"bad version": func(t *testing.T, tools string) {
			put(t, tools, "x", "tool-x-linux-amd64.zip", makeZip(t, manifestJSON("x", "X", "one", "")))
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			tools, out := t.TempDir(), t.TempDir()
			setup(t, tools)
			if _, err := Generate(Options{ToolsDir: tools, OutDir: out}); err == nil {
				t.Fatal("expected an error")
			}
			if entries, _ := os.ReadDir(out); len(entries) != 0 {
				t.Errorf("a refused run must write nothing, found %d entries", len(entries))
			}
		})
	}
}

// The folder repotool writes works as a static site: the app's own fetcher
// loads the file, and a package downloads and matches its checksum.
func TestOutputFolderServesAsARepository(t *testing.T) {
	out := t.TempDir()
	if _, err := Generate(Options{ToolsDir: fixture(t), OutDir: out}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(out)))
	defer srv.Close()

	f := &fetch.Fetcher{Timeout: 5 * time.Second}
	link := srv.URL + "/repository.json"
	res, err := f.Get(context.Background(), link, catalog.MaxFileBytes, fetch.LocalByFirstHop)
	if err != nil {
		t.Fatal(err)
	}
	c, err := catalog.Parse(res.Body, link)
	if err != nil {
		t.Fatal(err)
	}
	browser, _ := c.Tool("browser")
	cur, _ := browser.Current()
	p, ok := cur.PickPackage("darwin", "arm64")
	if !ok {
		t.Fatal("no package")
	}
	dl, err := f.Get(context.Background(), p.URL, catalog.MaxPackageSizeBytes, fetch.LocalAllowed)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(dl.Body)
	if hex.EncodeToString(sum[:]) != p.SHA256 || !strings.HasPrefix(p.URL, srv.URL) {
		t.Error("downloaded package does not match its listed checksum")
	}
}

func TestInPlaceLinksPackagesWhereTheyAreAndCopiesNothing(t *testing.T) {
	tools := fixture(t)
	res, err := Generate(Options{ToolsDir: tools, InPlace: true, Name: "In place"})
	if err != nil {
		t.Fatal(err)
	}
	if res.File != filepath.Join(tools, "repository.json") {
		t.Errorf("file: %s", res.File)
	}
	if _, err := os.Stat(filepath.Join(tools, FilesDir)); !os.IsNotExist(err) {
		t.Error("in-place must not create a files folder")
	}
	browser, _ := res.Catalog.Tool("browser")
	if u := browser.Versions[0].Packages[0].URL; u != "browser/versions/tool-browser-darwin-arm64.zip" {
		t.Errorf("link: %s", u)
	}

	// served as a static folder, the links resolve and the checksums hold
	srv := httptest.NewServer(http.FileServer(http.Dir(tools)))
	defer srv.Close()
	f := &fetch.Fetcher{Timeout: 5 * time.Second}
	link := srv.URL + "/repository.json"
	got, err := f.Get(context.Background(), link, catalog.MaxFileBytes, fetch.LocalByFirstHop)
	if err != nil {
		t.Fatal(err)
	}
	c, err := catalog.Parse(got.Body, link)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range c.Tools {
		cur, _ := tl.Current()
		for _, p := range cur.Packages {
			dl, err := f.Get(context.Background(), p.URL, catalog.MaxPackageSizeBytes, fetch.LocalAllowed)
			if err != nil {
				t.Fatalf("%s: %v", p.URL, err)
			}
			sum := sha256.Sum256(dl.Body)
			if hex.EncodeToString(sum[:]) != p.SHA256 {
				t.Errorf("checksum mismatch for %s", p.URL)
			}
		}
	}
}

func TestAssetsURLLinksEveryPackageFlatAndStaysValid(t *testing.T) {
	assets := "https://github.com/a-digi/cinqo/releases/download/tools-1/"
	res, err := Generate(Options{ToolsDir: fixture(t), InPlace: true, AssetsURL: assets})
	if err != nil {
		t.Fatal(err)
	}
	browser, _ := res.Catalog.Tool("browser")
	for _, p := range browser.Versions[0].Packages {
		want := "https://github.com/a-digi/cinqo/releases/download/tools-1/tool-browser-" + p.OS + "-" + p.Arch + ".zip"
		if p.URL != want {
			t.Errorf("%s, want %s", p.URL, want)
		}
	}
	// the file on disk parses with the app's own validator, from its raw address
	data, _ := os.ReadFile(res.File)
	if _, err := catalog.Parse(data, "https://raw.githubusercontent.com/a-digi/cinqo/0.0.1/tools/repository.json"); err != nil {
		t.Fatal(err)
	}
	// AssetsURL wins over BaseURL, and a plain-http address is refused by the validator
	if _, err := Generate(Options{ToolsDir: fixture(t), OutDir: t.TempDir(), AssetsURL: "http://example.com/dl"}); err == nil {
		t.Error("a plain http assets address for a public host must be refused")
	}
	res, err = Generate(Options{ToolsDir: fixture(t), OutDir: t.TempDir(), BaseURL: "https://ignored.example.com", AssetsURL: assets})
	if err != nil || strings.Contains(res.Catalog.Tools[0].Versions[0].Packages[0].URL, "ignored") {
		t.Errorf("assets url must win: %v", err)
	}
}
