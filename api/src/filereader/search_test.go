package filereader

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// searchT is Search from the start of a line (no offset into it).
func searchT(e *Engine, ctx context.Context, path, q string, from int64, limit int, cs bool) (SearchResult, error) {
	return e.Search(ctx, path, q, from, 0, limit, cs)
}

// every returns a file of n lines where each line k is "row k" and every `step`-th line also says "NEEDLE".
func every(t *testing.T, n, step int) (path string, hits []int64) {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if i%step == 0 {
			fmt.Fprintf(&b, "row %d has a Needle in it\n", i)
			hits = append(hits, int64(i))
		} else {
			fmt.Fprintf(&b, "row %d\n", i)
		}
	}
	return writeFile(t, b.String()), hits
}

func numbers(ms []Match) []int64 {
	out := make([]int64, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.N)
	}
	return out
}

// searchAll follows NextFrom and NextOffset until Done and returns every match found (as "line" or
// "line:offset"), checking none shows up twice or out of order.
func searchAllAt(t *testing.T, e *Engine, path, q string, caseSensitive bool) []Match {
	t.Helper()
	var all []Match
	from, off := int64(1), int64(0)
	for calls := 0; calls < 100000; calls++ {
		res, err := e.Search(context.Background(), path, q, from, off, 1000, caseSensitive)
		if err != nil {
			if errors.Is(err, ErrNotIndexed) {
				indexFully(t, e, path)
				continue
			}
			t.Fatal(err)
		}
		for _, m := range res.Matches {
			if n := len(all); n > 0 && (m.N < all[n-1].N || (m.N == all[n-1].N && m.Offset <= all[n-1].Offset)) {
				t.Fatalf("match %d:%d reported twice or out of order", m.N, m.Offset)
			}
			all = append(all, m)
		}
		if res.Done {
			return all
		}
		if res.NextFrom < from || (res.NextFrom == from && res.NextOffset <= off) {
			t.Fatalf("search did not move on: from %d:%d next %d:%d", from, off, res.NextFrom, res.NextOffset)
		}
		from, off = res.NextFrom, res.NextOffset
	}
	t.Fatal("search never finished")
	return nil
}

// searchAll is searchAllAt reduced to the line numbers.
func searchAll(t *testing.T, e *Engine, path, q string, caseSensitive bool) []int64 {
	t.Helper()
	var out []int64
	for _, m := range searchAllAt(t, e, path, q, caseSensitive) {
		out = append(out, m.N)
	}
	return out
}

func TestSearchFindsEveryMatchOnceAcrossPages(t *testing.T) {
	p, hits := every(t, 3000, 7)
	e := New(smallLimits()) // 50 matches a page
	got := searchAll(t, e, p, "needle", false)
	if fmt.Sprint(got) != fmt.Sprint(hits) {
		t.Fatalf("found %d matches, want %d", len(got), len(hits))
	}
}

func TestSearchPageIsBoundedAndContinues(t *testing.T) {
	p, hits := every(t, 3000, 7)
	e := New(smallLimits())
	res, err := searchT(e, context.Background(), p, "needle", 1, 100000, false)
	if err != nil || len(res.Matches) != 50 || res.Done || res.NextFrom != hits[49] || res.NextOffset == 0 {
		t.Fatalf("first page: %v len=%d done=%v next=%d", err, len(res.Matches), res.Done, res.NextFrom)
	}
	if res.Matches[0].N != hits[0] || !strings.Contains(res.Matches[0].Preview, "Needle") {
		t.Fatalf("first match: %+v", res.Matches[0])
	}
}

func TestSearchCase(t *testing.T) {
	e := New(smallLimits())
	p := writeFile(t, "Alpha\nalpha\nALPHA\nbeta\n")
	if got := searchAll(t, e, p, "alpha", false); fmt.Sprint(got) != "[1 2 3]" {
		t.Fatalf("ignoring case: %v", got)
	}
	if got := searchAll(t, e, p, "alpha", true); fmt.Sprint(got) != "[2]" {
		t.Fatalf("case sensitive: %v", got)
	}
	if got := searchAll(t, e, writeFile(t, "Ärger\närger\nöl\n"), "ÄRGER", false); fmt.Sprint(got) != "[1 2]" {
		t.Fatalf("unicode: %v", got)
	}
}

func TestSearchIsPlainTextNeverAPattern(t *testing.T) {
	e := New(smallLimits())
	p := writeFile(t, "a.c\nabc\n(x)\n.*\nfoo[0]\n")
	for q, want := range map[string]string{".": "[1 4]", "(x)": "[3]", ".*": "[4]", "[0]": "[5]", "a.c": "[1]"} {
		if got := searchAll(t, e, p, q, true); fmt.Sprint(got) != want {
			t.Errorf("query %q: %v want %s", q, got, want)
		}
	}
}

func TestSearchNeverMatchesAcrossLines(t *testing.T) {
	e := New(smallLimits())
	p := writeFile(t, "ab\ncd\nxy\n")
	if got := searchAll(t, e, p, "bc", false); len(got) != 0 {
		t.Fatalf("a match crossed a line ending: %v", got)
	}
	// a search text with a line break could only ever match across lines, so it is refused
	for _, q := range []string{"b\nc", "b\r\nc", "a\r", "\n"} {
		if _, err := searchT(e, context.Background(), p, q, 1, 5, false); !errors.Is(err, ErrBadQuery) {
			t.Errorf("query %q: %v", q, err)
		}
	}
}

func TestSearchStartsWhereAsked(t *testing.T) {
	p, hits := every(t, 3000, 100)
	e := New(smallLimits())
	indexFully(t, e, p)
	res, err := searchT(e, context.Background(), p, "needle", hits[3]+1, 2, false)
	if err != nil || len(res.Matches) != 2 || res.Matches[0].N != hits[4] || res.Matches[1].N != hits[5] {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestSearchRunsOutOfBytesAndSaysWhereToContinue(t *testing.T) {
	lim := smallLimits()
	lim.SearchMaxBytes = 2000 // about 150 short lines per call
	e := New(lim)
	p, hits := every(t, 3000, 700)
	res, err := searchT(e, context.Background(), p, "needle", 1, 50, false)
	if err != nil || res.Done || res.NextFrom <= 1 || res.ScannedBytes < 2000 || res.ScannedBytes > 2000+200 {
		t.Fatalf("a call out of bytes must stop early without an error: %v %+v", err, res)
	}
	if got := searchAll(t, e, p, "needle", false); fmt.Sprint(got) != fmt.Sprint(hits) {
		t.Fatalf("continuing over many calls lost or repeated matches: %v want %v", got, hits)
	}
}

func TestSearchRunsOutOfTime(t *testing.T) {
	lim := smallLimits()
	lim.SearchBudget = time.Nanosecond
	e := New(lim)
	p, _ := every(t, 5000, 3)
	res, err := searchT(e, context.Background(), p, "zzz-no-match", 1, 50, false)
	if err != nil || res.Done || res.NextFrom < 1 {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestSearchNeedsTheIndexForADeepStart(t *testing.T) {
	p, _ := every(t, 5000, 9)
	lim := smallLimits()
	lim.IndexSlice = time.Nanosecond
	e := New(lim)
	if _, err := searchT(e, context.Background(), p, "needle", 4000, 5, false); !errors.Is(err, ErrNotIndexed) {
		t.Fatalf("want ErrNotIndexed, got %v", err)
	}
	// a start at the top never needs it
	if _, err := searchT(e, context.Background(), p, "needle", 1, 5, false); err != nil {
		t.Fatal(err)
	}
	indexFully(t, e, p)
	if res, err := searchT(e, context.Background(), p, "needle", 99999, 5, false); err != nil || !res.Done || len(res.Matches) != 0 {
		t.Fatalf("past the end: %v %+v", err, res)
	}
}

func TestSearchRefusesBadInput(t *testing.T) {
	e := New(smallLimits())
	p := numbered(t, 5)
	ctx := context.Background()
	for _, q := range []string{"", strings.Repeat("a", 201), "a\x00b", "bad\xff"} {
		if _, err := searchT(e, ctx, p, q, 1, 5, false); !errors.Is(err, ErrBadQuery) {
			t.Errorf("query %q: %v", q, err)
		}
	}
	if _, err := searchT(e, ctx, p, "L", 0, 5, false); !errors.Is(err, ErrBadRange) {
		t.Errorf("from 0: %v", err)
	}
	if _, err := searchT(e, ctx, p, "L", 1, 0, false); !errors.Is(err, ErrBadRange) {
		t.Errorf("limit 0: %v", err)
	}
	if _, err := searchT(e, ctx, writeFile(t, "a\x00b\n"), "a", 1, 5, false); !errors.Is(err, ErrBinary) {
		t.Errorf("binary: %v", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := searchT(e, cctx, p, "L", 1, 5, false); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}

func TestSearchFindsMatchesFarInsideALongLine(t *testing.T) {
	lim := smallLimits()
	e := New(lim)
	long := strings.Repeat("x", lim.MaxLineBytes+500) + "NEEDLE" + strings.Repeat("y", 3000) + "needle"
	p := writeFile(t, long+"\nshort NEEDLE\n")
	got := searchAllAt(t, e, p, "needle", false)
	if len(got) != 3 || got[0] != (Match{N: 1, Offset: int64(lim.MaxLineBytes + 500), Preview: got[0].Preview}) ||
		got[1].N != 1 || got[1].Offset != int64(lim.MaxLineBytes+500+6+3000) || got[2].N != 2 || got[2].Offset != 6 {
		t.Fatalf("matches: %+v", got)
	}
	if !strings.Contains(got[0].Preview, "NEEDLE") || !strings.HasPrefix(got[0].Preview, "…") {
		t.Fatalf("a match inside a long line carries a cut preview: %q", got[0].Preview)
	}
}

func TestSearchPreview(t *testing.T) {
	e := New(smallLimits())
	line := strings.Repeat("a", 500) + "TARGET" + strings.Repeat("b", 500)
	p := writeFile(t, "TARGET at start\n"+line+"\n")
	res, _ := searchT(e, context.Background(), p, "target", 1, 10, false)
	if len(res.Matches) != 2 {
		t.Fatal(res)
	}
	if res.Matches[0].Preview != "TARGET at start" {
		t.Fatalf("a short line is shown whole: %q", res.Matches[0].Preview)
	}
	pv := res.Matches[1].Preview
	if !strings.Contains(pv, "TARGET") || !strings.HasPrefix(pv, "…") || !strings.HasSuffix(pv, "…") || len([]rune(pv)) > e.lim.PreviewRunes+2 {
		t.Fatalf("a long line is cut around the match: %d runes, %q", len([]rune(pv)), pv)
	}
	// control characters never reach a preview
	p2 := writeFile(t, "x\x1b[31mred NEEDLE\n")
	res, _ = searchT(e, context.Background(), p2, "needle", 1, 1, false)
	if strings.ContainsRune(res.Matches[0].Preview, 0x1b) {
		t.Fatalf("escape in preview: %q", res.Matches[0].Preview)
	}
}

func TestSearchCRLFAndLastLineWithoutNewline(t *testing.T) {
	e := New(smallLimits())
	p := writeFile(t, "one\r\ntwo needle\r\nlast needle")
	if got := searchAll(t, e, p, "needle", false); fmt.Sprint(got) != "[2 3]" {
		t.Fatalf("%v", got)
	}
	if got := searchAll(t, e, writeFile(t, ""), "x", false); len(got) != 0 {
		t.Fatalf("empty file: %v", got)
	}
}

func TestSearchDoneOnlyWhenNothingIsLeft(t *testing.T) {
	e := New(smallLimits())
	// the limit is reached on the last match, but a line ending is still unread: not done yet, and the next
	// call (from where this one stopped) finishes with no matches
	p := writeFile(t, "needle\nx\nneedle\n")
	res, _ := searchT(e, context.Background(), p, "needle", 1, 2, false)
	if res.Done || len(res.Matches) != 2 {
		t.Fatalf("%+v", res)
	}
	again, err := e.Search(context.Background(), p, "needle", res.NextFrom, res.NextOffset, 2, false)
	if err != nil || !again.Done || len(again.Matches) != 0 {
		t.Fatalf("%v %+v", err, again)
	}
	// a file with no line ending after the last match is done at once
	p2 := writeFile(t, "needle\nx\nneedle")
	res, _ = searchT(e, context.Background(), p2, "needle", 1, 2, false)
	if !res.Done {
		t.Fatalf("%+v", res)
	}
	// more matches after the limit
	p3 := writeFile(t, "needle\nx\nneedle\nneedle\n")
	res, _ = searchT(e, context.Background(), p3, "needle", 1, 2, false)
	if res.Done {
		t.Fatalf("%+v", res)
	}
}

func TestSearchMemoryStaysFlat(t *testing.T) {
	p, n := writeBig(t, 64)
	e := New(DefaultLimits())
	var allocs func() uint64 = totalAlloc
	before := allocs()
	res, err := searchT(e, context.Background(), p, "99999999", 1, 50, false) // matches no line: scans up to the byte budget
	if err != nil {
		t.Fatal(err)
	}
	used := allocs() - before
	// the bytes it scanned are bounded; the memory it allocated must not grow with them (per-line scratch is
	// freed as it goes)
	if res.ScannedBytes > e.lim.SearchMaxBytes+int64(e.lim.MaxLineBytes) || n == 0 {
		t.Fatalf("scanned %d", res.ScannedBytes)
	}
	t.Logf("scanned %d MiB, allocated %d MiB in total", res.ScannedBytes>>20, used>>20)
	if used > 24<<20 {
		t.Fatalf("scanning %d MiB allocated %d MiB: matching must not copy every line", res.ScannedBytes>>20, used>>20)
	}
}

func TestIndexFoldASCII(t *testing.T) {
	cases := []struct {
		hay, needle string
		want        int
	}{
		{"Hello World", "world", 6}, {"Hello", "hello", 0}, {"abc", "abcd", -1}, {"", "a", -1},
		{"xxNeEdLe", "needle", 2}, {"aab", "ab", 1}, {"a", "a", 0}, {"ÄÖ needle", "needle", 5},
	}
	for _, c := range cases {
		if got := indexFoldASCII([]byte(c.hay), []byte(c.needle)); got != c.want {
			t.Errorf("%q in %q: got %d want %d", c.needle, c.hay, got, c.want)
		}
	}
}
