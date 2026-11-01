package textclean

import (
	"regexp"
	"strings"
)

// Reading a <style> element through textContent gives its CSS without the tags around it, so stripping tags cannot find it: the
// text arrives as ".job-details-sct { display: flex !important; }" and "@media (max-width: 700px) { … }" lines in the middle of the
// description. stripCSS recognises such rule blocks by their shape (a selector, then braces holding only "property: value;"
// declarations, or an @-rule holding them) and removes them with the comments around them. See step 147.

var (
	// A declaration: a property name, a colon, a value.
	declStart = regexp.MustCompile(`^-?[A-Za-z][A-Za-z0-9-]*\s*:\s*\S`)
	// A declaration ended by a semicolon anywhere inside an @-rule's body (its nested rules hold them).
	declAnywhere = regexp.MustCompile(`-?[A-Za-z][A-Za-z0-9-]*\s*:\s*[^;{}]+;`)
	// "@import …;", "@charset …;" and "@namespace …;" statements stand alone, without braces.
	atStatement = regexp.MustCompile(`(?m)^[ \t]*@(?:import|charset|namespace)\b[^;\n]*;[ \t]*`)
	// An @-rule's head: from a "@" that starts a token to the end of the text before the brace.
	atHead = regexp.MustCompile(`(?:^|\s)(@[A-Za-z-][^{};]*)$`)
)

// htmlTags are the bare element names accepted as a selector on their own ("body { … }").
var htmlTags = map[string]bool{
	"html": true, "body": true, "a": true, "p": true, "div": true, "span": true, "ul": true, "ol": true, "li": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "table": true, "tr": true, "td": true, "th": true,
	"img": true, "input": true, "button": true, "form": true, "label": true, "header": true, "footer": true, "nav": true,
	"section": true, "article": true, "main": true, "aside": true, "pre": true, "code": true, "blockquote": true,
	"strong": true, "em": true, "b": true, "i": true, "u": true, "select": true, "textarea": true, "hr": true, "br": true,
}

func stripCSS(s string) string {
	s = atStatement.ReplaceAllString(s, "")
	if !strings.Contains(s, "{") {
		return s
	}
	var out strings.Builder
	floor, pos := 0, 0 // floor: text before it is decided; pos: where to look for the next "{"
	for {
		k := strings.IndexByte(s[pos:], '{')
		if k < 0 {
			break
		}
		k += pos
		closeAt := matchingBrace(s, k)
		if closeAt < 0 {
			pos = k + 1
			continue
		}
		ps := preludeStart(s, k, floor)
		if ps < 0 || !cssBody(s[k+1:closeAt], strings.HasPrefix(strings.TrimSpace(s[ps:k]), "@")) {
			pos = k + 1
			continue
		}
		start := commentsBefore(s, ps, floor)
		end := commentsAfter(s, closeAt+1)
		out.WriteString(s[floor:start])
		out.WriteByte('\n') // the text on both sides stays apart
		floor, pos = end, end
	}
	out.WriteString(s[floor:])
	return out.String()
}

// matchingBrace is the index of the "}" that closes the "{" at s[open], skipping quoted strings and comments; -1 when it never closes.
func matchingBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch c := s[i]; {
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				return -1
			}
			i += j + 3
		case c == '"' || c == '\'':
			for i++; i < len(s) && s[i] != c && s[i] != '\n'; i++ {
			}
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

type token struct {
	at   int
	text string
}

// tokens splits s into whitespace-separated words with their offsets.
func tokens(s string, base int) []token {
	var out []token
	i := 0
	for i < len(s) {
		for i < len(s) && strings.IndexByte(" \t\r\n", s[i]) >= 0 {
			i++
		}
		j := i
		for j < len(s) && strings.IndexByte(" \t\r\n", s[j]) < 0 {
			j++
		}
		if j > i {
			out = append(out, token{base + i, s[i:j]})
		}
		i = j
	}
	return out
}

// preludeStart is where the selector (or @-rule head) of the block opening at s[k] starts, looking back no further than floor;
// -1 when what stands before the brace does not read as a selector. The selector is the line of the brace, or the line above when
// the brace has a line to itself, plus lines above that end in a comma (a selector list).
func preludeStart(s string, k, floor int) int {
	lineStart := func(upTo int) int { // start of the line that ends at upTo
		if j := strings.LastIndexByte(s[floor:upTo], '\n'); j >= 0 {
			return floor + j + 1
		}
		return floor
	}
	ls := lineStart(k)
	if strings.TrimSpace(s[ls:k]) == "" {
		above := strings.TrimRight(s[floor:ls], " \t\r\n")
		if above == "" {
			return -1
		}
		ls = lineStart(floor + len(above))
	}
	for ls > floor {
		above := strings.TrimRight(s[floor:ls], " \t\r\n")
		if !strings.HasSuffix(above, ",") || strings.Count(s[floor+len(above):ls], "\n") > 1 {
			break
		}
		ls = lineStart(floor + len(above))
	}
	pre := s[ls:k]
	if m := atHead.FindStringSubmatchIndex(pre); m != nil {
		return ls + m[2]
	}
	toks := tokens(pre, ls)
	first := len(toks)
	for first > 0 && selectorToken(toks[first-1].text) {
		first--
	}
	if first == len(toks) {
		return -1
	}
	return toks[first].at
}

// selectorToken says whether a word of the text before a brace can belong to a selector.
func selectorToken(t string) bool {
	t = strings.TrimSuffix(t, ",")
	switch t {
	case "":
		return false
	case ">", "+", "~", "*", "|":
		return true
	}
	if strings.IndexByte(".#:[*&>+~|", t[0]) >= 0 || strings.ContainsAny(t, ".#:[]>+~=*") {
		return true
	}
	return htmlTags[strings.ToLower(t)]
}

// cssBody says whether the text between the braces is what a stylesheet puts there: for a plain rule only declarations; for an
// @-rule (@media, @font-face, @keyframes …) at least one declaration somewhere inside.
func cssBody(body string, atRule bool) bool {
	body = stripComments(body)
	if atRule {
		return declAnywhere.MatchString(body)
	}
	if strings.ContainsAny(withoutQuoted(body), "{}") {
		return false // nested rules: only an @-rule has those
	}
	found := false
	for _, part := range splitDeclarations(body) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !declStart.MatchString(part) {
			return false
		}
		found = true
	}
	return found
}

func stripComments(s string) string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			return s[:i]
		}
		s = s[:i] + " " + s[i+2+j+2:]
	}
}

// splitDeclarations splits on ";" outside quotes and parentheses (a data: URL holds a ";").
func splitDeclarations(body string) []string {
	var parts []string
	depth, from := 0, 0
	var quote byte
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(':
			depth++
		case c == ')' && depth > 0:
			depth--
		case c == ';' && depth == 0:
			parts = append(parts, body[from:i])
			from = i + 1
		}
	}
	return append(parts, body[from:])
}

// commentsBefore moves from back over the /* … */ comments (and the blanks between them) that stand right before it.
func commentsBefore(s string, from, floor int) int {
	for {
		t := strings.TrimRight(s[floor:from], " \t\r\n")
		if !strings.HasSuffix(t, "*/") {
			return from
		}
		i := strings.LastIndex(t, "/*")
		if i < 0 {
			return from
		}
		from = floor + i
	}
}

// commentsAfter moves from forward over the comments (and blanks) that follow it.
func commentsAfter(s string, from int) int {
	for {
		rest := s[from:]
		trimmed := strings.TrimLeft(rest, " \t\r\n")
		if !strings.HasPrefix(trimmed, "/*") {
			return from
		}
		j := strings.Index(trimmed, "*/")
		if j < 0 {
			return from
		}
		from += len(rest) - len(trimmed) + j + 2
	}
}

// withoutQuoted is s with the text inside quotes taken out, so a brace in a string is not mistaken for a block.
func withoutQuoted(s string) string {
	var b strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote || c == '\n' {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
