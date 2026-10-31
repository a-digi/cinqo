package filereader

import (
	"bufio"
	"context"
	"io"
	"os"
	"time"
)

// guard stops a read that has gone on too long: it is checked every few MB while a long line is read through,
// not only between lines, so one huge line can neither run past the time budget nor ignore a cancelled request.
type guard struct {
	ctx      context.Context
	deadline time.Time
}

func (e *Engine) newGuard(ctx context.Context) *guard {
	return &guard{ctx: ctx, deadline: time.Now().Add(e.lim.ReadBudget)}
}

func (g *guard) check() error {
	if err := g.ctx.Err(); err != nil {
		return err
	}
	if time.Now().After(g.deadline) {
		return ErrSlow
	}
	return nil
}

// checkEveryChunks is how many 64 KB pieces of one line are read between two looks at the guard (about 4 MB).
const checkEveryChunks = 64

// cursor reads lines one after another from a position in a file, and knows the byte offset where the next
// line starts. A line the index knows to be long is jumped over by seeking, never read through: only its first
// bytes are read.
type cursor struct {
	f     *os.File
	size  int64
	r     *bufio.Reader
	line  int64 // the number of the line next() returns
	pos   int64 // the byte offset where that line starts
	longs []longLine
	g     *guard
	// target is the line a caller wants the cursor to reach (see skipErr); zero when unused.
	target int64
}

func newCursor(f *os.File, size, pos, line int64, longs []longLine, g *guard) *cursor {
	return &cursor{
		f: f, size: size, line: line, pos: pos, longs: longs, g: g,
		r: bufio.NewReaderSize(io.NewSectionReader(f, pos, size-pos), 64<<10),
	}
}

// next returns the next line, keeping at most keep bytes of it (see readLine). ok is false at the end of the
// file.
func (c *cursor) next(keep int, scratch []byte) (lineRead, error) {
	if long, isLong := longAt(c.longs, c.line); isLong && long.start == c.pos {
		return c.jump(long, keep, scratch)
	}
	lr, err := readLine(c.r, keep, scratch, c.g)
	if err != nil || !lr.ok {
		return lr, err
	}
	c.pos += int64(lr.total + lr.eol)
	c.line++
	return lr, nil
}

// jump serves a line the index knows is long: its head is read at its known position and the cursor moves
// straight past it.
func (c *cursor) jump(long longLine, keep int, scratch []byte) (lineRead, error) {
	if err := c.g.check(); err != nil {
		return lineRead{}, err
	}
	head := scratch[:0]
	if want := min(int64(keep), long.length); want > 0 {
		if int64(cap(head)) < want {
			head = make([]byte, want)
		}
		head = head[:want]
		if n, err := c.f.ReadAt(head, long.start); err != nil && n < len(head) {
			return lineRead{}, err
		}
	}
	total := int(long.length)
	next := long.start + long.length
	eol := 0
	if next < c.size {
		eol = 1 // the "\n"
	}
	// a "\r" just before the "\n" is part of the line ending
	if long.length > 0 {
		var last [1]byte
		if _, err := c.f.ReadAt(last[:], long.start+long.length-1); err == nil && last[0] == '\r' && eol == 1 {
			total--
			eol = 2
		}
	}
	c.pos = long.start + int64(total+eol)
	c.line++
	c.r.Reset(io.NewSectionReader(c.f, c.pos, c.size-c.pos))
	return lineRead{text: head, total: total, eol: eol, ok: true}, nil
}

// skipTo reads and drops lines until `line` is the next one. It reports false when the file ends first.
func (c *cursor) skipTo(line int64) (bool, error) {
	for c.line < line {
		lr, err := c.next(0, nil)
		if err != nil {
			return false, err
		}
		if !lr.ok {
			return false, nil
		}
	}
	return true, nil
}

// more reports whether any bytes are left.
func (c *cursor) more() bool { return c.pos < c.size }
