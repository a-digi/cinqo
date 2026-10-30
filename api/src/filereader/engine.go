package filereader

import (
	"bufio"
	"container/list"
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

// Line is one line of a window. N is its 1-based line number in the file.
type Line struct {
	N    int64  `json:"n"`
	Text string `json:"text"`
	// Bytes is the line's real length; Text is shorter when Truncated.
	Bytes     int  `json:"bytes"`
	Truncated bool `json:"truncated,omitempty"`
}

// Window is a run of consecutive lines. NextFrom is the line to ask for next; HasMore is false once the
// end of the file was reached.
type Window struct {
	From     int64  `json:"from"`
	Lines    []Line `json:"lines"`
	NextFrom int64  `json:"nextFrom"`
	HasMore  bool   `json:"hasMore"`
}

// Info describes a file and how far its index has got. TotalLines is exact when Complete, otherwise a
// lower bound that grows with every call.
type Info struct {
	SizeBytes    int64     `json:"sizeBytes"`
	ModifiedAt   time.Time `json:"modifiedAt"`
	ScannedBytes int64     `json:"scannedBytes"`
	TotalLines   int64     `json:"totalLines"`
	Complete     bool      `json:"complete"`
}

// Engine reads windows of lines from files. It is safe for concurrent use; one Engine is shared by the
// whole process so its limits (concurrent reads, cached indexes) are global.
type Engine struct {
	lim Limits
	sem chan struct{}

	mu    sync.Mutex
	cache map[string]*list.Element
	lru   *list.List
}

type cacheEntry struct {
	path string
	ix   *index
}

func New(lim Limits) *Engine {
	return &Engine{
		lim:   lim,
		sem:   make(chan struct{}, lim.MaxConcurrent),
		cache: map[string]*list.Element{},
		lru:   list.New(),
	}
}

func (e *Engine) acquire(ctx context.Context) (release func(), err error) {
	timer := time.NewTimer(e.lim.MaxWait)
	defer timer.Stop()
	select {
	case e.sem <- struct{}{}:
		return func() { <-e.sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, ErrBusy
	}
}

// open stats and opens the file, refusing anything that is not a regular, text file of an allowed size, and
// returns the index for its current size and modification time (a fresh one when the file changed).
func (e *Engine) open(path string) (*os.File, *index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, nil, ErrNotRegular
	}
	if st.Size() > e.lim.MaxFileBytes {
		f.Close()
		return nil, nil, ErrTooLarge
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if el, ok := e.cache[path]; ok {
		ent := el.Value.(*cacheEntry)
		if ent.ix.matches(st.Size(), st.ModTime()) {
			e.lru.MoveToFront(el)
			return f, ent.ix, nil
		}
		e.lru.Remove(el)
		delete(e.cache, path)
	}
	// a new (or changed) file: refuse binary content once, before anything is indexed
	if err := checkText(f, st.Size()); err != nil {
		f.Close()
		return nil, nil, err
	}
	ix := newIndex(st.Size(), st.ModTime())
	e.cache[path] = e.lru.PushFront(&cacheEntry{path: path, ix: ix})
	for e.lru.Len() > e.lim.MaxCachedFiles {
		oldest := e.lru.Back()
		e.lru.Remove(oldest)
		delete(e.cache, oldest.Value.(*cacheEntry).path)
	}
	return f, ix, nil
}

const binarySniffBytes = 8 << 10

func checkText(f *os.File, size int64) error {
	n := int64(binarySniffBytes)
	if size < n {
		n = size
	}
	head := make([]byte, n)
	if _, err := f.ReadAt(head, 0); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	for _, b := range head {
		if b == 0 {
			return ErrBinary
		}
	}
	return nil
}

// Info reports the file's size and how far the index has got, and advances the index by one slice of time
// (IndexSlice), so calling it repeatedly walks the whole file without any single call taking long.
func (e *Engine) Info(ctx context.Context, path string) (Info, error) {
	release, err := e.acquire(ctx)
	if err != nil {
		return Info{}, err
	}
	defer release()

	f, ix, err := e.open(path)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()

	ix.mu.Lock()
	defer ix.mu.Unlock()
	if err := ix.advance(ctx, f, e.lim); err != nil {
		return Info{}, err
	}
	return Info{
		SizeBytes:    ix.size,
		ModifiedAt:   ix.modTime,
		ScannedBytes: ix.scanned,
		TotalLines:   ix.totalLines(),
		Complete:     ix.complete,
	}, nil
}

// ReadLines returns up to count lines starting at line from (both 1-based; count is clamped to MaxLines).
// The first lines of a file need no index; further in it needs a checkpoint, and answers ErrNotIndexed when
// the index has not reached that far yet. A from past the end of a fully indexed file is an empty window.
func (e *Engine) ReadLines(ctx context.Context, path string, from int64, count int) (Window, error) {
	if from < 1 || count < 1 {
		return Window{}, ErrBadRange
	}
	if count > e.lim.MaxLines {
		count = e.lim.MaxLines
	}
	release, err := e.acquire(ctx)
	if err != nil {
		return Window{}, err
	}
	defer release()

	f, ix, err := e.open(path)
	if err != nil {
		return Window{}, err
	}
	defer f.Close()

	ix.mu.Lock()
	offset, line, ok := ix.startOf(from, e.lim.CheckpointEvery)
	complete, total := ix.complete, ix.totalLines()
	ix.mu.Unlock()
	if !ok {
		if complete && from > total {
			return Window{From: from, Lines: []Line{}, NextFrom: from}, nil
		}
		return Window{}, ErrNotIndexed
	}

	r := bufio.NewReaderSize(io.NewSectionReader(f, offset, ix.size-offset), 64<<10)
	return e.collect(ctx, r, line, from, count)
}

// collect skips lines until `from`, then reads up to count lines, honouring the response size cap.
func (e *Engine) collect(ctx context.Context, r *bufio.Reader, line, from int64, count int) (Window, error) {
	ok, err := skipLines(ctx, r, line, from)
	if err != nil {
		return Window{}, err
	}
	if !ok {
		return Window{From: from, Lines: []Line{}, NextFrom: from}, nil
	}
	line = from

	w := Window{From: from, Lines: make([]Line, 0, min(count, 64))}
	var scratch []byte
	used := 0
	for len(w.Lines) < count {
		if err := ctx.Err(); err != nil {
			return Window{}, err
		}
		raw, total, ok, err := readLine(r, e.lim.MaxLineBytes, scratch)
		if err != nil {
			return Window{}, err
		}
		scratch = raw
		if !ok {
			w.NextFrom = line
			return w, nil
		}
		truncated := total > len(raw)
		if truncated {
			raw = trimPartialRune(raw)
		}
		text := sanitize(raw)
		// the cap ends a window early, but never before it holds one line (so a window always moves on)
		if len(w.Lines) > 0 && used+len(text) > e.lim.MaxResponseBytes {
			w.NextFrom, w.HasMore = line, true
			return w, nil
		}
		used += len(text)
		w.Lines = append(w.Lines, Line{N: line, Text: text, Bytes: total, Truncated: truncated})
		line++
	}
	w.NextFrom = line
	_, peekErr := r.Peek(1)
	w.HasMore = peekErr == nil
	return w, nil
}

// skipLines reads and drops the lines before `from` (at most CheckpointEvery-1 of them), keeping none of
// their text. It reports false when the file ends first.
func skipLines(ctx context.Context, r *bufio.Reader, line, from int64) (bool, error) {
	for ; line < from; line++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		_, _, ok, err := readLine(r, 0, nil)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
