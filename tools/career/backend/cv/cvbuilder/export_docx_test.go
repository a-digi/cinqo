package cvbuilder

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"reflect"
	"strings"
	"testing"

	"career-tool-backend/cv/cvbuilder/templates"
)

// The Word export of a CV (plan 251, step 6).

type docxPara struct {
	Style  string // the style id, "" for a plain paragraph
	Text   string
	Italic bool
}

func openDocx(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("not a zip file: %v", err)
	}
	parts := map[string][]byte{}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts[f.Name] = data
	}
	return parts
}

func wellFormedXML(b []byte) error {
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = true
	for {
		_, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// docxParagraphs reads the paragraphs of word/document.xml: the style id, the text of all its runs and whether a run is italic.
func docxParagraphs(t *testing.T, document []byte) []docxPara {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(document))
	var out []docxPara
	var cur *docxPara
	inText, inRunProps := false, false
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("document.xml: %v", err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "p":
				out = append(out, docxPara{})
				cur = &out[len(out)-1]
			case "pStyle":
				if cur != nil {
					cur.Style = e.Attr[0].Value
				}
			case "rPr":
				inRunProps = true
			case "i":
				if inRunProps && cur != nil {
					cur.Italic = len(e.Attr) == 0 || (e.Attr[0].Value != "false" && e.Attr[0].Value != "0")
				}
			case "t":
				inText = true
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "t":
				inText = false
			case "rPr":
				inRunProps = false
			case "p":
				cur = nil
			}
		case xml.CharData:
			if inText && cur != nil {
				cur.Text += string(e)
			}
		}
	}
}

func docxOf(t *testing.T, d templates.CvData) []docxPara {
	t.Helper()
	b, err := RenderDocx(d)
	if err != nil {
		t.Fatal(err)
	}
	return docxParagraphs(t, openDocx(t, b)["word/document.xml"])
}

func TestTheWordFileOfACvIsExactlyWhatItsDataSays(t *testing.T) {
	want := []docxPara{
		{"Title", "Ada Lovelace", false},
		{"", "Senior Engineer", false},
		{"", "Berlin, Germany · LinkedIn: https://linkedin.com/in/ada · Portfolio: https://example.com/my page", false},
		{"Heading1", "Summary", false},
		{"", "Builds things.", false},
		{"", "Loves maths.", false},
		{"Heading1", "Experience", false},
		{"Heading2", "Lead – Acme", false},
		{"", "2020 – Present", true},
		{"ListBullet", "Led a team", false},
		{"ListBullet", "Cut costs by 30 %", false},
		{"", "Technologies: Go, SQL", false},
		{"Heading2", "Dev", false},
		{"", "2018 – 2019", true},
		{"Heading1", "Education", false},
		{"Heading2", "MSc, CS – TU Berlin", false},
		{"", "2010 – 2012", true},
		{"", "Thesis on compilers", false},
		{"Heading1", "Courses", false},
		{"Heading2", "Kubernetes – CNCF", false},
		{"", "2023-05", true},
		{"Heading1", "Skills", false},
		{"", "Go, snake_case, C++", false},
		{"Heading1", "Languages", false},
		{"", "German (Native), English (C1)", false},
		{"Heading1", "Hobbies", false},
		{"", "Chess, Climbing", false},
	}
	got := docxOf(t, richCv())
	if !reflect.DeepEqual(got, want) {
		for i := 0; i < max(len(got), len(want)); i++ {
			var g, w any = "-", "-"
			if i < len(got) {
				g = got[i]
			}
			if i < len(want) {
				w = want[i]
			}
			if !reflect.DeepEqual(g, w) {
				t.Errorf("paragraph %d: got %v, want %v", i, g, w)
			}
		}
	}
}

func TestASectionWithoutEntriesIsLeftOutAndANameAloneIsAWordDocument(t *testing.T) {
	if got := docxOf(t, templates.CvData{}); !reflect.DeepEqual(got, []docxPara{{"Title", "CV", false}}) {
		t.Errorf("an empty CV = %v", got)
	}
	if got := docxOf(t, templates.CvData{FullName: "  Ada \n Lovelace "}); !reflect.DeepEqual(got, []docxPara{{"Title", "Ada Lovelace", false}}) {
		t.Errorf("a name alone = %v", got)
	}
	// entries with nothing to show, blank items and empty lists write no heading
	d := templates.CvData{
		FullName: "A", Summary: " \n ", Skills: []string{"", "  "},
		Experience: []templates.ExperienceEntry{{StartDate: "2020", Description: "text without a title"}},
		Education:  []templates.EducationEntry{{}}, Courses: []templates.CourseEntry{{}},
		Languages: []templates.LanguageEntry{{}}, Hobbies: []templates.HobbyEntry{{Name: " "}},
	}
	if got := docxOf(t, d); !reflect.DeepEqual(got, []docxPara{{"Title", "A", false}}) {
		t.Errorf("sections without anything to show were written: %v", got)
	}
	// an entry with a company only still has a title line; one without dates has no date line
	got := docxOf(t, templates.CvData{FullName: "A", Experience: []templates.ExperienceEntry{{Company: "Acme"}}})
	if !reflect.DeepEqual(got, []docxPara{{"Title", "A", false}, {"Heading1", "Experience", false}, {"Heading2", "Acme", false}}) {
		t.Errorf("a company only = %v", got)
	}
}

// Word ignores a style id it does not know, so every style the file uses must be declared in its styles part (a bullet written with the style's name, "List Bullet",
// instead of its id would have been a plain paragraph).
func TestEveryStyleTheWordFileUsesIsDeclared(t *testing.T) {
	b, err := RenderDocx(richCv())
	if err != nil {
		t.Fatal(err)
	}
	parts := openDocx(t, b)
	styles := string(parts["word/styles.xml"])
	used := map[string]bool{}
	for _, p := range docxParagraphs(t, parts["word/document.xml"]) {
		if p.Style != "" {
			used[p.Style] = true
		}
	}
	if len(used) != 4 {
		t.Errorf("styles used = %v, want Title, Heading1, Heading2, ListBullet", used)
	}
	for s := range used {
		if !strings.Contains(styles, `w:styleId="`+s+`"`) {
			t.Errorf("style %q is used but not declared in word/styles.xml", s)
		}
	}
}

func TestTheWordFileIsAWholeAndHonestPackage(t *testing.T) {
	b, err := RenderDocx(richCv())
	if err != nil {
		t.Fatal(err)
	}
	parts := openDocx(t, b)
	for _, must := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/_rels/document.xml.rels", "word/styles.xml", "docProps/core.xml"} {
		if _, ok := parts[must]; !ok {
			t.Errorf("part %s is missing", must)
		}
	}
	for name, data := range parts {
		if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
			if err := wellFormedXML(data); err != nil {
				t.Errorf("%s is not well-formed XML: %v", name, err)
			}
		}
	}
	// no relationship points at a part that is not in the file (the thumbnail of the library's template is gone with its relationship)
	for _, rels := range []string{"_rels/.rels", "word/_rels/document.xml.rels"} {
		base := ""
		if rels == "word/_rels/document.xml.rels" {
			base = "word/"
		}
		var doc struct {
			Rel []struct {
				Target, TargetMode string `xml:",attr"`
			} `xml:"Relationship"`
		}
		if err := xml.Unmarshal(parts[rels], &doc); err != nil {
			t.Fatal(err)
		}
		for _, r := range doc.Rel {
			if r.TargetMode == "External" {
				continue
			}
			if target := path.Clean(base + r.Target); parts[target] == nil {
				t.Errorf("%s points at %s, which is not in the file", rels, target)
			}
		}
	}
	if _, ok := parts["docProps/thumbnail.jpeg"]; ok {
		t.Error("the template's thumbnail must not be in the file")
	}
	core := string(parts["docProps/core.xml"])
	for _, bad := range []string{"gomutex", "godocx", "2013"} {
		if strings.Contains(core, bad) {
			t.Errorf("the file still says %q about itself: %s", bad, core)
		}
	}
	if !strings.Contains(core, "<dc:title>Ada Lovelace</dc:title>") || !strings.Contains(core, "<dc:creator>Ada Lovelace</dc:creator>") {
		t.Errorf("the title and author are the person's: %s", core)
	}
	// no empty run (the date line used to start with one), no photo, no link relationship
	if strings.Contains(string(parts["word/document.xml"]), "<w:t></w:t>") {
		t.Error("an empty text run")
	}
	for name := range parts {
		if strings.Contains(name, "media/") {
			t.Errorf("the Word file has no photo, found %s", name)
		}
	}
}

func TestHostileTextStaysTextInTheWordFile(t *testing.T) {
	hostile := []string{
		`<script>alert(1)</script>`, `a & b < c > d "e" 'f'`, `]]>`, `<!-- x -->`, `<?xml version="1.0"?>`, `&amp; &#x41; &lt;`,
		"# not a heading", "- not a bullet in a skill", "> quote", "[x](http://evil)", "**bold**", "`code`",
		"line one\x00zero\x07bell\x0bvt\x1bescape", "￾￿", "bad \xff\xfe utf8", "tab\tin", strings.Repeat("long ", 2000),
	}
	d := templates.CvData{FullName: "N"}
	for _, h := range hostile {
		d.Headline, d.Summary, d.Location = h, h, h
		d.Skills = append(d.Skills, h)
		d.Experience = append(d.Experience, templates.ExperienceEntry{Title: h, Company: h, StartDate: h, Description: "- " + h + "\n" + h})
		d.Education = append(d.Education, templates.EducationEntry{Institution: h, Degree: h, FieldOfStudy: h, Description: h})
		d.Courses = append(d.Courses, templates.CourseEntry{Title: h, Provider: h})
		d.Languages = append(d.Languages, templates.LanguageEntry{Language: h, Level: h})
		d.Hobbies = append(d.Hobbies, templates.HobbyEntry{Name: h})
		d.ExternalLinks = append(d.ExternalLinks, templates.ExternalLink{Platform: h, URL: h})
	}
	b, err := RenderDocx(d)
	if err != nil {
		t.Fatal(err)
	}
	parts := openDocx(t, b)
	for name, data := range parts {
		if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
			if err := wellFormedXML(data); err != nil {
				t.Fatalf("%s is not well-formed XML with hostile text: %v", name, err)
			}
		}
	}
	paras := docxParagraphs(t, parts["word/document.xml"])
	// Markdown means nothing in Word: the text is there as it was written, and it is only text
	text := ""
	for _, p := range paras {
		text += p.Text + "\n"
	}
	for _, want := range []string{`<script>alert(1)</script>`, `a & b < c > d "e" 'f'`, `]]>`, "# not a heading", "[x](http://evil)", "**bold**", "`code`", "line onezerobellvtescape"} {
		if !strings.Contains(text, want) {
			t.Errorf("the text %q is not in the file", want)
		}
	}
	doc := string(parts["word/document.xml"])
	for _, raw := range []string{"<script", "<!--", "<?xml version=\"1.0\"?><", "\x00", "\x07", "\x0b", "\x1b"} {
		if strings.Contains(doc[strings.Index(doc, "<w:body>"):], raw) {
			t.Errorf("document.xml contains %q unescaped", raw)
		}
	}
	// the structure is the CV's own: one Title, the 7 sections, one entry heading per experience, education and course
	count := map[string]int{}
	for _, p := range paras {
		count[p.Style]++
	}
	n := len(hostile)
	if count["Title"] != 1 || count["Heading1"] != 7 || count["Heading2"] != 3*n {
		t.Errorf("structure: %v, want 1 Title, 7 Heading1 and %d Heading2", count, 3*n)
	}
}

func TestBulletsAreBulletsAndOtherLinesAreParagraphs(t *testing.T) {
	d := templates.CvData{FullName: "A", Experience: []templates.ExperienceEntry{{Title: "T", Description: "intro\n- one\n  - nested\n* two\n• three\n– four\n-\n\n   \nclosing\n-not a bullet"}}}
	got := docxOf(t, d)[3:]
	want := []docxPara{
		{"", "intro", false}, {"ListBullet", "one", false}, {"ListBullet", "nested", false}, {"ListBullet", "two", false},
		{"ListBullet", "three", false}, {"ListBullet", "four", false}, {"", "-", false}, {"", "closing", false}, {"", "-not a bullet", false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("description lines = %v\nwant %v", got, want)
	}
}

// --- the route ---

func TestTheExportRouteAnswersWithTheStoredCvAsAWordDownload(t *testing.T) {
	pid := setupCvDB(t)
	doc, err := InsertCvDocument(pid, "classic", `Ada – "Backend"/2026`, richCv(), "media-1", "")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	ExportHandler(rec, httptest.NewRequest(http.MethodGet, "/cv-documents/export?cvDocumentId="+doc.ID+"&format=docx", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	h := rec.Header()
	if h.Get("Content-Type") != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" || h.Get("Cache-Control") != "no-store" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers: %v", h)
	}
	if got := h.Get("Content-Disposition"); got != `attachment; filename="Ada Backend2026.docx"; filename*=UTF-8''Ada%20Backend2026.docx` {
		t.Errorf("Content-Disposition = %q", got)
	}
	paras := docxParagraphs(t, openDocx(t, rec.Body.Bytes())["word/document.xml"])
	if len(paras) != 27 || paras[0].Text != "Ada Lovelace" || paras[7].Text != "Lead – Acme" {
		t.Errorf("the body is not the Word file of the stored data: %d paragraphs, first %v", len(paras), paras[0])
	}
	// the Markdown of the same CV is still served
	md := httptest.NewRecorder()
	ExportHandler(md, httptest.NewRequest(http.MethodGet, "/cv-documents/export?cvDocumentId="+doc.ID+"&format=md", nil))
	if md.Code != http.StatusOK || md.Body.String() != richMarkdown || !strings.HasSuffix(md.Header().Get("Content-Disposition"), "filename*=UTF-8''Ada%20Backend2026.md") {
		t.Errorf("the Markdown export changed: %d %q", md.Code, md.Header().Get("Content-Disposition"))
	}
}

func TestTheExportRouteRefusesWhatIsNotAWordRequest(t *testing.T) {
	pid := setupCvDB(t)
	doc, _ := InsertCvDocument(pid, "classic", "CV", richCv(), "media-1", "")
	for _, c := range []struct {
		name, method, query string
		want                int
	}{
		{"a capital letter is not docx", http.MethodGet, "cvDocumentId=" + doc.ID + "&format=DOCX", http.StatusBadRequest},
		{"the old Word format is not offered", http.MethodGet, "cvDocumentId=" + doc.ID + "&format=doc", http.StatusBadRequest},
		{"unknown id", http.MethodGet, "cvDocumentId=nope&format=docx", http.StatusNotFound},
		{"no id", http.MethodGet, "format=docx", http.StatusBadRequest},
		{"a POST", http.MethodPost, "cvDocumentId=" + doc.ID + "&format=docx", http.StatusMethodNotAllowed},
	} {
		rec := httptest.NewRecorder()
		ExportHandler(rec, httptest.NewRequest(c.method, "/cv-documents/export?"+c.query, nil))
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body.String())
		}
		if rec.Header().Get("Content-Disposition") != "" {
			t.Errorf("%s: a refusal must not be a download", c.name)
		}
	}
}

// A name with characters XML treats as markup is written into the file's properties and into the text, and the file stays well-formed.
func TestAMarkupNameIsEscapedInThePropertiesAndTheText(t *testing.T) {
	b, err := RenderDocx(templates.CvData{FullName: `A & <B> "C"`})
	if err != nil {
		t.Fatal(err)
	}
	parts := openDocx(t, b)
	for _, name := range []string{"docProps/core.xml", "word/document.xml"} {
		if err := wellFormedXML(parts[name]); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	var core struct {
		Title   string `xml:"title"`
		Creator string `xml:"creator"`
	}
	if err := xml.Unmarshal(parts["docProps/core.xml"], &core); err != nil || core.Title != `A & <B> "C"` || core.Creator != core.Title {
		t.Errorf("core properties: %+v %v", core, err)
	}
	if got := docxParagraphs(t, parts["word/document.xml"]); len(got) != 1 || got[0].Text != `A & <B> "C"` {
		t.Errorf("text: %v", got)
	}
}

// Line endings of every kind separate lines, and a control character is dropped, so a description pasted from anywhere reads as the person wrote it.
func TestLineEndingsAndControlCharactersInADescription(t *testing.T) {
	d := templates.CvData{FullName: "A", Experience: []templates.ExperienceEntry{{Title: "T", Description: "a\r\n- b\rc\x00d\x0be\n\r\nf"}}}
	got := docxOf(t, d)[3:]
	want := []docxPara{{"", "a", false}, {"ListBullet", "b", false}, {"", "cde", false}, {"", "f", false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %v, want %v", got, want)
	}
}
