// Copied from tools/pdf_tools/backend/convert (clean.go and its test, unchanged apart from the package name): the Career tool reads a CV the
// same way the pdf_to_markdown tool does, so the code-driven import (plan 209) sees exactly the text a model would. A change to the cleaning
// there must be made here too; the test below is the shared contract.

package pdftext

import (
	"strings"
	"unicode"
)

// CleanText makes the text a PDF extractor returned cheaper for a language model to read, without changing what it says: the words, their
// order and every character that is not white space stay exactly as they were. It is pure and idempotent (CleanText(CleanText(x)) equals
// CleanText(x)) and never makes a text longer.
//
// What the extractor gives (markitdown-go's GetPlainText on a typical two-column CV) is one word to a line with a white-space-only line after it
// and several of them between lines of the page: a 10,837 character CV of 2,191 lines, 1,132 of them blank, that costs 3,317 tokens. Measured with
// the Phi-4-mini tokenizer: dropping the blank lines saves 12%, putting the words back into lines saves 41%. The rules:
//
//  1. Always: line endings become "\n"; the no-break and other Unicode spaces become a plain space; the zero-width space and the byte order mark
//     are removed (the zero-width joiners that emoji and some scripts need are kept); every line loses the white space around it and inside it
//     runs of spaces and tabs become one space; leading and trailing blank lines go.
//  2. A text that is not in the extractor's one-word-a-line shape keeps its lines: a run of blank lines becomes one blank line (a paragraph break).
//  3. A text that is in that shape (see wordPerLine) is put back into lines: words with one blank line between them, or none, join with a space,
//     and two or more blank lines between them are a line break.
func CleanText(s string) string {
	lines := strings.Split(normalizeSpace(s), "\n")
	for i, l := range lines {
		lines[i] = squeeze(strings.TrimSpace(l))
	}
	if wordPerLine(lines) {
		return reflow(lines)
	}
	return collapseBlankRuns(lines)
}

// normalizeSpace unifies line endings and the kinds of space, and removes what is invisible and means nothing.
func normalizeSpace(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\u200b' || r == '\ufeff': // zero-width space, byte order mark
			return -1
		case r == '\n' || r == '\t':
			return r
		case r == '\u00a0' || r == '\u202f' || r == '\u205f' || r == '\u3000' || (r >= '\u2000' && r <= '\u200a'):
			return ' '
		case r == '\u2028' || r == '\u2029' || r == '\f' || r == '\v' || r == '\u0085': // line, paragraph and page separators
			return '\n'
		}
		return r
	}, s)
}

// squeeze turns every run of spaces and tabs in a line into one space (the line is already trimmed).
func squeeze(line string) string {
	if !strings.ContainsAny(line, "\t ") {
		return line
	}
	var b strings.Builder
	b.Grow(len(line))
	space := false
	for _, r := range line {
		if r == ' ' || r == '\t' || unicode.Is(unicode.Zs, r) {
			if !space {
				b.WriteByte(' ')
			}
			space = true
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

const (
	// minWordLines is how many non-blank lines a text needs before it can be taken for the extractor's shape: a short list of single words
	// (a few skills, a few names) is what it says it is.
	minWordLines = 20
	// minSingleWordShare is how many of the non-blank lines hold at most one word, and minBlankShare how many of all lines are blank, for a text
	// to be in that shape. Ordinary text has long lines and few blank ones, a list of skills one to a line has no blank lines at all.
	minSingleWordShare = 0.8
	minBlankShare      = 0.25
)

// wordPerLine says whether the lines have the extractor's one-word-a-line shape: enough lines, nearly all of them one word or less, and a good
// part of the lines blank (the separators the extractor puts between the words). Only the lines from the first word to the last count: the
// blank lines at either end (and a final line ending) say nothing about the text between.
func wordPerLine(lines []string) bool {
	first, last := -1, -1
	for i, l := range lines {
		if l != "" {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return false
	}
	nonBlank, single := 0, 0
	for _, l := range lines[first : last+1] {
		if l == "" {
			continue
		}
		nonBlank++
		if !strings.Contains(l, " ") {
			single++
		}
	}
	if nonBlank < minWordLines {
		return false
	}
	blank := last + 1 - first - nonBlank
	return float64(single) >= minSingleWordShare*float64(nonBlank) && float64(blank) >= minBlankShare*float64(last+1-first)
}

// reflow puts the words back into lines: between two words one blank line or none is a space, two or more are a line break.
func reflow(lines []string) string {
	var out []string
	var cur []string
	blank := 0
	for _, l := range lines {
		if l == "" {
			blank++
			continue
		}
		if len(cur) > 0 && blank >= 2 {
			out = append(out, strings.Join(cur, " "))
			cur = cur[:0]
		}
		cur = append(cur, l)
		blank = 0
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, " "))
	}
	return strings.Join(out, "\n")
}

// collapseBlankRuns keeps the lines, turns every run of blank lines into one and drops the blank lines at both ends.
func collapseBlankRuns(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
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
