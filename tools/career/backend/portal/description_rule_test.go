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

const oldDetailYAML = `fields:
  - label: description
    selector: ".job-description"
  - label: company
    selector: ".company"
`

const ruleDetailYAML = `descriptionRule:
  format: structured
  limit: 40000
  parts:
    - include: "#job-description"
      heading: auto
      exclude:
        - ".share-buttons"
        - ".apply-now"
      cutAfter:
        - "Similar jobs"
    - include: ".requirements"
      heading: selector
      headingSelector: "h3.req"
`

// filled is n non-empty strings (empty ones would be refused for being empty, hiding the count check).
func filled(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "x"
	}
	return out
}

func rule() *DescriptionRule {
	return &DescriptionRule{Parts: []DescriptionPart{{Include: "#box"}}}
}

func TestDescriptionRuleValidation(t *testing.T) {
	long := strings.Repeat("a", MaxRuleSelectorLen+1)
	manyParts := make([]DescriptionPart, MaxDescriptionParts+1)
	for i := range manyParts {
		manyParts[i] = DescriptionPart{Include: "div"}
	}
	if err := ValidateDescriptionRule(rule()); err != nil {
		t.Fatalf("a minimal rule is valid: %v", err)
	}
	if err := ValidateDescriptionRule(nil); err != nil {
		t.Fatalf("no rule is fine: %v", err)
	}
	for _, c := range []struct {
		name string
		mod  func(*DescriptionRule)
	}{
		{"no parts", func(r *DescriptionRule) { r.Parts = nil }},
		{"too many parts", func(r *DescriptionRule) { r.Parts = manyParts }},
		{"empty include", func(r *DescriptionRule) { r.Parts[0].Include = " " }},
		{"long include", func(r *DescriptionRule) { r.Parts[0].Include = long }},
		{"control character", func(r *DescriptionRule) { r.Parts[0].Include = "div\x00" }},
		{"unknown format", func(r *DescriptionRule) { r.Format = "markdown" }},
		{"limit too small", func(r *DescriptionRule) { r.Limit = MinDescriptionLimit - 1 }},
		{"limit too large", func(r *DescriptionRule) { r.Limit = MaxDescriptionLimit + 1 }},
		{"negative limit", func(r *DescriptionRule) { r.Limit = -1 }},
		{"unknown heading", func(r *DescriptionRule) { r.Parts[0].Heading = "first" }},
		{"heading selector without the selector heading", func(r *DescriptionRule) { r.Parts[0].HeadingSelector = "h2" }},
		{"selector heading without selector", func(r *DescriptionRule) { r.Parts[0].Heading = "selector" }},
		{"too many excludes", func(r *DescriptionRule) { r.Parts[0].Exclude = filled(MaxRuleExcludes + 1) }},
		{"empty exclude", func(r *DescriptionRule) { r.Parts[0].Exclude = []string{""} }},
		{"too many cut-after", func(r *DescriptionRule) { r.Parts[0].CutAfter = filled(MaxRuleCutAfter + 1) }},
		{"empty cut-after", func(r *DescriptionRule) { r.Parts[0].CutAfter = []string{" "} }},
		{"long cut-after", func(r *DescriptionRule) { r.Parts[0].CutAfter = []string{strings.Repeat("x", MaxRuleCutAfterLen+1)} }},
		{"another label", func(r *DescriptionRule) { r.Label = "summary" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := rule()
			c.mod(r)
			err := ValidateDescriptionRule(r)
			if err == nil {
				t.Fatal("expected a refusal")
			}
			if !errors.Is(err, ErrInvalidCrawlInstructions) {
				t.Fatalf("the refusal must wrap ErrInvalidCrawlInstructions: %v", err)
			}
		})
	}
	r := rule()
	r.Label = "description"
	r.Format, r.Limit = "structured", MaxDescriptionLimit
	r.Parts[0].Heading, r.Parts[0].HeadingSelector = "selector", "h2"
	if err := ValidateDescriptionRule(r); err != nil {
		t.Errorf("the explicit forms are valid: %v", err)
	}
}

func TestInstructionsWithoutARuleAreReadExactlyAsBefore(t *testing.T) {
	if err := validateJobDetailCrawlInstructions(oldDetailYAML); err != nil {
		t.Fatal(err)
	}
	req, err := BuildJobDetailCrawlRequestFromYAML(oldDetailYAML)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(req)
	want := `{"fields":[{"label":"description","selector":".job-description"},{"label":"company","selector":".company"}],"nextSelector":"","requestedMaxPages":1,"effectiveMaxPages":1}`
	if string(raw) != want {
		t.Fatalf("the request changed for old instructions:\n got %s\nwant %s", raw, want)
	}
	// and the old refusals are unchanged
	for name, yamlText := range map[string]string{
		"no fields":           "fields: []\n",
		"no description":      "fields:\n  - label: company\n    selector: .c\n",
		"field without label": "fields:\n  - selector: .c\n",
		"not yaml":            "fields: [",
	} {
		if err := validateJobDetailCrawlInstructions(yamlText); !errors.Is(err, ErrInvalidCrawlInstructions) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestARuleSatisfiesTheDescriptionLabelAndFieldsAreOptional(t *testing.T) {
	if err := validateJobDetailCrawlInstructions(ruleDetailYAML); err != nil {
		t.Fatalf("a rule alone is a valid instruction document: %v", err)
	}
	withField := "fields:\n  - label: company\n    selector: .company\n" + ruleDetailYAML
	if err := validateJobDetailCrawlInstructions(withField); err != nil {
		t.Fatalf("a rule plus a company field: %v", err)
	}
	both := oldDetailYAML + ruleDetailYAML // a description field and a rule: the Browser lets the rule win
	if err := validateJobDetailCrawlInstructions(both); err != nil {
		t.Fatalf("a field and a rule for the description: %v", err)
	}
	// the other fields are still checked
	bad := "fields:\n  - label: company\n" + ruleDetailYAML
	if err := validateJobDetailCrawlInstructions(bad); !errors.Is(err, ErrInvalidCrawlInstructions) {
		t.Errorf("a field without a selector next to a rule: %v", err)
	}
}

func TestABadRuleInsideInstructionsIsRefused(t *testing.T) {
	for name, y := range map[string]string{
		"no parts":      "descriptionRule:\n  parts: []\n",
		"misspelled":    "descriptionRule:\n  part:\n    - include: '#a'\n",
		"bad format":    "descriptionRule:\n  format: html\n  parts:\n    - include: '#a'\n",
		"other label":   "descriptionRule:\n  label: summary\n  parts:\n    - include: '#a'\n",
		"empty include": "descriptionRule:\n  parts:\n    - include: ''\n",
	} {
		if err := validateJobDetailCrawlInstructions(y); !errors.Is(err, ErrInvalidCrawlInstructions) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestTheRequestCarriesTheRuleInTheBrowsersShape(t *testing.T) {
	req, err := BuildJobDetailCrawlRequestFromYAML(ruleDetailYAML)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(req)
	var got struct {
		Fields          []any `json:"fields"`
		DescriptionRule struct {
			Format string `json:"format"`
			Limit  int    `json:"limit"`
			Parts  []struct {
				Include         string   `json:"include"`
				Heading         string   `json:"heading"`
				HeadingSelector string   `json:"headingSelector"`
				Exclude         []string `json:"exclude"`
				CutAfter        []string `json:"cutAfter"`
			} `json:"parts"`
		} `json:"descriptionRule"`
		MaxPages int `json:"effectiveMaxPages"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	r := got.DescriptionRule
	if got.Fields == nil || len(got.Fields) != 0 || got.MaxPages != 1 {
		t.Errorf("a rule-only request has an empty fields array and one page: %s", raw)
	}
	if r.Format != "structured" || r.Limit != 40000 || len(r.Parts) != 2 {
		t.Fatalf("rule: %s", raw)
	}
	p := r.Parts[0]
	if p.Include != "#job-description" || p.Heading != "auto" || len(p.Exclude) != 2 || p.Exclude[1] != ".apply-now" || len(p.CutAfter) != 1 || p.CutAfter[0] != "Similar jobs" {
		t.Errorf("part 1: %+v", p)
	}
	if r.Parts[1].Heading != "selector" || r.Parts[1].HeadingSelector != "h3.req" {
		t.Errorf("part 2: %+v", r.Parts[1])
	}
	// an empty heading and empty lists are not sent at all
	small, _ := json.Marshal(CrawlRequest{DescriptionRule: rule()})
	if strings.Contains(string(small), "heading") || strings.Contains(string(small), "exclude") || strings.Contains(string(small), "cutAfter") {
		t.Errorf("empty keys sent: %s", small)
	}
}

// The two tools are separate modules, so the bounds are copied; this fails when the copies drift apart.
func TestTheBoundsMirrorTheBrowsersRule(t *testing.T) {
	raw, err := os.ReadFile("../../browser/backend/crawler/description_rule.go")
	if err != nil {
		t.Skipf("the Browser's source is not next to this one: %v", err)
	}
	src := string(raw)
	for name, want := range map[string]int{
		"DefaultDescriptionLimit": DefaultDescriptionLimit,
		"MinDescriptionLimit":     MinDescriptionLimit,
		"MaxDescriptionLimit":     MaxDescriptionLimit,
		"maxDescriptionParts":     MaxDescriptionParts,
		"maxRuleSelectorLen":      MaxRuleSelectorLen,
		"maxRuleExcludes":         MaxRuleExcludes,
		"maxRuleCutAfter":         MaxRuleCutAfter,
		"maxRuleCutAfterLen":      MaxRuleCutAfterLen,
	} {
		m := regexp.MustCompile(`(?m)^\s*` + name + `\s*=\s*(\d+)\s*$`).FindStringSubmatch(src)
		if m == nil {
			t.Errorf("%s not found in the Browser's description_rule.go", name)
			continue
		}
		if got, _ := strconv.Atoi(m[1]); got != want {
			t.Errorf("%s: the Browser says %d, this tool says %d", name, got, want)
		}
	}
	for _, word := range []string{`"plain"`, `"structured"`, `"auto"`, `"selector"`, `"none"`, `"description"`} {
		if !strings.Contains(src, word) {
			t.Errorf("the Browser's rule no longer mentions %s", word)
		}
	}
	for _, key := range []string{`json:"include"`, `json:"heading,omitempty"`, `json:"headingSelector,omitempty"`, `json:"exclude,omitempty"`, `json:"cutAfter,omitempty"`, `json:"parts"`, `json:"format,omitempty"`, `json:"limit,omitempty"`, `json:"label,omitempty"`} {
		if !strings.Contains(src, key) {
			t.Errorf("the Browser's rule no longer has the key %s", key)
		}
	}
}
