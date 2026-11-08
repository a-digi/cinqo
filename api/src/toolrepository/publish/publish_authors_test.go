package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/toolrepository/catalog"
)

func manifestWithMeta(slug, version, authors, license string) string {
	extra := ""
	if authors != "" {
		extra += `,"authors":` + authors
	}
	if license != "" {
		extra += fmt.Sprintf(`,"license":%q`, license)
	}
	return fmt.Sprintf(`{"slug":%q,"name":%q,"version":%q%s}`, slug, strings.ToUpper(slug[:1])+slug[1:], version, extra)
}

const (
	adaAuthors = `[{"name":"Ada Lovelace","email":"ada@example.com"}]`
	goodMeta   = `{"name":"Acme tools","description":"From the meta file","authors":[{"name":"Acme team","url":"https://acme.example"}],"company":{"name":"Acme GmbH"},"links":{"homepage":"https://acme.example","security":"mailto:security@acme.example"}}`
)

func writeMeta(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, MetaFileName)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func attributedFixture(t *testing.T) string {
	t.Helper()
	tools := t.TempDir()
	for _, plat := range []string{"darwin-arm64", "linux-amd64"} {
		put(t, tools, "browser", "tool-browser-"+plat+".zip", makeZip(t, manifestWithMeta("browser", "0.0.190", adaAuthors, "MIT")))
	}
	put(t, tools, "career", "tool-career-darwin-arm64.zip", makeZip(t, manifestWithMeta("career", "0.0.10", `[{"name":"Grace Hopper"}]`, "Apache-2.0")))
	writeMeta(t, tools, goodMeta)
	return tools
}

func TestWithEveryAuthorInPlaceTheFileIsWrittenInTheCurrentFormat(t *testing.T) {
	tools, out := attributedFixture(t), t.TempDir()
	now := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	res, err := Generate(Options{ToolsDir: tools, OutDir: out, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	c := res.Catalog
	if c.FormatVersion != catalog.FormatVersion || len(res.Warnings) != 0 || c.Legacy() {
		t.Fatalf("format %d, warnings %v", c.FormatVersion, res.Warnings)
	}
	// the repository's own facts come from the meta file
	if c.Name != "Acme tools" || c.Description != "From the meta file" || c.Authors[0].Name != "Acme team" || c.Company.Name != "Acme GmbH" ||
		c.Links.Security != "mailto:security@acme.example" || c.GeneratedAt != "2026-10-31T10:00:00Z" {
		t.Fatalf("%+v", c)
	}
	// each version's facts are its manifest's, copied
	browser, _ := c.Tool("browser")
	v, _ := browser.Current()
	if v.Authors[0].Name != "Ada Lovelace" || v.Authors[0].Email != "ada@example.com" || v.License != "MIT" {
		t.Fatalf("%+v", v.Info)
	}
	career, _ := c.Tool("career")
	if cv, _ := career.Current(); cv.Authors[0].Name != "Grace Hopper" || cv.License != "Apache-2.0" {
		t.Fatalf("%+v", cv.Info)
	}
	// and what was written is a file the app reads
	raw, _ := os.ReadFile(res.File)
	back, err := catalog.Parse(raw, "https://example.com/repository.json")
	if err != nil || back.FormatVersion != 2 || back.Authors[0].Name != "Acme team" {
		t.Fatalf("%v", err)
	}
}

func TestOptionsOverrideTheMetaFileNameAndDescription(t *testing.T) {
	res, err := Generate(Options{ToolsDir: attributedFixture(t), OutDir: t.TempDir(), Name: "Flag name", Description: "Flag description"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Catalog.Name != "Flag name" || res.Catalog.Description != "Flag description" {
		t.Fatalf("%+v", res.Catalog)
	}
}

func TestWithoutANowThereIsNoTimestampAndTheFileIsDeterministic(t *testing.T) {
	tools := attributedFixture(t)
	a, err := Generate(Options{ToolsDir: tools, OutDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Generate(Options{ToolsDir: tools, OutDir: t.TempDir()})
	ra, _ := os.ReadFile(a.File)
	rb, _ := os.ReadFile(b.File)
	if a.Catalog.GeneratedAt != "" || string(ra) != string(rb) {
		t.Fatal("generating twice must give the same bytes when no clock is given")
	}
}

func TestMissingAuthorsFallBackToTheOlderFormatAndSayWhat(t *testing.T) {
	tools := t.TempDir()
	put(t, tools, "browser", "tool-browser-darwin-arm64.zip", makeZip(t, manifestWithMeta("browser", "0.0.190", adaAuthors, ""))) // no licence
	put(t, tools, "career", "tool-career-darwin-arm64.zip", makeZip(t, manifestWithMeta("career", "0.0.10", "", "MIT")))          // no authors
	// no meta file at all: the repository has no authors either
	res, err := Generate(Options{ToolsDir: tools, OutDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Catalog.FormatVersion != catalog.LegacyFormatVersion {
		t.Fatalf("format %d", res.Catalog.FormatVersion)
	}
	joined := strings.Join(res.Warnings, "\n")
	for _, want := range []string{"the repository has no authors", "browser 0.0.190: its manifest.json has no license", "career 0.0.10: its manifest.json has no authors"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing warning %q in:\n%s", want, joined)
		}
	}
	if len(res.Warnings) != 3 {
		t.Fatalf("%v", res.Warnings)
	}
	// what was known is still written
	b, _ := res.Catalog.Tool("browser")
	if v, _ := b.Current(); v.Authors[0].Name != "Ada Lovelace" {
		t.Fatalf("%+v", v.Info)
	}
}

func TestRequireAuthorsTurnsTheGapsIntoAnError(t *testing.T) {
	tools := t.TempDir()
	put(t, tools, "browser", "tool-browser-darwin-arm64.zip", makeZip(t, manifestWithMeta("browser", "0.0.190", "", "")))
	_, err := Generate(Options{ToolsDir: tools, OutDir: t.TempDir(), RequireAuthors: true})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"who made it is missing", "the repository has no authors", "browser 0.0.190: its manifest.json has no authors", "no license"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
	if _, statErr := os.Stat(filepath.Join(tools, FileName)); statErr == nil {
		t.Fatal("no file may be written when it was refused")
	}
	// with everything in place the same option passes
	if _, err := Generate(Options{ToolsDir: attributedFixture(t), OutDir: t.TempDir(), RequireAuthors: true}); err != nil {
		t.Fatal(err)
	}
}

func TestPackagesOfOneVersionMustAgreeOnWhoMadeIt(t *testing.T) {
	tools := t.TempDir()
	put(t, tools, "browser", "tool-browser-darwin-arm64.zip", makeZip(t, manifestWithMeta("browser", "0.0.190", adaAuthors, "MIT")))
	put(t, tools, "browser", "tool-browser-linux-amd64.zip", makeZip(t, manifestWithMeta("browser", "0.0.190", `[{"name":"Someone Else"}]`, "MIT")))
	_, err := Generate(Options{ToolsDir: tools, OutDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "different authors") {
		t.Fatalf("%v", err)
	}
}

func TestTheMetaFile(t *testing.T) {
	tools := attributedFixture(t)
	// an unknown field is a typo, not something to ignore
	writeMeta(t, tools, `{"name":"X","autors":[{"name":"typo"}]}`)
	if _, err := Generate(Options{ToolsDir: tools, OutDir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), MetaFileName) {
		t.Fatalf("an unknown field must be refused: %v", err)
	}
	// broken JSON
	writeMeta(t, tools, `{not json`)
	if _, err := Generate(Options{ToolsDir: tools, OutDir: t.TempDir()}); err == nil {
		t.Fatal("broken JSON accepted")
	}
	// an author with no name: the generated file is checked by the app's own validator
	writeMeta(t, tools, `{"authors":[{"name":""}]}`)
	if _, err := Generate(Options{ToolsDir: tools, OutDir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "authors[0].name") {
		t.Fatalf("%v", err)
	}
	// a file named explicitly must exist; the default one may be absent
	if _, err := Generate(Options{ToolsDir: tools, OutDir: t.TempDir(), MetaFile: filepath.Join(tools, "nope.json")}); err == nil {
		t.Fatal("a missing explicit meta file must be an error")
	}
	os.Remove(filepath.Join(tools, MetaFileName))
	if _, err := Generate(Options{ToolsDir: tools, OutDir: t.TempDir()}); err != nil {
		t.Fatalf("an absent default meta file is fine: %v", err)
	}
	// an explicit file elsewhere
	other := filepath.Join(t.TempDir(), "mine.json")
	os.WriteFile(other, []byte(goodMeta), 0o644)
	if res, err := Generate(Options{ToolsDir: tools, OutDir: t.TempDir(), MetaFile: other}); err != nil || res.Catalog.Authors[0].Name != "Acme team" {
		t.Fatalf("%v", err)
	}
}
