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
//
// The opposite slip is read too: a call object closed too early, so that its own "name" came after it (a real run of 2026-11-04):
//
//	[{"arguments":{...,"title":"x"},"items":"1"},"name":"cv_import_add_experience"}]      (the call closed after "items")
//
// A "}" that closes an object standing directly in a list and is followed by , "key": cannot be right, because a list holds values and
// never keys: the object goes on, so that "}" is left out and the object closes at its real end. The same words after an object that
// sits inside another object are an ordinary next key and are not touched. (A call at the top level, not in a list, that is closed too
// early is a complete valid object followed by text, so it never gets here and is not read: it was not seen in a real run.)
// keyFollows says whether s, after white space, goes on with a comma and then a string and a colon: the start of another member of
// an object, `, "key":`.
func keyFollows(s string) bool {
	skip := func(i int) int {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
			i++
		}
		return i
	}
	i := skip(0)
	if i >= len(s) || s[i] != ',' {
		return false
	}
	i = skip(i + 1)
	if i >= len(s) || s[i] != '"' {
		return false
	}
	for i++; i < len(s); i++ { // to the end of the string, over its escapes
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '"' {
			break
		}
	}
	if i >= len(s) {
		return false
	}
	i = skip(i + 1)
	return i < len(s) && s[i] == ':'
}

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
			if c == '}' && depth == len(stack)-1 && len(stack) >= 2 && stack[len(stack)-2] == ']' && keyFollows(s[i+1:]) {
				continue // closed too early: the object goes on with the key that follows
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
