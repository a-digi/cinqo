// description_rule.go (plan 186, step 3) — the description rule in a link's stored job-detail instructions.
//
// A job-detail instruction document is `fields: [...]` (as before). It may now also carry a `descriptionRule`: how to read
// the job's full description off its page (the boxes to take, what to leave out, a heading, where to stop, the format
// and the limit). The rule is the Browser tool's `descriptionRule` (tools/browser/backend/crawler/description_rule.go)
// written in YAML: the same keys, the same bounds, validated here as well because this tool validates what it stores and
// the Browser validates what it runs. A test reads the Browser's constants and fails if the two drift apart.
//
// Backward compatible: an instruction document without the key is read, validated and sent exactly as before. With the
// key, the label `description` is satisfied by the rule (a document may then have no `fields` at all).
package portal

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The bounds of a rule; they mirror the Browser tool's (see the file comment).
const (
	DefaultDescriptionLimit = 60000
	MinDescriptionLimit     = 1000
	MaxDescriptionLimit     = 200000
	MaxDescriptionParts     = 8
	MaxRuleSelectorLen      = 300
	MaxRuleExcludes         = 20
	MaxRuleCutAfter         = 10
	MaxRuleCutAfterLen      = 200
	// DescriptionLabel is the key the description is stored under; a rule can only produce this one.
	DescriptionLabel = "description"
)

// DescriptionPart is one box of a description rule.
type DescriptionPart struct {
	// A part takes either an Include (a box selector) or a Source and Path (the text the page states in its structured data, for example the
	// JSON-LD description), never both.
	Include         string   `yaml:"include,omitempty" json:"include,omitempty"`
	Source          string   `yaml:"source,omitempty" json:"source,omitempty"`
	Path            string   `yaml:"path,omitempty" json:"path,omitempty"`
	Heading         string   `yaml:"heading,omitempty" json:"heading,omitempty"`
	HeadingSelector string   `yaml:"headingSelector,omitempty" json:"headingSelector,omitempty"`
	Exclude         []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	CutAfter        []string `yaml:"cutAfter,omitempty" json:"cutAfter,omitempty"`
}

// DescriptionRule is how a job's description is read off its page.
type DescriptionRule struct {
	// Label must be empty or "description": the saved job reads that key.
	Label  string            `yaml:"label,omitempty" json:"label,omitempty"`
	Parts  []DescriptionPart `yaml:"parts" json:"parts"`
	Format string            `yaml:"format,omitempty" json:"format,omitempty"`
	Limit  int               `yaml:"limit,omitempty" json:"limit,omitempty"`
}

func checkRuleSelector(what, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is empty", what)
	}
	if utf8.RuneCountInString(s) > MaxRuleSelectorLen {
		return fmt.Errorf("%s is longer than %d characters", what, MaxRuleSelectorLen)
	}
	for _, r := range s {
		if r == 0 || (unicode.IsControl(r) && r != '\t') {
			return fmt.Errorf("%s contains a control character", what)
		}
	}
	return nil
}

// ValidateDescriptionRule checks a rule without changing it (the stored text stays as written; the Browser fills the
// defaults). The error wraps ErrInvalidCrawlInstructions.
func ValidateDescriptionRule(r *DescriptionRule) error {
	if r == nil {
		return nil
	}
	if err := validateDescriptionRule(r); err != nil {
		return fmt.Errorf("%w: descriptionRule: %v", ErrInvalidCrawlInstructions, err)
	}
	return nil
}

func validateDescriptionRule(r *DescriptionRule) error {
	if l := strings.TrimSpace(r.Label); l != "" && l != DescriptionLabel {
		return fmt.Errorf("the label must be %q (the saved job reads that key)", DescriptionLabel)
	}
	switch r.Format {
	case "", "plain", "structured":
	default:
		return fmt.Errorf("the format %q is not known: use plain or structured", r.Format)
	}
	if r.Limit != 0 && (r.Limit < MinDescriptionLimit || r.Limit > MaxDescriptionLimit) {
		return fmt.Errorf("the limit must be between %d and %d characters", MinDescriptionLimit, MaxDescriptionLimit)
	}
	switch {
	case len(r.Parts) == 0:
		return fmt.Errorf("the description rule needs at least one part")
	case len(r.Parts) > MaxDescriptionParts:
		return fmt.Errorf("a description rule has at most %d parts", MaxDescriptionParts)
	}
	for i, p := range r.Parts {
		where := fmt.Sprintf("part %d", i+1)
		if p.Source != "" || p.Path != "" {
			if p.Include != "" {
				return fmt.Errorf("%s: a part takes either a box selector or a structured source, not both", where)
			}
			if err := ValidateStructuredSource(where, p.Source, p.Path); err != nil {
				return err
			}
		} else if err := checkRuleSelector(where+": the box selector", p.Include); err != nil {
			return err
		}
		switch p.Heading {
		case "", "none", "auto":
			if p.HeadingSelector != "" {
				return fmt.Errorf("%s: a heading selector needs heading \"selector\"", where)
			}
		case "selector":
			if err := checkRuleSelector(where+": the heading selector", p.HeadingSelector); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: the heading %q is not known: use none, auto or selector", where, p.Heading)
		}
		if len(p.Exclude) > MaxRuleExcludes {
			return fmt.Errorf("%s: at most %d exclude selectors", where, MaxRuleExcludes)
		}
		for j, e := range p.Exclude {
			if err := checkRuleSelector(fmt.Sprintf("%s: exclude %d", where, j+1), e); err != nil {
				return err
			}
		}
		if len(p.CutAfter) > MaxRuleCutAfter {
			return fmt.Errorf("%s: at most %d cut-after texts", where, MaxRuleCutAfter)
		}
		for j, c := range p.CutAfter {
			if strings.TrimSpace(c) == "" {
				return fmt.Errorf("%s: cut-after text %d is empty", where, j+1)
			}
			if utf8.RuneCountInString(c) > MaxRuleCutAfterLen {
				return fmt.Errorf("%s: cut-after text %d is longer than %d characters", where, j+1, MaxRuleCutAfterLen)
			}
		}
	}
	return nil
}
