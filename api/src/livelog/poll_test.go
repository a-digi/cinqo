package livelog_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/livelog"
	"github.com/a-digi/cinqo/src/livelog/livelogtest"
)

type fixed struct {
	b   livelog.Batch
	err error
}

func (f fixed) Poll(context.Context, string, int) (livelog.Batch, error) { return f.b, f.err }

func TestPollClampsTheNumberOfLinesAsked(t *testing.T) {
	l := livelogtest.NewLog()
	lim := livelog.Limits{DefaultLines: 7, MaxLines: 20, PollTimeout: time.Second}
	for _, asked := range []int{0, -5, 3, 20, 21, 100000} {
		if _, err := livelog.Poll(context.Background(), l, "", asked, lim); err != nil {
			t.Fatal(err)
		}
	}
	if want := []int{7, 7, 3, 20, 20, 20}; !reflect.DeepEqual(l.Asked, want) {
		t.Errorf("limits asked of the follower = %v, want %v", l.Asked, want)
	}
}

func TestPollWithEmptyLimitsUsesTheDefaults(t *testing.T) {
	l := livelogtest.NewLog()
	if _, err := livelog.Poll(context.Background(), l, "", 0, livelog.Limits{}); err != nil {
		t.Fatalf("zero limits must not end the poll at once: %v", err)
	}
	if l.Asked[0] != livelog.DefaultLimits().DefaultLines {
		t.Errorf("asked %d", l.Asked[0])
	}
}

func TestPollNeverCallsAFollowerWithABadCursor(t *testing.T) {
	l := livelogtest.NewLog("a")
	if _, err := livelog.Poll(context.Background(), l, "garbage", 10, livelog.DefaultLimits()); !errors.Is(err, livelog.ErrBadCursor) {
		t.Fatalf("err = %v", err)
	}
	if len(l.Asked) != 0 {
		t.Error("the follower was called with a cursor that never was one")
	}
}

func TestASlowFollowerIsGivenUpOnAtTheTimeoutEvenWhenItIgnoresItsContext(t *testing.T) {
	for _, ignore := range []bool{false, true} {
		l := livelogtest.NewLog("a")
		l.Delay, l.IgnoreContext = 2*time.Second, ignore
		start := time.Now()
		_, err := livelog.Poll(context.Background(), l, "", 10, livelog.Limits{PollTimeout: 50 * time.Millisecond})
		if !errors.Is(err, livelog.ErrTimeout) {
			t.Errorf("ignoreContext=%v: err = %v", ignore, err)
		}
		if time.Since(start) > time.Second {
			t.Errorf("ignoreContext=%v: waited %v", ignore, time.Since(start))
		}
	}
}

func TestACancelledCallerStopsThePollAtOnce(t *testing.T) {
	l := livelogtest.NewLog("a")
	l.Delay = 2 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := livelog.Poll(ctx, l, "", 10, livelog.DefaultLimits())
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Errorf("err = %v after %v", err, time.Since(start))
	}
	already, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if _, err := livelog.Poll(already, livelogtest.NewLog(), "", 10, livelog.DefaultLimits()); err == nil {
		t.Error("a context that is already cancelled must not start a poll that succeeds")
	}
}

func TestAFollowersOwnErrorsPassThroughUnchanged(t *testing.T) {
	for _, want := range []error{livelog.ErrNotFound, livelog.ErrBusy, errors.New("disk on fire")} {
		l := livelogtest.NewLog()
		l.Err = want
		if _, err := livelog.Poll(context.Background(), l, "", 10, livelog.DefaultLimits()); err != want {
			t.Errorf("err = %v, want %v", err, want)
		}
	}
}

func TestABrokenFollowerCannotBreakTheContract(t *testing.T) {
	cur := livelog.Cursor{Offset: 3}.Encode()
	three := []livelog.Line{{Offset: 0, Text: "a"}, {Offset: 2, Text: "b"}, {Offset: 4, Text: "c"}}
	for name, f := range map[string]fixed{
		"more lines than asked":    {b: livelog.Batch{Lines: three, Cursor: cur}},
		"no cursor":                {b: livelog.Batch{Lines: three[:1]}},
		"a cursor that is not one": {b: livelog.Batch{Lines: three[:1], Cursor: "junk"}},
	} {
		if _, err := livelog.Poll(context.Background(), f, "", 2, livelog.DefaultLimits()); !errors.Is(err, livelog.ErrBadBatch) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	b, err := livelog.Poll(context.Background(), fixed{b: livelog.Batch{Cursor: cur, EOF: true}}, "", 2, livelog.DefaultLimits())
	if err != nil || b.Lines == nil || len(b.Lines) != 0 {
		t.Errorf("a batch without lines must carry an empty list, not null: %#v %v", b, err)
	}
	if b, err := livelog.Poll(context.Background(), fixed{b: livelog.Batch{Lines: three[:2], Cursor: cur}}, "", 2, livelog.DefaultLimits()); err != nil || len(b.Lines) != 2 {
		t.Errorf("exactly the asked number is fine: %v %v", len(b.Lines), err)
	}
}

// ---- the in-memory log used by the tests above follows the contract itself, so the later layers can rely on it ----

func poll(t *testing.T, l *livelogtest.Log, cursor string, limit int) livelog.Batch {
	t.Helper()
	b, err := livelog.Poll(context.Background(), l, cursor, limit, livelog.DefaultLimits())
	if err != nil {
		t.Fatal(err)
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

func TestTheFakeLogDeliversEachLineOnceInOrderAndStartsAtTheEnd(t *testing.T) {
	l := livelogtest.NewLog("one", "two", "three", "four")
	b := poll(t, l, "", 2)
	if !reflect.DeepEqual(texts(b), []string{"three", "four"}) || !b.EOF || b.Reset {
		t.Fatalf("start at the end: %v eof=%v reset=%v", texts(b), b.EOF, b.Reset)
	}
	if b.Lines[0].Offset != 8 || b.Lines[1].Offset != 14 {
		t.Errorf("offsets %d %d", b.Lines[0].Offset, b.Lines[1].Offset)
	}
	cur := b.Cursor
	if again := poll(t, l, cur, 10); len(again.Lines) != 0 || !again.EOF || again.Cursor != cur {
		t.Fatalf("nothing new must be empty with the same cursor: %+v", again)
	}
	l.Append("five", "six", "seven")
	b = poll(t, l, cur, 2)
	if !reflect.DeepEqual(texts(b), []string{"five", "six"}) || b.EOF {
		t.Fatalf("more than the limit: %v eof=%v", texts(b), b.EOF)
	}
	b = poll(t, l, b.Cursor, 2)
	if !reflect.DeepEqual(texts(b), []string{"seven"}) || !b.EOF {
		t.Fatalf("the rest: %v eof=%v", texts(b), b.EOF)
	}
}

func TestTheFakeLogSaysResetWhenTruncatedOrReplaced(t *testing.T) {
	l := livelogtest.NewLog("aaaa", "bbbb", "cccc")
	cur := poll(t, l, "", 10).Cursor
	l.Truncate()
	l.Append("new")
	b := poll(t, l, cur, 10)
	if !b.Reset || !reflect.DeepEqual(texts(b), []string{"new"}) {
		t.Fatalf("truncated: %+v", b)
	}
	cur = b.Cursor
	l.Replace("x", "y")
	b = poll(t, l, cur, 10)
	if !b.Reset || !reflect.DeepEqual(texts(b), []string{"x", "y"}) {
		t.Fatalf("replaced: %+v", b)
	}
	if b2 := poll(t, l, b.Cursor, 10); b2.Reset || len(b2.Lines) != 0 {
		t.Fatalf("after the reset the next poll is normal: %+v", b2)
	}
}

func TestTheFakeProviderOpensOnlyWhatItHas(t *testing.T) {
	p := &livelogtest.Provider{IDValue: "fake", TitleValue: "Fake", ScopeList: []string{"s"}, Logs: map[string]*livelogtest.Log{"a": livelogtest.NewLog("x")}}
	if f, err := p.Open("a"); err != nil || f == nil {
		t.Fatalf("Open(a) = %v %v", f, err)
	}
	if _, err := p.Open("missing"); !errors.Is(err, livelog.ErrNotFound) {
		t.Errorf("Open(missing) = %v", err)
	}
}
