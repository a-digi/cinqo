package filereader

import (
	"context"
	"io"
	"unicode/utf8"
)

// Segment is one piece of a single line, read by byte offset: how a line of any length (a whole file that is one
// line, say) is read past the first MaxLineBytes a window shows. Offset is where the text really starts in the
// line (moved forward past the middle of a character); NextOffset is where the following segment starts, always
// on a character boundary, so asking for NextOffset next never splits one and never skips a byte.
type Segment struct {
	Line       int64  `json:"line"`
	Offset     int64  `json:"offset"`
	Text       string `json:"text"`
	NextOffset int64  `json:"nextOffset"`
	// LineBytes is the line's whole length in bytes, without its line ending.
	LineBytes int64 `json:"lineBytes"`
	HasMore   bool  `json:"hasMore"`
}

// ReadSegment returns up to length bytes (clamped to MaxSegmentBytes) of line `line`, starting at byte `offset`
// of that line. The line must be reachable: an index that has not got that far answers ErrNotIndexed. A line the
// index knows to be long is read directly at its known position, never read through from its start; the read is
// bounded by ReadBudget and the request's context. An offset at or past the end of the line is an empty segment.
func (e *Engine) ReadSegment(ctx context.Context, path string, line, offset int64, length int) (Segment, error) {
	if line < 1 || offset < 0 || length < 1 {
		return Segment{}, ErrBadRange
	}
	if length > e.lim.MaxSegmentBytes {
		length = e.lim.MaxSegmentBytes
	}
	release, err := e.acquire(ctx)
	if err != nil {
		return Segment{}, err
	}
	defer release()

	f, ix, err := e.open(path)
	if err != nil {
		return Segment{}, err
	}
	defer f.Close()

	c, status := e.cursorAt(ctx, f, ix, line)
	switch status {
	case atEnd:
		return Segment{Line: line, Offset: offset, NextOffset: offset}, nil
	case notIndexed:
		return Segment{}, ErrNotIndexed
	}
	if err := c.skipErr(); err != nil {
		return Segment{}, err
	}
	if c.line != line {
		return Segment{Line: line, Offset: offset, NextOffset: offset}, nil
	}

	// where the line starts and how long it is: the index knows for a long line, otherwise it is read through
	// (a line short enough not to be recorded, so cheap) without keeping any of it
	start := c.pos
	lr, err := c.next(0, nil)
	if err != nil {
		return Segment{}, err
	}
	if !lr.ok {
		return Segment{Line: line, Offset: offset, NextOffset: offset}, nil
	}
	lineBytes := int64(lr.total)

	seg := Segment{Line: line, Offset: offset, NextOffset: offset, LineBytes: lineBytes}
	if offset >= lineBytes {
		return seg, nil
	}

	// a few bytes more than asked for, to find the character boundaries at both ends
	want := min(int64(length)+utf8.UTFMax, lineBytes-offset)
	buf := make([]byte, want)
	n, err := f.ReadAt(buf, start+offset)
	if err != nil && err != io.EOF {
		return Segment{}, err
	}
	buf = buf[:n]

	// begin on a character boundary: step over the continuation bytes of one cut in half
	skipped := 0
	for skipped < utf8.UTFMax-1 && skipped < len(buf) && !utf8.RuneStart(buf[skipped]) {
		skipped++
	}
	buf = buf[skipped:]
	seg.Offset = offset + int64(skipped)

	end := min(int64(len(buf)), int64(length))
	text := buf[:end]
	if seg.Offset+end < lineBytes {
		// more follows: end on a character boundary too, and hand the cut bytes to the next segment
		text = trimPartialRune(text)
	}
	seg.NextOffset = seg.Offset + int64(len(text))
	seg.HasMore = seg.NextOffset < lineBytes
	seg.Text = sanitize(text)
	return seg, nil
}
