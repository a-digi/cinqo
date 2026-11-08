// structured_source.go (plan 186, step 10, Phase B) — a job-detail field read from the structured data of the page (its JobPosting JSON-LD,
// its meta tags) instead of from an element.
//
// In the instructions a field is `{label, source: jsonld|meta, path}` (no selector, attribute or multiple), and a part of a description
// rule may take `{source, path}` in place of `include`. The Browser tool reads it (its extraction and its description rule); this file is
// Career's copy of the bounds the Browser validates, and a test reads the Browser's source and fails if the two drift apart. A document
// without any source is read, validated and sent exactly as before.
package portal

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// The sources a field may be read from.
const (
	StructuredSourceJSONLD = "jsonld"
	StructuredSourceMeta   = "meta"
	// MaxStructuredPathLen bounds a path.
	MaxStructuredPathLen = 120
)

var (
	structuredJSONLDPath = regexp.MustCompile(`^[A-Za-z0-9_@$-]+(\.[A-Za-z0-9_@$-]+)*$`)
	structuredMetaPath   = regexp.MustCompile(`^[A-Za-z0-9_:.-]+$`)
)

// ValidateStructuredSource checks a source and a path (a field's, or a description part's); what names the field in the message.
func ValidateStructuredSource(what, source, path string) error {
	var re *regexp.Regexp
	switch source {
	case StructuredSourceJSONLD:
		re = structuredJSONLDPath
	case StructuredSourceMeta:
		re = structuredMetaPath
	default:
		return fmt.Errorf("%s: the source %q is not known: use %s or %s", what, source, StructuredSourceJSONLD, StructuredSourceMeta)
	}
	if path == "" || len(path) > MaxStructuredPathLen || !re.MatchString(path) {
		return fmt.Errorf("%s: the path %q is not valid for %s (at most %d characters)", what, path, source, MaxStructuredPathLen)
	}
	return nil
}

// UsesStructuredSources reports whether the instructions read anything from the page's structured data (a field or a description part).
// The Browser tool must be able to do that: an older one cannot.
func UsesStructuredSources(yamlText string) bool {
	var doc jobDetailCrawlInstructionsDoc
	if err := yaml.Unmarshal([]byte(yamlText), &doc); err != nil {
		return false
	}
	for _, f := range doc.Fields {
		if f.Source != "" {
			return true
		}
	}
	if doc.DescriptionRule != nil {
		for _, p := range doc.DescriptionRule.Parts {
			if p.Source != "" {
				return true
			}
		}
	}
	return false
}

// UsesStructured reports the same for a built request.
func (r CrawlRequest) UsesStructured() bool {
	for _, f := range r.Fields {
		if f.Source != "" {
			return true
		}
	}
	if r.DescriptionRule != nil {
		for _, p := range r.DescriptionRule.Parts {
			if p.Source != "" {
				return true
			}
		}
	}
	return false
}
