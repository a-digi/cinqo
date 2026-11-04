package convert

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// extractorStyle writes visual lines the way the extractor does for a two-column page: every word on a line of its own, one white-space-only
// line after each word, and a run of two to five of them between the lines of the page.
func extractorStyle(rng *rand.Rand, visual []string) string {
	var b strings.Builder
	b.WriteString("\n  \n") // it starts with blanks too
	for i, line := range visual {
		words := strings.Fields(line)
		for j, w := range words {
			b.WriteString(w + "\n")
			if j < len(words)-1 {
				b.WriteString(" \n") // between two words of a line: one blank line
			}
		}
		if i < len(visual)-1 {
			for k := 0; k < 2+rng.Intn(4); k++ { // between lines: two to five
				b.WriteString([]string{" \n", "  \n", "\n", "   \n"}[rng.Intn(4)])
			}
		}
	}
	b.WriteString(" \n \n")
	return b.String()
}

func page() []string {
	return []string{
		"~ whoami :", "Jane Example", "KONTAKT", "Email: jane@example.org Anschrift : Hamburg, Deutschland", "/in/jane-example-123456",
		"KURZPROFIL", "> 13+ Jahre Erfahrung als Angestellter in der Softwareentwicklung", "> Umfangreiche Erfahrung als Team und Tech Lead",
		"ABSCHLÜSSE", "2010 Bachelor of Informatik-Ingenieurwesen", "BERUFLICHE FÄHIGKEITEN",
		"PHP", "TYPESCRIPT", "Go", "Docker, Kubernetes, Terraform", "AWS (Lambda, ECS, EKS) · 5 Jahre",
		"09/2024 - aktuell, München (remote), Deutschland", "Team Lead & Senior Software Developer", "VERSO GmbH",
		"• Entwurf und Umsetzung von Microservices mit PHP 8.3 und Symfony 7", "• Reduktion der AWS-Kosten um 45 %",
		"Eingesetzte Technologien: PHP, Symfony, PostgreSQL, Redis, RabbitMQ", "06/2017 - 06/2020, Hamburg", "Senior Full Stack Softwareentwickler",
	}
}

func TestTheExtractorsOneWordALineShapeComesBackAsTheLinesOfThePage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	want := strings.Join(page(), "\n")
	for i := 0; i < 50; i++ {
		raw := extractorStyle(rng, page())
		if got := CleanText(raw); got != want {
			t.Fatalf("round %d:\n got %q\nwant %q", i, got, want)
		}
	}
	raw := extractorStyle(rng, page())
	lines := strings.Count(raw, "\n") + 1
	if lines < 150 || len(CleanText(raw)) >= len(raw)*7/10 {
		t.Errorf("the fixture must be the kind of text the cleaning is for: %d lines, %d bytes became %d", lines, len(raw), len(CleanText(raw)))
	}
}

func TestOrdinaryTextKeepsItsLinesAndParagraphs(t *testing.T) {
	in := "First paragraph line one\nline two of it\n\n\n\nSecond paragraph\n \n \nThird, after blanks\n"
	want := "First paragraph line one\nline two of it\n\nSecond paragraph\n\nThird, after blanks"
	if got := CleanText(in); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestEveryLineIsTrimmedAndRunsOfSpacesAndTabsAreOneSpace(t *testing.T) {
	for in, want := range map[string]string{
		"  indented  \t text   with\t\tgaps  ": "indented text with gaps",
		"a\t\t\tb":                             "a b",
		"Email:     jane@example.org":          "Email: jane@example.org",
		"\t\n   \n x \n\t":                     "x",
		"line one  \nline two\t":               "line one\nline two",
	} {
		if got := CleanText(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestLineEndingsAndOddSpacesAreUnified(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"CRLF":                       {"a\r\nb\r\n", "a\nb"},
		"a lone CR":                  {"a\rb", "a\nb"},
		"no-break space":             {"a\u00a0b", "a b"},
		"narrow no-break space":      {"10\u202f000", "10 000"},
		"em space and thin space":    {"a\u2003b\u2009c", "a b c"},
		"ideographic space":          {"a\u3000b", "a b"},
		"form feed (a page)":         {"end of page\fstart of next", "end of page\nstart of next"},
		"vertical tab":               {"a\vb", "a\nb"},
		"next line":                  {"a\u0085b", "a\nb"},
		"line separator":             {"a\u2028b", "a\nb"},
		"medium mathematical space":  {"a\u205fb", "a b"},
		"the first of the en spaces": {"a\u2000b", "a b"},
		"the last of the en spaces":  {"a\u200ab", "a b"},
		"paragraph separator":        {"a\u2029b", "a\nb"},
		"zero-width space":           {"Soft\u200bware", "Software"},
		"byte order mark":            {"\ufeffHello", "Hello"},
	} {
		if got := CleanText(c.in); got != c.want {
			t.Errorf("%s: got %q want %q", name, got, c.want)
		}
	}
}

func TestWhatNeedsToStayStays(t *testing.T) {
	for _, s := range []string{
		"Größe ë Ç — naïve café",                // letters with marks
		"👨\u200d👩\u200d👧 family",                // a zero-width JOINER keeps an emoji together
		"e\u0301 (e and a combining acute)",     // a combining mark
		"https://example.org/a//b?x=1&y=2#frag", // a URL
		"jane.doe+tag@example.org",              // an e-mail address
		"C:\\Users\\jane\\cv.pdf",               // backslashes
		"42,5 %  ->  45 %",                      // symbols (the gaps squeeze, the rest stays)
		"{\"a\": [1, 2, 3]}",                    // braces and quotes
	} {
		got := CleanText(s)
		if strings.Join(strings.Fields(got), " ") != strings.Join(strings.Fields(s), " ") {
			t.Errorf("%q became %q", s, got)
		}
		if !utf8.ValidString(got) {
			t.Errorf("%q became invalid UTF-8", s)
		}
	}
}

func TestEmptyAndBlankTextsAreEmpty(t *testing.T) {
	for _, s := range []string{"", " ", "\n", "\n\n \n\t\n", "\r\n \r\n", "\u00a0\u200b\ufeff"} {
		if got := CleanText(s); got != "" {
			t.Errorf("%q became %q", s, got)
		}
	}
}

// ---- when the extractor's shape is recognised ----

func wordsPerLine(n int, blankBetween string) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(fmt.Sprintf("word%d\n%s", i, blankBetween))
	}
	return b.String()
}

func TestAListOfSkillsOneToALineIsNotReflowed(t *testing.T) {
	in := wordsPerLine(30, "") // thirty one-word lines and no blank line at all: a list, not the extractor's shape
	if got := CleanText(in); got != strings.TrimRight(in, "\n") {
		t.Errorf("a list of single words was changed:\n%q", got)
	}
}

// collapseExpected is what a text that is not reflowed becomes: its lines with the blank runs collapsed to one blank line.
func collapseExpected(in string) string {
	var out []string
	for _, l := range strings.Split(in, "\n") {
		l = strings.TrimSpace(l)
		if l == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, l)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// shaped builds a text of `single` one-word lines and `multi` two-word lines in which the first `blank` lines are followed by a blank line
// and the others are not, so the shares the shape test looks at are exactly what the caller says.
func shaped(single, multi, blank int) string {
	var b strings.Builder
	for i := 0; i < single+multi; i++ {
		if i < single {
			b.WriteString(fmt.Sprintf("w%d\n", i))
		} else {
			b.WriteString(fmt.Sprintf("two w%d\n", i))
		}
		if i < blank {
			b.WriteString(" \n")
		}
	}
	return b.String()
}

// recognised says whether CleanText took the text for the extractor's shape: only then does it differ from the plain collapse of its blank lines.
func recognised(in string) bool { return CleanText(in) != collapseExpected(in) }

// The literal numbers below are the contract (20 lines, 80% one word, a quarter blank): a test that used the constants would move with them.
func TestTheShapeIsRecognisedExactlyAtItsLimits(t *testing.T) {
	for _, c := range []struct {
		name                 string
		single, multi, blank int
		want                 bool
	}{
		{"20 non-blank lines, the least", 20, 0, 20, true},
		{"19 non-blank lines", 19, 0, 19, false},
		{"1 line", 1, 0, 1, false},
		{"80% one-word lines, the least", 80, 20, 60, true},
		{"79% one-word lines", 79, 21, 60, false},
		{"78% one-word lines", 39, 11, 25, false},
		{"all one-word lines", 100, 0, 100, true},
		{"a quarter of the lines blank, the least", 30, 0, 10, true},
		{"just under a quarter blank", 30, 0, 9, false},
		{"26% blank", 40, 0, 14, true},
		{"24% blank", 40, 0, 13, false},
		{"no blank lines at all", 40, 0, 0, false},
		{"almost all blank", 40, 0, 40, true},
	} {
		if got := recognised(shaped(c.single, c.multi, c.blank)); got != c.want {
			t.Errorf("%s: recognised = %v, want %v", c.name, got, c.want)
		}
	}
	// at the least, all the words of lines with one blank line between them become one line
	if got := CleanText(shaped(20, 0, 20)); strings.Contains(got, "\n") {
		t.Errorf("twenty words with one blank line between each must be one line: %q", got)
	}
}

// Blank lines before the first word and after the last say nothing about the text between them: a list of skills with a page of blank lines
// around it is still a list.
func TestBlankLinesAtTheEndsDoNotMakeAListLookLikeTheExtractorsShape(t *testing.T) {
	list := shaped(30, 0, 0)
	for name, in := range map[string]string{
		"trailing blank lines": list + strings.Repeat(" \n", 60),
		"leading blank lines":  strings.Repeat(" \n", 60) + list,
		"both":                 strings.Repeat("\n", 40) + list + strings.Repeat("\n", 40),
	} {
		if got := CleanText(in); got != strings.TrimRight(list, "\n") {
			t.Errorf("%s: a list was changed:\n%q", name, got)
		}
	}
	// and the extractor's shape with blank lines around it is still recognised
	if !recognised("\n \n \n" + shaped(30, 0, 30) + "\n\n") {
		t.Error("the shape with blank lines around it must still be recognised")
	}
	if wordPerLine(nil) || wordPerLine([]string{}) || wordPerLine([]string{"", "", ""}) {
		t.Error("no words is not the extractor's shape")
	}
}

func TestTwoOrMoreBlankLinesAreALineBreakAndOneOrNoneIsASpace(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 30; i++ { // enough to be recognised
		b.WriteString(fmt.Sprintf("a%d\n \n", i))
	}
	b.WriteString("x\n\n\ny\n \nz\nq\n \n \n \nlast\n \n")
	got := CleanText(b.String())
	if !strings.HasSuffix(got, " a29 x\ny z q\nlast") { // x follows a29 after one blank line: it joins that line
		t.Errorf("the tail %q", got[len(got)-30:])
	}
}

// ---- properties over random text ----

func randomText(rng *rand.Rand) string {
	pieces := []string{"word", "Wörter", "e-mail@x.org", "a", "1.5", "—", "👍", "https://x.io/a", "}", " ", "  ", "\t", "\n", "\n", "\n \n", "\r\n", "\u00a0", "\u2003", "\f", "x\u200by", "\ufeff", "\n\n\n", " \n \n"}
	var b strings.Builder
	for i, n := 0, rng.Intn(400); i < n; i++ {
		b.WriteString(pieces[rng.Intn(len(pieces))])
	}
	return b.String()
}

func TestCleaningIsIdempotentNeverLongerAndKeepsTheWordsInOrder(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 3000; i++ {
		in := randomText(rng)
		out := CleanText(in)
		if again := CleanText(out); again != out {
			t.Fatalf("not idempotent:\n in %q\n out %q\n again %q", in, out, again)
		}
		if len(out) > len(in) {
			t.Fatalf("longer than its input:\n in %q\n out %q", in, out)
		}
		// the words, in order (the zero-width space is removed before words are told apart; the rest of the white space separates them)
		clean := strings.NewReplacer("\u200b", "", "\ufeff", "").Replace(in)
		if strings.Join(strings.Fields(out), "\x00") != strings.Join(strings.Fields(clean), "\x00") {
			t.Fatalf("the words changed:\n in %q\n out %q", in, out)
		}
		if !utf8.ValidString(out) {
			t.Fatalf("invalid UTF-8 from %q", in)
		}
		if strings.HasPrefix(out, "\n") || strings.HasSuffix(out, "\n") || strings.Contains(out, "\r") || strings.Contains(out, "\n\n\n") {
			t.Fatalf("loose ends in %q", out)
		}
		for _, l := range strings.Split(out, "\n") {
			if l != strings.TrimSpace(l) || strings.Contains(l, "  ") || strings.ContainsFunc(l, func(r rune) bool { return r != ' ' && unicode.IsSpace(r) }) {
				t.Fatalf("a line with loose white space: %q in %q", l, out)
			}
		}
	}
}

func TestExtractorStyleTextRoundTripsForAnyPageOfLines(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	words := []string{"alpha", "Beta", "gamma-3", "Δ", "ä", "x@y.zz", "100%", "(a)", "—", "1.", "•"}
	for i := 0; i < 400; i++ {
		var visual []string
		for l, n := 0, 6+rng.Intn(14); l < n; l++ {
			var ws []string
			for w, m := 0, 1+rng.Intn(7); w < m; w++ {
				ws = append(ws, words[rng.Intn(len(words))])
			}
			visual = append(visual, strings.Join(ws, " "))
		}
		raw := extractorStyle(rng, visual)
		if strings.Count(raw, "\n") < 4*minWordLines {
			continue // too small to be recognised: its own kind of text
		}
		if got, want := CleanText(raw), strings.Join(visual, "\n"); got != want {
			t.Fatalf("round %d:\n got %q\nwant %q", i, got, want)
		}
	}
}

func TestACleanedTextOfTenMegabytesIsDoneQuickly(t *testing.T) {
	if testing.Short() {
		t.Skip("size test")
	}
	rng := rand.New(rand.NewSource(3))
	var b strings.Builder
	for b.Len() < 10<<20 {
		b.WriteString(randomText(rng))
	}
	in := b.String()
	if out := CleanText(in); len(out) >= len(in) {
		t.Errorf("%d bytes became %d", len(in), len(out))
	}
}
