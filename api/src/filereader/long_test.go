package filereader

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// longLimits makes "long" small so the tests stay fast: a window keeps 300 bytes of a line, a line over 1,000
// bytes is long, a segment holds up to 300 bytes.
func longLimits() Limits {
	l := smallLimits()
	l.MaxLineBytes = 300
	l.LongLineBytes = 1000
	l.MaxSegmentBytes = 300
	return l
}

// mixed builds a file of short, medium and long lines (some with CRLF, some with multi-byte characters, the last
// without a line ending) and returns it with the lines as they should read (line endings removed).
func mixed(t *testing.T, seed int64, n int) (path string, lines []string) {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	var b strings.Builder
	for i := 1; i <= n; i++ {
		var line string
		switch rng.Intn(10) {
		case 0:
			line = strings.Repeat("léon-", 200+rng.Intn(800)) // long, multi-byte
		case 1:
			line = strings.Repeat("m", 301+rng.Intn(700)) // longer than a window keeps, not "long"
		case 2:
			line = ""
		default:
			line = fmt.Sprintf("line %d %s", i, strings.Repeat("s", rng.Intn(80)))
		}
		lines = append(lines, line)
		b.WriteString(line)
		if i < n {
			if rng.Intn(4) == 0 {
				b.WriteString("\r\n")
			} else {
				b.WriteString("\n")
			}
		}
	}
	return writeFile(t, b.String()), lines
}

func TestIndexRecordsLongLines(t *testing.T) {
	lim := longLimits()
	e := New(lim)
	long := strings.Repeat("a", 1500)
	p := writeFile(t, "short\n"+long+"\r\nshort2\n"+strings.Repeat("b", 2500)) // the last one has no newline
	indexFully(t, e, p)
	ix := e.cache[p].Value.(*cacheEntry).ix
	if len(ix.longs) != 2 {
		t.Fatalf("longs = %+v", ix.longs)
	}
	if l := ix.longs[0]; l.n != 2 || l.start != 6 || l.length != 1501 { // the "\r" is before the "\n": part of the length
		t.Fatalf("first: %+v", l)
	}
	if l := ix.longs[1]; l.n != 4 || l.length != 2500 {
		t.Fatalf("last line without a newline: %+v", l)
	}
}

func TestWindowsMatchTheFileWhateverTheLineLengths(t *testing.T) {
	lim := longLimits()
	for _, indexed := range []bool{false, true} {
		for seed := int64(1); seed <= 3; seed++ {
			p, lines := mixed(t, seed, 700)
			e := New(lim)
			maxFrom := 700
			if indexed {
				indexFully(t, e, p)
			} else {
				maxFrom = lim.CheckpointEvery // no index yet: only the first block can be reached
			}
			rng := rand.New(rand.NewSource(seed))
			for i := 0; i < 200; i++ {
				from := 1 + rng.Intn(maxFrom)
				w, err := e.ReadLines(context.Background(), p, int64(from), 1+rng.Intn(60))
				if err != nil {
					t.Fatalf("indexed=%v seed=%d from=%d: %v", indexed, seed, from, err)
				}
				for j, l := range w.Lines {
					want := lines[from-1+j]
					if l.N != int64(from+j) || l.Bytes != len(want) {
						t.Fatalf("indexed=%v seed=%d line %d: n=%d bytes=%d want bytes %d", indexed, seed, from+j, l.N, l.Bytes, len(want))
					}
					if l.Truncated != (len(want) > lim.MaxLineBytes) {
						t.Fatalf("line %d: truncated=%v for %d bytes", l.N, l.Truncated, len(want))
					}
					if !l.Truncated && l.Text != want {
						t.Fatalf("line %d: text differs", l.N)
					}
					if l.Truncated && !strings.HasPrefix(want, l.Text) {
						t.Fatalf("line %d: a cut line must be a prefix of the real one", l.N)
					}
				}
			}
		}
	}
}

// segments follows NextOffset from byte 0 and returns every piece, checking each is valid text on character
// boundaries and that nothing is skipped or repeated.
func segments(t *testing.T, e *Engine, path string, line int64, size int) []Segment {
	t.Helper()
	var out []Segment
	off := int64(0)
	for i := 0; i < 1_000_000; i++ {
		seg, err := e.ReadSegment(context.Background(), path, line, off, size)
		if err != nil {
			t.Fatal(err)
		}
		if seg.Offset != off {
			t.Fatalf("segment %d starts at %d, asked for %d (a boundary must never move on its own)", i, seg.Offset, off)
		}
		if !utf8.ValidString(seg.Text) || len(seg.Text) > size {
			t.Fatalf("segment %d: valid=%v len=%d", i, utf8.ValidString(seg.Text), len(seg.Text))
		}
		out = append(out, seg)
		if !seg.HasMore {
			return out
		}
		if seg.NextOffset <= off {
			t.Fatalf("segment %d did not move on", i)
		}
		off = seg.NextOffset
	}
	t.Fatal("never ended")
	return nil
}

func TestSegmentsReassembleEveryLine(t *testing.T) {
	lim := longLimits()
	p, lines := mixed(t, 7, 300)
	e := New(lim)
	indexFully(t, e, p)
	for i, want := range lines {
		segs := segments(t, e, p, int64(i+1), 250)
		var got strings.Builder
		for _, s := range segs {
			got.WriteString(s.Text)
			if s.LineBytes != int64(len(want)) {
				t.Fatalf("line %d: LineBytes %d want %d", i+1, s.LineBytes, len(want))
			}
		}
		if got.String() != want {
			t.Fatalf("line %d: reassembled %d bytes, want %d", i+1, got.Len(), len(want))
		}
		if last := segs[len(segs)-1]; last.NextOffset != int64(len(want)) || last.HasMore {
			t.Fatalf("line %d: last segment %+v", i+1, last)
		}
	}
}

func TestSegmentOfASingleLineFile(t *testing.T) {
	// a whole file that is one line, with no line ending at all
	content := strings.Repeat("héllo wörld ✓ ", 40000)
	p := writeFile(t, content)
	e := New(longLimits())
	var got strings.Builder
	for _, s := range segments(t, e, p, 1, 300) {
		got.WriteString(s.Text)
	}
	if got.String() != content {
		t.Fatalf("reassembled %d bytes of %d", got.Len(), len(content))
	}
}

func TestSegmentOffsetsInTheMiddleOfACharacterMoveForward(t *testing.T) {
	e := New(longLimits())
	p := writeFile(t, "aé✓b\nnext\n") // é is 2 bytes (offsets 1-2), ✓ is 3 bytes (3-5)
	for _, tc := range []struct {
		offset, wantOffset int64
		wantText           string
	}{
		{0, 0, "aé✓b"}, {1, 1, "é✓b"}, {2, 3, "✓b"}, {3, 3, "✓b"}, {4, 6, "b"}, {5, 6, "b"}, {6, 6, "b"},
	} {
		s, err := e.ReadSegment(context.Background(), p, 1, tc.offset, 100)
		if err != nil || s.Offset != tc.wantOffset || s.Text != tc.wantText || s.HasMore {
			t.Fatalf("offset %d: %v %+v", tc.offset, err, s)
		}
	}
	// at and past the end: an empty segment that says there is nothing more
	for _, off := range []int64{7, 8, 1000} {
		s, err := e.ReadSegment(context.Background(), p, 1, off, 100)
		if err != nil || s.Text != "" || s.HasMore || s.LineBytes != 7 {
			t.Fatalf("offset %d: %v %+v", off, err, s)
		}
	}
	// the size of a segment is clamped, and a tiny one still moves on
	s, _ := e.ReadSegment(context.Background(), p, 1, 0, 1)
	if s.Text == "" || s.NextOffset <= 0 {
		t.Fatalf("one-byte request: %+v", s)
	}
}

func TestSegmentRefusesBadInput(t *testing.T) {
	e := New(longLimits())
	p := numbered(t, 5)
	ctx := context.Background()
	for _, c := range [][3]int64{{0, 0, 10}, {1, -1, 10}, {1, 0, 0}, {1, 0, -4}} {
		if _, err := e.ReadSegment(ctx, p, c[0], c[1], int(c[2])); !errors.Is(err, ErrBadRange) {
			t.Errorf("%v: %v", c, err)
		}
	}
	if s, err := e.ReadSegment(ctx, p, 99, 0, 10); err != nil || s.Text != "" || s.HasMore {
		t.Errorf("a line past the end of a short file: %v %+v", err, s)
	}
	big := writeFile(t, strings.Repeat("x\n", 5000))
	lim := longLimits()
	lim.IndexSlice = time.Nanosecond
	if _, err := New(lim).ReadSegment(ctx, big, 4000, 0, 10); !errors.Is(err, ErrNotIndexed) {
		t.Errorf("not indexed: %v", err)
	}
	if _, err := e.ReadSegment(ctx, writeFile(t, "a\x00b\n"), 1, 0, 10); !errors.Is(err, ErrBinary) {
		t.Errorf("binary: %v", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := e.ReadSegment(cctx, p, 1, 0, 10); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}

// a window reaches lines past a very long one without reading it through
func TestWindowJumpsOverALongLine(t *testing.T) {
	lim := longLimits()
	e := New(lim)
	path := filepath.Join(t.TempDir(), "f.txt")
	f, _ := os.Create(path)
	w := bufio.NewWriter(f)
	fmt.Fprint(w, "first\n")
	for i := 0; i < 8<<20; i += 64 {
		w.WriteString(strings.Repeat("z", 64))
	}
	fmt.Fprint(w, "\nlast one\nreally last")
	w.Flush()
	f.Close()
	indexFully(t, e, path)

	win, err := e.ReadLines(context.Background(), path, 1, 10)
	if err != nil || len(win.Lines) != 4 {
		t.Fatalf("%v %+v", err, win)
	}
	if l := win.Lines[1]; !l.Truncated || l.Bytes != 8<<20 || len(l.Text) > lim.MaxLineBytes {
		t.Fatalf("the long line: truncated=%v bytes=%d text=%d", l.Truncated, l.Bytes, len(l.Text))
	}
	if win.Lines[2].Text != "last one" || win.Lines[3].Text != "really last" || win.Lines[3].N != 4 {
		t.Fatalf("lines after the long one: %+v", win.Lines[2:])
	}
	// a window that starts after it
	win, _ = e.ReadLines(context.Background(), path, 3, 10)
	if len(win.Lines) != 2 || win.Lines[0].Text != "last one" {
		t.Fatalf("%+v", win)
	}
}

func TestReadBudgetStopsAHugeLine(t *testing.T) {
	lim := longLimits()
	lim.ReadBudget = time.Nanosecond
	e := New(lim)
	p := writeFile(t, strings.Repeat("q", 24<<20)) // one 24 MB line; the index has not seen it (no Info call)
	if _, err := e.ReadLines(context.Background(), p, 1, 1); !errors.Is(err, ErrSlow) {
		t.Fatalf("want ErrSlow, got %v", err)
	}
	// with the budget back, the same line reads fine (and is cut)
	lim.ReadBudget = time.Minute
	w, err := New(lim).ReadLines(context.Background(), p, 1, 1)
	if err != nil || len(w.Lines) != 1 || !w.Lines[0].Truncated || w.Lines[0].Bytes != 24<<20 {
		t.Fatalf("%v %+v", err, w)
	}
}

func TestReadLineStopsWhenCancelledInTheMiddleOfALine(t *testing.T) {
	p := writeFile(t, strings.Repeat("q", 16<<20))
	f, _ := os.Open(p)
	defer f.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newCursor(f, 16<<20, 0, 1, nil, &guard{ctx: ctx, deadline: time.Now().Add(time.Hour)})
	if _, err := c.next(10, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled read must stop inside a long line, got %v", err)
	}
}

// ---- search across chunk boundaries and inside long lines, against a plain reference ----

type hit struct{ n, off int64 }

func reference(lines []string, needle string, fold bool) []hit {
	var out []hit
	for i, l := range lines {
		hay, nd := l, needle
		if fold {
			hay, nd = strings.ToLower(l), strings.ToLower(needle)
		}
		for from := 0; ; {
			at := strings.Index(hay[from:], nd)
			if at < 0 {
				break
			}
			out = append(out, hit{int64(i + 1), int64(from + at)})
			from += at + len(nd)
		}
	}
	return out
}

func TestSearchMatchesAReferenceAcrossChunksAndLongLines(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	var lines []string
	var b strings.Builder
	for i := 1; i <= 400; i++ {
		var l string
		switch rng.Intn(6) {
		case 0:
			l = strings.Repeat("ab", 400+rng.Intn(2500)) // long
		case 1:
			l = ""
		default:
			l = fmt.Sprintf("row %d %s", i, strings.Repeat("c", rng.Intn(60)))
		}
		// plant the needle (and its upper-case form) at random places, often right around a chunk boundary
		for k := rng.Intn(3); k > 0 && len(l) > 8; k-- {
			at := rng.Intn(len(l) - 6)
			word := "NeEdLe"
			if rng.Intn(2) == 0 {
				word = "needle"
			}
			l = l[:at] + word + l[at+6:]
		}
		lines = append(lines, l)
		b.WriteString(l)
		if i < 400 {
			if rng.Intn(5) == 0 {
				b.WriteString("\r\n")
			} else {
				b.WriteString("\n")
			}
		}
	}
	p := writeFile(t, b.String())
	for _, chunk := range []int{64, 1024, 4096} {
		lim := longLimits()
		lim.SearchChunkBytes = chunk
		lim.SearchMaxMatches = 7 // many pages
		e := New(lim)
		for _, fold := range []bool{true, false} {
			got := searchAllAt(t, e, p, "needle", !fold)
			want := reference(lines, "needle", fold)
			if len(got) != len(want) {
				t.Fatalf("chunk=%d fold=%v: %d matches, want %d", chunk, fold, len(got), len(want))
			}
			for i := range want {
				if got[i].N != want[i].n || got[i].Offset != want[i].off {
					t.Fatalf("chunk=%d fold=%v match %d: %d:%d want %d:%d", chunk, fold, i, got[i].N, got[i].Offset, want[i].n, want[i].off)
				}
			}
		}
	}
}

func TestSearchInAFileThatIsOneHugeLine(t *testing.T) {
	lim := longLimits()
	lim.SearchChunkBytes = 4096
	lim.SearchMaxBytes = 200 << 10 // many calls
	e := New(lim)
	body := strings.Repeat("abcdefghij ", 500000) // ~5.5 MB, one line, no newline
	at := []int{0, 4_000_000, len(body) - 8}
	b := []byte(body)
	for _, a := range at {
		copy(b[a:], "TARGET!!")
	}
	p := writeFile(t, string(b))
	got := searchAllAt(t, e, p, "target!!", false)
	if len(got) != 3 {
		t.Fatalf("matches: %+v", got)
	}
	for i, a := range at {
		if got[i].N != 1 || got[i].Offset != int64(a) {
			t.Fatalf("match %d at %d:%d, want 1:%d", i, got[i].N, got[i].Offset, a)
		}
	}
	// a segment read at a match's offset shows the text
	seg, err := e.ReadSegment(context.Background(), p, 1, got[1].Offset, 100)
	if err != nil || !strings.HasPrefix(seg.Text, "TARGET!!") {
		t.Fatalf("%v %q", err, seg.Text)
	}
}

func TestSearchPreviewInsideALongLine(t *testing.T) {
	lim := longLimits()
	lim.SearchChunkBytes = 512
	e := New(lim)
	p := writeFile(t, strings.Repeat("x", 3000)+"needle"+strings.Repeat("y", 3000)+"\n")
	res, err := searchT(e, context.Background(), p, "needle", 1, 5, false)
	if err != nil || len(res.Matches) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	pv := res.Matches[0].Preview
	if !strings.Contains(pv, "needle") || !strings.HasPrefix(pv, "…") || !strings.HasSuffix(pv, "…") || len([]rune(pv)) > e.lim.PreviewRunes+2 {
		t.Fatalf("%d runes: %q", len([]rune(pv)), pv)
	}
}

// Searching a file that is one very long line allocates a bounded amount, however long the line is.
func TestSearchMemoryOnOneHugeLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "huge.txt")
	f, _ := os.Create(p)
	w := bufio.NewWriterSize(f, 1<<20)
	chunk := strings.Repeat("abcdefghij ", 1000)
	for i := 0; i < 96<<20/len(chunk); i++ {
		w.WriteString(chunk)
	}
	w.WriteString("THE-END")
	w.Flush()
	f.Close()

	e := New(DefaultLimits())
	before := totalAlloc()
	res, err := e.Search(context.Background(), p, "no-such-text", 1, 0, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	used := totalAlloc() - before
	if res.ScannedBytes < 60<<20 || used > 8<<20 {
		t.Fatalf("scanned %d MiB, allocated %d MiB", res.ScannedBytes>>20, used>>20)
	}
	t.Logf("scanned %d MiB of one line, allocated %d KiB", res.ScannedBytes>>20, used>>10)
}
