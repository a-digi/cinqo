// Package jsonview is the JSON beautifier of Cinqo's log reader (plan/ai/file-reader/step-202-json-beautifier.md): a streaming
// re-indenter and validator. It reads JSON of any size through one fixed buffer and writes it with two-space indentation, never building
// a tree, so memory is a buffer and not the file. String contents are copied byte for byte (an escape such as > stays >): the
// output is the same JSON reformatted, never reinterpreted. Invalid JSON stops the conversion at the first error with its byte offset and
// what was expected, built from fixed phrases and never from the file's own text.
//
// It imports only the standard library and nothing else in Cinqo imports it except the file reader's handler.
package jsonview

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
)

const (
	// DefaultMaxDepth is how deeply arrays and objects may nest. A deeper document is refused, not parsed.
	DefaultMaxDepth = 128
	// DefaultMaxBytes is the largest input Format reads (32 MiB).
	DefaultMaxBytes = 32 << 20
	// DefaultMaxOutputBytes is the most Format writes (128 MiB). Indenting can multiply a document (a million short elements nested
	// deep each get their own indented line), so the output is bounded as well as the input.
	DefaultMaxOutputBytes = 128 << 20

	indentUnit = "  "
	bufSize    = 64 << 10
)

var (
	// ErrTooLarge: the input is longer than Limits.MaxBytes, or its indented form would be longer than Limits.MaxOutputBytes.
	ErrTooLarge = errors.New("jsonview: input is too large to format")
	// ErrTooDeep: arrays and objects nest deeper than Limits.MaxDepth.
	ErrTooDeep = errors.New("jsonview: nesting is too deep")
)

// SyntaxError is the first thing wrong with the input. Offset is the 0-based byte where it was noticed; Expected is a fixed phrase.
// Neither carries any of the input's own text.
type SyntaxError struct {
	Offset   int64
	Expected string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("not valid JSON: expected %s at byte %d", e.Expected, e.Offset)
}

// Limits bounds one conversion. A zero field takes its default.
type Limits struct {
	MaxBytes       int64
	MaxOutputBytes int64
	MaxDepth       int
}

func (l Limits) withDefaults() Limits {
	if l.MaxBytes <= 0 {
		l.MaxBytes = DefaultMaxBytes
	}
	if l.MaxOutputBytes <= 0 {
		l.MaxOutputBytes = DefaultMaxOutputBytes
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = DefaultMaxDepth
	}
	return l
}

// Format reads one JSON value from src (compact or already indented) and writes it indented to dst, without a trailing newline: the same
// bytes encoding/json's Indent(compact, "", "  ") gives. It returns the bytes written. Errors: *SyntaxError (not valid JSON),
// ErrTooLarge, ErrTooDeep, the context's error when it ends, or an error of src or dst. Nothing is guaranteed about what was already
// written to dst when it fails: a caller that keeps the output writes it to a temporary file first.
func Format(ctx context.Context, dst io.Writer, src io.Reader, lim Limits) (int64, error) {
	f := &formatter{ctx: ctx, r: src, buf: make([]byte, bufSize), w: bufio.NewWriterSize(dst, bufSize), lim: lim.withDefaults()}
	if err := f.run(); err != nil {
		return f.written, err
	}
	if err := f.w.Flush(); err != nil {
		return f.written, err
	}
	return f.written, nil
}

type state int

const (
	sTop      state = iota // before the one top-level value
	sArrFirst              // just after '[': a value or ']'
	sArrNext               // after a comma in an array: a value
	sObjFirst              // just after '{': a key or '}'
	sObjNext               // after a comma in an object: a key
	sColon                 // after a key: ':'
	sObjValue              // after ':': a value
	sAfter                 // after a value inside an array or object: ',' or the closing bracket
	sDone                  // after the top-level value: only whitespace may follow
)

type formatter struct {
	ctx context.Context
	r   io.Reader
	buf []byte
	pos int
	end int
	// base is the offset in the input of buf[0].
	base  int64
	total int64
	eof   bool

	w       *bufio.Writer
	written int64
	lim     Limits

	stack []byte // '[' or '{' for each open container
}

// fill reads more input into the buffer. It reports false at the end of the input.
func (f *formatter) fill() (bool, error) {
	if f.eof {
		return false, nil
	}
	if err := f.ctx.Err(); err != nil {
		return false, err
	}
	f.base += int64(f.end)
	f.pos, f.end = 0, 0
	for {
		n, err := f.r.Read(f.buf)
		if n > 0 {
			f.end = n
			f.total += int64(n)
			if f.total > f.lim.MaxBytes {
				return false, ErrTooLarge
			}
			if err != nil && err != io.EOF {
				return false, err
			}
			if err == io.EOF {
				f.eof = true
			}
			return true, nil
		}
		if err == io.EOF {
			f.eof = true
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}

func (f *formatter) offset() int64 { return f.base + int64(f.pos) }

func (f *formatter) syntax(expected string) error {
	return &SyntaxError{Offset: f.offset(), Expected: expected}
}

// peek returns the next byte without consuming it. ok is false at the end of the input.
func (f *formatter) peek() (byte, bool, error) {
	if f.pos >= f.end {
		more, err := f.fill()
		if err != nil || !more {
			return 0, false, err
		}
	}
	return f.buf[f.pos], true, nil
}

func (f *formatter) take() (byte, bool, error) {
	c, ok, err := f.peek()
	if ok {
		f.pos++
	}
	return c, ok, err
}

func (f *formatter) write(p []byte) error {
	n, err := f.w.Write(p)
	f.written += int64(n)
	return err
}

func (f *formatter) writeByte(c byte) error {
	if err := f.w.WriteByte(c); err != nil {
		return err
	}
	f.written++
	return nil
}

func (f *formatter) writeString(s string) error {
	n, err := f.w.WriteString(s)
	f.written += int64(n)
	return err
}

func (f *formatter) newline() error {
	// Indentation is where a document grows; everything else is copied one for one, so this is the one place to bound the output.
	if f.written > f.lim.MaxOutputBytes {
		return ErrTooLarge
	}
	if err := f.writeByte('\n'); err != nil {
		return err
	}
	for i := 0; i < len(f.stack); i++ {
		if err := f.writeString(indentUnit); err != nil {
			return err
		}
	}
	return nil
}

// nextNonSpace consumes whitespace and returns the next significant byte (consumed). ok is false at the end of the input.
func (f *formatter) nextNonSpace() (byte, bool, error) {
	for {
		c, ok, err := f.take()
		if err != nil || !ok {
			return 0, false, err
		}
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return c, true, nil
		}
	}
}

func closerOf(open byte) byte {
	if open == '[' {
		return ']'
	}
	return '}'
}

// expectedAt names what the state was waiting for, for an end of input or a wrong byte.
func expectedAt(s state) string {
	switch s {
	case sArrFirst:
		return "a value or ]"
	case sObjFirst:
		return "a key (a string) or }"
	case sObjNext:
		return "a key (a string)"
	case sColon:
		return "a colon"
	case sAfter:
		return "a comma or a closing bracket"
	case sDone:
		return "the end of the input"
	}
	return "a value"
}

func (f *formatter) run() error {
	st := sTop
	for {
		c, ok, err := f.nextNonSpace()
		if err != nil {
			return err
		}
		if !ok {
			if st == sDone {
				return nil
			}
			return f.syntax(expectedAt(st))
		}
		// f.pos is now one past c; errors about c itself report c's offset.
		at := f.offset() - 1
		bad := func(expected string) error { return &SyntaxError{Offset: at, Expected: expected} }

		switch st {
		case sDone:
			return bad("the end of the input")
		case sTop, sObjValue:
			if st, err = f.value(c, at); err != nil {
				return err
			}
		case sArrFirst:
			if c == ']' {
				f.stack = f.stack[:len(f.stack)-1]
				if err := f.writeByte(']'); err != nil {
					return err
				}
				st = f.afterValue()
				continue
			}
			if err := f.newline(); err != nil {
				return err
			}
			if st, err = f.value(c, at); err != nil {
				return err
			}
		case sArrNext:
			if err := f.newline(); err != nil {
				return err
			}
			if st, err = f.value(c, at); err != nil {
				return err
			}
		case sObjFirst:
			if c == '}' {
				f.stack = f.stack[:len(f.stack)-1]
				if err := f.writeByte('}'); err != nil {
					return err
				}
				st = f.afterValue()
				continue
			}
			fallthrough
		case sObjNext:
			if c != '"' {
				return bad(expectedAt(st))
			}
			if err := f.newline(); err != nil {
				return err
			}
			if err := f.str(); err != nil {
				return err
			}
			st = sColon
		case sColon:
			if c != ':' {
				return bad("a colon")
			}
			if err := f.writeString(": "); err != nil {
				return err
			}
			st = sObjValue
		case sAfter:
			top := f.stack[len(f.stack)-1]
			switch c {
			case ',':
				if err := f.writeByte(','); err != nil {
					return err
				}
				if top == '[' {
					st = sArrNext
				} else {
					st = sObjNext
				}
			case closerOf(top):
				f.stack = f.stack[:len(f.stack)-1]
				if err := f.newline(); err != nil {
					return err
				}
				if err := f.writeByte(c); err != nil {
					return err
				}
				st = f.afterValue()
			default:
				return bad("a comma or a closing bracket")
			}
		}
	}
}

// afterValue is the state once a value (a scalar, or a container just closed) is complete.
func (f *formatter) afterValue() state {
	if len(f.stack) == 0 {
		return sDone
	}
	return sAfter
}

// value handles the first byte c of a value and returns the state that follows.
func (f *formatter) value(c byte, at int64) (state, error) {
	switch c {
	case '{', '[':
		if len(f.stack) >= f.lim.MaxDepth {
			return 0, ErrTooDeep
		}
		f.stack = append(f.stack, c)
		if err := f.writeByte(c); err != nil {
			return 0, err
		}
		if c == '[' {
			return sArrFirst, nil
		}
		return sObjFirst, nil
	case '"':
		if err := f.str(); err != nil {
			return 0, err
		}
	case 't':
		if err := f.literal("true"); err != nil {
			return 0, err
		}
	case 'f':
		if err := f.literal("false"); err != nil {
			return 0, err
		}
	case 'n':
		if err := f.literal("null"); err != nil {
			return 0, err
		}
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		if err := f.number(c); err != nil {
			return 0, err
		}
	default:
		return 0, &SyntaxError{Offset: at, Expected: "a value"}
	}
	return f.afterValue(), nil
}

// literal copies the rest of true, false or null (its first byte was consumed and is word[0]).
func (f *formatter) literal(word string) error {
	if err := f.writeByte(word[0]); err != nil {
		return err
	}
	for i := 1; i < len(word); i++ {
		c, ok, err := f.peek()
		if err != nil {
			return err
		}
		if !ok || c != word[i] {
			return f.syntax(word)
		}
		f.pos++
		if err := f.writeByte(c); err != nil {
			return err
		}
	}
	return nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// digits copies a run of digits and reports how many there were.
func (f *formatter) digits() (int, error) {
	n := 0
	for {
		c, ok, err := f.peek()
		if err != nil {
			return n, err
		}
		if !ok || !isDigit(c) {
			return n, nil
		}
		f.pos++
		n++
		if err := f.writeByte(c); err != nil {
			return n, err
		}
	}
}

// number copies a JSON number whose first byte, first, was consumed. The byte after it is left for the caller's state to judge.
func (f *formatter) number(first byte) error {
	if err := f.writeByte(first); err != nil {
		return err
	}
	switch {
	case first == '-':
		c, ok, err := f.peek()
		if err != nil {
			return err
		}
		if !ok || !isDigit(c) {
			return f.syntax("a digit")
		}
		f.pos++
		if err := f.writeByte(c); err != nil {
			return err
		}
		if c != '0' {
			if _, err := f.digits(); err != nil {
				return err
			}
		}
	case first != '0':
		if _, err := f.digits(); err != nil {
			return err
		}
	}
	if c, ok, err := f.peek(); err != nil {
		return err
	} else if ok && c == '.' {
		f.pos++
		if err := f.writeByte('.'); err != nil {
			return err
		}
		n, err := f.digits()
		if err != nil {
			return err
		}
		if n == 0 {
			return f.syntax("a digit")
		}
	}
	if c, ok, err := f.peek(); err != nil {
		return err
	} else if ok && (c == 'e' || c == 'E') {
		f.pos++
		if err := f.writeByte(c); err != nil {
			return err
		}
		if s, ok, err := f.peek(); err != nil {
			return err
		} else if ok && (s == '+' || s == '-') {
			f.pos++
			if err := f.writeByte(s); err != nil {
				return err
			}
		}
		n, err := f.digits()
		if err != nil {
			return err
		}
		if n == 0 {
			return f.syntax("a digit")
		}
	}
	return nil
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// str copies a string whose opening quote was consumed: every byte up to and including the closing quote, validating the escapes and
// refusing raw control characters. Runs of ordinary bytes are written in one piece.
func (f *formatter) str() error {
	if err := f.writeByte('"'); err != nil {
		return err
	}
	for {
		if f.pos >= f.end {
			more, err := f.fill()
			if err != nil {
				return err
			}
			if !more {
				return f.syntax("a closing quote")
			}
		}
		i := f.pos
		for i < f.end {
			c := f.buf[i]
			if c == '"' || c == '\\' || c < 0x20 {
				break
			}
			i++
		}
		if i > f.pos {
			if err := f.write(f.buf[f.pos:i]); err != nil {
				return err
			}
			f.pos = i
		}
		if f.pos >= f.end {
			continue
		}
		c := f.buf[f.pos]
		switch {
		case c == '"':
			f.pos++
			return f.writeByte('"')
		case c < 0x20:
			return f.syntax("no raw control character inside a string")
		default: // a backslash
			f.pos++
			if err := f.writeByte('\\'); err != nil {
				return err
			}
			e, ok, err := f.peek()
			if err != nil {
				return err
			}
			if !ok {
				return f.syntax("an escape")
			}
			switch e {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				f.pos++
				if err := f.writeByte(e); err != nil {
					return err
				}
			case 'u':
				f.pos++
				if err := f.writeByte('u'); err != nil {
					return err
				}
				for k := 0; k < 4; k++ {
					h, ok, err := f.peek()
					if err != nil {
						return err
					}
					if !ok || !isHex(h) {
						return f.syntax("four hex digits")
					}
					f.pos++
					if err := f.writeByte(h); err != nil {
						return err
					}
				}
			default:
				return f.syntax("a valid escape")
			}
		}
	}
}
