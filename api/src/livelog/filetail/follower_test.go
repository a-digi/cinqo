package filetail_test

import (
	"context"
	"errors"
	"hash/fnv"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/livelog"
	"github.com/a-digi/cinqo/src/livelog/filetail"
)

// clock is a settable time, so "the file has been quiet" does not depend on real sleeping.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type rig struct {
	t    *testing.T
	path string
	clk  *clock
	f    *filetail.Follower
}

func newRig(t *testing.T, opts filetail.Options) *rig {
	t.Helper()
	r := &rig{t: t, path: filepath.Join(t.TempDir(), "app.log"), clk: &clock{t: time.Now()}}
	opts.Now = r.clk.now
	r.f = filetail.New(r.path, opts)
	return r
}

func (r *rig) write(s string) {
	r.t.Helper()
	if err := os.WriteFile(r.path, []byte(s), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) append(s string) {
	r.t.Helper()
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		r.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		r.t.Fatal(err)
	}
}

// quietFor makes the file look as if it was last written d ago (against the rig's clock).
func (r *rig) quietFor(d time.Duration) {
	r.t.Helper()
	mt := r.clk.t.Add(-d)
	if err := os.Chtimes(r.path, mt, mt); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) poll(cursor string, limit int) livelog.Batch {
	r.t.Helper()
	b, err := livelog.Poll(context.Background(), r.f, cursor, limit, livelog.DefaultLimits())
	if err != nil {
		r.t.Fatalf("poll: %v", err)
	}
	return b
}

func texts(b livelog.Batch) []string {
	out := []string{}
	for _, l := range b.Lines {
		out = append(out, l.Text)
	}
	return out
}

func TestStartAtTheEndGivesTheLastLinesWithTheirOffsets(t *testing.T) {
	r := newRig(t, filetail.Options{})
	r.write("one\ntwo\nthree\nfour\n")
	b := r.poll("", 2)
	if !reflect.DeepEqual(texts(b), []string{"three", "four"}) || !b.EOF || b.Reset {
		t.Fatalf("got %v eof=%v reset=%v", texts(b), b.EOF, b.Reset)
	}
	if b.Lines[0].Offset != 8 || b.Lines[1].Offset != 14 || b.Lines[0].Bytes != 5 {
		t.Errorf("offsets/bytes: %+v", b.Lines)
	}
	if all := r.poll("", 100); !reflect.DeepEqual(texts(all), []string{"one", "two", "three", "four"}) {
		t.Errorf("fewer lines than the limit: %v", texts(all))
	}
}

func TestAnEmptyFileHasNoLinesAndFollowsFromTheStart(t *testing.T) {
	r := newRig(t, filetail.Options{})
	r.write("")
	b := r.poll("", 10)
	if len(b.Lines) != 0 || !b.EOF || b.Cursor == "" {
		t.Fatalf("%+v", b)
	}
	r.append("first\n")
	b = r.poll(b.Cursor, 10)
	if !reflect.DeepEqual(texts(b), []string{"first"}) || b.Lines[0].Offset != 0 {
		t.Fatalf("%+v", b)
	}
}

func TestAppendedLinesArriveOnceAndInOrderAndNothingNewIsEmptyWithTheSameCursor(t *testing.T) {
	r := newRig(t, filetail.Options{})
	r.write("a\nb\n")
	cur := r.poll("", 10).Cursor
	if again := r.poll(cur, 10); len(again.Lines) != 0 || !again.EOF || again.Cursor != cur || again.Reset {
		t.Fatalf("nothing new: %+v", again)
	}
	r.append("c\nd\ne\n")
	b := r.poll(cur, 2)
	if !reflect.DeepEqual(texts(b), []string{"c", "d"}) || b.EOF {
		t.Fatalf("limited: %v eof=%v", texts(b), b.EOF)
	}
	b = r.poll(b.Cursor, 2)
	if !reflect.DeepEqual(texts(b), []string{"e"}) || !b.EOF {
		t.Fatalf("the rest: %v eof=%v", texts(b), b.EOF)
	}
}

func TestAHalfWrittenLastLineWaitsUntilItIsCompleteOrTheFileIsQuiet(t *testing.T) {
	r := newRig(t, filetail.Options{Grace: 2 * time.Second})
	r.write("done\npart")
	r.quietFor(0)
	b := r.poll("", 10)
	if !reflect.DeepEqual(texts(b), []string{"done"}) || !b.EOF {
		t.Fatalf("held back: %v", texts(b))
	}
	r.append("ial\nnext\n")
	b2 := r.poll(b.Cursor, 10)
	if !reflect.DeepEqual(texts(b2), []string{"partial", "next"}) || b2.Lines[0].Offset != 5 {
		t.Fatalf("completed: %+v", b2)
	}
	// a log that never ends its last line is not hidden for ever
	r.append("tail")
	r.quietFor(time.Second)
	if got := r.poll(b2.Cursor, 10); len(got.Lines) != 0 {
		t.Fatalf("not quiet long enough: %v", texts(got))
	}
	r.quietFor(3 * time.Second)
	got := r.poll(b2.Cursor, 10)
	if !reflect.DeepEqual(texts(got), []string{"tail"}) {
		t.Fatalf("quiet: %v", texts(got))
	}
	if again := r.poll(got.Cursor, 10); len(again.Lines) != 0 {
		t.Fatalf("a line is delivered once: %v", texts(again))
	}
	// the same rule when starting at the end
	if start := r.poll("", 10); start.Lines[len(start.Lines)-1].Text != "tail" {
		t.Errorf("start at the end with a quiet unterminated line: %v", texts(start))
	}
	r.quietFor(0)
	if start := r.poll("", 10); start.Lines[len(start.Lines)-1].Text == "tail" {
		t.Errorf("start at the end must hold the unfinished line back: %v", texts(start))
	}
}

func TestLineEndingsBlankLinesAndControlCharacters(t *testing.T) {
	r := newRig(t, filetail.Options{})
	r.write("crlf\r\n\r\nblank above\nesc\x1b[31mred\ttab\nbad\xff\xfeutf\ndel\x7fx\n")
	b := r.poll("", 100)
	want := []string{"crlf", "", "blank above", "esc␛[31mred\ttab", "bad�utf", "del␡x"}
	if !reflect.DeepEqual(texts(b), want) {
		t.Fatalf("got %q want %q", texts(b), want)
	}
	if b.Lines[0].Bytes != 4 || b.Lines[1].Bytes != 0 {
		t.Errorf("the CR is not part of a line: %+v", b.Lines[:2])
	}
}

func TestALongLineIsCutOnACharacterAndFlaggedWithItsRealLength(t *testing.T) {
	lim := livelog.DefaultLimits()
	lim.MaxLineBytes = 10
	r := newRig(t, filetail.Options{Limits: lim})
	r.write("short\n" + strings.Repeat("é", 20) + "\nafter\n") // 40 bytes of two-byte characters
	b := r.poll("", 10)
	long := b.Lines[1]
	if !long.Truncated || long.Bytes != 40 || long.Text != strings.Repeat("é", 5) {
		t.Fatalf("long line: %+v", long)
	}
	if b.Lines[0].Truncated || b.Lines[2].Text != "after" {
		t.Errorf("neighbours: %+v", b.Lines)
	}
	lim.MaxLineBytes = 9 // an odd cut lands in the middle of a character: the half is dropped
	r2 := newRig(t, filetail.Options{Limits: lim})
	r2.write(strings.Repeat("é", 20) + "\n")
	if got := r2.poll("", 10).Lines[0].Text; got != strings.Repeat("é", 4) {
		t.Errorf("cut in the middle of a character: %q", got)
	}
}

func TestALineLongerThanAWholeBatchIsDeliveredInFlaggedPiecesAndNeverStalls(t *testing.T) {
	lim := livelog.DefaultLimits()
	lim.MaxBatchBytes, lim.MaxLineBytes = 100, 50
	r := newRig(t, filetail.Options{Limits: lim})
	r.write("")
	cur := r.poll("", 10).Cursor
	r.append(strings.Repeat("x", 350) + "\nlast\n")
	var seen []livelog.Line
	for i := 0; i < 20; i++ {
		b := r.poll(cur, 10)
		seen = append(seen, b.Lines...)
		if cur == b.Cursor && len(b.Lines) == 0 {
			break
		}
		cur = b.Cursor
	}
	total, pieces := 0, 0
	for _, l := range seen {
		if l.Text == "last" {
			continue
		}
		pieces++
		total += l.Bytes
		if !l.Truncated {
			// the final piece of the long line ends at its newline: it is complete as a line but shorter than the cut
			if l.Bytes > 50 {
				t.Errorf("a piece that is not flagged is over the cut: %+v", l)
			}
		}
	}
	if total != 350 || pieces < 3 || seen[len(seen)-1].Text != "last" {
		t.Fatalf("pieces=%d total=%d lines=%d last=%q", pieces, total, len(seen), seen[len(seen)-1].Text)
	}
}

func TestBinaryContentIsRefused(t *testing.T) {
	r := newRig(t, filetail.Options{})
	r.write("text\n\x00\x01\x02binary\n")
	if _, err := r.f.Poll(context.Background(), "", 10); !errors.Is(err, livelog.ErrNotText) {
		t.Fatalf("start at the end: %v", err)
	}
	r.write("ok\n")
	cur := r.poll("", 10).Cursor
	r.append("\x00\x00\n")
	if _, err := r.f.Poll(context.Background(), cur, 10); !errors.Is(err, livelog.ErrNotText) {
		t.Fatalf("appended binary: %v", err)
	}
}

func TestATruncatedOrReplacedOrRewrittenFileSaysResetAndStartsAtTheEnd(t *testing.T) {
	long := strings.Repeat("old line of the first file\n", 20)
	t.Run("truncated", func(t *testing.T) {
		r := newRig(t, filetail.Options{})
		r.write(long)
		cur := r.poll("", 5).Cursor
		r.write("fresh\n")
		b := r.poll(cur, 10)
		if !b.Reset || !reflect.DeepEqual(texts(b), []string{"fresh"}) {
			t.Fatalf("%+v", b)
		}
		if next := r.poll(b.Cursor, 10); next.Reset || len(next.Lines) != 0 {
			t.Fatalf("the poll after a reset is normal: %+v", next)
		}
	})
	t.Run("replaced by a rotated file (a new file at the same path)", func(t *testing.T) {
		r := newRig(t, filetail.Options{})
		r.write(long)
		cur := r.poll("", 5).Cursor
		if err := os.Rename(r.path, r.path+".1"); err != nil {
			t.Fatal(err)
		}
		r.write(long + long) // already longer than the old cursor
		b := r.poll(cur, 3)
		if !b.Reset || len(b.Lines) != 3 {
			t.Fatalf("%+v", b)
		}
	})
	t.Run("rewritten in place, already longer than the cursor (copytruncate)", func(t *testing.T) {
		r := newRig(t, filetail.Options{})
		r.write(long)
		cur := r.poll("", 5).Cursor
		f, err := os.OpenFile(r.path, os.O_WRONLY|os.O_TRUNC, 0o644) // the same inode, new content
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(strings.Repeat("a completely different beginning\n", 40))
		f.Close()
		if b := r.poll(cur, 3); !b.Reset {
			t.Fatalf("a rewritten file must not continue at the old offset: %+v", b)
		}
	})
	t.Run("a cursor beyond the end, or of another file", func(t *testing.T) {
		r := newRig(t, filetail.Options{})
		r.write(long)
		beyond := livelog.Cursor{Offset: 1 << 30, Identity: "x|0|0"}.Encode()
		if b := r.poll(beyond, 3); !b.Reset {
			t.Errorf("beyond the end: %+v", b)
		}
		other := newRig(t, filetail.Options{})
		other.write(long)
		oc := other.poll("", 3).Cursor
		if b := r.poll(oc, 3); !b.Reset {
			t.Errorf("a cursor of another file: %+v", b)
		}
	})
	t.Run("a cursor whose identity is not ours", func(t *testing.T) {
		r := newRig(t, filetail.Options{})
		r.write(long)
		for _, id := range []string{"", "a|b", "x|99|0", "x|-1|0", "x|3|zz", "|||"} {
			if b := r.poll(livelog.Cursor{Offset: 3, Identity: id}.Encode(), 3); !b.Reset {
				t.Errorf("identity %q: %+v", id, b)
			}
		}
	})
}

func TestACursorThatIsNotOneIsRefusedAndAMissingFileIsNotFound(t *testing.T) {
	r := newRig(t, filetail.Options{})
	r.write("x\n")
	if _, err := r.f.Poll(context.Background(), "garbage", 10); !errors.Is(err, livelog.ErrBadCursor) {
		t.Errorf("bad cursor: %v", err)
	}
	gone := filetail.New(filepath.Join(t.TempDir(), "missing.log"), filetail.Options{})
	if _, err := gone.Poll(context.Background(), "", 10); !errors.Is(err, livelog.ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	dir := filetail.New(t.TempDir(), filetail.Options{})
	if _, err := dir.Poll(context.Background(), "", 10); !errors.Is(err, livelog.ErrNotFound) {
		t.Errorf("a directory: %v", err)
	}
	// the file appears (or comes back after a rotation) later: the same follower works
	late := filetail.New(filepath.Join(t.TempDir(), "late.log"), filetail.Options{})
	if _, err := late.Poll(context.Background(), "", 10); !errors.Is(err, livelog.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestACancelledContextReadsNothing(t *testing.T) {
	r := newRig(t, filetail.Options{})
	r.write("x\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.f.Poll(ctx, "", 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

// Reading a long file forward in batches delivers every line exactly once, in order, within the bounds of each poll.
func TestAWholeFileReadForwardInBatchesLosesAndRepeatsNothing(t *testing.T) {
	lim := livelog.DefaultLimits()
	lim.MaxBatchBytes = 4096
	r := newRig(t, filetail.Options{Limits: lim})
	r.write("")
	cur := r.poll("", 10).Cursor
	var want []string
	var sb strings.Builder
	for i := 0; i < 3000; i++ {
		line := strings.Repeat("x", i%97) + "#" + itoa(i)
		want = append(want, line)
		sb.WriteString(line + "\n")
	}
	r.append(sb.String())
	var got []string
	polls := 0
	for {
		b, err := livelog.Poll(context.Background(), r.f, cur, 100, lim)
		if err != nil {
			t.Fatal(err)
		}
		polls++
		if len(b.Lines) > 100 {
			t.Fatalf("a poll returned %d lines", len(b.Lines))
		}
		got = append(got, texts(b)...)
		cur = b.Cursor
		if b.EOF {
			break
		}
		if polls > 5000 {
			t.Fatal("never reached the end")
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %d lines, want %d (first difference at %d)", len(got), len(want), firstDiff(got, want))
	}
}

func itoa(i int) string {
	const d = "0123456789"
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{d[i%10]}, b...)
	}
	return string(b)
}

func firstDiff(a, b []string) int {
	for i := range a {
		if i >= len(b) || a[i] != b[i] {
			return i
		}
	}
	return len(a)
}

// A poll costs the new bytes, not the file's size: a 1 GiB (sparse) file with a text tail is polled with a few MB at most allocated.
func TestAPollCostsTheNewBytesNotTheFileSize(t *testing.T) {
	if testing.Short() {
		t.Skip("allocation test")
	}
	r := newRig(t, filetail.Options{})
	f, err := os.Create(r.path)
	if err != nil {
		t.Fatal(err)
	}
	const size = 1 << 30
	if err := f.Truncate(size); err != nil {
		t.Skipf("no sparse files here: %v", err)
	}
	// a text tail at least as long as one start window, so the window never reaches the NUL-filled body (which is never read)
	tail := []byte(strings.Repeat("a line of the tail of the log\n", 12000)) // about 360 KiB
	if _, err := f.WriteAt(tail, size); err != nil {
		t.Fatal(err)
	}
	f.Close()
	measure := func(cursor string) (livelog.Batch, uint64) {
		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)
		b, err := livelog.Poll(context.Background(), r.f, cursor, 100, livelog.DefaultLimits())
		runtime.ReadMemStats(&m2)
		if err != nil {
			t.Fatal(err)
		}
		return b, m2.TotalAlloc - m1.TotalAlloc
	}
	b, alloc := measure("")
	if len(b.Lines) != 100 || b.Lines[0].Offset < size {
		t.Errorf("the start poll must return the last 100 lines of the tail: %d lines from offset %d", len(b.Lines), b.Lines[0].Offset)
	}
	if alloc > 2<<20 {
		t.Errorf("a start poll allocated %d bytes on a 1 GiB file", alloc)
	}
	b2, alloc2 := measure(b.Cursor)
	if len(b2.Lines) != 0 || alloc2 > 1<<20 {
		t.Errorf("a quiet follow poll: %d lines, %d bytes allocated", len(b2.Lines), alloc2)
	}
}

func TestPollsFromManyGoroutinesWhileTheFileGrowAreSafe(t *testing.T) {
	r := newRig(t, filetail.Options{})
	r.write("seed\n")
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				r.append("line " + itoa(i) + "\n")
			}
		}
	}()
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cur := ""
			for i := 0; i < 50; i++ {
				b, err := r.f.Poll(context.Background(), cur, 20)
				if err != nil {
					t.Error(err)
					return
				}
				cur = b.Cursor
			}
		}()
	}
	time.AfterFunc(500*time.Millisecond, func() { close(stop) })
	wg.Wait()
}

// ---- cursors that look right but do not describe this file ----

func realCursor(t *testing.T, r *rig) (livelog.Cursor, string) {
	t.Helper()
	c, present, err := livelog.ParseCursor(r.poll("", 5).Cursor)
	if err != nil || !present {
		t.Fatal(err)
	}
	return c, strings.Split(c.Identity, "|")[0]
}

func fnv64(b []byte) string {
	h := fnv.New64a()
	h.Write(b)
	return strconv.FormatUint(h.Sum64(), 16)
}

func TestAFileTruncatedToAPrefixOfItselfIsStillNoticed(t *testing.T) {
	r := newRig(t, filetail.Options{})
	content := strings.Repeat("the same first bytes every time\n", 30)
	r.write(content)
	cur := r.poll("", 5).Cursor
	if err := os.Truncate(r.path, 100); err != nil { // the same file, the same beginning, shorter than the cursor
		t.Fatal(err)
	}
	if b := r.poll(cur, 5); !b.Reset {
		t.Fatalf("a file shorter than the cursor must reset: %+v", b)
	}
}

func TestACursorWhoseHeadIsLongerThanItsOffsetOrThanAnyCursorIsNotTrusted(t *testing.T) {
	r := newRig(t, filetail.Options{})
	content := strings.Repeat("0123456789abcdef", 40) + "\n" // 641 bytes
	r.write(content + content)
	c, id := realCursor(t, r)
	head := func(n int) string { return fnv64([]byte(content[:n])) }
	// the head says 64 bytes, the offset only 5: the offset lies inside the head, so it is not a cursor this module wrote
	bad := livelog.Cursor{Offset: 5, Identity: id + "|64|" + head(64)}.Encode()
	if b := r.poll(bad, 5); !b.Reset {
		t.Errorf("a head longer than the offset must reset: %+v", b)
	}
	// a head longer than any cursor carries (65 or 100 bytes), with the right hash for it
	for _, n := range []int{65, 100} {
		long := livelog.Cursor{Offset: c.Offset, Identity: id + "|" + strconv.Itoa(n) + "|" + head(n)}.Encode()
		if b := r.poll(long, 5); !b.Reset {
			t.Errorf("a head of %d bytes must reset: %+v", n, b)
		}
	}
	// and the real one is accepted (the hash above is computed the way the follower does it)
	good := livelog.Cursor{Offset: c.Offset, Identity: id + "|64|" + head(64)}.Encode()
	if b := r.poll(good, 5); b.Reset {
		t.Errorf("a well formed cursor must continue: %+v", b)
	}
}

func TestADefaultGraceHoldsAJustWrittenLastLineBack(t *testing.T) {
	r := newRig(t, filetail.Options{}) // no Grace given: the default applies
	r.write("done\nhalf")
	r.quietFor(500 * time.Millisecond)
	if b := r.poll("", 10); !reflect.DeepEqual(texts(b), []string{"done"}) {
		t.Fatalf("under the default grace: %v", texts(b))
	}
	r.quietFor(filetail.DefaultGrace + time.Second)
	if b := r.poll("", 10); !reflect.DeepEqual(texts(b), []string{"done", "half"}) {
		t.Fatalf("after the default grace: %v", texts(b))
	}
}

func TestTheWindowOfAStartPollNeverBeginsInTheMiddleOfALine(t *testing.T) {
	lim := livelog.DefaultLimits()
	lim.MaxBatchBytes = 100
	r := newRig(t, filetail.Options{Limits: lim})
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("line-" + itoa(1000+i) + "-padding-padding\n") // 24 bytes: the 100 byte window starts inside a line
	}
	r.write(sb.String())
	b := r.poll("", 100)
	if len(b.Lines) == 0 {
		t.Fatal("no lines")
	}
	for _, l := range b.Lines {
		if !strings.HasPrefix(l.Text, "line-") || !strings.HasSuffix(l.Text, "-padding-padding") {
			t.Errorf("a partial line was returned: %q", l.Text)
		}
	}
	if last := b.Lines[len(b.Lines)-1].Text; last != "line-1049-padding-padding" {
		t.Errorf("the last line: %q", last)
	}
}

func TestMoreIsTrueWhileAQuietUnfinishedLastLineStillWaits(t *testing.T) {
	r := newRig(t, filetail.Options{Grace: time.Second})
	r.write("")
	cur := r.poll("", 10).Cursor
	r.append("a\ntail")
	r.quietFor(5 * time.Second)
	b := r.poll(cur, 1)
	if !reflect.DeepEqual(texts(b), []string{"a"}) || b.EOF {
		t.Fatalf("one line was asked for, the quiet unfinished one waits: %v eof=%v", texts(b), b.EOF)
	}
	if b2 := r.poll(b.Cursor, 1); !reflect.DeepEqual(texts(b2), []string{"tail"}) || !b2.EOF {
		t.Fatalf("then it comes: %v eof=%v", texts(b2), b2.EOF)
	}
}

func TestACancelledContextIsAnsweredBeforeTheFileIsLookedAt(t *testing.T) {
	gone := filetail.New(filepath.Join(t.TempDir(), "missing.log"), filetail.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := gone.Poll(ctx, "", 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
