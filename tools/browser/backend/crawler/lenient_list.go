package crawler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// Step 94 (part C): models often send a list argument as a string — a JSON
// array inside a string, or a comma-separated list — and the framework then
// rejects the call before the tool runs, costing a whole round of a limited
// budget. These types accept the array (unchanged) and those string forms.
// The published schema must allow both, otherwise the string is rejected
// before any of this runs (see fetchPageHTMLSchema). See
// plan/ai/tools/browser/step-94-find-repeating-items.md.

// SelectorList is removeSelectors: CSS selectors. A plain string is kept as
// ONE entry, because a comma-separated string is already a valid CSS
// selector list ("script, style, .banner") and splitting on commas would
// break selectors that contain them (":is(a, b)").
type SelectorList []string

// AttributeNameList is removeAttributes: attribute names, which never contain
// commas or spaces, so a plain string is split on both.
type AttributeNameList []string

func (l *SelectorList) UnmarshalJSON(data []byte) error {
	out, err := decodeStringList(data, "removeSelectors", func(s string) []string { return []string{s} })
	*l = out
	return err
}

func (l *AttributeNameList) UnmarshalJSON(data []byte) error {
	out, err := decodeStringList(data, "removeAttributes", func(s string) []string {
		return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	})
	*l = out
	return err
}

// decodeStringList accepts an array of strings, null, or a string: a JSON
// array of strings written inside the string, or a plain value handled by
// fromPlain.
func decodeStringList(data []byte, name string, fromPlain func(string) []string) ([]string, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	switch trimmed[0] {
	case '[':
		var list []string
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, fmt.Errorf("%s must be an array of strings: %w", name, err)
		}
		return list, nil
	case '"':
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return nil, err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, nil
		}
		if text[0] == '[' {
			var list []string
			if err := json.Unmarshal([]byte(text), &list); err != nil {
				return nil, fmt.Errorf("%s was a string that looks like a JSON array but is not valid JSON; pass an array of strings, or one comma-separated string", name)
			}
			return list, nil
		}
		return fromPlain(text), nil
	}
	return nil, fmt.Errorf("%s must be an array of strings (or one comma-separated string)", name)
}

// fetchPageHTMLSchema is the schema inferred from fetchPageHTMLArgs with the
// string form of both list properties allowed in addition to the array.
func fetchPageHTMLSchema() (*jsonschema.Schema, error) {
	schema, err := jsonschema.For[fetchPageHTMLArgs](nil)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"removeSelectors", "removeAttributes"} {
		prop := schema.Properties[name]
		if prop == nil {
			return nil, fmt.Errorf("fetch_page_html: inferred schema has no %s property", name)
		}
		switch {
		case len(prop.Types) > 0:
			prop.Types = append(prop.Types, "string")
		case prop.Type != "":
			prop.Types = []string{prop.Type, "string"}
			prop.Type = ""
		}
	}
	return schema, nil
}
