// description_rule.go (plan 186, step 1) — reading a job description by a rule the person set.
//
// A rule is an ordered list of parts. A part names the box to take (a CSS selector, every match, in page order), what
// to leave out of it (selectors) and, optionally, a heading and a text after which the part stops. The result carries
// the text, a flag when the limit cut it (the silent cut of the plain field reads is gone for descriptions), and a
// report (per part, the outline, what was taken out) so a screen can show that nothing real was lost.
//
// The rule is a new key next to `fields`, never a replacement: a request without it behaves exactly as before. The rule
// is validated here (sizes, counts, formats) because the Career tool validates what it stores and this tool validates
// what it runs; the CSS itself is checked by the page (an invalid selector is reported, never a crash).
package crawler

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/chromedp/chromedp"
)

//go:embed description_rule.js
var descriptionRuleJSTemplate string

const descriptionPayloadPlaceholder = "__DESCRIPTION_PAYLOAD__"

// The bounds that keep a rule small and the page read cheap.
const (
	DefaultDescriptionLimit = 60000
	MinDescriptionLimit     = 1000
	MaxDescriptionLimit     = 200000
	maxDescriptionParts     = 8
	maxRuleSelectorLen      = 300
	maxRuleExcludes         = 20
	maxRuleCutAfter         = 10
	maxRuleCutAfterLen      = 200
	maxDescriptionLabelLen  = 40
	defaultDescriptionLabel = "description"
)

// descriptionConsentSelector: boxes that are never part of a description (cookie notices, consent dialogs, modals).
const descriptionConsentSelector = `[role="dialog"], [role="alertdialog"], [aria-modal="true"], dialog, [class*="cookie" i], [class*="consent" i], [id*="cookie" i], [id*="consent" i]`

type descriptionPart struct {
	// Include is the CSS selector of the box; every match is taken, in page order, each once.
	Include string `json:"include"`
	// Heading: "" or "none" adds nothing; "auto" writes the nearest heading before the box as its own line (unless the
	// box starts with it); "selector" writes the first match of HeadingSelector.
	Heading         string `json:"heading,omitempty"`
	HeadingSelector string `json:"headingSelector,omitempty"`
	// Exclude lists elements inside the box that are removed before the text is read.
	Exclude []string `json:"exclude,omitempty"`
	// CutAfter: the part stops at the first line that starts with one of these texts (case-insensitive).
	CutAfter []string `json:"cutAfter,omitempty"`
}

type descriptionRule struct {
	// Label is the key the text is returned under ("description" when empty).
	Label string            `json:"label,omitempty"`
	Parts []descriptionPart `json:"parts"`
	// Format: "plain" (default; the lines the page has) or "structured" (headings on their own lines, "- " before items).
	Format string `json:"format,omitempty"`
	// Limit is the description's own size limit in characters (DefaultDescriptionLimit when 0).
	Limit int `json:"limit,omitempty"`
}

type descriptionPartReport struct {
	Index    int    `json:"index"`
	Include  string `json:"include"`
	Matches  int    `json:"matches"`
	Chars    int    `json:"chars"`
	Overlap  bool   `json:"overlap"`
	NotFound bool   `json:"notFound"`
	Cut      int    `json:"cut"`
	Removed  int    `json:"removed"`
}

type descriptionRemoved struct {
	Part     int    `json:"part"`
	Kind     string `json:"kind"` // exclude | consent | cutAfter
	Selector string `json:"selector"`
	Chars    int    `json:"chars"`
	Text     string `json:"text"`
}

// descriptionResult is what the page returned for a rule.
type descriptionResult struct {
	Text         string                  `json:"text"`
	Chars        int                     `json:"chars"`
	FullLength   int                     `json:"fullLength"`
	Truncated    bool                    `json:"truncated"`
	Limit        int                     `json:"limit"`
	Parts        []descriptionPartReport `json:"parts"`
	Outline      []string                `json:"outline"`
	Removed      []descriptionRemoved    `json:"removed"`
	RemovedTotal int                     `json:"removedTotal"`
	Errors       []string                `json:"errors"`
}

var (
	errRuleNoParts      = errors.New("the description rule needs at least one part")
	errRuleTooManyParts = fmt.Errorf("a description rule has at most %d parts", maxDescriptionParts)
)

func checkRuleSelector(what, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is empty", what)
	}
	if utf8.RuneCountInString(s) > maxRuleSelectorLen {
		return fmt.Errorf("%s is longer than %d characters", what, maxRuleSelectorLen)
	}
	for _, r := range s {
		if r == 0 || (unicode.IsControl(r) && r != '\t') {
			return fmt.Errorf("%s contains a control character", what)
		}
	}
	return nil
}

// validateDescriptionRule checks the rule and fills its defaults in place (label, format, limit, heading). It returns
// the first problem, in words a person can act on.
func validateDescriptionRule(r *descriptionRule) error {
	if r == nil {
		return nil
	}
	r.Label = strings.TrimSpace(r.Label)
	if r.Label == "" {
		r.Label = defaultDescriptionLabel
	}
	if utf8.RuneCountInString(r.Label) > maxDescriptionLabelLen {
		return fmt.Errorf("the label is longer than %d characters", maxDescriptionLabelLen)
	}
	for _, c := range r.Label {
		if unicode.IsSpace(c) || unicode.IsControl(c) {
			return errors.New("the label cannot contain a space or a control character")
		}
	}
	switch r.Format {
	case "":
		r.Format = "plain"
	case "plain", "structured":
	default:
		return fmt.Errorf("the format %q is not known: use plain or structured", r.Format)
	}
	switch {
	case r.Limit == 0:
		r.Limit = DefaultDescriptionLimit
	case r.Limit < MinDescriptionLimit || r.Limit > MaxDescriptionLimit:
		return fmt.Errorf("the limit must be between %d and %d characters", MinDescriptionLimit, MaxDescriptionLimit)
	}
	switch {
	case len(r.Parts) == 0:
		return errRuleNoParts
	case len(r.Parts) > maxDescriptionParts:
		return errRuleTooManyParts
	}
	for i := range r.Parts {
		p := &r.Parts[i]
		where := fmt.Sprintf("part %d", i+1)
		if err := checkRuleSelector(where+": the box selector", p.Include); err != nil {
			return err
		}
		switch p.Heading {
		case "", "none":
			p.Heading = "none"
			if p.HeadingSelector != "" {
				return fmt.Errorf("%s: a heading selector needs heading \"selector\"", where)
			}
		case "auto":
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
		if len(p.Exclude) > maxRuleExcludes {
			return fmt.Errorf("%s: at most %d exclude selectors", where, maxRuleExcludes)
		}
		for j, e := range p.Exclude {
			if err := checkRuleSelector(fmt.Sprintf("%s: exclude %d", where, j+1), e); err != nil {
				return err
			}
		}
		if len(p.CutAfter) > maxRuleCutAfter {
			return fmt.Errorf("%s: at most %d cut-after texts", where, maxRuleCutAfter)
		}
		for j, c := range p.CutAfter {
			if strings.TrimSpace(c) == "" {
				return fmt.Errorf("%s: cut-after text %d is empty", where, j+1)
			}
			if utf8.RuneCountInString(c) > maxRuleCutAfterLen {
				return fmt.Errorf("%s: cut-after text %d is longer than %d characters", where, j+1, maxRuleCutAfterLen)
			}
		}
	}
	return nil
}

// runDescriptionRule reads the description off the page in ctx by a validated rule. It never navigates and never
// changes the page.
func runDescriptionRule(ctx context.Context, rule *descriptionRule) (*descriptionResult, error) {
	payload := struct {
		Rule            *descriptionRule `json:"rule"`
		ConsentSelector string           `json:"consentSelector"`
	}{rule, descriptionConsentSelector}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to build the description payload: %w", err)
	}
	js := strings.Replace(descriptionRuleJSTemplate, descriptionPayloadPlaceholder, string(payloadJSON), 1)
	var res descriptionResult
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &res)); err != nil {
		return nil, err
	}
	if res.Parts == nil {
		res.Parts = []descriptionPartReport{}
	}
	if res.Outline == nil {
		res.Outline = []string{}
	}
	if res.Removed == nil {
		res.Removed = []descriptionRemoved{}
	}
	if res.Errors == nil {
		res.Errors = []string{}
	}
	return &res, nil
}

// descriptionNotFound reports whether no part of the rule found its box (the rule matched nothing at all).
func descriptionNotFound(res *descriptionResult) bool {
	if res == nil {
		return true
	}
	for _, p := range res.Parts {
		if !p.NotFound {
			return false
		}
	}
	return true
}

// expectedSelectorsFor is expectedSelectorsFromFields plus the boxes of a description rule: a page that shows none of
// them is not the real page (a challenge, an error), exactly like a page that shows none of the fields.
func expectedSelectorsFor(container string, fields []extractField, rule *descriptionRule) []string {
	selectors := expectedSelectorsFromFields(container, fields)
	if rule == nil || container != "" {
		return selectors
	}
	for _, p := range rule.Parts {
		if strings.TrimSpace(p.Include) != "" {
			selectors = append(selectors, p.Include)
		}
	}
	return selectors
}

// extractionFieldCount is how many things one page is asked for: the fields, plus the description when a rule is set. A
// result with fewer "not found" entries than this found something real.
func extractionFieldCount(fields []extractField, rule *descriptionRule) int {
	n := len(fields)
	if rule != nil {
		n++
	}
	return n
}
