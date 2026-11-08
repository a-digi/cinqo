package detailpick

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"career-tool-backend/portal"
)

// Errors of building instructions from choices; each wraps portal.ErrInvalidCrawlInstructions so callers answer them like any invalid
// instruction.
var (
	ErrNoDescription    = fmt.Errorf("%w: choose where the description is (a box or a description rule)", portal.ErrInvalidCrawlInstructions)
	ErrBothDescriptions = fmt.Errorf("%w: choose either a description box or a description rule, not both", portal.ErrInvalidCrawlInstructions)
	// ErrReplaceTitleNeedsTitle: replacing the title needs a title to replace it with.
	ErrReplaceTitleNeedsTitle = fmt.Errorf("%w: replacing the title needs the title field to be chosen", portal.ErrInvalidCrawlInstructions)
)

// FieldChoice is the element a person chose for one field. A field the person marked "not on this page" is simply not in the list.
type FieldChoice struct {
	Label     Field  `json:"label"`
	Selector  string `json:"selector,omitempty"`
	Attribute string `json:"attribute,omitempty"`
	// Source and Path read the value from the page's structured data (jsonld or meta) instead of an element: such a choice has no selector
	// or attribute (plan 186, Phase B).
	Source string `json:"source,omitempty"`
	Path   string `json:"path,omitempty"`
}

// Choices are everything a person decided on the picking screen.
type Choices struct {
	// Fields are title, company, location and postedAt (never description: that has its own two ways below).
	Fields []FieldChoice `json:"fields"`
	// DescriptionSelector reads the description from one element, as the AI's instructions do.
	DescriptionSelector string `json:"descriptionSelector,omitempty"`
	// Description reads it by a rule (parts, exclusions, headings, format, limit). Exactly one of the two must be set.
	Description *portal.DescriptionRule `json:"description,omitempty"`
	// ReplaceTitle lets the title the page states replace the job's saved title after a detail crawl (decision D2: off by default; needs
	// the title field; the replacement is said in the run log).
	ReplaceTitle bool `json:"replaceTitle,omitempty"`
}

const maxSelectorRunes = 300

var attributeName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9:_-]{0,39}$`)

func checkSelector(what, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%w: %s is empty", portal.ErrInvalidCrawlInstructions, what)
	}
	if utf8.RuneCountInString(s) > maxSelectorRunes {
		return fmt.Errorf("%w: %s is longer than %d characters", portal.ErrInvalidCrawlInstructions, what, maxSelectorRunes)
	}
	for _, r := range s {
		if r == 0 || (unicode.IsControl(r) && r != '\t') {
			return fmt.Errorf("%w: %s contains a control character", portal.ErrInvalidCrawlInstructions, what)
		}
	}
	return nil
}

func normAttribute(a string) (string, error) {
	a = strings.TrimSpace(a)
	if a == "" || a == "text" {
		return "", nil
	}
	if !attributeName.MatchString(a) {
		return "", fmt.Errorf("%w: %q is not a usable attribute name", portal.ErrInvalidCrawlInstructions, a)
	}
	return a, nil
}

var labelOrder = []Field{FieldTitle, FieldCompany, FieldLocation, FieldPostedAt}

type outField struct {
	Label     string `yaml:"label"`
	Selector  string `yaml:"selector,omitempty"`
	Attribute string `yaml:"attribute,omitempty"`
	Source    string `yaml:"source,omitempty"`
	Path      string `yaml:"path,omitempty"`
}

type outDoc struct {
	Fields          []outField              `yaml:"fields,omitempty"`
	DescriptionRule *portal.DescriptionRule `yaml:"descriptionRule,omitempty"`
	ReplaceTitle    bool                    `yaml:"replaceTitle,omitempty"`
}

// BuildYAML turns the choices into the stored job-detail instructions. The text is marshalled, never assembled from strings, so no
// selector can add a key or a field; the result is checked by the same function the save route uses, so anything BuildYAML returns is
// valid. The fields come in a fixed order (description, title, company, location, postedAt).
func BuildYAML(c Choices) (string, error) {
	hasBox := strings.TrimSpace(c.DescriptionSelector) != ""
	switch {
	case hasBox && c.Description != nil:
		return "", ErrBothDescriptions
	case !hasBox && c.Description == nil:
		return "", ErrNoDescription
	}
	var doc outDoc
	if hasBox {
		if err := checkSelector("the description selector", c.DescriptionSelector); err != nil {
			return "", err
		}
		doc.Fields = append(doc.Fields, outField{Label: string(FieldDescription), Selector: c.DescriptionSelector})
	} else {
		if err := portal.ValidateDescriptionRule(c.Description); err != nil {
			return "", err
		}
		rule := *c.Description
		doc.DescriptionRule = &rule
	}
	byLabel := map[Field]FieldChoice{}
	for _, f := range c.Fields {
		known := false
		for _, l := range labelOrder {
			if f.Label == l {
				known = true
			}
		}
		if !known {
			return "", fmt.Errorf("%w: %q is not a field a job page can fill (use title, company, location or postedAt)", portal.ErrInvalidCrawlInstructions, f.Label)
		}
		if _, dup := byLabel[f.Label]; dup {
			return "", fmt.Errorf("%w: the field %q is chosen twice", portal.ErrInvalidCrawlInstructions, f.Label)
		}
		if f.Source != "" || f.Path != "" {
			if strings.TrimSpace(f.Selector) != "" || strings.TrimSpace(f.Attribute) != "" {
				return "", fmt.Errorf("%w: %q is read from the page's structured data: it has no selector or attribute", portal.ErrInvalidCrawlInstructions, f.Label)
			}
			if err := portal.ValidateStructuredSource(string(f.Label), f.Source, f.Path); err != nil {
				return "", fmt.Errorf("%w: %v", portal.ErrInvalidCrawlInstructions, err)
			}
			f.Selector, f.Attribute = "", ""
			byLabel[f.Label] = f
			continue
		}
		if err := checkSelector("the selector of "+string(f.Label), f.Selector); err != nil {
			return "", err
		}
		attr, err := normAttribute(f.Attribute)
		if err != nil {
			return "", err
		}
		f.Attribute = attr
		byLabel[f.Label] = f
	}
	for _, l := range labelOrder {
		if f, ok := byLabel[l]; ok {
			doc.Fields = append(doc.Fields, outField{Label: string(f.Label), Selector: f.Selector, Attribute: f.Attribute, Source: f.Source, Path: f.Path})
		}
	}
	if c.ReplaceTitle {
		if _, ok := byLabel[FieldTitle]; !ok {
			return "", ErrReplaceTitleNeedsTitle
		}
		doc.ReplaceTitle = true
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("%w: %v", portal.ErrInvalidCrawlInstructions, err)
	}
	text := string(raw)
	if _, err := portal.BuildJobDetailCrawlRequestFromYAML(text); err != nil {
		return "", err
	}
	return text, nil
}

type inField struct {
	Label     string `yaml:"label"`
	Selector  string `yaml:"selector"`
	Attribute string `yaml:"attribute"`
	Multiple  bool   `yaml:"multiple"`
	Source    string `yaml:"source"`
	Path      string `yaml:"path"`
}

type inDoc struct {
	Container       string                  `yaml:"container"`
	Fields          []inField               `yaml:"fields"`
	Mapping         map[string]string       `yaml:"mapping"`
	Pagination      map[string]any          `yaml:"pagination"`
	DescriptionRule *portal.DescriptionRule `yaml:"descriptionRule"`
	ReplaceTitle    bool                    `yaml:"replaceTitle"`
}

// ParseYAML reads stored instructions back into choices, so the screen can show and edit them. What the screen cannot represent (a
// field it does not know, a `multiple` field, a container, a mapping, pagination, an attribute on the description) is not lost silently:
// it is listed in lossy, and saving the choices would drop it.
func ParseYAML(text string) (c Choices, lossy []string, err error) {
	var doc inDoc
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return Choices{}, nil, fmt.Errorf("%w: not valid YAML: %v", portal.ErrInvalidCrawlInstructions, err)
	}
	if doc.Container != "" {
		lossy = append(lossy, "a container")
	}
	if len(doc.Mapping) > 0 {
		lossy = append(lossy, "a field mapping")
	}
	for k, v := range doc.Pagination {
		if v != nil && v != "" && v != 0 {
			lossy = append(lossy, "pagination")
			break
		}
		_ = k
	}
	c.Description = doc.DescriptionRule
	c.ReplaceTitle = doc.ReplaceTitle
	// Never nil: the screen reads the JSON list (instructions with only a description must give [], not null).
	c.Fields = []FieldChoice{}
	for _, f := range doc.Fields {
		if f.Multiple {
			lossy = append(lossy, "the field "+f.Label+" collects every match")
		}
		switch Field(f.Label) {
		case FieldDescription:
			if c.Description != nil {
				lossy = append(lossy, "a description field next to a description rule (the rule is used)")
				continue
			}
			c.DescriptionSelector = f.Selector
			if f.Attribute != "" && f.Attribute != "text" {
				lossy = append(lossy, "an attribute on the description")
			}
		case FieldTitle, FieldCompany, FieldLocation, FieldPostedAt:
			c.Fields = append(c.Fields, FieldChoice{Label: Field(f.Label), Selector: f.Selector, Attribute: f.Attribute, Source: f.Source, Path: f.Path})
		default:
			lossy = append(lossy, "the field "+f.Label)
		}
	}
	return c, lossy, nil
}

// IsInvalid reports whether err is an invalid-instructions refusal.
func IsInvalid(err error) bool { return errors.Is(err, portal.ErrInvalidCrawlInstructions) }
