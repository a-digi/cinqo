package persona

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// Models often send a list argument as a TEXT that holds the list instead of as a list (seen in the live pass of
// plan/ai/tools/career/persona-ai-tools.md: the free model sent skills as a string 3 times and experiences once, each refused by the
// schema, each refusal costing a tool round). The bulk tools read such a text as the list it plainly is, and their schema says so.

// LooseStrings is a list of strings that is also read from a text holding it: a JSON list inside a string, the same wrapped in extra
// quotes, or a text of items separated by commas, semicolons or line breaks. Anything else (numbers, objects, a list of non-strings) is
// refused with a message saying what was expected.
type LooseStrings []string

// LooseExperiences is a list of experience entries that is also read from a text holding the JSON list (inside a string, possibly in
// extra quotes). Free text is not guessed at: an entry has several fields.
type LooseExperiences []BulkExperience

var errNotStringList = errors.New("expected a list of strings, or a text holding one (a JSON list, or items separated by commas, semicolons or line breaks)")

// maxTextLayers is how many times a text may be wrapped in quotes or in a JSON string before it is given up on.
const maxTextLayers = 3

func (l *LooseStrings) UnmarshalJSON(data []byte) error {
	t := bytes.TrimSpace(data)
	switch {
	case string(t) == "null":
		*l = nil
		return nil
	case len(t) > 0 && t[0] == '[':
		var list []string
		if err := json.Unmarshal(t, &list); err != nil {
			return errNotStringList
		}
		*l = list
		return nil
	case len(t) > 0 && t[0] == '"':
		var text string
		if err := json.Unmarshal(t, &text); err != nil {
			return errNotStringList
		}
		list, err := stringsFromText(text)
		if err != nil {
			return err
		}
		*l = list
		return nil
	}
	return errNotStringList
}

// unwrapText takes off the layers a model wraps a text in: surrounding white space and quotes, and JSON string encoding.
func unwrapText(text string) string {
	t := strings.TrimSpace(text)
	for i := 0; i < maxTextLayers && len(t) >= 2 && t[0] == '"' && t[len(t)-1] == '"'; i++ {
		var inner string
		if err := json.Unmarshal([]byte(t), &inner); err == nil {
			t = strings.TrimSpace(inner)
		} else {
			t = strings.TrimSpace(t[1 : len(t)-1])
		}
	}
	return t
}

func stringsFromText(text string) ([]string, error) {
	t := unwrapText(text)
	if strings.HasPrefix(t, "[") {
		var list []string
		if err := json.Unmarshal([]byte(t), &list); err != nil {
			return nil, errNotStringList
		}
		return list, nil
	}
	out := []string{}
	for _, part := range strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\r' }) {
		if item := strings.TrimSpace(part); item != "" { // a trailing comma or a blank line is not an item
			out = append(out, item)
		}
	}
	return out, nil
}

var errNotExperienceList = errors.New("expected a list of experience objects ({company, title, startDate, endDate, description}), or a text holding that JSON list")

func decodeExperiences(raw []byte) ([]BulkExperience, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields() // the same strictness as the schema: a misspelt field is refused, not dropped
	var list []BulkExperience
	if err := dec.Decode(&list); err != nil {
		return nil, fmt.Errorf("%w (%v)", errNotExperienceList, err)
	}
	return list, nil
}

func (l *LooseExperiences) UnmarshalJSON(data []byte) error {
	t := bytes.TrimSpace(data)
	switch {
	case string(t) == "null":
		*l = nil
		return nil
	case len(t) > 0 && t[0] == '[':
		list, err := decodeExperiences(t)
		if err != nil {
			return err
		}
		*l = list
		return nil
	case len(t) > 0 && t[0] == '"':
		var text string
		if err := json.Unmarshal(t, &text); err != nil {
			return errNotExperienceList
		}
		inner := unwrapText(text)
		if !strings.HasPrefix(inner, "[") {
			return errNotExperienceList
		}
		list, err := decodeExperiences([]byte(inner))
		if err != nil {
			return err
		}
		*l = list
		return nil
	}
	return errNotExperienceList
}

// looseSchema is the input schema of a bulk tool: the one its plain argument type gives, with the list property also allowed as a text.
// The tool's own schema is given explicitly because the SDK would otherwise infer a strict "array" and refuse a text before the tool
// could read it.
func looseSchema[T any](property, textHint string) (*jsonschema.Schema, error) {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		return nil, err
	}
	orig, ok := s.Properties[property]
	if !ok {
		return nil, fmt.Errorf("persona: no property %q in the schema", property)
	}
	list := *orig
	list.Description = ""
	s.Properties[property] = &jsonschema.Schema{
		Description: strings.TrimSpace(orig.Description + " " + textHint),
		AnyOf:       []*jsonschema.Schema{&list, {Type: "string"}},
	}
	return s, nil
}
