package phijson

import "strings"

// repairBrackets reads the JSON value that starts at s[0] ('[' or '{') the way a small model writes it: it may leave a closing
// bracket out, close in the wrong order, or stop before the end. It returns the value with the brackets that are missing put in,
// and how many bytes of s it read. A real run of the CV import (2026-11-03) had Phi write
//
//	[{"name": "cv_import_set","arguments":{..."value": {...}}]      (one "}" short)
//
// which is not JSON, so the call was read as plain text and nothing was stored. Only brackets are added or skipped: no value,
// key or string is changed, and what comes out is still checked by the caller against the offered tools. ok is false when the text
// stops inside a string (what was meant there is not known) or does not start with a bracket.
func repairBrackets(s string) (fixed string, consumed int, ok bool) {
	if s == "" || (s[0] != '[' && s[0] != '{') {
		return "", 0, false
	}
	var out strings.Builder
	var stack []byte // the closers still to come, innermost last
	inString, escaped := false, false
	closer := func(open byte) byte {
		if open == '[' {
			return ']'
		}
		return '}'
	}
	// A comma that was left hanging before a closer would make the repaired text invalid.
	trimComma := func() {
		t := strings.TrimRight(out.String(), " \t\r\n")
		if strings.HasSuffix(t, ",") {
			t = strings.TrimRight(strings.TrimSuffix(t, ","), " \t\r\n")
		}
		out.Reset()
		out.WriteString(t)
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			out.WriteByte(c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
			out.WriteByte(c)
		case '{', '[':
			stack = append(stack, closer(c))
			out.WriteByte(c)
		case '}', ']':
			// Find the opener this closer belongs to, from the innermost out; a closer with no opener is stray and skipped.
			depth := -1
			for k := len(stack) - 1; k >= 0; k-- {
				if stack[k] == c {
					depth = k
					break
				}
			}
			if depth < 0 {
				continue
			}
			for len(stack)-1 > depth { // the brackets left open inside it are closed first
				trimComma()
				out.WriteByte(stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}
			trimComma()
			out.WriteByte(c)
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return out.String(), i + 1, true
			}
		default:
			out.WriteByte(c)
		}
	}
	if inString {
		return "", 0, false
	}
	trimComma()
	for len(stack) > 0 { // the text stopped early: the brackets still open are closed
		out.WriteByte(stack[len(stack)-1])
		stack = stack[:len(stack)-1]
	}
	return out.String(), len(s), true
}
