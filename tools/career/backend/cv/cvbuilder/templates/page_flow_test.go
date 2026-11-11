package templates

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The page flow of every CV design (plan 251, step 1). A PDF page breaks where the content runs out; the rules below are what keeps a design from leaving a
// big empty space at the bottom of a page or a heading alone at the end of one. They are checked on every design of ListTemplates(), so a design added
// later has to follow them too.

var (
	avoidInside  = regexp.MustCompile(`(?i)(page-)?break-inside\s*:\s*avoid`)
	entryRule    = regexp.MustCompile(`\.entry\s*\{[^}]*\}`)
	gridDisplay  = regexp.MustCompile(`display\s*:\s*grid`)
	h2Rule       = regexp.MustCompile(`(?s)(?:^|\})\s*(?:/\*.*?\*/\s*)*h2\s*\{([^}]*)\}`)
	displayDecl  = regexp.MustCompile(`display\s*:\s*([a-z-]+)`)
	descPreLine  = regexp.MustCompile(`\.entry-desc[^{}]*\{[^}]*white-space\s*:\s*pre-line`)
	h2KeepNext   = regexp.MustCompile(`h2\s*\{[^}]*break-after\s*:\s*avoid`)
	titleAvoid   = regexp.MustCompile(`\.entry-title[^{}]*\{[^}]*break-after\s*:\s*avoid`)
	headRule     = regexp.MustCompile(`\.entry-head\s*\{[^}]*\}`)
	sectionRule  = regexp.MustCompile(`\.section-skills\s*\{[^}]*\}`)
	sectionWhole = regexp.MustCompile(`\.section-skills\s*\{[^}]*break-inside\s*:\s*avoid[^}]*page-break-inside\s*:\s*avoid`)
	headWhole    = regexp.MustCompile(`\.entry-head\s*\{[^}]*break-inside\s*:\s*avoid`)
	headWith     = regexp.MustCompile(`\.entry-head:not\(:last-child\)\s*\{[^}]*break-after\s*:\s*avoid`)
	anyRule      = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	entryPart    = regexp.MustCompile(`\.entry-(title|company|meta|head)\b`)
	keepsNext    = regexp.MustCompile(`break-after\s*:\s*avoid`)
	entryBlock   = regexp.MustCompile(`\.entry\s*\{\s*display\s*:\s*block\s*;?\s*\}`)
	bodyArea     = regexp.MustCompile(`\.entry-body\s*\{[^}]*grid-area\s*:\s*body`)
	bodyAreas    = regexp.MustCompile(`\.entry\s*\{[^}]*grid-template-areas\s*:\s*'meta body'`)
)

// withoutHead is the stylesheet without the two things that are kept whole on purpose: the head of an entry and a short skills-style section.
func withoutHead(css string) string {
	return sectionRule.ReplaceAllString(headRule.ReplaceAllString(css, ""), "")
}

// entryShape tells how the first (experience) entry of a design is built: "block" (title, company, dates and description one under the other, the 26 designs that
// never were a grid), "head" (the title, company and dates in one block, the description after it) or "body" (the dates in a column, the title, company and
// description in one block beside it). The grid designs used to put every part in its own grid row, and Chrome does not keep grid rows together: a page could
// end on a title line alone.
func entryShape(t *testing.T, id string) string {
	t.Helper()
	b, err := templateFS.ReadFile(id + "/template.html")
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	html := string(b)
	i := strings.Index(html, `<div class="entry">`)
	d := strings.Index(html[i:], `class="entry-desc"`)
	if i < 0 || d < 0 {
		t.Fatalf("%s: no entry with a description in the template", id)
	}
	seg := html[i : i+d]
	open := strings.Count(seg, "<div") - strings.Count(seg, "</div>")
	switch {
	case strings.Contains(seg, `class="entry-head"`) && open == 1:
		return "head"
	case strings.Contains(seg, `class="entry-body"`) && open == 2:
		return "body"
	case !strings.Contains(seg, `class="entry-head"`) && !strings.Contains(seg, `class="entry-body"`) && open == 1:
		return "block"
	}
	t.Fatalf("%s: the entry is built in a way no rule knows (open divs before the description: %d)", id, open)
	return ""
}

func styleOf(t *testing.T, id string) string {
	t.Helper()
	b, err := templateFS.ReadFile(id + "/style.css")
	if err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	return string(b)
}

// h2Display is the display of a heading after every rule of the stylesheet: the last declaration wins.
func h2Display(css string) string {
	last := "block"
	for _, m := range h2Rule.FindAllStringSubmatch(css, -1) {
		if d := displayDecl.FindStringSubmatch(m[1]); d != nil {
			last = d[1]
		}
	}
	return last
}

func TestEveryDesignFlowsOverPages(t *testing.T) {
	metasList := ListTemplates()
	if len(metasList) < 100 {
		t.Fatalf("expected the whole set of designs, got %d", len(metasList))
	}
	for _, m := range metasList {
		css := styleOf(t, m.ID)
		t.Run(m.ID, func(t *testing.T) {
			if !strings.Contains(css, "Page flow (plan 251)") {
				t.Fatal("the page-flow block is missing")
			}
			entry := entryRule.FindString(css)
			if entry == "" {
				t.Fatal("no .entry rule")
			}
			// a long entry may continue on the next page: nothing keeps an entry whole (a whole entry that did not fit used to jump over and leave a big gap); only
			// the head of an entry (its title, company and dates) is kept in one piece
			if avoidInside.MatchString(withoutHead(css)) {
				t.Error("something other than the entry head forbids breaking inside it: an entry that does not fit would jump to the next page and leave a gap")
			}
			if strings.Contains(css, "entry-long") {
				t.Error("there is no long-entry class any more")
			}
			// The title line stays with the first lines of its description, in every kind of design.
			switch shape := entryShape(t, m.ID); shape {
			case "block":
				if !titleAvoid.MatchString(css) {
					t.Error("block entry: the title line is not kept with its first lines")
				}
				if gridDisplay.MatchString(entry) {
					t.Error("block entry: it must not be a grid (Chrome does not keep grid rows together)")
				}
			case "head":
				if !headWhole.MatchString(css) || !headWith.MatchString(css) {
					t.Error("head entry: the head is not kept in one piece, and with the description when there is one (break-inside: avoid; break-after: avoid when it is not the last child)")
				}
				if !entryBlock.MatchString(css) {
					t.Error("head entry: the entry itself must be a block, so the description follows the head in normal flow")
				}
			case "body":
				if !bodyArea.MatchString(css) || !bodyAreas.MatchString(css) {
					t.Error("body entry: the body must sit beside the dates in the grid (grid-area: body, 'meta body')")
				}
				if !titleAvoid.MatchString(css) {
					t.Error("body entry: the title line is not kept with its first lines")
				}
			}
			// "Keep with the next" only when something follows. A part of an entry that carries it as the last child hands it on to the next entry, and a run of
			// entries without a description (education, courses) would then move to the next page as one block, leaving half a page empty.
			for _, r := range anyRule.FindAllStringSubmatch(css, -1) {
				if !keepsNext.MatchString(r[2]) {
					continue
				}
				for _, sel := range strings.Split(r[1], ",") {
					if entryPart.MatchString(sel) && !strings.Contains(sel, ":not(:last-child)") {
						t.Errorf("%q keeps with the next even as the last child of an entry (add :not(:last-child))", strings.TrimSpace(sel))
					}
				}
			}
			// A short section (skills, languages, hobbies) is kept whole, so its heading cannot end a page above a list that did not fit.
			if html, err := templateFS.ReadFile(m.ID + "/template.html"); err == nil && strings.Contains(string(html), "section-skills") && !sectionWhole.MatchString(css) {
				t.Error("a skills-style section is not kept whole (.section-skills { break-inside: avoid })")
			}
			// no one or two lines are left alone
			if !strings.Contains(css, "orphans: 3") || !strings.Contains(css, "widows: 3") {
				t.Error("orphans and widows are not 3")
			}
			// the person's line breaks stay line breaks
			if !descPreLine.MatchString(css) {
				t.Error("the description does not keep its line breaks (white-space: pre-line)")
			}
			// a heading stays with what follows, and a heading is a block (an inline box ignores break-after)
			if !h2KeepNext.MatchString(css) {
				t.Error("a heading is not kept with what follows it")
			}
			if d := h2Display(css); d == "inline-block" || d == "inline" {
				t.Errorf("headings are %s: an inline box ignores break-after, so a heading could be left alone at the bottom of a page", d)
			}
		})
	}
}

// Every design renders with the flow rules in it, for a CV with a long description, and the line breaks of the description reach the page.
func TestRenderedDesignsCarryTheFlowRules(t *testing.T) {
	data := CvData{
		FullName: "Test Person", Headline: "Engineer",
		Experience: []ExperienceEntry{{Company: "Acme", Title: "Lead", StartDate: "2020", Description: "- one\n- two\n- three"}},
	}
	for _, m := range ListTemplates() {
		x, err := Render(m.ID, data, "")
		if err != nil {
			t.Fatalf("%s: %v", m.ID, err)
		}
		if !strings.Contains(x, "Page flow (plan 251)") || avoidInside.MatchString(withoutHead(x)) || !strings.Contains(x, "- one\n- two\n- three") || !strings.Contains(x, `<div class="entry">`) {
			t.Errorf("%s: the rendered page lacks the flow rules, still forbids a break, or lost the line breaks of the description", m.ID)
		}
	}
}

// The designs are edited in frontend-app and copied here by sync-from-frontend.sh; a stale copy would put the old rules back in the PDF.
func TestBackendCopiesMatchTheFrontendDesigns(t *testing.T) {
	front := filepath.Join("..", "..", "..", "..", "frontend-app", "src", "Components", "CvBuilder", "Template")
	if _, err := os.Stat(front); err != nil {
		t.Skip("the frontend sources are not part of this checkout")
	}
	for _, m := range ListTemplates() {
		for _, file := range []string{"template.html", "style.css"} {
			a, err := os.ReadFile(filepath.Join(front, m.ID, file))
			if err != nil {
				t.Errorf("%s/%s: %v", m.ID, file, err)
				continue
			}
			b, err := templateFS.ReadFile(m.ID + "/" + file)
			if err != nil || string(a) != string(b) {
				t.Errorf("%s/%s: the copy in the backend differs from frontend-app (run sync-from-frontend.sh)", m.ID, file)
			}
		}
	}
}

// executive-charcoal puts the Summary in its wide column and the contact box beside it. Without a summary the box used to sit in the wide column with an empty
// column beside it (a hole half a page high); it now takes the whole width.
func TestExecutiveCharcoalHasNoHoleWithoutASummary(t *testing.T) {
	with, err := Render("executive-charcoal", CvData{FullName: "A", Summary: "A summary.", Skills: []string{"Go"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	without, err := Render("executive-charcoal", CvData{FullName: "A", Skills: []string{"Go"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(with, `<div class="duo">`) || strings.Contains(with, `<div class="duo duo-single">`) {
		t.Error("with a summary the row must not be the single-column one")
	}
	if !strings.Contains(without, `<div class="duo duo-single">`) {
		t.Error("without a summary the contact box must take the whole width")
	}
	css := styleOf(t, "executive-charcoal")
	if !regexp.MustCompile(`\.duo-single\s*\{[^}]*grid-template-columns\s*:\s*1fr\s*;`).MatchString(css) ||
		!regexp.MustCompile(`\.duo-single \.aside \.skills\s*\{[^}]*column-count\s*:\s*3`).MatchString(css) {
		t.Error("the single-column row is not styled (one column, skills in three columns)")
	}
}
