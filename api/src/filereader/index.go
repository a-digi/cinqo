package filereader

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"sync"
	"time"
)

// index is what is known about one file so far: where every CheckpointEvery-th line starts, so a jump to
// line N costs a seek and at most CheckpointEvery-1 lines of reading. It is built by streaming over the
// file in slices of time (advance), so a request never waits for a whole file to be scanned, and it is only
// valid while the file's size and modification time stay the same.
type index struct {
	mu sync.Mutex

	size    int64
	modTime time.Time

	// checkpoints[k] is the byte offset where line k*every+1 starts; checkpoints[0] is always 0.
	checkpoints []int64
	// scanned is how many bytes have been looked at; newlines how many "\n" were among them.
	scanned  int64
	newlines int64
	lastByte byte
	complete bool

	// lineStart is the byte offset where the line being scanned began; longs lists, in order, every line
	// scanned so far that is longer than LongLineBytes (append-only: readers may use a prefix of it).
	lineStart int64
	longs     []longLine
}

// longLine is a line longer than LongLineBytes: its number, where it starts and how many bytes it holds
// (up to, not including, its "\n"; a "\r" before it counts).
type longLine struct {
	n      int64
	start  int64
	length int64
}

// longAt finds the long-line record for line n in a prefix of the table (sorted by line number).
func longAt(longs []longLine, n int64) (longLine, bool) {
	i := sort.Search(len(longs), func(i int) bool { return longs[i].n >= n })
	if i < len(longs) && longs[i].n == n {
		return longs[i], true
	}
	return longLine{}, false
}

func newIndex(size int64, modTime time.Time) *index {
	return &index{size: size, modTime: modTime, checkpoints: []int64{0}}
}

func (ix *index) matches(size int64, modTime time.Time) bool {
	return ix.size == size && ix.modTime.Equal(modTime)
}

// totalLines is exact once the scan is complete and a lower bound before that. A last line without a
// newline counts.
func (ix *index) totalLines() int64 {
	n := ix.newlines
	if ix.complete && ix.size > 0 && ix.lastByte != '\n' {
		n++
	}
	return n
}

// advance scans forward until the slice of time is used up, the file is done or ctx ends. The caller holds
// ix.mu.
func (ix *index) advance(ctx context.Context, f *os.File, lim Limits) error {
	if ix.complete {
		return nil
	}
	deadline := time.Now().Add(lim.IndexSlice)
	buf := make([]byte, lim.IndexChunkBytes)
	for ix.scanned < ix.size {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := f.ReadAt(buf, ix.scanned)
		if n > 0 {
			ix.scan(buf[:n], lim.CheckpointEvery, int64(lim.LongLineBytes))
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			break
		}
	}
	if ix.scanned >= ix.size {
		ix.complete = true
		// a last line with no newline is a line too, and may be the long one
		if ix.size > ix.lineStart && ix.size-ix.lineStart > int64(lim.LongLineBytes) {
			ix.longs = append(ix.longs, longLine{n: ix.newlines + 1, start: ix.lineStart, length: ix.size - ix.lineStart})
		}
	}
	return nil
}

func (ix *index) scan(chunk []byte, every int, longMin int64) {
	pos := 0
	for {
		i := bytes.IndexByte(chunk[pos:], '\n')
		if i < 0 {
			break
		}
		pos += i + 1
		end := ix.scanned + int64(pos) - 1 // where this line's "\n" is
		if end-ix.lineStart > longMin {
			ix.longs = append(ix.longs, longLine{n: ix.newlines + 1, start: ix.lineStart, length: end - ix.lineStart})
		}
		ix.lineStart = end + 1
		ix.newlines++
		if ix.newlines%int64(every) == 0 {
			// the line after this newline is line newlines+1, the start of the next block
			ix.checkpoints = append(ix.checkpoints, ix.scanned+int64(pos))
		}
	}
	ix.scanned += int64(len(chunk))
	ix.lastByte = chunk[len(chunk)-1]
}

// startOf returns the byte offset of the checkpoint at or before line `from` (1-based) and the line number
// that checkpoint starts at.
func (ix *index) startOf(from int64, every int) (offset int64, line int64, ok bool) {
	k := (from - 1) / int64(every)
	if k >= int64(len(ix.checkpoints)) {
		return 0, 0, false
	}
	return ix.checkpoints[k], k*int64(every) + 1, true
}
