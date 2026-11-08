package filereader

import (
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
	if err := ctx.Err(); err != nil {
		return nil, err // a request already cancelled never takes a place
	}
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

	c, status := e.cursorAt(ctx, f, ix, from)
	switch status {
	case atEnd:
		return Window{From: from, Lines: []Line{}, NextFrom: from}, nil
	case notIndexed:
		return Window{}, ErrNotIndexed
	}
	if err := c.skipErr(); err != nil {
		return Window{}, err
	}
	return e.collect(c, from, count)
}

type startStatus int

const (
	positioned startStatus = iota
	atEnd
	notIndexed
)

// cursorAt places a cursor at the checkpoint at or before line `from`, with a guard for the read budget and
// the request's own context. The cursor still has to skip forward to `from` (see skipErr).
func (e *Engine) cursorAt(ctx context.Context, f *os.File, ix *index, from int64) (*cursor, startStatus) {
	ix.mu.Lock()
	offset, line, ok := ix.startOf(from, e.lim.CheckpointEvery)
	complete, total := ix.complete, ix.totalLines()
	longs := ix.longs
	ix.mu.Unlock()
	if !ok {
		if complete && from > total {
			return nil, atEnd
		}
		return nil, notIndexed
	}
	c := newCursor(f, ix.size, offset, line, longs, e.newGuard(ctx))
	c.target = from
	return c, positioned
}

// skipErr moves the cursor up to its target line. A file that ends first leaves the cursor at the end, which
// the callers see as "nothing to read".
func (c *cursor) skipErr() error {
	_, err := c.skipTo(c.target)
	return err
}

// collect reads up to count lines from the cursor (already at line `from`), honouring the response size cap.
func (e *Engine) collect(c *cursor, from int64, count int) (Window, error) {
	w := Window{From: from, Lines: make([]Line, 0, min(count, 64))}
	if c.line != from {
		// the file ended before `from`
		w.Lines = []Line{}
		w.NextFrom = from
		return w, nil
	}
	var scratch []byte
	used := 0
	for len(w.Lines) < count {
		if err := c.g.check(); err != nil {
			return Window{}, err
		}
		lr, err := c.next(e.lim.MaxLineBytes, scratch)
		if err != nil {
			return Window{}, err
		}
		scratch = lr.text
		if !lr.ok {
			w.NextFrom = c.line
			return w, nil
		}
		raw := lr.text
		truncated := lr.total > len(raw)
		if truncated {
			raw = trimPartialRune(raw)
		}
		text := sanitize(raw)
		n := c.line - 1 // c.next has already moved on
		// the cap ends a window early, but never before it holds one line (so a window always moves on)
		if len(w.Lines) > 0 && used+len(text) > e.lim.MaxResponseBytes {
			// give the line back: it is the first of the next window
			w.NextFrom, w.HasMore = n, true
			return w, nil
		}
		used += len(text)
		w.Lines = append(w.Lines, Line{N: n, Text: text, Bytes: lr.total, Truncated: truncated})
	}
	w.NextFrom = c.line
	w.HasMore = c.more()
	return w, nil
}
