package filetail

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// trimPartialRune drops a multi-byte character cut in half at the end of b (a line cut in the middle of one). Copied, not shared, from
// the file reader (api/src/filereader/text.go): the two modules never depend on each other.
func trimPartialRune(b []byte) []byte {
	for i := 0; i < utf8.UTFMax && len(b) > 0; i++ {
		if r, size := utf8.DecodeLastRune(b); r != utf8.RuneError || size != 1 {
			return b
		}
		b = b[:len(b)-1]
	}
	return b
}

// sanitize turns raw line bytes into text that is safe to show: invalid UTF-8 becomes U+FFFD, and control characters (ESC and friends)
// become their visible symbols (the U+2400 block), so a log can never paint over a terminal. A tab stays a tab. The same rule as the
// file reader's, so the two viewers show a line alike.
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
