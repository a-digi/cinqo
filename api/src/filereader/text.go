package filereader

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// readLine reads one line from r without ever holding more than keep bytes of it: the rest of a long
// line is read and counted but thrown away. total is the line's length in bytes without its line ending;
// the line ending ("\n" or "\r\n") is consumed and not included. ok is false when r had nothing left.
// A last line with no newline is still a line. The text is built in scratch (reused, so reading a million lines
// does not allocate a million slices): it is only valid until the next call with the same scratch.
func readLine(r *bufio.Reader, keep int, scratch []byte) (text []byte, total int, ok bool, err error) {
	kept := scratch[:0]
	read := false
	for {
		chunk, e := r.ReadSlice('\n')
		if len(chunk) > 0 {
			read = true
		}
		hasNewline := len(chunk) > 0 && chunk[len(chunk)-1] == '\n'
		body := chunk
		if hasNewline {
			body = chunk[:len(chunk)-1]
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
			if total == len(kept) && len(kept) > 0 && kept[len(kept)-1] == '\r' {
				kept = kept[:len(kept)-1]
				total--
			}
			return kept, total, true, nil
		case errors.Is(e, bufio.ErrBufferFull):
			// a long line: keep reading the rest of it
			continue
		case errors.Is(e, io.EOF):
			if !read {
				return nil, 0, false, nil
			}
			if total == len(kept) && len(kept) > 0 && kept[len(kept)-1] == '\r' {
				kept = kept[:len(kept)-1]
				total--
			}
			return kept, total, true, nil
		default:
			return nil, 0, false, e
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
