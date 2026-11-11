package cvbuilder

import (
	"regexp"
	"strings"

	"career-tool-backend/cv/cvbuilder/templates"
)

// The Markdown export of a CV (plan 251, step 5): made from the CV's own data, not from the PDF or the HTML, so it is exact and the same for every design.
//
//	# Name
//	Headline
//	Location · [LinkedIn](url)
//	## Summary, ## Experience, ## Education, ## Courses, ## Skills, ## Languages, ## Hobbies
//
// An entry is "### Title – Company" with its dates in italics and its description under it (the person's own "- " lines are Markdown bullets already, every other
// line is a paragraph). A section without entries is left out. The text is plain text from the person, so everything that Markdown would read as markup is escaped:
// nothing of the CV can turn into a heading, a link, a list or an emphasis it was not written as.

var (
	mdSpecial = strings.NewReplacer(`\`, `\\`, "`", "\\`", "*", "\\*", "[", "\\[", "]", "\\]", "<", "\\<", ">", "\\>")
	// an underscore only matters at the edge of a word (snake_case inside a word is not emphasis)
	underscoreEdge = regexp.MustCompile(`(^|[\s(])_|_($|[\s).,;:!?])`)
	// the start of a line that Markdown would read as a heading, a list item or a numbered list
	blockStart = regexp.MustCompile(`^(#{1,6}(\s|$)|[-+](\s|$)|\d+[.)](\s|$)|=+$)`)
	bulletLine = regexp.MustCompile(`^(\s*)[-*•–]\s+(.*)$`)
)

// escapeInline escapes what Markdown would read as markup inside a line of text.
func escapeInline(s string) string {
	s = mdSpecial.Replace(s)
	s = underscoreEdge.ReplaceAllStringFunc(s, func(m string) string { return strings.Replace(m, "_", `\_`, 1) })
	return s
}

// escapeHeading is escapeInline for the text of a heading, where a closing run of # would be dropped ("### F#" is a heading "F").
func escapeHeading(s string) string {
	s = escapeInline(s)
	if strings.HasSuffix(s, "#") {
		s = s[:len(s)-1] + `\#`
	}
	return s
}

// escapeLine escapes a whole line: inline markup, and a start that would make it a heading, a list or a numbered list.
func escapeLine(s string) string { return lineStart(escapeInline(s)) }

// lineStart escapes a start that Markdown would read as a heading, a list or a numbered list, in a line whose inline markup is already escaped.
func lineStart(s string) string {
	if blockStart.MatchString(s) {
		if s[0] >= '0' && s[0] <= '9' { // "1. text": escape the dot
			i := strings.IndexAny(s, ".)")
			return s[:i] + `\` + s[i:]
		}
		return `\` + s
	}
	return s
}

// mdClean makes line endings \n and drops control characters (a stray NUL or escape has no place in a text file the person opens).
func mdClean(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// oneLine is a single-line field: line breaks and tabs become spaces, then it is trimmed and escaped.
func mdOneLine(s string) string { return escapeInline(mdSingleLine(s)) }

// mdSingleLine is a single-line field before escaping: line breaks and tabs become spaces, then it is trimmed.
func mdSingleLine(s string) string { return strings.Join(strings.Fields(mdClean(s)), " ") }

// mdBlock is a multi-line text (a summary, a description): the person's "- " lines stay bullets (nested ones keep their indent), every other line is a paragraph of its
// own, with a blank line between them so the line breaks survive.
func mdBlock(s string) string {
	var out []string
	paragraph := false // the last thing written was a paragraph
	for _, raw := range strings.Split(mdClean(s), "\n") {
		line := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if m := bulletLine.FindStringSubmatch(line); m != nil && strings.TrimSpace(m[2]) != "" {
			indent := strings.Repeat(" ", min(len(strings.ReplaceAll(m[1], "\t", "  ")), 8))
			if paragraph { // strict Markdown wants a blank line between a paragraph and a list
				out = append(out, "")
			}
			out = append(out, indent+"- "+escapeInline(strings.TrimSpace(m[2])))
			paragraph = false
			continue
		}
		if len(out) > 0 {
			out = append(out, "") // a paragraph after a bullet or another paragraph
		}
		out = append(out, escapeLine(strings.TrimSpace(line)))
		paragraph = true
	}
	return strings.Join(out, "\n")
}

func mdJoin(parts []string, by string) string {
	var keep []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, by)
}

// link is "[Platform](url)"; an address with a space or a bracket is written between < > so it still works, and one that cannot be written safely is plain text.
func mdLink(platform, url string) string {
	url = strings.TrimSpace(mdClean(url))
	p := mdOneLine(platform)
	if url == "" {
		return p
	}
	if strings.ContainsAny(url, "<>\n") {
		return mdJoin([]string{p, mdOneLine(url)}, ": ")
	}
	if p == "" {
		p = escapeInline(url)
	}
	if strings.ContainsAny(url, " ()") {
		return "[" + p + "](<" + url + ">)"
	}
	return "[" + p + "](" + url + ")"
}

// entry is one "### Title – Company" block with its dates and its description; an entry with neither a title nor a company is nothing.
func mdEntry(title, company, dates, description string) string {
	head := mdJoin([]string{escapeHeading(mdSingleLine(title)), escapeHeading(mdSingleLine(company))}, " – ")
	if head == "" {
		return ""
	}
	lines := []string{"### " + head}
	if d := mdOneLine(dates); d != "" {
		lines = append(lines, "*"+d+"*")
	}
	if b := mdBlock(description); b != "" {
		lines = append(lines, b)
	}
	return strings.Join(lines, "\n\n")
}

func mdSection(heading string, parts ...string) string {
	body := mdJoin(parts, "\n\n")
	if body == "" {
		return ""
	}
	return "## " + heading + "\n\n" + body
}

func mdCommaList(items []string) string {
	var keep []string
	for _, it := range items {
		if s := mdOneLine(it); s != "" {
			keep = append(keep, s)
		}
	}
	return lineStart(strings.Join(keep, ", "))
}

// RenderMarkdown is the CV as a Markdown document, ending with one line break.
func RenderMarkdown(d templates.CvData) string {
	name := escapeHeading(mdSingleLine(d.FullName))
	if name == "" {
		name = "CV"
	}
	var links []string
	for _, l := range d.ExternalLinks {
		if s := mdLink(l.Platform, l.URL); s != "" {
			links = append(links, s)
		}
	}
	top := []string{"# " + name, lineStart(mdOneLine(d.Headline)), lineStart(mdJoin(append([]string{mdOneLine(d.Location)}, links...), " · "))}
	blocks := []string{mdJoin(top, "\n\n")}

	blocks = append(blocks, mdSection("Summary", mdBlock(d.Summary)))
	var exp []string
	for _, e := range d.Experience {
		exp = append(exp, mdEntry(e.Title, e.Company, templates.EducationEntry{StartDate: e.StartDate, EndDate: e.EndDate}.DateRange(), e.Description))
	}
	blocks = append(blocks, mdSection("Experience", exp...))
	var edu []string
	for _, e := range d.Education {
		edu = append(edu, mdEntry(e.Title(), e.Company(), e.DateRange(), e.Description))
	}
	blocks = append(blocks, mdSection("Education", edu...))
	var crs []string
	for _, c := range d.Courses {
		crs = append(crs, mdEntry(c.Title, c.Company(), c.DateRange(), ""))
	}
	blocks = append(blocks, mdSection("Courses", crs...))
	blocks = append(blocks, mdSection("Skills", mdCommaList(d.Skills)))
	var langs, hobs []string
	for _, l := range d.Languages {
		langs = append(langs, l.Display())
	}
	for _, h := range d.Hobbies {
		hobs = append(hobs, h.Display())
	}
	blocks = append(blocks, mdSection("Languages", mdCommaList(langs)), mdSection("Hobbies", mdCommaList(hobs)))
	return mdJoin(blocks, "\n\n") + "\n"
}
