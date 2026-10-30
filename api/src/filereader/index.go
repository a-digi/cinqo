package filereader

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
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
			ix.scan(buf[:n], lim.CheckpointEvery)
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
	}
	return nil
}

func (ix *index) scan(chunk []byte, every int) {
	pos := 0
	for {
		i := bytes.IndexByte(chunk[pos:], '\n')
		if i < 0 {
			break
		}
		pos += i + 1
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
