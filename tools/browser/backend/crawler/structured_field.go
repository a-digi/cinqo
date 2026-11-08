// structured_field.go (plan 186, step 10, Phase B) — a field read from the structured data of the page (its JobPosting JSON-LD, its meta
// tags) instead of from an element.
//
// A field with `source` ("jsonld" or "meta") and `path` has no selector, attribute or `multiple`; it is read by structured_reader.js on the
// live page. The same source and path may stand in place of `include` in a part of a description rule. A request without them behaves
// exactly as before. Microdata is not offered: none of the 23 pages of the step 0 spike carried any.
//
// Validated here because the Career tool validates what it stores and this tool validates what it runs; Career keeps a copy of these
// bounds and a test reads this file to check the two agree.
package crawler

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

//go:embed structured_reader.js
var structuredReaderJS string

// structuredReaderPlaceholder is the token the reader is spliced in at, in extract.js and description_rule.js.
const structuredReaderPlaceholder = "/*STRUCTURED_READER_HERE*/"

// The sources a field may be read from.
const (
	StructuredSourceJSONLD = "jsonld"
	StructuredSourceMeta   = "meta"
)

// MaxStructuredPathLen bounds a path.
const MaxStructuredPathLen = 120

var (
	jsonldPath = regexp.MustCompile(`^[A-Za-z0-9_@$-]+(\.[A-Za-z0-9_@$-]+)*$`)
	metaPath   = regexp.MustCompile(`^[A-Za-z0-9_:.-]+$`)
)

// withStructuredReader splices the reader into a script.
func withStructuredReader(script string) string {
	return strings.Replace(script, structuredReaderPlaceholder, structuredReaderJS, 1)
}

// validateStructuredSource checks a source and path (a field's, or a description part's).
func validateStructuredSource(what, source, path string) error {
	var re *regexp.Regexp
	switch source {
	case StructuredSourceJSONLD:
		re = jsonldPath
	case StructuredSourceMeta:
		re = metaPath
	default:
		return fmt.Errorf("%s: the source %q is not known: use %s or %s", what, source, StructuredSourceJSONLD, StructuredSourceMeta)
	}
	if path == "" || len(path) > MaxStructuredPathLen || !re.MatchString(path) {
		return fmt.Errorf("%s: the path %q is not valid for %s (at most %d characters)", what, path, source, MaxStructuredPathLen)
	}
	return nil
}

// validateExtractField checks one field of an extraction: a selector field needs its selector, a structured one its source and path and
// nothing of an element.
func validateExtractField(f extractField) error {
	if f.Label == "" {
		return errors.New("every field requires a label")
	}
	if f.Source == "" {
		if f.Path != "" {
			return fmt.Errorf("field %q: a path needs a source", f.Label)
		}
		if f.Selector == "" {
			return errors.New("every field requires both a label and a selector")
		}
		return nil
	}
	if f.Selector != "" || f.Attribute != "" || f.Multiple {
		return fmt.Errorf("field %q: a field read from the page's structured data has no selector, attribute or multiple", f.Label)
	}
	return validateStructuredSource(fmt.Sprintf("field %q", f.Label), f.Source, f.Path)
}

// validateExtractFields checks every field; structured fields cannot be read per container.
func validateExtractFields(container string, fields []extractField) error {
	for _, f := range fields {
		if err := validateExtractField(f); err != nil {
			return err
		}
		if f.Source != "" && container != "" {
			return fmt.Errorf("field %q: structured data cannot be read per container", f.Label)
		}
	}
	return nil
}

// CapabilitiesHandler handles GET /capabilities: what this build of the tool can do, so a caller that needs a newer feature (Career's
// structured fields) can refuse politely when the installed tool is older (an older tool has no such route).
func CapabilitiesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]bool{"structuredFields": true, "descriptionRule": true})
}
