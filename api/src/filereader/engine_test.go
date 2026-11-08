package filereader

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// numbered writes "L1".."Ln", one per line.
func numbered(t *testing.T, n int) string {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "L%d\n", i)
	}
	return writeFile(t, b.String())
}

func smallLimits() Limits {
	l := DefaultLimits()
	l.IndexChunkBytes = 4096
	l.CheckpointEvery = 100
	l.SearchChunkBytes = 1024
	return l
}

// indexFully calls Info until the index is complete.
func indexFully(t *testing.T, e *Engine, path string) Info {
	t.Helper()
	for i := 0; i < 100000; i++ {
		info, err := e.Info(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Complete {
			return info
		}
	}
	t.Fatal("index never completed")
	return Info{}
}

func TestWindowsAndJumps(t *testing.T) {
	p := numbered(t, 2500)
	e := New(smallLimits())
	info := indexFully(t, e, p)
	if info.TotalLines != 2500 {
		t.Fatalf("total = %d", info.TotalLines)
	}

	w, err := e.ReadLines(context.Background(), p, 1, 500)
	if err != nil || len(w.Lines) != 500 || w.Lines[0].Text != "L1" || w.Lines[499].Text != "L500" || !w.HasMore || w.NextFrom != 501 {
		t.Fatalf("first window: %v %+v", err, w)
	}
	w, _ = e.ReadLines(context.Background(), p, 2491, 500)
	if len(w.Lines) != 10 || w.Lines[9].Text != "L2500" || w.HasMore || w.NextFrom != 2501 {
		t.Fatalf("last window: %+v", w)
	}
	w, err = e.ReadLines(context.Background(), p, 9999, 10)
	if err != nil || len(w.Lines) != 0 || w.HasMore {
		t.Fatalf("past the end: %v %+v", err, w)
	}
	// count is clamped to MaxLines
	w, _ = e.ReadLines(context.Background(), p, 1, 100000)
	if len(w.Lines) != 500 {
		t.Fatalf("clamp: %d", len(w.Lines))
	}
}

func TestRandomJumpsLandOnTheRightLine(t *testing.T) {
	p := numbered(t, 5000)
	e := New(smallLimits())
	indexFully(t, e, p)
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 300; i++ {
		from := int64(rng.Intn(5000) + 1)
		w, err := e.ReadLines(context.Background(), p, from, 3)
		if err != nil || len(w.Lines) == 0 {
			t.Fatalf("from %d: %v %+v", from, err, w)
		}
		if want := fmt.Sprintf("L%d", from); w.Lines[0].Text != want || w.Lines[0].N != from {
			t.Fatalf("from %d got %+v", from, w.Lines[0])
		}
	}
}

func TestIndexIsProgressive(t *testing.T) {
	p := numbered(t, 20000) // ~110 KB: many 4 KB chunks
	lim := smallLimits()
	lim.IndexSlice = time.Nanosecond // one chunk per Info call
	e := New(lim)

	if _, err := e.ReadLines(context.Background(), p, 15000, 1); !errors.Is(err, ErrNotIndexed) {
		t.Fatalf("want ErrNotIndexed before indexing, got %v", err)
	}
	// the first page needs no index at all
	if w, err := e.ReadLines(context.Background(), p, 1, 2); err != nil || len(w.Lines) != 2 {
		t.Fatalf("first page: %v", err)
	}
	first, _ := e.Info(context.Background(), p)
	if first.Complete || first.ScannedBytes == 0 || first.ScannedBytes > 2*4096 {
		t.Fatalf("first slice should cover about one chunk: %+v", first)
	}
	second, _ := e.Info(context.Background(), p)
	if second.ScannedBytes <= first.ScannedBytes || second.TotalLines < first.TotalLines {
		t.Fatalf("index did not advance: %+v -> %+v", first, second)
	}
	done := indexFully(t, e, p)
	if done.TotalLines != 20000 || done.ScannedBytes != done.SizeBytes {
		t.Fatalf("complete: %+v", done)
	}
}

func TestLineEndingsAndEdges(t *testing.T) {
	e := New(smallLimits())
	ctx := context.Background()

	p := writeFile(t, "a\r\nb\r\n\r\nlast")
	info := indexFully(t, e, p)
	if info.TotalLines != 4 {
		t.Fatalf("total = %d", info.TotalLines)
	}
	w, _ := e.ReadLines(ctx, p, 1, 10)
	got := []string{}
	for _, l := range w.Lines {
		got = append(got, l.Text)
	}
	if strings.Join(got, "|") != "a|b||last" || w.HasMore {
		t.Fatalf("lines = %q more=%v", got, w.HasMore)
	}

	empty := writeFile(t, "")
	if info := indexFully(t, e, empty); info.TotalLines != 0 {
		t.Fatalf("empty total = %d", info.TotalLines)
	}
	if w, err := e.ReadLines(ctx, empty, 1, 5); err != nil || len(w.Lines) != 0 {
		t.Fatalf("empty file: %v %+v", err, w)
	}

	trailing := writeFile(t, "x\ny\n")
	if info := indexFully(t, e, trailing); info.TotalLines != 2 {
		t.Fatalf("trailing newline total = %d", info.TotalLines)
	}
}

func TestLongLinesAreCutAndCounted(t *testing.T) {
	lim := smallLimits()
	e := New(lim)
	// 40000 two-byte characters: a cut in the middle of a character must not leave a broken one
	long := strings.Repeat("é", 40000)
	p := writeFile(t, "short\n"+long+"\nafter\n")
	w, err := e.ReadLines(context.Background(), p, 1, 10)
	if err != nil || len(w.Lines) != 3 {
		t.Fatalf("%v %+v", err, w)
	}
	l := w.Lines[1]
	if !l.Truncated || l.Bytes != 80000 || len(l.Text) > lim.MaxLineBytes || strings.ContainsRune(l.Text, '�') {
		t.Fatalf("long line: truncated=%v bytes=%d text=%d", l.Truncated, l.Bytes, len(l.Text))
	}
	if w.Lines[2].Text != "after" {
		t.Fatalf("the line after a long one is wrong: %q", w.Lines[2].Text)
	}
}

func TestResponseByteCapMovesOnWithoutGaps(t *testing.T) {
	lim := smallLimits()
	lim.MaxResponseBytes = 100 << 10
	e := New(lim)
	var b strings.Builder
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&b, "%d %s\n", i, strings.Repeat("x", 10000))
	}
	p := writeFile(t, b.String())
	var next int64 = 1
	seen := 0
	for next != 0 {
		w, err := e.ReadLines(context.Background(), p, next, 500)
		if err != nil {
			if errors.Is(err, ErrNotIndexed) {
				indexFully(t, e, p)
				continue
			}
			t.Fatal(err)
		}
		if len(w.Lines) == 0 || len(w.Lines) > 11 {
			t.Fatalf("a capped window holds ~10 lines, got %d", len(w.Lines))
		}
		if w.Lines[0].N != next {
			t.Fatalf("gap or overlap: asked %d, got %d", next, w.Lines[0].N)
		}
		seen += len(w.Lines)
		if !w.HasMore {
			break
		}
		next = w.NextFrom
	}
	if seen != 300 {
		t.Fatalf("saw %d lines", seen)
	}
}

func TestSanitize(t *testing.T) {
	e := New(smallLimits())
	p := writeFile(t, "ok\tTab\n\x1b[31mred\x1b[0m\nbad\xff\xfebytes\n")
	w, _ := e.ReadLines(context.Background(), p, 1, 5)
	if w.Lines[0].Text != "ok\tTab" {
		t.Fatalf("tab: %q", w.Lines[0].Text)
	}
	if strings.ContainsRune(w.Lines[1].Text, 0x1b) || !strings.Contains(w.Lines[1].Text, "␛") {
		t.Fatalf("escape not made visible: %q", w.Lines[1].Text)
	}
	if !strings.Contains(w.Lines[2].Text, "�") {
		t.Fatalf("invalid utf-8 not replaced: %q", w.Lines[2].Text)
	}
}

func TestRefusals(t *testing.T) {
	ctx := context.Background()
	e := New(smallLimits())

	if _, err := e.ReadLines(ctx, writeFile(t, "a\x00b\n"), 1, 1); !errors.Is(err, ErrBinary) {
		t.Fatalf("binary: %v", err)
	}
	if _, err := e.Info(ctx, t.TempDir()); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("directory: %v", err)
	}
	if _, err := e.Info(ctx, filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	small := smallLimits()
	small.MaxFileBytes = 10
	if _, err := New(small).Info(ctx, writeFile(t, strings.Repeat("a\n", 100))); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too large: %v", err)
	}
	p := numbered(t, 10)
	if _, err := e.ReadLines(ctx, p, 0, 5); !errors.Is(err, ErrBadRange) {
		t.Fatalf("from 0: %v", err)
	}
	if _, err := e.ReadLines(ctx, p, 1, 0); !errors.Is(err, ErrBadRange) {
		t.Fatalf("count 0: %v", err)
	}
}

func TestChangedFileIsReindexed(t *testing.T) {
	e := New(smallLimits())
	p := numbered(t, 300)
	if info := indexFully(t, e, p); info.TotalLines != 300 {
		t.Fatal(info.TotalLines)
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	fmt.Fprint(f, "extra1\nextra2\n")
	f.Close()
	info, _ := e.Info(context.Background(), p)
	if info.Complete && info.TotalLines == 300 {
		t.Fatal("stale index served for a grown file")
	}
	if info := indexFully(t, e, p); info.TotalLines != 302 {
		t.Fatalf("total after growth = %d", info.TotalLines)
	}
	w, _ := e.ReadLines(context.Background(), p, 301, 5)
	if len(w.Lines) != 2 || w.Lines[1].Text != "extra2" {
		t.Fatalf("%+v", w)
	}
}

func TestBusyAndCancelled(t *testing.T) {
	lim := smallLimits()
	lim.MaxConcurrent = 1
	lim.MaxWait = 30 * time.Millisecond
	e := New(lim)
	p := numbered(t, 10)

	release, err := e.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ReadLines(context.Background(), p, 1, 5); !errors.Is(err, ErrBusy) {
		t.Fatalf("want ErrBusy, got %v", err)
	}
	release()
	if _, err := e.ReadLines(context.Background(), p, 1, 5); err != nil {
		t.Fatalf("after release: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.ReadLines(ctx, p, 1, 5); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestCacheIsBounded(t *testing.T) {
	lim := smallLimits()
	lim.MaxCachedFiles = 3
	e := New(lim)
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		p := filepath.Join(dir, fmt.Sprintf("f%d.txt", i))
		os.WriteFile(p, []byte("a\nb\n"), 0o600)
		if _, err := e.Info(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	if len(e.cache) != 3 || e.lru.Len() != 3 {
		t.Fatalf("cache holds %d/%d entries", len(e.cache), e.lru.Len())
	}
}

// writeBig writes about `mb` MiB of 100-byte lines.
func writeBig(t *testing.T, mb int) (path string, lines int) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "big.txt")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	row := strings.Repeat("x", 90)
	n := mb << 20 / 100
	for i := 1; i <= n; i++ {
		fmt.Fprintf(w, "%08d %s\n", i, row) // 9 + 90 + 1 = 100 bytes
	}
	w.Flush()
	f.Close()
	return p, n
}

// A window deep inside a big file costs a bounded amount of memory, whatever the file's size.
func TestMemoryStaysFlat(t *testing.T) {
	p, n := writeBig(t, 64)
	e := New(DefaultLimits())
	info := indexFully(t, e, p)
	if info.TotalLines != int64(n) {
		t.Fatalf("total %d want %d", info.TotalLines, n)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	w, err := e.ReadLines(context.Background(), p, int64(n/2), 500)
	if err != nil || len(w.Lines) != 500 || w.Lines[0].N != int64(n/2) {
		t.Fatalf("%v %d", err, len(w.Lines))
	}
	runtime.ReadMemStats(&after)
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 4<<20 {
		t.Fatalf("one window allocated %d bytes", alloc)
	}
	if got := len(e.cache[p].Value.(*cacheEntry).ix.checkpoints); got > n/1000+2 {
		t.Fatalf("index has %d checkpoints for %d lines", got, n)
	}
}

// A heavy run for by-hand use: FILEREADER_STRESS=1 go test -run Stress -v ./src/filereader
func TestStress1GiB(t *testing.T) {
	if os.Getenv("FILEREADER_STRESS") == "" {
		t.Skip("set FILEREADER_STRESS=1 to run")
	}
	p, n := writeBig(t, 1000)
	e := New(DefaultLimits())
	start := time.Now()
	info := indexFully(t, e, p)
	t.Logf("indexed %d lines (%d MiB) in %s", info.TotalLines, info.SizeBytes>>20, time.Since(start))
	if info.TotalLines != int64(n) {
		t.Fatalf("total %d want %d", info.TotalLines, n)
	}
	start = time.Now()
	w, err := e.ReadLines(context.Background(), p, int64(n-100), 500)
	if err != nil || len(w.Lines) != 101 {
		t.Fatalf("%v %d", err, len(w.Lines))
	}
	t.Logf("window near the end in %s", time.Since(start))
}

func totalAlloc() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.TotalAlloc
}
