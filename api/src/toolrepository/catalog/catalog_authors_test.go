package catalog

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// v2 builds a format version 2 file with the given top-level fragment and one tool whose one version carries the
// given fragment (both without surrounding commas).
func v2(top, version string) string {
	if top != "" {
		top += ","
	}
	if version != "" {
		version += ","
	}
	return `{"formatVersion":2,` + top + `"tools":[{"id":"browser","currentVersionId":"v1","versions":[{` + version +
		`"versionId":"v1","name":"Browser","version":"1.0.0","packages":[{"os":"any","arch":"any","url":"a.zip","sha256":"` + sum + `"}]}]}]}`
}

const (
	topOK     = `"authors":[{"name":"Cinqo team","url":"https://example.com/cinqo"}]`
	versionOK = `"authors":[{"name":"Ada Lovelace","email":"ada@example.com"}],"license":"MIT"`
)

func TestFormatVersion2NeedsAuthorsEverywhere(t *testing.T) {
	c, err := Parse([]byte(v2(topOK, versionOK)), "https://e.com/r.json")
	if err != nil {
		t.Fatal(err)
	}
	if c.Legacy() || c.FormatVersion != 2 || c.Authors[0].Name != "Cinqo team" {
		t.Fatalf("%+v", c)
	}
	v := c.Tools[0].Versions[0]
	if v.Authors[0].Name != "Ada Lovelace" || v.Authors[0].Email != "ada@example.com" || v.License != "MIT" {
		t.Fatalf("a version's attribution must be read from its own entry: %+v", v.Info)
	}

	// missing at the top and in the version: both are reported together, with their paths
	ps := problems(t, v2("", ""), "https://e.com/r.json")
	if !hasProblem(ps, "authors", "required") || !hasProblem(ps, "tools[0].versions[0].authors", "required") || !hasProblem(ps, "tools[0].versions[0].license", "required") || len(ps) != 3 {
		t.Fatalf("%v", ps)
	}
	// each one on its own
	if ps := problems(t, v2("", versionOK), "https://e.com/r.json"); len(ps) != 1 || !hasProblem(ps, "authors", "required") {
		t.Fatalf("%v", ps)
	}
	if ps := problems(t, v2(topOK, `"license":"MIT"`), "https://e.com/r.json"); len(ps) != 1 || !hasProblem(ps, "tools[0].versions[0].authors", "required") {
		t.Fatalf("%v", ps)
	}
	if ps := problems(t, v2(topOK, `"authors":[{"name":"A"}]`), "https://e.com/r.json"); len(ps) != 1 || !hasProblem(ps, "tools[0].versions[0].license", "required") {
		t.Fatalf("%v", ps)
	}
}

func TestEveryOptionalFieldIsRead(t *testing.T) {
	top := topOK + `,"company":{"name":"Acme GmbH","url":"https://acme.example"},"links":{"homepage":"https://acme.example","security":"mailto:security@acme.example"},"generatedAt":"2026-10-31T10:00:00Z"`
	ver := versionOK + `,"company":{"name":"Acme"},"links":{"source":"https://github.com/acme/x","issues":"https://github.com/acme/x/issues"},"copyright":"© 2026 Acme","category":"browser","keywords":["crawl","chrome"]`
	c, err := Parse([]byte(v2(top, ver)), "https://e.com/r.json")
	if err != nil {
		t.Fatal(err)
	}
	if c.Company.Name != "Acme GmbH" || c.Links.Security != "mailto:security@acme.example" || c.GeneratedAt != "2026-10-31T10:00:00Z" {
		t.Fatalf("%+v", c)
	}
	v := c.Tools[0].Versions[0]
	if v.Company.Name != "Acme" || v.Links.Source != "https://github.com/acme/x" || v.Copyright != "© 2026 Acme" || v.Category != "browser" || len(v.Keywords) != 2 {
		t.Fatalf("%+v", v.Info)
	}
}

func TestBadMetadataIsReportedWithItsPath(t *testing.T) {
	cases := []struct{ name, top, ver, path, msg string }{
		{"empty author name", `"authors":[{"name":""}]`, versionOK, "authors[0].name", "required"},
		{"author url http", `"authors":[{"name":"A","url":"http://x.example"}]`, versionOK, "authors[0].url", "https"},
		{"version author email", topOK, `"authors":[{"name":"A","email":"nope"}],"license":"MIT"`, "tools[0].versions[0].authors[0].email", "name@example.com"},
		{"company without name", topOK + `,"company":{"url":"https://a.example"}`, versionOK, "company.name", "required"},
		{"version company url", topOK, versionOK + `,"company":{"name":"A","url":"javascript:1"}`, "tools[0].versions[0].company.url", "https"},
		{"link not https", topOK + `,"links":{"homepage":"http://a.example"}`, versionOK, "links.homepage", "https"},
		{"version link", topOK, versionOK + `,"links":{"support":"ftp://a.example"}`, "tools[0].versions[0].links.support", "https"},
		{"bad licence", topOK, `"authors":[{"name":"A"}],"license":"not a licence!"`, "tools[0].versions[0].license", "SPDX"},
		{"bad category", topOK, versionOK + `,"category":"games"`, "tools[0].versions[0].category", "must be one of"},
		{"too many keywords", topOK, versionOK + `,"keywords":["a","b","c","d","e","f","g","h","i","j","k"]`, "tools[0].versions[0].keywords", "at most 10"},
		{"long copyright", topOK, versionOK + `,"copyright":"` + strings.Repeat("c", 201) + `"`, "tools[0].versions[0].copyright", "200"},
		{"bad generatedAt", topOK + `,"generatedAt":"yesterday"`, versionOK, "generatedAt", "RFC 3339"},
	}
	for _, c := range cases {
		if ps := problems(t, v2(c.top, c.ver), "https://e.com/r.json"); !hasProblem(ps, c.path, c.msg) {
			t.Errorf("%s: want %s %q, got %v", c.name, c.path, c.msg, ps)
		}
	}
}

func TestEveryMetadataProblemIsReportedAtOnce(t *testing.T) {
	data := v2(`"authors":[{"name":""},{"name":"B","email":"bad"}],"links":{"homepage":"http://x"}`, `"authors":[{"name":"C","url":"http://x"}],"license":"??","category":"zzz"`)
	ps := problems(t, data, "https://e.com/r.json")
	if len(ps) != 6 {
		t.Fatalf("%d problems, want 6: %v", len(ps), ps)
	}
}

// A file of the older format has no authors and is still read.
func TestFormatVersion1StillParsesWithoutAuthors(t *testing.T) {
	c, err := Parse([]byte(validFile), "https://e.com/r.json")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Legacy() || len(c.Authors) != 0 || len(c.Tools[0].Versions[0].Authors) != 0 || c.Tools[0].Versions[0].License != "" {
		t.Fatalf("%+v", c)
	}
	// ... but what a version 1 file does carry is still checked
	bad := strings.Replace(validFile, `"formatVersion": 1,`, `"formatVersion": 1, "authors":[{"name":""}],`, 1)
	if ps := problems(t, bad, "https://e.com/r.json"); !hasProblem(ps, "authors[0].name", "required") {
		t.Fatalf("%v", ps)
	}
	withAuthors := strings.Replace(validFile, `"formatVersion": 1,`, `"formatVersion": 1, "authors":[{"name":"Old file author"}],`, 1)
	if c, err := Parse([]byte(withAuthors), "https://e.com/r.json"); err != nil || c.Authors[0].Name != "Old file author" {
		t.Fatalf("a version 1 file that has authors keeps them: %v", err)
	}
}

func TestLegacyReportsOnlyVersion1(t *testing.T) {
	if !(&Catalog{FormatVersion: 1}).Legacy() || (&Catalog{FormatVersion: 2}).Legacy() {
		t.Fatal("Legacy")
	}
}

// What the generator and the app write and read round-trips with the new fields.
func TestMetadataSurvivesAJSONRoundTrip(t *testing.T) {
	c, err := Parse([]byte(v2(topOK+`,"company":{"name":"Acme"}`, versionOK+`,"links":{"homepage":"https://a.example"},"keywords":["x"]`)), "https://e.com/r.json")
	if err != nil {
		t.Fatal(err)
	}
	out, err := jsonMarshal(c)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(out, "https://e.com/r.json")
	if err != nil {
		t.Fatalf("a file this package writes must be one it reads: %v\n%s", err, out)
	}
	v, w := c.Tools[0].Versions[0], again.Tools[0].Versions[0]
	if !reflect.DeepEqual(v.Info, w.Info) || again.Authors[0].Name != "Cinqo team" || again.Company.Name != "Acme" {
		t.Fatalf("%+v vs %+v", v.Info, w.Info)
	}
	if !strings.Contains(string(out), `"authors"`) || strings.Contains(string(out), `"copyright"`) {
		t.Fatalf("an empty optional field must not be written: %s", out)
	}
}

func TestMetadataProblemsKeepTheirErrorType(t *testing.T) {
	_, err := Parse([]byte(v2("", "")), "https://e.com/r.json")
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Problems) == 0 || !strings.Contains(err.Error(), "authors") {
		t.Fatalf("%v", err)
	}
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
