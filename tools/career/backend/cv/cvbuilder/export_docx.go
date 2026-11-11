package cvbuilder

import (
	"bytes"
	"encoding/xml"
	"strings"
	"time"

	"career-tool-backend/cv/cvbuilder/templates"

	"github.com/gomutex/godocx"
	"github.com/gomutex/godocx/docx"
)

// RenderDocx is a CV as a Word (.docx) file (plan 251, step 6). Like the Markdown export it is made from the CV's own data, not from the PDF or the HTML, so it is
// one plain, ATS-friendly layout whatever design the CV was printed with: the name as the title, the headline and the contact line, a heading per section
// (Summary, Experience, Education, Courses, Skills, Languages, Hobbies; one without entries is left out), a heading per entry with its dates under it, and the
// description as paragraphs and bullets. No photo. Addresses are written as text, so a recruiter's tool reads them as they are.
func RenderDocx(d templates.CvData) ([]byte, error) {
	doc, err := godocx.NewDocument()
	if err != nil {
		return nil, err
	}
	name := mdSingleLine(d.FullName)
	if name == "" {
		name = "CV"
	}
	if _, err := doc.AddHeading(name, 0); err != nil {
		return nil, err
	}
	if h := mdSingleLine(d.Headline); h != "" {
		doc.AddParagraph(h)
	}
	contact := []string{mdSingleLine(d.Location)}
	for _, l := range d.ExternalLinks {
		contact = append(contact, mdJoin([]string{mdSingleLine(l.Platform), mdSingleLine(l.URL)}, ": "))
	}
	if c := mdJoin(contact, " · "); c != "" {
		doc.AddParagraph(c)
	}

	if lines := docxLines(d.Summary); len(lines) > 0 {
		if err := docxSection(doc, "Summary"); err != nil {
			return nil, err
		}
		docxBody(doc, lines)
	}
	type entry struct{ title, company, dates, description string }
	entries := func(heading string, list []entry) error {
		var kept []entry
		for _, e := range list {
			if mdJoin([]string{mdSingleLine(e.title), mdSingleLine(e.company)}, " – ") != "" {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			return nil
		}
		if err := docxSection(doc, heading); err != nil {
			return err
		}
		for _, e := range kept {
			if _, err := doc.AddHeading(mdJoin([]string{mdSingleLine(e.title), mdSingleLine(e.company)}, " – "), 2); err != nil {
				return err
			}
			if dates := mdSingleLine(e.dates); dates != "" {
				doc.AddEmptyParagraph().AddText(dates).Italic(true)
			}
			docxBody(doc, docxLines(e.description))
		}
		return nil
	}
	oneLine := func(heading string, items []string) error {
		var keep []string
		for _, it := range items {
			if s := mdSingleLine(it); s != "" {
				keep = append(keep, s)
			}
		}
		if len(keep) == 0 {
			return nil
		}
		if err := docxSection(doc, heading); err != nil {
			return err
		}
		doc.AddParagraph(strings.Join(keep, ", "))
		return nil
	}

	var exp, edu, crs []entry
	for _, e := range d.Experience {
		exp = append(exp, entry{e.Title, e.Company, templates.EducationEntry{StartDate: e.StartDate, EndDate: e.EndDate}.DateRange(), e.Description})
	}
	for _, e := range d.Education {
		edu = append(edu, entry{e.Title(), e.Company(), e.DateRange(), e.Description})
	}
	for _, c := range d.Courses {
		crs = append(crs, entry{c.Title, c.Company(), c.DateRange(), ""})
	}
	var langs, hobs []string
	for _, l := range d.Languages {
		langs = append(langs, l.Display())
	}
	for _, h := range d.Hobbies {
		hobs = append(hobs, h.Display())
	}
	for _, step := range []func() error{
		func() error { return entries("Experience", exp) },
		func() error { return entries("Education", edu) },
		func() error { return entries("Courses", crs) },
		func() error { return oneLine("Skills", d.Skills) },
		func() error { return oneLine("Languages", langs) },
		func() error { return oneLine("Hobbies", hobs) },
	} {
		if err := step(); err != nil {
			return nil, err
		}
	}

	docxOwnProperties(doc, name)
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func docxSection(doc *docx.RootDoc, heading string) error {
	_, err := doc.AddHeading(heading, 1)
	return err
}

// docxLine is one line of a description: a paragraph or a bullet. A nested bullet is written flat: the layout has one level of bullets.
type docxLine struct {
	text   string
	bullet bool
}

// docxLines reads a description the way the Markdown export does: the person's own "- " lines are bullets, every other line is a paragraph, blank lines are dropped.
func docxLines(s string) []docxLine {
	var out []docxLine
	for _, raw := range strings.Split(mdClean(s), "\n") {
		line := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if m := bulletLine.FindStringSubmatch(line); m != nil { // the line has no trailing blanks, so a bullet always has text after "- "
			out = append(out, docxLine{text: strings.TrimSpace(m[2]), bullet: true})
			continue
		}
		out = append(out, docxLine{text: strings.TrimSpace(line)})
	}
	return out
}

func docxBody(doc *docx.RootDoc, lines []docxLine) {
	for _, l := range lines {
		p := doc.AddParagraph(l.text)
		if l.bullet {
			p.Style("ListBullet") // the id of the style, not its name: Word ignores an id it does not know
		}
	}
}

// docxOwnProperties replaces what the library's template says about the file ("gomutex", "generated by godocx", a date in 2013 and a picture of its sample page)
// with the CV's own: the person as title and author, and the time it was made. The thumbnail is dropped with its relationship.
func docxOwnProperties(doc *docx.RootDoc, name string) {
	var esc bytes.Buffer
	_ = xml.EscapeText(&esc, []byte(name))
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	core := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + esc.String() + `</dc:title><dc:creator>` + esc.String() + `</dc:creator>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + now + `</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">` + now + `</dcterms:modified></cp:coreProperties>`
	doc.FileMap.Store("docProps/core.xml", []byte(core))
	doc.FileMap.Delete("docProps/thumbnail.jpeg")
	kept := doc.RootRels.Relationships[:0]
	for _, r := range doc.RootRels.Relationships {
		if r.Target != "docProps/thumbnail.jpeg" {
			kept = append(kept, r)
		}
	}
	doc.RootRels.Relationships = kept
}
