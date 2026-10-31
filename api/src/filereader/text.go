package filereader

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// lineRead is one line as readLine returns it. text holds at most keep bytes of it; total is its length in
// bytes without the line ending; eol is how many bytes the ending took (0 at the end of a file with no final
// newline, 1 for "\n", 2 for "\r\n"); ok is false when there was nothing left to read.
type lineRead struct {
	text  []byte
	total int
	eol   int
	ok    bool
}

// readLine reads one line from r without ever holding more than keep bytes of it: the rest of a long line is
// read and counted but thrown away. A last line with no newline is still a line. The text is built in scratch
// (reused, so reading a million lines does not allocate a million slices): it is only valid until the next call
// with the same scratch. g, when set, is checked every few MB of a long line, so a huge line can be cut short.
func readLine(r *bufio.Reader, keep int, scratch []byte, g *guard) (lineRead, error) {
	kept := scratch[:0]
	total := 0
	var last byte // the last byte of the line seen so far, kept or not: a "\r" before the line ending is found by it
	read := false
	finish := func(eol int) lineRead {
		// a "\r" just before the "\n" belongs to the line ending, even when the line is too long to have been kept
		if total > 0 && last == '\r' {
			total--
			eol++
			if len(kept) > total {
				kept = kept[:total]
			}
		}
		return lineRead{text: kept, total: total, eol: eol, ok: true}
	}
	for chunks := 1; ; chunks++ {
		if g != nil && chunks%checkEveryChunks == 0 {
			if err := g.check(); err != nil {
				return lineRead{}, err
			}
		}
		chunk, e := r.ReadSlice('\n')
		if len(chunk) > 0 {
			read = true
		}
		hasNewline := len(chunk) > 0 && chunk[len(chunk)-1] == '\n'
		body := chunk
		if hasNewline {
			body = chunk[:len(chunk)-1]
		}
		if len(body) > 0 {
			last = body[len(body)-1]
		}
		total += len(body)
		if room := keep - len(kept); room > 0 {
			if len(body) < room {
				room = len(body)
			}
			kept = append(kept, body[:room]...)
		}
		switch {
		case e == nil:
			// the newline arrived: the line is complete
			return finish(1), nil
		case errors.Is(e, bufio.ErrBufferFull):
			// a long line: keep reading the rest of it
			continue
		case errors.Is(e, io.EOF):
			if !read {
				return lineRead{}, nil
			}
			return finish(0), nil
		default:
			return lineRead{}, e
		}
	}
}

// trimPartialRune drops a multi-byte character cut in half at the end of b (a line truncated in the
// middle of one): trailing bytes are removed until what is left ends on a complete character.
func trimPartialRune(b []byte) []byte {
	for i := 0; i < utf8.UTFMax && len(b) > 0; i++ {
		if r, size := utf8.DecodeLastRune(b); r != utf8.RuneError || size != 1 {
			return b
		}
		b = b[:len(b)-1]
	}
	return b
}

// sanitize turns raw line bytes into text that is safe to show: invalid UTF-8 becomes U+FFFD, and control
// characters (ESC and friends) become their visible symbols (U+2400 block) so a log can never paint over a
// terminal. A tab stays a tab.
func sanitize(b []byte) string {
	clean := !bytes.ContainsFunc(b, func(r rune) bool { return r < 0x20 && r != '\t' || r == 0x7f })
	if clean && utf8.Valid(b) {
		return string(b)
	}
	s := strings.ToValidUTF8(string(b), "�")
	var out strings.Builder
	out.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			out.WriteRune(r)
		case r < 0x20:
			out.WriteRune('␀' + r)
		case r == 0x7f:
			out.WriteRune('␡')
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
