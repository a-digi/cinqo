package crawler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// FieldList is the `fields` argument of extract_page_data and extract_from_url:
// a list of {label, selector, ...} objects. Weaker models often send the whole
// list as a string (a JSON array written inside a string) and the framework then
// rejects the call before the tool runs, which wastes a round of a limited
// tool-call budget (the "keeps sending invalid tool arguments" failure). This
// accepts the array unchanged and that string form. The published schema must
// allow both, otherwise the string is rejected before any of this runs (see
// withStringFields). Same approach as SelectorList in lenient_list.go.
type FieldList []extractField

const fieldsShapeHint = `pass fields as a list of objects, each {"label": "title", "selector": "h2 a"}, not as text`

func (l *FieldList) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*l = nil
		return nil
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return err
		}
		trimmed = bytes.TrimSpace([]byte(text))
		if len(trimmed) == 0 {
			*l = nil
			return nil
		}
		if trimmed[0] != '[' {
			return fmt.Errorf("fields was a string that is not a JSON list; %s", fieldsShapeHint)
		}
		if err := l.decodeList(trimmed); err != nil {
			return fmt.Errorf("fields was a string that looks like a JSON list but is not valid: %v; %s", err, fieldsShapeHint)
		}
		return nil
	}
	if trimmed[0] != '[' {
		return fmt.Errorf("fields must be a list of objects; %s", fieldsShapeHint)
	}
	if err := l.decodeList(trimmed); err != nil {
		return fmt.Errorf("fields must be a list of objects with a label and a selector: %v; %s", err, fieldsShapeHint)
	}
	return nil
}

func (l *FieldList) decodeList(data []byte) error {
	var list []extractField
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	*l = list
	return nil
}

// withStringFields returns the schema inferred from T with the string form of
// the `fields` property allowed in addition to the array.
func withStringFields[T any](tool string) (*jsonschema.Schema, error) {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		return nil, err
	}
	prop := schema.Properties["fields"]
	if prop == nil {
		return nil, fmt.Errorf("%s: inferred schema has no fields property", tool)
	}
	allowString(prop)
	prop.Description = strings.TrimSpace(prop.Description) + ` — a list of {"label", "selector"} objects (optionally "attribute", "multiple"); send it as a real list, not as a string`
	return schema, nil
}

// allowString adds "string" to a property's accepted types, whether the schema
// states one type or several.
func allowString(prop *jsonschema.Schema) {
	switch {
	case len(prop.Types) > 0:
		prop.Types = append(prop.Types, "string")
	case prop.Type != "":
		prop.Types = []string{prop.Type, "string"}
		prop.Type = ""
	}
}

func mustSchema(schema *jsonschema.Schema, err error) *jsonschema.Schema {
	if err != nil {
		panic(err) // a programming error caught by the first test run, not a runtime condition
	}
	return schema
}
