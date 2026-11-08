package portal

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestValidateStructuredSource(t *testing.T) {
	for _, c := range []struct {
		source, path string
		ok           bool
	}{
		{"jsonld", "title", true}, {"jsonld", "hiringOrganization.name", true}, {"jsonld", "jobLocation.0.address", true}, {"jsonld", "@type", true},
		{"jsonld", "", false}, {"jsonld", "a..b", false}, {"jsonld", ".a", false}, {"jsonld", "a.", false}, {"jsonld", "a b", false}, {"jsonld", "a[0]", false},
		{"jsonld", `a"b`, false}, {"jsonld", "a:b", false}, {"jsonld", strings.Repeat("a", MaxStructuredPathLen+1), false}, {"jsonld", strings.Repeat("a", MaxStructuredPathLen), true},
		{"meta", "og:title", true}, {"meta", "twitter:title", true}, {"meta", "article:published_time", true}, {"meta", "", false}, {"meta", "og title", false}, {"meta", `og"title`, false},
		{"microdata", "title", false}, {"", "title", false}, {"JSONLD", "title", false},
	} {
		if err := ValidateStructuredSource("x", c.source, c.path); (err == nil) != c.ok {
			t.Errorf("%s %q: %v, want ok=%v", c.source, c.path, err, c.ok)
		}
	}
}

const structuredDetailYAML = `fields:
  - label: title
    source: jsonld
    path: title
  - label: company
    source: jsonld
    path: hiringOrganization
  - label: location
    source: jsonld
    path: jobLocation
  - label: postedAt
    source: jsonld
    path: datePosted
  - label: og
    source: meta
    path: og:title
descriptionRule:
  format: structured
  parts:
    - source: jsonld
      path: description
      cutAfter:
        - Similar jobs
`

func TestStructuredFieldsAndDescriptionPartsAreValidInstructions(t *testing.T) {
	if err := validateJobDetailCrawlInstructions(structuredDetailYAML); err != nil {
		t.Fatal(err)
	}
	// a structured description part alone satisfies the description label
	only := "descriptionRule:\n  parts:\n    - source: jsonld\n      path: description\n"
	if err := validateJobDetailCrawlInstructions(only); err != nil {
		t.Errorf("a structured part alone: %v", err)
	}
	// a part may mix boxes and structured text
	mixed := "descriptionRule:\n  parts:\n    - include: '#a'\n    - source: jsonld\n      path: description\n"
	if err := validateJobDetailCrawlInstructions(mixed); err != nil {
		t.Errorf("mixed parts: %v", err)
	}
}

func TestBadStructuredFieldsAreRefused(t *testing.T) {
	desc := "  - label: description\n    selector: .d\n"
	for name, y := range map[string]string{
		"path without source":    "fields:\n" + desc + "  - label: a\n    path: title\n    selector: h1\n",
		"source with selector":   "fields:\n" + desc + "  - label: a\n    source: jsonld\n    path: title\n    selector: h1\n",
		"source with attribute":  "fields:\n" + desc + "  - label: a\n    source: jsonld\n    path: title\n    attribute: href\n",
		"source with multiple":   "fields:\n" + desc + "  - label: a\n    source: jsonld\n    path: title\n    multiple: true\n",
		"unknown source":         "fields:\n" + desc + "  - label: a\n    source: microdata\n    path: title\n",
		"source without path":    "fields:\n" + desc + "  - label: a\n    source: jsonld\n",
		"a bad path":             "fields:\n" + desc + "  - label: a\n    source: jsonld\n    path: a b\n",
		"a meta path for jsonld": "fields:\n" + desc + "  - label: a\n    source: jsonld\n    path: og:title\n",
		"part with both":         "descriptionRule:\n  parts:\n    - include: '#a'\n      source: jsonld\n      path: description\n",
		"part source only":       "descriptionRule:\n  parts:\n    - source: jsonld\n",
		"part path only":         "descriptionRule:\n  parts:\n    - path: description\n",
		"part bad source":        "descriptionRule:\n  parts:\n    - source: microdata\n      path: description\n",
	} {
		if err := validateJobDetailCrawlInstructions(y); !errors.Is(err, ErrInvalidCrawlInstructions) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// the listing instructions never take a source: their fields need a selector as before
	listing := "container: .j\nfields:\n  - label: title\n    source: jsonld\n    path: title\n  - label: url\n    selector: a\n    attribute: href\npagination: {}\n"
	if err := validateCrawlInstructions(listing); !errors.Is(err, ErrInvalidCrawlInstructions) {
		t.Errorf("a listing field with a source: %v", err)
	}
}

func TestTheRequestCarriesSourceAndPathAndOldInstructionsAreUnchanged(t *testing.T) {
	req, err := BuildJobDetailCrawlRequestFromYAML(structuredDetailYAML)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(req)
	var got struct {
		Fields []struct {
			Label, Selector, Source, Path, Attribute string
			Multiple                                 bool
		} `json:"fields"`
		DescriptionRule struct {
			Parts []struct{ Include, Source, Path string } `json:"parts"`
		} `json:"descriptionRule"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Fields) != 5 || got.Fields[1].Source != "jsonld" || got.Fields[1].Path != "hiringOrganization" || got.Fields[1].Selector != "" || got.Fields[4].Source != "meta" || got.Fields[4].Path != "og:title" {
		t.Fatalf("fields: %s", raw)
	}
	if p := got.DescriptionRule.Parts; len(p) != 1 || p[0].Source != "jsonld" || p[0].Path != "description" || p[0].Include != "" {
		t.Fatalf("part: %s", raw)
	}
	if !strings.Contains(string(raw), `"source":"jsonld"`) {
		t.Errorf("source key missing: %s", raw)
	}
	// instructions without a source: not a single new key in what the Browser is sent
	old, _ := BuildJobDetailCrawlRequestFromYAML(oldDetailYAML)
	oldRaw, _ := json.Marshal(old)
	if strings.Contains(string(oldRaw), "source") || strings.Contains(string(oldRaw), "path") {
		t.Errorf("old instructions changed: %s", oldRaw)
	}
	// a listing request never carries a source, whatever the stored text says
	listingReq, err := BuildCrawlRequestFromYAML("container: .j\nfields:\n  - label: title\n    selector: h2\n    source: jsonld\n    path: title\n  - label: url\n    selector: a\n    attribute: href\npagination: {}\n")
	if err == nil && strings.Contains(mustJSON(listingReq), "source") {
		t.Errorf("a listing field leaked a source: %s", mustJSON(listingReq))
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestUsesStructuredSources(t *testing.T) {
	for name, c := range map[string]struct {
		yaml string
		want bool
	}{
		"structured fields":      {structuredDetailYAML, true},
		"only a structured part": {"descriptionRule:\n  parts:\n    - source: jsonld\n      path: description\n", true},
		"old instructions":       {oldDetailYAML, false},
		"a rule of boxes":        {ruleDetailYAML, false},
		"not yaml":               {"fields: [", false},
		"empty":                  {"", false},
	} {
		if got := UsesStructuredSources(c.yaml); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
		if req, err := BuildJobDetailCrawlRequestFromYAML(c.yaml); err == nil && req.UsesStructured() != c.want {
			t.Errorf("%s (request): %v", name, req.UsesStructured())
		}
	}
}

// The two tools are separate modules, so the bounds are copied; this fails when the copies drift apart.
func TestTheStructuredBoundsMirrorTheBrowsers(t *testing.T) {
	raw, err := os.ReadFile("../../browser/backend/crawler/structured_field.go")
	if err != nil {
		t.Skipf("the Browser's source is not next to this one: %v", err)
	}
	src := string(raw)
	m := regexp.MustCompile(`MaxStructuredPathLen\s*=\s*(\d+)`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("MaxStructuredPathLen not found in the Browser's structured_field.go")
	}
	if n, _ := strconv.Atoi(m[1]); n != MaxStructuredPathLen {
		t.Errorf("the Browser says %d, this tool says %d", n, MaxStructuredPathLen)
	}
	for name, re := range map[string]*regexp.Regexp{"jsonldPath": structuredJSONLDPath, "metaPath": structuredMetaPath} {
		mm := regexp.MustCompile(name + "\\s*=\\s*regexp\\.MustCompile\\(`([^`]*)`\\)").FindStringSubmatch(src)
		if mm == nil {
			t.Errorf("%s not found in the Browser's source", name)
			continue
		}
		if mm[1] != re.String() {
			t.Errorf("%s: the Browser says %s, this tool says %s", name, mm[1], re.String())
		}
	}
	for _, want := range []string{`StructuredSourceJSONLD = "jsonld"`, `StructuredSourceMeta   = "meta"`, `json:"source,omitempty"`} {
		if !strings.Contains(src+readFileOr(t, "../../browser/backend/crawler/extract.go"), want) {
			t.Errorf("the Browser no longer has %q", want)
		}
	}
}

func readFileOr(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
