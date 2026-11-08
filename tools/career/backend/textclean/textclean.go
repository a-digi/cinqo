// Package textclean turns what a crawl read off a job page into plain text (step 147). The page text often carries markup:
// the content of <style> and <script> elements, <noscript> fallbacks (a browser hands those over as literal text, e.g. a
// "<style>…</style><img src=…>" pair in front of the real text), images, comments. None of it is information about the job, so
// Description removes it and keeps the words, with line breaks where the page had paragraphs, list items and the like.
// It is pure: no database, no network. See plan/ai/tools/career/step-147-clean-job-description-text.md.
package textclean

import (
	"html"
	"strings"
)

// dropWithContent are the elements whose whole content goes with the tag: it is code, styling or a fallback, never text.
var dropWithContent = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true,
	"svg": true, "iframe": true, "object": true, "canvas": true,
	"picture": true, "video": true, "audio": true,
}

// breaks are the elements that start a new line (their opening and closing tag both do); cells get a space instead.
var breaks = map[string]bool{
	"br": true, "p": true, "div": true, "li": true, "ul": true, "ol": true, "dl": true, "dt": true, "dd": true,
	"tr": true, "table": true, "section": true, "article": true, "header": true, "footer": true, "nav": true,
	"aside": true, "main": true, "blockquote": true, "pre": true, "hr": true, "form": true, "fieldset": true,
	"figure": true, "figcaption": true, "address": true, "details": true, "summary": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

var cells = map[string]bool{"td": true, "th": true}

// Description returns the plain text of s: elements that carry no text removed with their content, other tags dropped, entities
// decoded, whitespace tidied. A "<" that does not open a tag ("a < b", "<3") stays. Text without markup comes out the same
// apart from the whitespace tidying (see tidy).
func Description(s string) string {
	// Taking a tag or a rule out can put a "<" next to letters that now read as a tag ("<<img>a>"); go on until nothing changes (a few rounds at most).
	for round := 0; round < maxRounds; round++ {
		next := stripCSS(stripMarkup(s))
		if next == s {
			break
		}
		s = next
	}
	return tidy(html.UnescapeString(s))
}

// maxRounds bounds the repeated stripping; each round removes at least one tag, and real text needs one or two.
const maxRounds = 8

// stripMarkup is one pass over s: markup out, the text it stands for in.
func stripMarkup(s string) string {
	var b strings.Builder
	lower := asciiLower(s)
	i := 0
	for i < len(s) {
		if s[i] != '<' {
			next := strings.IndexByte(s[i:], '<')
			if next < 0 {
				b.WriteString(s[i:])
				break
			}
			b.WriteString(s[i : i+next])
			i += next
			continue
		}
		end, text := readMarkup(s, lower, i)
		if end == i {
			b.WriteByte('<') // not a tag
			i++
			continue
		}
		b.WriteString(text)
		i = end
	}
	return b.String()
}

// readMarkup reads the markup that starts at s[i] == '<': the index after it and the text it stands for ("\n", " " or ""). It
// returns i itself when the "<" does not start markup.
func readMarkup(s, lower string, i int) (end int, text string) {
	rest := s[i:]
	switch {
	case strings.HasPrefix(rest, "<!--"):
		if j := strings.Index(rest[4:], "-->"); j >= 0 {
			return i + 4 + j + 3, ""
		}
		return len(s), "" // an unclosed comment runs to the end, as in a browser
	case strings.HasPrefix(rest, "<!") || strings.HasPrefix(rest, "<?"):
		if j := strings.IndexByte(rest, '>'); j >= 0 {
			return i + j + 1, ""
		}
		return i, ""
	}
	closing := len(rest) > 1 && rest[1] == '/'
	nameStart := 1
	if closing {
		nameStart = 2
	}
	nameEnd := nameStart
	for nameEnd < len(rest) && isNameByte(rest[nameEnd]) {
		nameEnd++
	}
	if nameEnd == nameStart || !isLetter(rest[nameStart]) {
		return i, ""
	}
	name := lower[i+nameStart : i+nameEnd]
	tagEnd := tagEndIndex(rest)
	if tagEnd < 0 {
		return i, "" // "<b" with no ">" anywhere: plain text
	}
	end = i + tagEnd + 1
	selfClosing := rest[tagEnd-1] == '/'
	if !closing && !selfClosing && dropWithContent[name] {
		if j := closerIndex(lower, name, end); j >= 0 {
			if k := strings.IndexByte(s[j:], '>'); k >= 0 {
				return j + k + 1, ""
			}
		}
		return len(s), "" // never closed: the rest of the text is its content
	}
	switch {
	case breaks[name]:
		return end, "\n"
	case cells[name]:
		return end, " "
	}
	return end, ""
}

// tagEndIndex is the index of the ">" that ends the tag at the start of rest, skipping a ">" inside a quoted attribute value;
// when the quotes never balance it falls back to the first ">". -1 when there is none.
func tagEndIndex(rest string) int {
	var quote byte
	for j := 1; j < len(rest); j++ {
		c := rest[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return j
		}
	}
	return strings.IndexByte(rest, '>')
}

// closerIndex is the index of the "</name" that closes an element (the name followed by ">", "/" or a space), from index from.
func closerIndex(lower, name string, from int) int {
	needle := "</" + name
	for from < len(lower) {
		j := strings.Index(lower[from:], needle)
		if j < 0 {
			return -1
		}
		at := from + j
		after := at + len(needle)
		if after >= len(lower) || lower[after] == '>' || lower[after] == '/' || lower[after] == ' ' || lower[after] == '\t' || lower[after] == '\n' || lower[after] == '\r' {
			return at
		}
		from = after
	}
	return -1
}

// tidy makes the whitespace tidy: line endings become "\n", invisible characters (soft hyphen, zero-width space) are dropped, runs of spaces and tabs one space,
// each line is trimmed, runs of blank lines become one blank line, and the whole is trimmed.
func tidy(s string) string {
	// Soft hyphens, zero-width spaces and byte order marks are invisible; they only break words apart for a search.
	s = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\u00ad", "", "\u200b", "", "\ufeff", "").Replace(s)
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func isNameByte(c byte) bool { return isLetter(c) || c >= '0' && c <= '9' || c == '-' || c == ':' }

// asciiLower lowercases only ASCII letters, so byte offsets stay the same as in s (strings.ToLower can change the length).
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
