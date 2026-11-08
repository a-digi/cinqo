// Package filetail is a livelog.Follower for one regular text file: it reads what was appended since a byte offset, so a poll costs the
// new bytes and never the file's size, and it notices when the file was truncated or replaced. It knows nothing of sources, scopes or
// HTTP: a Provider (api/src/livelog/providers) resolves a reference to a path and hands it here. It imports only the standard library and
// the module's core.
package filetail

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/a-digi/cinqo/src/livelog"
)

const (
	// headBytes is how much of the file's start a cursor vouches for: if those bytes change, the file is another one.
	headBytes = 64
	// DefaultGrace is how long a file must be quiet before an unterminated last line is returned as a line.
	DefaultGrace = 2 * time.Second
)

// Options are the follower's bounds. Unset values are the defaults.
type Options struct {
	// Limits bounds one poll (lines, bytes read, line length); unset fields take livelog.DefaultLimits.
	Limits livelog.Limits
	// Grace is how long the file must have been quiet (by its modification time) before a last line without a line ending is
	// returned: until then it is held back, as it may be half written. A log that never ends its last line is not hidden for ever.
	Grace time.Duration
	// Now is the clock (tests set it).
	Now func() time.Time
}

// Follower follows one file. It holds no state beyond its settings: everything it needs to continue is in the cursor.
type Follower struct {
	path  string
	lim   livelog.Limits
	grace time.Duration
	now   func() time.Time
}

// New follows the file at path. The path is trusted: the provider that resolved it is the one that kept it inside its root.
func New(path string, o Options) *Follower {
	f := &Follower{path: path, lim: o.Limits.Normalized(), grace: o.Grace, now: o.Now}
	if f.grace <= 0 {
		f.grace = DefaultGrace
	}
	if f.now == nil {
		f.now = time.Now
	}
	return f
}

var _ livelog.Follower = (*Follower)(nil)

// Poll follows the contract of livelog.Follower: no cursor starts at the end with the last lines; a cursor continues from its offset;
// one that does not fit the file (truncated, replaced, beyond the end) starts again at the end with Reset set.
func (f *Follower) Poll(ctx context.Context, cursor string, limit int) (livelog.Batch, error) {
	c, present, err := livelog.ParseCursor(cursor)
	if err != nil {
		return livelog.Batch{}, err
	}
	if err := ctx.Err(); err != nil {
		return livelog.Batch{}, err
	}
	limit = f.lim.Lines(limit)

	file, err := os.Open(f.path)
	if err != nil {
		return livelog.Batch{}, livelog.ErrNotFound
	}
	defer file.Close()
	st, err := file.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return livelog.Batch{}, livelog.ErrNotFound
	}
	size, id := st.Size(), identityOf(st)

	reset := false
	if present {
		ok, err := continues(file, c, id, size)
		if err != nil {
			return livelog.Batch{}, err
		}
		if !ok {
			present, reset = false, true
		}
	}
	if err := ctx.Err(); err != nil {
		return livelog.Batch{}, err
	}
	quiet := f.now().Sub(st.ModTime()) >= f.grace
	var b livelog.Batch
	if present {
		b, err = f.forward(file, size, id, c.Offset, limit, quiet)
	} else {
		b, err = f.fromEnd(file, size, id, limit, quiet)
	}
	b.Reset = reset
	return b, err
}

// ---- the cursor's identity: "<device.inode>|<head length>|<hash of the head>" ----

func hashHead(r io.ReaderAt, n int) (uint64, error) {
	buf := make([]byte, n)
	got, err := r.ReadAt(buf, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	h := fnv.New64a()
	h.Write(buf[:got])
	return h.Sum64(), nil
}

// cursorAt is the cursor for a continuation at offset: the head it vouches for is the file's first headBytes bytes (fewer while the
// file is shorter than that, so a young file is still recognised).
func cursorAt(file io.ReaderAt, id string, offset int64) (string, error) {
	n := int(min(offset, headBytes))
	sum, err := hashHead(file, n)
	if err != nil {
		return "", err
	}
	ident := id + "|" + strconv.Itoa(n) + "|" + strconv.FormatUint(sum, 16)
	return livelog.Cursor{Offset: offset, Identity: ident}.Encode(), nil
}

// continues says whether the cursor still describes this file: the same file, at least as long as the offset, and the same first bytes.
func continues(file io.ReaderAt, c livelog.Cursor, id string, size int64) (bool, error) {
	parts := strings.Split(c.Identity, "|")
	if len(parts) != 3 || parts[0] != id {
		return false, nil
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 0 || n > headBytes || int64(n) > c.Offset || c.Offset > size {
		return false, nil
	}
	want, err := strconv.ParseUint(parts[2], 16, 64)
	if err != nil {
		return false, nil
	}
	got, err := hashHead(file, n)
	if err != nil {
		return false, err
	}
	return got == want, nil
}

// ---- reading ----

// lineAt builds the Line for raw (a line's bytes without its ending) that starts at offset.
func (f *Follower) lineAt(offset int64, raw []byte) livelog.Line {
	raw = trimCR(raw)
	l := livelog.Line{Offset: offset, Bytes: len(raw)}
	if len(raw) > f.lim.MaxLineBytes {
		raw = trimPartialRune(raw[:f.lim.MaxLineBytes])
		l.Truncated = true
	}
	l.Text = sanitize(raw)
	return l
}

func trimCR(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\r' {
		return b[:n-1]
	}
	return b
}

func hasNUL(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// readAt reads up to n bytes at offset; a file that shrank under us gives what is there.
func readAt(file io.ReaderAt, offset int64, n int64) ([]byte, error) {
	if n <= 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	got, err := file.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("filetail: read: %w", err)
	}
	return buf[:got], nil
}

// fromEnd returns the last complete lines (at most limit) that fit in one batch of bytes, and a cursor at the end. A last line without a
// line ending is held back until the file has been quiet for the grace time.
func (f *Follower) fromEnd(file io.ReaderAt, size int64, id string, limit int, quiet bool) (livelog.Batch, error) {
	b := livelog.Batch{Lines: []livelog.Line{}, EOF: true}
	if size == 0 {
		cur, err := cursorAt(file, id, 0)
		b.Cursor = cur
		return b, err
	}
	pos := max(0, size-int64(f.lim.MaxBatchBytes))
	buf, err := readAt(file, pos, size-pos)
	if err != nil {
		return b, err
	}
	if hasNUL(buf) {
		return livelog.Batch{}, livelog.ErrNotText
	}
	end := len(buf) // where the lines we may return end
	if len(buf) > 0 && buf[len(buf)-1] != '\n' && !quiet {
		if i := lastNewline(buf); i >= 0 {
			end = i + 1
		} else {
			end = 0
		}
	}
	start := 0
	if pos > 0 { // the window starts in the middle of a line: its first part is not ours
		if i := indexByte(buf, '\n'); i >= 0 {
			start = i + 1
		} else if end == len(buf) {
			// one line longer than a whole batch: show its last part, flagged as cut
			l := f.lineAt(pos, buf)
			l.Truncated = true
			b.Lines = append(b.Lines, l)
			cur, err := cursorAt(file, id, size)
			b.Cursor = cur
			return b, err
		}
	}
	var lines []livelog.Line
	for s := start; s < end; {
		e := indexByte(buf[s:end], '\n')
		next := end
		raw := buf[s:end]
		if e >= 0 {
			raw, next = buf[s:s+e], s+e+1
		}
		lines = append(lines, f.lineAt(pos+int64(s), raw))
		s = next
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	if lines != nil {
		b.Lines = lines
	}
	cur, err := cursorAt(file, id, pos+int64(end))
	b.Cursor = cur
	return b, err
}

func lastNewline(b []byte) int {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == '\n' {
			return i
		}
	}
	return -1
}

// forward returns the lines appended after offset: at most limit lines and one batch of bytes, complete lines only (the last, without a
// line ending, once the file has been quiet for the grace time).
func (f *Follower) forward(file io.ReaderAt, size int64, id string, offset int64, limit int, quiet bool) (livelog.Batch, error) {
	b := livelog.Batch{Lines: []livelog.Line{}, EOF: true}
	if offset >= size {
		cur, err := cursorAt(file, id, offset)
		b.Cursor = cur
		return b, err
	}
	want := min(size-offset, int64(f.lim.MaxBatchBytes))
	buf, err := readAt(file, offset, want)
	if err != nil {
		return b, err
	}
	if hasNUL(buf) {
		return livelog.Batch{}, livelog.ErrNotText
	}
	reachesEnd := offset+int64(len(buf)) >= size
	consumed := 0
	more := !reachesEnd // the batch did not hold everything that is there
	for consumed < len(buf) && len(b.Lines) < limit {
		i := indexByte(buf[consumed:], '\n')
		if i < 0 {
			// the rest has no line ending
			switch {
			case reachesEnd && quiet:
				// the file's last line, quiet long enough: it is a line
				b.Lines = append(b.Lines, f.lineAt(offset+int64(consumed), buf[consumed:]))
				consumed = len(buf)
			case !reachesEnd && consumed == 0:
				// one line longer than a whole batch: delivered in pieces, each flagged as cut
				l := f.lineAt(offset, buf)
				l.Truncated = true
				b.Lines = append(b.Lines, l)
				consumed = len(buf)
			}
			break
		}
		b.Lines = append(b.Lines, f.lineAt(offset+int64(consumed), buf[consumed:consumed+i]))
		consumed += i + 1
	}
	if consumed < len(buf) && len(b.Lines) >= limit && (indexByte(buf[consumed:], '\n') >= 0 || (reachesEnd && quiet)) {
		more = true // lines are still waiting in this batch (the last one, without a line ending, once the file is quiet)
	}
	b.EOF = !more
	cur, err := cursorAt(file, id, offset+int64(consumed))
	b.Cursor = cur
	return b, err
}
