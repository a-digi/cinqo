package manifest

import (
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/toolmeta"
)

// testInfo is the attribution every test manifest carries: authors and a licence are required to install.
func testInfo() toolmeta.Info {
	return toolmeta.Info{Authors: []toolmeta.Author{{Name: "Test Author"}}, License: "MIT"}
}

func metaManifest(mut func(*Manifest)) Manifest {
	m := Manifest{Info: testInfo(), Slug: "demo", Name: "Demo", Version: "1.0.0"}
	if mut != nil {
		mut(&m)
	}
	return m
}

var frontendOnlyInput = ValidationInput{HasFrontendBundle: true, CurrentAppVersion: "1.0.0"}

func TestAManifestWithoutAuthorsOrLicenseIsRefusedAndTheFieldsAreNamed(t *testing.T) {
	_, err := Validate(metaManifest(func(m *Manifest) { m.Info = toolmeta.Info{} }), frontendOnlyInput)
	if err == nil {
		t.Fatal("a manifest with no authors and no licence was accepted")
	}
	for _, want := range []string{"authors", "license", "required"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message must name %q: %v", want, err)
		}
	}
	// each on its own
	if _, err := Validate(metaManifest(func(m *Manifest) { m.Authors = nil }), frontendOnlyInput); err == nil || !strings.Contains(err.Error(), "authors") || strings.Contains(err.Error(), "license") {
		t.Fatalf("no authors: %v", err)
	}
	if _, err := Validate(metaManifest(func(m *Manifest) { m.License = "" }), frontendOnlyInput); err == nil || !strings.Contains(err.Error(), "license") || strings.Contains(err.Error(), "authors") {
		t.Fatalf("no licence: %v", err)
	}
}

func TestAManifestWithAllTheMetadataIsAccepted(t *testing.T) {
	m := metaManifest(func(m *Manifest) {
		m.Authors = []toolmeta.Author{{Name: "Ada Lovelace", Email: "ada@example.com", URL: "https://ada.example"}, {Name: "Grace Hopper"}}
		m.Company = &toolmeta.Company{Name: "Acme GmbH", URL: "https://acme.example"}
		m.License = "MIT OR Apache-2.0"
		m.Links = &toolmeta.Links{Homepage: "https://acme.example", Source: "https://github.com/acme/x", Security: "mailto:security@acme.example"}
		m.Copyright = "© 2026 Acme GmbH"
		m.Category = "browser"
		m.Keywords = []string{"crawl", "chrome"}
	})
	if _, err := Validate(m, frontendOnlyInput); err != nil {
		t.Fatal(err)
	}
}

func TestEveryMetadataProblemIsReportedAtOnce(t *testing.T) {
	m := metaManifest(func(m *Manifest) {
		m.Authors = []toolmeta.Author{{Name: ""}, {Name: "B", Email: "bad"}}
		m.License = "??"
		m.Links = &toolmeta.Links{Homepage: "http://x.example"}
		m.Category = "games"
	})
	err := ValidateMetadata(m)
	if err == nil {
		t.Fatal("accepted")
	}
	for _, path := range []string{"authors[0].name", "authors[1].email", "license", "links.homepage", "category"} {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("missing %q in %v", path, err)
		}
	}
}

func TestParseReadsTheMetadataFromTheTopLevelOfTheJSON(t *testing.T) {
	raw := `{"slug":"demo","name":"Demo","version":"1.0.0",
	  "authors":[{"name":"Ada Lovelace","email":"ada@example.com"}],
	  "company":{"name":"Acme GmbH","url":"https://acme.example"},
	  "license":"MIT",
	  "links":{"homepage":"https://acme.example","issues":"https://acme.example/issues"},
	  "copyright":"© 2026 Acme GmbH","category":"browser","keywords":["crawl"],
	  "someFutureField":{"x":1}}`
	m, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("unknown fields must still parse: %v", err)
	}
	if m.Authors[0].Name != "Ada Lovelace" || m.Company.Name != "Acme GmbH" || m.License != "MIT" || m.Links.Issues != "https://acme.example/issues" ||
		m.Copyright != "© 2026 Acme GmbH" || m.Category != "browser" || m.Keywords[0] != "crawl" {
		t.Fatalf("%+v", m.Info)
	}
	if _, err := Validate(m, frontendOnlyInput); err != nil {
		t.Fatal(err)
	}
}

// A manifest written before this change still parses (so a tool already installed, and an older package, can
// be read); it just does not validate for an install.
func TestAnOldManifestParsesButDoesNotInstall(t *testing.T) {
	m, err := Parse([]byte(`{"slug":"demo","name":"Demo","version":"1.0.0"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Authors) != 0 || m.License != "" {
		t.Fatalf("%+v", m.Info)
	}
	if _, err := Validate(m, frontendOnlyInput); err == nil {
		t.Fatal("an old manifest must not install")
	}
}
