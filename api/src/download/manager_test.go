package download

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/toolrepository/fetch"
)

// ---- test helpers ----

func content(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7 + i/251)
	}
	return b
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

const goodURL = "https://models.example.com/a/b/resolve/78eb92a46fc37e6b524df991ed9aca9bc6aa7b80/model.gguf"

func allowExample(host string) bool { return host == "models.example.com" }

func specFor(t *testing.T, data []byte) Spec {
	t.Helper()
	return Spec{Label: "Test model", URL: goodURL, Dest: filepath.Join(t.TempDir(), "models", "m.gguf"), SizeBytes: int64(len(data)), SHA256: sum(data), AllowHost: allowExample, Scope: "cinqo:platform:manage"}
}

// streamFunc is a scripted file server.
type streamFunc func(ctx context.Context, rawURL string, opts fetch.StreamOptions) (*fetch.Stream, error)

func (f streamFunc) Stream(ctx context.Context, rawURL string, opts fetch.StreamOptions) (*fetch.Stream, error) {
	return f(ctx, rawURL, opts)
}

// parseRange reads "bytes=5-9" (or "bytes=5-") against a file of size n and returns the first and last byte asked for.
func parseRange(h string, n int) (first, last int, ok bool) {
	rest, found := strings.CutPrefix(h, "bytes=")
	if !found {
		return 0, 0, false
	}
	a, b, found := strings.Cut(rest, "-")
	if !found {
		return 0, 0, false
	}
	first, err := strconv.Atoi(a)
	if err != nil {
		return 0, 0, false
	}
	last = n - 1
	if b != "" {
		if last, err = strconv.Atoi(b); err != nil {
			return 0, 0, false
		}
	}
	if last > n-1 {
		last = n - 1
	}
	return first, last, true
}

// serve answers like a file server that supports Range requests: every request gets a 206 with exactly the bytes asked for.
func serve(data []byte, calls *int32, ranges *[]string) streamFunc {
	return scripted(data, calls, ranges)
}

// behaviour is what one request of a scripted server does instead of answering properly.
type behaviour struct {
	status   int    // answer with this status and an empty body
	openErr  error  // the request cannot be made at all
	cutAfter int    // deliver this many bytes of the answer and then fail (0 = deliver all)
	eofEarly bool   // cut with a clean end of the stream instead of an error
	ignore   bool   // ignore the Range header and answer 200 with the whole file
	badRange string // answer 206 with this Content-Range
}

// scripted is a file server with range support; behaviours (optional) say how the first requests go wrong, in order. The
// requests after them are answered properly.
func scripted(data []byte, calls *int32, ranges *[]string, behaviours ...behaviour) streamFunc {
	var mu sync.Mutex
	return func(ctx context.Context, rawURL string, opts fetch.StreamOptions) (*fetch.Stream, error) {
		i := int(atomic.AddInt32(calls, 1)) - 1
		rng := opts.Header.Get("Range")
		mu.Lock()
		*ranges = append(*ranges, rng)
		mu.Unlock()
		var b behaviour
		if i < len(behaviours) {
			b = behaviours[i]
		}
		if b.openErr != nil {
			return nil, b.openErr
		}
		if b.status != 0 {
			return &fetch.Stream{Status: b.status, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		}
		if b.ignore || rng == "" {
			return &fetch.Stream{Status: 200, ContentLength: int64(len(data)), Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
		}
		first, last, ok := parseRange(rng, len(data))
		if !ok {
			return &fetch.Stream{Status: 416, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		}
		h := http.Header{}
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", first, last, len(data)))
		if b.badRange != "" {
			h.Set("Content-Range", b.badRange)
		}
		part := data[first : last+1]
		var body io.Reader = bytes.NewReader(part)
		if b.cutAfter > 0 && b.cutAfter < len(part) {
			if b.eofEarly {
				body = bytes.NewReader(part[:b.cutAfter])
			} else {
				body = &breaking{data: part, n: b.cutAfter}
			}
		}
		return &fetch.Stream{Status: 206, ContentLength: int64(len(part)), Header: h, Body: io.NopCloser(body)}, nil
	}
}

// breaking is a body that delivers n bytes and then fails like a dropped connection.
type breaking struct {
	data []byte
	n    int
	pos  int
}

func (b *breaking) Read(p []byte) (int, error) {
	if b.pos >= b.n {
		return 0, errors.New("connection reset by peer")
	}
	end := b.pos + len(p)
	if end > b.n {
		end = b.n
	}
	c := copy(p, b.data[b.pos:end])
	b.pos += c
	return c, nil
}

// blocking delivers the first n bytes and then waits until the request is cancelled.
type blocking struct {
	ctx  context.Context
	data []byte
	n    int
	pos  int
}

func (b *blocking) Read(p []byte) (int, error) {
	if b.pos < b.n {
		end := b.pos + len(p)
		if end > b.n {
			end = b.n
		}
		c := copy(p, b.data[b.pos:end])
		b.pos += c
		return c, nil
	}
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func newManager(s Streamer, mods ...func(*Options)) *Manager {
	o := Options{Fetcher: s, StallAfter: 30 * time.Millisecond, StallFailAfter: 400 * time.Millisecond, MaxRetries: 3, RetryBackoff: func(int) time.Duration { return time.Millisecond }}
	for _, m := range mods {
		m(&o)
	}
	return New(o)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func finished(m *Manager, id string) func() bool {
	return func() bool {
		v, ok := m.Get(id)
		return ok && !v.State.Active()
	}
}

func run(t *testing.T, m *Manager, spec Spec) Progress {
	t.Helper()
	id, err := m.Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the job to finish", finished(m, id))
	v, _ := m.Get(id)
	return v.Progress
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// ---- the tests ----

func TestAGoodFileIsDownloadedVerifiedAndPutInPlace(t *testing.T) {
	data := content(3<<20 + 123)
	var calls int32
	var ranges []string
	spec := specFor(t, data)
	var states []State
	var m *Manager
	var id string
	spec.Install = func(path string) error {
		v, _ := m.Get(id)
		states = append(states, v.State)
		if path != spec.Dest {
			t.Errorf("install got %q, want the destination", path)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, data) {
			t.Errorf("the file must be in place and complete when Install runs (err %v)", err)
		}
		return nil
	}
	m = newManager(serve(data, &calls, &ranges))
	var err error
	id, err = m.Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "done", finished(m, id))
	v, _ := m.Get(id)
	if v.State != StateDone || v.BytesDone != int64(len(data)) || v.BytesTotal != int64(len(data)) || v.Error != "" || v.FinishedAt == "" || v.StartedAt == "" {
		t.Errorf("progress = %+v", v.Progress)
	}
	if len(states) != 1 || states[0] != StateInstalling {
		t.Errorf("Install ran in states %v, want [installing]", states)
	}
	got, err := os.ReadFile(spec.Dest)
	if err != nil || !bytes.Equal(got, data) {
		t.Errorf("destination content wrong (err %v)", err)
	}
	if fileExists(spec.Dest + ".part") {
		t.Error("the part file must be gone after the rename")
	}
	if want := fmt.Sprintf("bytes=0-%d", len(data)-1); len(ranges) != 1 || ranges[0] != want {
		t.Errorf("ranges = %v, want one request for %s", ranges, want)
	}
}

func TestAWrongChecksumDeletesTheFileAndFails(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	spec.SHA256 = sum([]byte("something else"))
	var calls int32
	var ranges []string
	installed := false
	spec.Install = func(string) error { installed = true; return nil }
	p := run(t, newManager(serve(data, &calls, &ranges)), spec)
	if p.State != StateFailed || !strings.Contains(p.Error, "does not match its checksum") {
		t.Errorf("progress = %+v", p)
	}
	if fileExists(spec.Dest) || fileExists(spec.Dest+".part") {
		t.Error("a file that fails its checksum must not stay anywhere")
	}
	if installed {
		t.Error("nothing may be installed from an unchecked file")
	}
}

func TestTheFileIsRenamedOnlyAfterTheChecksumMatched(t *testing.T) {
	data := content(2 << 20)
	spec := specFor(t, data)
	var calls int32
	var ranges []string
	var m *Manager
	var sawDest, sawPart bool
	m = newManager(serve(data, &calls, &ranges), func(o *Options) {
		o.onVerify = func(id string) {
			sawDest = fileExists(spec.Dest)
			sawPart = fileExists(spec.Dest + ".part")
			v, _ := m.Get(id)
			if v.State != StateVerifying || v.BytesTotal != int64(len(data)) {
				t.Errorf("during verification: %+v", v.Progress)
			}
		}
	})
	if p := run(t, m, spec); p.State != StateDone {
		t.Fatalf("progress = %+v", p)
	}
	if sawDest || !sawPart {
		t.Errorf("while verifying: destination exists %v, part exists %v, want false and true", sawDest, sawPart)
	}
}

func TestTheServerSendingMoreThanAnnouncedIsCutOff(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	over := append(append([]byte{}, data...), content(5000)...)
	s := streamFunc(func(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
		return &fetch.Stream{Status: 200, ContentLength: -1, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(over))}, nil
	})
	p := run(t, newManager(s), spec)
	if p.State != StateFailed || !strings.Contains(p.Error, "more data than the announced size") {
		t.Errorf("progress = %+v", p)
	}
	if fileExists(spec.Dest) {
		t.Error("no destination file after an overlong transfer")
	}
	if st, err := os.Stat(spec.Dest + ".part"); err != nil || st.Size() != int64(len(data)) {
		t.Errorf("the part file must hold exactly the announced size (err %v)", err)
	}
}

func TestAnAnnouncedLengthThatDiffersFromTheExpectedSizeIsRefused(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	s := streamFunc(func(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
		return &fetch.Stream{Status: 200, ContentLength: int64(len(data)) + 1, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	p := run(t, newManager(s), spec)
	if p.State != StateFailed || !strings.Contains(p.Error, "announces") {
		t.Errorf("progress = %+v", p)
	}
}

func TestAConnectionThatDropsIsReopenedAtTheByteWhereItStopped(t *testing.T) {
	data := content(2<<20 + 77)
	spec := specFor(t, data)
	var calls int32
	var ranges []string
	cut := 1<<20 + 5
	m := newManager(scripted(data, &calls, &ranges, behaviour{cutAfter: cut}))
	p := run(t, m, spec)
	if p.State != StateDone || p.Error != "" || p.Notice != "" {
		t.Fatalf("progress = %+v, want done without the user doing anything", p)
	}
	last := len(data) - 1
	want := []string{fmt.Sprintf("bytes=0-%d", last), fmt.Sprintf("bytes=%d-%d", cut, last)}
	if fmt.Sprint(ranges) != fmt.Sprint(want) {
		t.Errorf("ranges = %v, want %v: the second request must start where the first one broke", ranges, want)
	}
	got, _ := os.ReadFile(spec.Dest)
	if !bytes.Equal(got, data) {
		t.Error("the file differs from the original")
	}
}

func TestEveryBatchIsItsOwnRangedRequest(t *testing.T) {
	data := content(1000000)
	spec := specFor(t, data)
	var calls int32
	var ranges []string
	m := newManager(scripted(data, &calls, &ranges), func(o *Options) { o.BatchBytes = 300000 })
	if p := run(t, m, spec); p.State != StateDone {
		t.Fatalf("progress = %+v", p)
	}
	want := []string{"bytes=0-299999", "bytes=300000-599999", "bytes=600000-899999", "bytes=900000-999999"}
	if fmt.Sprint(ranges) != fmt.Sprint(want) {
		t.Errorf("ranges = %v, want %v", ranges, want)
	}
	got, _ := os.ReadFile(spec.Dest)
	if !bytes.Equal(got, data) {
		t.Error("the file differs from the original")
	}
}

func TestABatchThatBreaksIsContinuedFromTheBreak(t *testing.T) {
	data := content(1000000)
	spec := specFor(t, data)
	var calls int32
	var ranges []string
	m := newManager(scripted(data, &calls, &ranges, behaviour{}, behaviour{cutAfter: 100000}), func(o *Options) { o.BatchBytes = 300000 })
	if p := run(t, m, spec); p.State != StateDone {
		t.Fatalf("progress = %+v", p)
	}
	want := []string{"bytes=0-299999", "bytes=300000-599999", "bytes=400000-699999", "bytes=700000-999999"}
	if fmt.Sprint(ranges) != fmt.Sprint(want) {
		t.Errorf("ranges = %v, want %v: after the break at byte 400000 the next batch starts there, not at 300000", ranges, want)
	}
	got, _ := os.ReadFile(spec.Dest)
	if !bytes.Equal(got, data) {
		t.Error("the file differs from the original")
	}
}

func TestTheUserSeesAReconnectNoticeWhileItWaits(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	var calls int32
	var ranges []string
	m := newManager(scripted(data, &calls, &ranges, behaviour{cutAfter: 400000}), func(o *Options) {
		o.RetryBackoff = func(int) time.Duration { return 300 * time.Millisecond }
		o.StallAfter = 5 * time.Millisecond
	})
	id, _ := m.Start(spec)
	waitFor(t, "the notice", func() bool { v, _ := m.Get(id); return v.Notice != "" })
	time.Sleep(40 * time.Millisecond) // long past StallAfter: the wait for a reconnect is not a stall
	v, _ := m.Get(id)
	if v.State != StateDownloading || v.BytesDone != 400000 || v.Stalled || !strings.Contains(v.Notice, "Trying again in 1 s (attempt 1 of 3)") {
		t.Errorf("while waiting: %+v", v.Progress)
	}
	waitFor(t, "done", finished(m, id))
	if v, _ := m.Get(id); v.State != StateDone || v.Notice != "" {
		t.Errorf("at the end: %+v", v.Progress)
	}
}

func TestRetriesInARowWithoutANewByteEndThenJobKeepingThePart(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	var calls int32
	var ranges []string
	cut := 300000
	// The first request delivers some bytes; every later one fails at once without a byte.
	m := newManager(scripted(data, &calls, &ranges, behaviour{cutAfter: cut}, behaviour{status: 503}, behaviour{status: 503}, behaviour{status: 503}, behaviour{status: 503}, behaviour{status: 503}))
	p := run(t, m, spec)
	if p.State != StateFailed || !strings.Contains(p.Error, "kept failing") || !strings.Contains(p.Error, "status 503") || !strings.Contains(p.Error, "Start the download again") {
		t.Errorf("progress = %+v", p)
	}
	if atomic.LoadInt32(&calls) != 1+4 {
		t.Errorf("%d requests, want the first (which made progress) and then 4: three retries and the one that gives up", calls)
	}
	if PartialBytes(spec.Dest) != int64(cut) || fileExists(spec.Dest) {
		t.Errorf("the part file must keep the %d bytes that arrived", cut)
	}

	// A new manager (a restart of the app) continues from the part file.
	var calls2 int32
	var ranges2 []string
	p = run(t, newManager(scripted(data, &calls2, &ranges2)), spec)
	if p.State != StateDone || len(ranges2) != 1 || ranges2[0] != fmt.Sprintf("bytes=%d-%d", cut, len(data)-1) {
		t.Errorf("second run: %+v ranges %v", p, ranges2)
	}
}

func TestProgressBetweenFailuresStartsTheCountOver(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	var calls int32
	var ranges []string
	// Ten drops in a row, each after new bytes: far more than MaxRetries (3) in total, but never three in a row without progress.
	var bs []behaviour
	for i := 0; i < 10; i++ {
		bs = append(bs, behaviour{cutAfter: 50000})
	}
	if p := run(t, newManager(scripted(data, &calls, &ranges, bs...)), spec); p.State != StateDone {
		t.Errorf("progress = %+v, want done", p)
	}
	if atomic.LoadInt32(&calls) != 11 {
		t.Errorf("%d requests, want 11", calls)
	}
}

func TestOnlyTransientFailuresAreRetried(t *testing.T) {
	data := content(1 << 20)
	cases := map[string]struct {
		b         behaviour
		wantCalls int32
		wantDone  bool
		wantText  string
	}{
		"a 503 once":                           {behaviour{status: 503}, 2, true, ""},
		"a 429 once":                           {behaviour{status: 429}, 2, true, ""},
		"a 408 once":                           {behaviour{status: 408}, 2, true, ""},
		"a 500 once":                           {behaviour{status: 500}, 2, true, ""},
		"a 599 once":                           {behaviour{status: 599}, 2, true, ""},
		"a server that cannot be reached once": {behaviour{openErr: errors.New("the server could not be reached")}, 2, true, ""},
		"a timeout once":                       {behaviour{openErr: errors.New("the server took too long to answer")}, 2, true, ""},
		"a clean early end once":               {behaviour{cutAfter: 1000, eofEarly: true}, 2, true, ""},
		"a 403":                                {behaviour{status: 403}, 1, false, "status 403"},
		"a 404":                                {behaviour{status: 404}, 1, false, "status 404"},
		"a 401":                                {behaviour{status: 401}, 1, false, "status 401"},
		"a 400":                                {behaviour{status: 400}, 1, false, "status 400"},
		"a refused host":                       {behaviour{openErr: &fetch.RefusedError{Reason: "this host is not on the list of allowed download hosts"}}, 1, false, "not on the list"},
		"a local address":                      {behaviour{openErr: errors.New("a public address must not lead to a local address")}, 1, false, "could not start"},
		"a blocked address":                    {behaviour{openErr: errors.New("this address is not allowed")}, 1, false, "could not start"},
		"plain http":                           {behaviour{openErr: errors.New("plain http is only allowed for local addresses")}, 1, false, "could not start"},
		"a redirect with credentials":          {behaviour{openErr: errors.New("a redirect to a link with credentials is not followed")}, 1, false, "could not start"},
		"an invalid address":                   {behaviour{openErr: errors.New("the link is not a valid http(s) address")}, 1, false, "could not start"},
		"credentials in the address":           {behaviour{openErr: errors.New("the link must not contain credentials")}, 1, false, "could not start"},
		"an unreadable range end":              {behaviour{badRange: "bytes 0-x/1048576"}, 1, false, "different file"},
		"a range end before its start":         {behaviour{badRange: "bytes 10-5/1048576"}, 1, false, "different file"},
		"one byte more than asked for":         {behaviour{badRange: "bytes 0-1048576/1048576"}, 1, false, "different file"},
		"too many redirects":                   {behaviour{openErr: errors.New("too many redirects (more than 5)")}, 1, false, "could not start"},
		"another file":                         {behaviour{badRange: "bytes 5-100/1048576"}, 1, false, "different file"},
		"a range for another total":            {behaviour{badRange: "bytes 0-1048575/999"}, 1, false, "different file"},
		"more bytes than asked for":            {behaviour{badRange: "bytes 0-9999999/1048576"}, 1, false, "different file"},
	}
	for name, c := range cases {
		spec := specFor(t, data)
		var calls int32
		var ranges []string
		p := run(t, newManager(scripted(data, &calls, &ranges, c.b)), spec)
		if (p.State == StateDone) != c.wantDone || atomic.LoadInt32(&calls) != c.wantCalls || !strings.Contains(p.Error, c.wantText) {
			t.Errorf("%s: state %s error %q after %d requests, want done=%v after %d containing %q", name, p.State, p.Error, calls, c.wantDone, c.wantCalls, c.wantText)
		}
	}
}

func TestCancelDuringTheWaitToReconnectStopsAtOnce(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	var calls int32
	var ranges []string
	m := newManager(scripted(data, &calls, &ranges, behaviour{cutAfter: 100000}), func(o *Options) {
		o.RetryBackoff = func(int) time.Duration { return time.Hour }
	})
	id, _ := m.Start(spec)
	waitFor(t, "the notice", func() bool { v, _ := m.Get(id); return v.Notice != "" })
	if !m.Cancel(id) {
		t.Fatal("a job that is waiting to reconnect must be cancellable")
	}
	waitFor(t, "cancelled", finished(m, id))
	if v, _ := m.Get(id); v.State != StateCancelled {
		t.Errorf("progress = %+v", v.Progress)
	}
	waitFor(t, "the part file to go", func() bool { return !fileExists(spec.Dest + ".part") })
}

func TestTheBackoffGrowsAndIsCapped(t *testing.T) {
	want := map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 4: 8 * time.Second, 5: 16 * time.Second, 6: 30 * time.Second, 7: 30 * time.Second, 20: 30 * time.Second, 60: 30 * time.Second}
	for attempt, d := range want {
		if got := defaultBackoff(attempt); got != d {
			t.Errorf("defaultBackoff(%d) = %v, want %v", attempt, got, d)
		}
	}
	var asked []int
	data := content(1 << 20)
	var calls int32
	var ranges []string
	m := newManager(scripted(data, &calls, &ranges, behaviour{status: 503}, behaviour{status: 503}, behaviour{status: 503}), func(o *Options) {
		o.RetryBackoff = func(a int) time.Duration { asked = append(asked, a); return time.Millisecond }
	})
	if p := run(t, m, specFor(t, data)); p.State != StateDone {
		t.Fatal(p)
	}
	if fmt.Sprint(asked) != "[1 2 3]" {
		t.Errorf("backoff asked for attempts %v, want [1 2 3] (the attempts in a row)", asked)
	}
}

func TestAServerThatIgnoresTheRangeStartsOverFromTheBeginning(t *testing.T) {
	data := content(1<<20 + 9)
	spec := specFor(t, data)
	if err := os.MkdirAll(filepath.Dir(spec.Dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.Dest+".part", data[:4096], 0o644); err != nil {
		t.Fatal(err)
	}
	s := streamFunc(func(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
		return &fetch.Stream{Status: 200, ContentLength: int64(len(data)), Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	p := run(t, newManager(s), spec)
	if p.State != StateDone {
		t.Fatalf("progress = %+v", p)
	}
	got, _ := os.ReadFile(spec.Dest)
	if !bytes.Equal(got, data) {
		t.Error("the file must be complete and correct, not the old part plus a full copy")
	}
}

func TestARangeThatCannotBeSatisfiedIsRetriedOnceFromScratch(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	if err := os.MkdirAll(filepath.Dir(spec.Dest), 0o755); err != nil {
		t.Fatal(err)
	}
	junk := []byte("garbage that is not a prefix")
	if err := os.WriteFile(spec.Dest+".part", junk, 0o644); err != nil {
		t.Fatal(err)
	}
	var calls int32
	var ranges []string
	p := run(t, newManager(scripted(data, &calls, &ranges, behaviour{status: 416})), spec)
	want := []string{fmt.Sprintf("bytes=%d-%d", len(junk), len(data)-1), fmt.Sprintf("bytes=0-%d", len(data)-1)}
	if p.State != StateDone || fmt.Sprint(ranges) != fmt.Sprint(want) {
		t.Errorf("progress = %+v ranges %v, want done after %v", p, ranges, want)
	}
	got, _ := os.ReadFile(spec.Dest)
	if !bytes.Equal(got, data) {
		t.Error("the file must be the complete original, not the junk plus a copy")
	}

	// A server that answers 416 even from the start cannot be resumed at all: no endless retry.
	spec2 := specFor(t, data)
	if err := os.MkdirAll(filepath.Dir(spec2.Dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec2.Dest+".part", junk, 0o644); err != nil {
		t.Fatal(err)
	}
	var calls2 int32
	var ranges2 []string
	always := []behaviour{{status: 416}, {status: 416}, {status: 416}, {status: 416}}
	if p := run(t, newManager(scripted(data, &calls2, &ranges2, always...)), spec2); p.State != StateFailed || !strings.Contains(p.Error, "cannot resume") || atomic.LoadInt32(&calls2) != 2 {
		t.Errorf("progress = %+v after %d requests, want failed after 2", p, calls2)
	}
}

func TestAResumeAnswerForADifferentFileIsRefused(t *testing.T) {
	data := content(1 << 20)
	for name, header := range map[string]string{"wrong start": "bytes 0-1048575/1048576", "wrong total": "bytes 4096-1048575/999999", "no header": ""} {
		spec := specFor(t, data)
		if err := os.MkdirAll(filepath.Dir(spec.Dest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(spec.Dest+".part", data[:4096], 0o644); err != nil {
			t.Fatal(err)
		}
		s := streamFunc(func(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
			h := http.Header{}
			if header != "" {
				h.Set("Content-Range", header)
			}
			return &fetch.Stream{Status: 206, ContentLength: 100, Header: h, Body: io.NopCloser(bytes.NewReader(data[4096:]))}, nil
		})
		if p := run(t, newManager(s), spec); p.State != StateFailed || !strings.Contains(p.Error, "different file") {
			t.Errorf("%s: progress = %+v", name, p)
		}
	}
}

func TestAnOtherStatusFailsWithItsNumber(t *testing.T) {
	data := content(1 << 20)
	s := streamFunc(func(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
		return &fetch.Stream{Status: 403, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader([]byte("secret body text")))}, nil
	})
	p := run(t, newManager(s), specFor(t, data))
	if p.State != StateFailed || !strings.Contains(p.Error, "status 403") || strings.Contains(p.Error, "secret body") {
		t.Errorf("progress = %+v", p)
	}
}

func TestOnlyAllowedHostsOverHTTPSAreContacted(t *testing.T) {
	data := content(1 << 20)
	var captured func(*url.URL) error
	s := streamFunc(func(ctx context.Context, _ string, opts fetch.StreamOptions) (*fetch.Stream, error) {
		captured = opts.AllowURL
		return &fetch.Stream{Status: 200, ContentLength: int64(len(data)), Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	if p := run(t, newManager(s), specFor(t, data)); p.State != StateDone {
		t.Fatal(p)
	}
	if captured == nil {
		t.Fatal("the manager must pass a host rule to the transport")
	}
	check := func(raw string, wantRefused bool) {
		u, _ := url.Parse(raw)
		err := captured(u)
		var refused *fetch.RefusedError
		if wantRefused != errors.As(err, &refused) {
			t.Errorf("%s: err = %v, want refused=%v", raw, err, wantRefused)
		}
	}
	check("https://models.example.com/x", false)
	check("https://evil.example.org/x", true)
	check("https://models.example.com.evil.org/x", true)
	check("http://models.example.com/x", true) // a redirect to plain http
	// The transport also never reaches a local address for a download.
	s2 := streamFunc(func(ctx context.Context, _ string, opts fetch.StreamOptions) (*fetch.Stream, error) {
		if opts.Policy != fetch.LocalForbidden {
			t.Errorf("policy = %v, want LocalForbidden", opts.Policy)
		}
		return nil, errors.New("this address is not allowed")
	})
	if p := run(t, newManager(s2), specFor(t, data)); p.State != StateFailed || !strings.Contains(p.Error, "could not start") {
		t.Errorf("progress = %+v", p)
	}
}

func TestNotEnoughFreeSpaceFailsBeforeAnyRequest(t *testing.T) {
	data := content(1 << 20)
	var calls int32
	var ranges []string
	m := newManager(serve(data, &calls, &ranges), func(o *Options) {
		o.DiskFree = func(string) (uint64, error) { return uint64(len(data)), nil } // the file alone would fit, the reserve not
	})
	p := run(t, m, specFor(t, data))
	if p.State != StateFailed || !strings.Contains(p.Error, "not enough free disk space") || atomic.LoadInt32(&calls) != 0 {
		t.Errorf("progress = %+v after %d requests", p, calls)
	}
	// Exactly enough (the file plus the reserve) is accepted.
	m = newManager(serve(data, &calls, &ranges), func(o *Options) {
		o.DiskFree = func(string) (uint64, error) { return uint64(len(data)) + 64<<20, nil }
	})
	if p := run(t, m, specFor(t, data)); p.State != StateDone {
		t.Errorf("exactly enough space: %+v", p)
	}
}

func TestOnlyTheMissingPartNeedsFreeSpaceWhenResuming(t *testing.T) {
	data := content(2 << 20)
	spec := specFor(t, data)
	if err := os.MkdirAll(filepath.Dir(spec.Dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.Dest+".part", data[:1<<20], 0o644); err != nil {
		t.Fatal(err)
	}
	var asked uint64
	var calls int32
	var ranges []string
	m := newManager(serve(data, &calls, &ranges), func(o *Options) {
		o.DiskFree = func(string) (uint64, error) { asked++; return 1<<20 + 64<<20, nil } // room for the missing megabyte only
	})
	if p := run(t, m, spec); p.State != StateDone {
		t.Errorf("progress = %+v, want done: only the missing part must fit", p)
	}
}

func TestAnUnknownFreeSpaceDoesNotBlockTheDownload(t *testing.T) {
	data := content(1 << 20)
	var calls int32
	var ranges []string
	m := newManager(serve(data, &calls, &ranges), func(o *Options) {
		o.DiskFree = func(string) (uint64, error) { return 0, errors.New("unknown") }
	})
	if p := run(t, m, specFor(t, data)); p.State != StateDone {
		t.Errorf("progress = %+v", p)
	}
}

func TestAStalledTransferIsShownRetriedAndThenFailsKeepingThePartFile(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	s := streamFunc(func(ctx context.Context, _ string, opts fetch.StreamOptions) (*fetch.Stream, error) {
		first, last, _ := parseRange(opts.Header.Get("Range"), len(data))
		h := http.Header{}
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", first, last, len(data)))
		// Every request delivers 300000 bytes (the first one) or nothing, and then stalls.
		n := 0
		if first == 0 {
			n = 300000
		}
		return &fetch.Stream{Status: 206, ContentLength: int64(last - first + 1), Header: h, Body: io.NopCloser(&blocking{ctx: ctx, data: data[first:], n: n})}, nil
	})
	m := newManager(s, func(o *Options) { o.StallFailAfter = 120 * time.Millisecond; o.MaxRetries = 2 })
	id, err := m.Start(spec)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the stalled flag", func() bool { v, _ := m.Get(id); return v.Stalled && v.State == StateDownloading })
	v, _ := m.Get(id)
	if v.BytesDone != 300000 || v.SecondsLeft != nil || v.Notice != "" {
		t.Errorf("while stalled: %+v", v.Progress)
	}
	waitFor(t, "the failure", finished(m, id))
	v, _ = m.Get(id)
	if v.State != StateFailed || !strings.Contains(v.Error, "kept failing") || !strings.Contains(v.Error, "no data arrived") || !strings.Contains(v.Error, "Start the download again") {
		t.Errorf("progress = %+v", v.Progress)
	}
	if PartialBytes(spec.Dest) != 300000 {
		t.Errorf("the part file must stay for a resume, has %d bytes", PartialBytes(spec.Dest))
	}
}

func TestCancelStopsTheTransferAndDeletesThePartFile(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	s := streamFunc(func(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
		return &fetch.Stream{Status: 200, ContentLength: int64(len(data)), Header: http.Header{}, Body: io.NopCloser(&blocking{ctx: ctx, data: data, n: 100000})}, nil
	})
	m := newManager(s, func(o *Options) { o.StallFailAfter = time.Minute })
	id, _ := m.Start(spec)
	waitFor(t, "some bytes", func() bool { v, _ := m.Get(id); return v.BytesDone == 100000 })
	if !m.Cancel(id) {
		t.Fatal("cancelling an active job must report true")
	}
	waitFor(t, "cancelled", finished(m, id))
	v, _ := m.Get(id)
	if v.State != StateCancelled || v.Error != "" {
		t.Errorf("progress = %+v", v.Progress)
	}
	waitFor(t, "the part file to go", func() bool { return !fileExists(spec.Dest + ".part") })
	if m.Cancel(id) {
		t.Error("cancelling a finished job must report false")
	}
	if m.Cancel("no-such-job") {
		t.Error("cancelling an unknown job must report false")
	}
}

func TestCancelWhileVerifyingDeletesThePartFile(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	var calls int32
	var ranges []string
	var m *Manager
	var once sync.Once
	m = newManager(serve(data, &calls, &ranges), func(o *Options) {
		o.onVerify = func(id string) { once.Do(func() { m.Cancel(id) }) }
	})
	p := run(t, m, spec)
	if p.State != StateCancelled || fileExists(spec.Dest+".part") || fileExists(spec.Dest) {
		t.Errorf("progress = %+v, part exists %v, dest exists %v", p, fileExists(spec.Dest+".part"), fileExists(spec.Dest))
	}
}

func TestTwoStartsOfTheSameFileShareOneJob(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	release := make(chan struct{})
	var calls int32
	s := streamFunc(func(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
		atomic.AddInt32(&calls, 1)
		<-release
		return &fetch.Stream{Status: 200, ContentLength: int64(len(data)), Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	m := newManager(s)
	a, _ := m.Start(spec)
	b, _ := m.Start(spec)
	if a != b {
		t.Errorf("ids %s and %s: a second start of an active download must join it", a, b)
	}
	close(release)
	waitFor(t, "done", finished(m, a))
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("%d requests, want 1", calls)
	}
	// Once it has finished, a start is a new job (that finds the file already in place).
	c, _ := m.Start(spec)
	if c == a {
		t.Error("a start after the job finished must be a new job")
	}
	waitFor(t, "second done", finished(m, c))
	if atomic.LoadInt32(&calls) != 1 {
		t.Error("a file already in place and correct must not be downloaded again")
	}
}

func TestAFileInPlaceWithTheWrongContentIsReplaced(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	if err := os.MkdirAll(filepath.Dir(spec.Dest), 0o755); err != nil {
		t.Fatal(err)
	}
	wrong := content(1 << 20)
	wrong[500] ^= 0xff
	if err := os.WriteFile(spec.Dest, wrong, 0o644); err != nil {
		t.Fatal(err)
	}
	var calls int32
	var ranges []string
	if p := run(t, newManager(serve(data, &calls, &ranges)), spec); p.State != StateDone || atomic.LoadInt32(&calls) != 1 {
		t.Errorf("progress = %+v after %d requests", p, calls)
	}
	got, _ := os.ReadFile(spec.Dest)
	if !bytes.Equal(got, data) {
		t.Error("the wrong file must have been replaced")
	}
}

func TestOnlyAsManyJobsAsSlotsTransferAtOnce(t *testing.T) {
	data := content(1 << 20)
	release := make(chan struct{})
	s := streamFunc(func(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &fetch.Stream{Status: 200, ContentLength: int64(len(data)), Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	m := newManager(s, func(o *Options) { o.MaxConcurrent = 1 })
	first, _ := m.Start(specFor(t, data))
	second, _ := m.Start(specFor(t, data))
	waitFor(t, "the first to download", func() bool { v, _ := m.Get(first); return v.State == StateDownloading })
	time.Sleep(30 * time.Millisecond)
	if v, _ := m.Get(second); v.State != StateQueued {
		t.Errorf("second = %v, want queued while the only slot is taken", v.State)
	}
	close(release)
	waitFor(t, "both done", func() bool { return finished(m, first)() && finished(m, second)() })
	for _, id := range []string{first, second} {
		if v, _ := m.Get(id); v.State != StateDone {
			t.Errorf("%s: %+v", id, v.Progress)
		}
	}
}

func TestAQueuedJobCanBeCancelled(t *testing.T) {
	data := content(1 << 20)
	block := make(chan struct{})
	s := streamFunc(func(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return nil, errors.New("unused")
	})
	m := newManager(s, func(o *Options) { o.MaxConcurrent = 1 })
	first, _ := m.Start(specFor(t, data))
	second, _ := m.Start(specFor(t, data))
	waitFor(t, "first downloading", func() bool { v, _ := m.Get(first); return v.State == StateDownloading })
	if !m.Cancel(second) {
		t.Fatal("a queued job must be cancellable")
	}
	waitFor(t, "second cancelled", finished(m, second))
	if v, _ := m.Get(second); v.State != StateCancelled {
		t.Errorf("second = %+v", v.Progress)
	}
	m.Cancel(first)
	waitFor(t, "first cancelled", finished(m, first))
}

func TestAFailingInstallFailsTheJobButKeepsTheFile(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	spec.Install = func(string) error { return errors.New("not a valid archive") }
	var calls int32
	var ranges []string
	p := run(t, newManager(serve(data, &calls, &ranges)), spec)
	if p.State != StateFailed || !strings.Contains(p.Error, "installing the file failed: not a valid archive") {
		t.Errorf("progress = %+v", p)
	}
	if !fileExists(spec.Dest) {
		t.Error("the checked file stays")
	}
}

func TestProgressNumbersFollowTheClock(t *testing.T) {
	data := content(10000)
	spec := specFor(t, data)
	var now atomic.Int64
	now.Store(t0.UnixNano())
	chunks := make(chan int)
	body := &chunked{data: data, next: chunks}
	s := streamFunc(func(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
		return &fetch.Stream{Status: 200, ContentLength: int64(len(data)), Header: http.Header{}, Body: io.NopCloser(body)}, nil
	})
	m := newManager(s, func(o *Options) {
		o.Clock = func() time.Time { return time.Unix(0, now.Load()).UTC() }
		o.StallAfter = time.Hour
		o.StallFailAfter = time.Hour
	})
	id, _ := m.Start(spec)

	// 1000 bytes arrive at the start of every second.
	for sec := 0; sec <= 3; sec++ {
		now.Store(t0.Add(time.Duration(sec) * time.Second).UnixNano())
		chunks <- 1000
		want := int64(sec+1) * 1000
		waitFor(t, "the chunk to be counted", func() bool { v, _ := m.Get(id); return v.BytesDone == want })
		if sec == 0 {
			if v, _ := m.Get(id); v.BytesPerSecond != 0 || v.SecondsLeft != nil {
				t.Errorf("at the first byte: %+v, want no speed and no time left yet", v.Progress)
			}
		}
	}
	v, _ := m.Get(id)
	if v.State != StateDownloading || v.BytesDone != 4000 || v.BytesTotal != 10000 {
		t.Fatalf("progress = %+v", v.Progress)
	}
	if v.BytesPerSecond < 900 || v.BytesPerSecond > 1100 {
		t.Errorf("speed = %v, want about 1000 B/s", v.BytesPerSecond)
	}
	if v.SecondsLeft == nil || *v.SecondsLeft < 5 || *v.SecondsLeft > 7 {
		t.Errorf("secondsLeft = %v, want about 6 (6000 bytes at 1000 B/s)", v.SecondsLeft)
	}
	close(chunks)
	waitFor(t, "done", finished(m, id))
}

// chunked delivers the data in chunks whose size the test chooses; closing the channel delivers the rest.
type chunked struct {
	data []byte
	pos  int
	next chan int
}

func (c *chunked) Read(p []byte) (int, error) {
	n, ok := <-c.next
	if !ok {
		if c.pos < len(c.data) {
			n = len(c.data) - c.pos
		} else {
			return 0, io.EOF
		}
	}
	if n > len(p) {
		n = len(p)
	}
	copy(p, c.data[c.pos:c.pos+n])
	c.pos += n
	return n, nil
}

func TestFinishedJobsStayListedForTheRetentionTimeAndThenGo(t *testing.T) {
	data := content(1 << 20)
	var calls int32
	var ranges []string
	var now atomic.Int64
	now.Store(t0.UnixNano())
	m := newManager(serve(data, &calls, &ranges), func(o *Options) {
		o.Clock = func() time.Time { return time.Unix(0, now.Load()).UTC() }
		o.Retention = 10 * time.Minute
	})
	id, _ := m.Start(specFor(t, data))
	waitFor(t, "done", finished(m, id))
	now.Store(t0.Add(9 * time.Minute).UnixNano())
	if _, ok := m.Get(id); !ok {
		t.Error("a finished job must still be there inside the retention time")
	}
	if len(m.List()) != 1 {
		t.Error("and listed")
	}
	now.Store(t0.Add(11 * time.Minute).UnixNano())
	if _, ok := m.Get(id); ok {
		t.Error("a finished job must be forgotten after the retention time")
	}
	if len(m.List()) != 0 {
		t.Error("and not listed")
	}
}

func TestListIsNewestFirstAndCarriesTheScope(t *testing.T) {
	data := content(1 << 20)
	var calls int32
	var ranges []string
	var now atomic.Int64
	now.Store(t0.UnixNano())
	m := newManager(serve(data, &calls, &ranges), func(o *Options) {
		o.Clock = func() time.Time { return time.Unix(0, now.Load()).UTC() }
	})
	a := specFor(t, data)
	a.Label = "first"
	idA, _ := m.Start(a)
	waitFor(t, "a", finished(m, idA))
	now.Store(t0.Add(time.Minute).UnixNano())
	b := specFor(t, data)
	b.Label = "second"
	b.Scope = "cinqo:other"
	idB, _ := m.Start(b)
	waitFor(t, "b", finished(m, idB))
	list := m.List()
	if len(list) != 2 || list[0].Label != "second" || list[1].Label != "first" {
		t.Fatalf("list = %+v", list)
	}
	if list[0].Scope != "cinqo:other" || list[1].Scope != "cinqo:platform:manage" {
		t.Errorf("scopes = %q %q", list[0].Scope, list[1].Scope)
	}
	if _, ok := m.Get("unknown"); ok {
		t.Error("an unknown id must not be found")
	}
}

func TestASpecThatCannotWorkIsRefusedWithoutStartingAnything(t *testing.T) {
	data := content(1 << 20)
	var calls int32
	var ranges []string
	m := newManager(serve(data, &calls, &ranges))
	good := specFor(t, data)
	bad := map[string]func(*Spec){
		"no label":            func(s *Spec) { s.Label = " " },
		"a long label":        func(s *Spec) { s.Label = strings.Repeat("l", 101) },
		"a relative path":     func(s *Spec) { s.Dest = "models/m.gguf" },
		"an unclean path":     func(s *Spec) { s.Dest = filepath.Join(filepath.Dir(s.Dest), "..", "m.gguf") + "/." },
		"a path ending in /":  func(s *Spec) { s.Dest += "/" },
		"no size":             func(s *Spec) { s.SizeBytes = 0 },
		"a negative size":     func(s *Spec) { s.SizeBytes = -1 },
		"a size over the cap": func(s *Spec) { s.SizeBytes = MaxBytes + 1 },
		"an upper-case hash":  func(s *Spec) { s.SHA256 = strings.ToUpper(s.SHA256) },
		"a short hash":        func(s *Spec) { s.SHA256 = "abc" },
		"no host rule":        func(s *Spec) { s.AllowHost = nil },
		"no scope":            func(s *Spec) { s.Scope = "" },
		"an http address":     func(s *Spec) { s.URL = strings.Replace(s.URL, "https", "http", 1) },
		"credentials":         func(s *Spec) { s.URL = "https://u:p@models.example.com/x" },
		"a host not allowed":  func(s *Spec) { s.URL = "https://evil.example.org/x" },
		"not a URL":           func(s *Spec) { s.URL = "::" },
		"a file address":      func(s *Spec) { s.URL = "file:///etc/passwd" },
	}
	for name, mutate := range bad {
		s := good
		mutate(&s)
		if _, err := m.Start(s); !errors.Is(err, ErrInvalidSpec) {
			t.Errorf("%s: err = %v, want ErrInvalidSpec", name, err)
		}
	}
	if atomic.LoadInt32(&calls) != 0 || len(m.List()) != 0 {
		t.Error("a refused spec must not start anything")
	}
	atMax := good
	atMax.SizeBytes = MaxBytes
	if _, err := m.Start(atMax); err != nil {
		t.Errorf("a size exactly at the cap must be accepted: %v", err)
	}
	m.Cancel(m.List()[0].ID)
}

func TestHumanBytes(t *testing.T) {
	cases := map[uint64]string{0: "0 MiB", 1 << 20: "1 MiB", 1536 << 10: "2 MiB", 1 << 30: "1.0 GiB", 3<<30 + 1<<29: "3.5 GiB"}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestACompletePartFileIsCheckedWithoutAnyRequest(t *testing.T) {
	data := content(1<<20 + 11)
	spec := specFor(t, data)
	if err := os.MkdirAll(filepath.Dir(spec.Dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.Dest+".part", data, 0o644); err != nil {
		t.Fatal(err)
	}
	var calls int32
	var ranges []string
	if p := run(t, newManager(serve(data, &calls, &ranges)), spec); p.State != StateDone {
		t.Fatalf("progress = %+v", p)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Errorf("%d requests, want none for a part file that already has every byte", calls)
	}
	got, _ := os.ReadFile(spec.Dest)
	if !bytes.Equal(got, data) {
		t.Error("the file must be put in place")
	}

	// A part file with more bytes than the file can have is thrown away and downloaded again.
	spec2 := specFor(t, data)
	if err := os.MkdirAll(filepath.Dir(spec2.Dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec2.Dest+".part", append(append([]byte{}, data...), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	calls = 0
	ranges = nil
	if p := run(t, newManager(serve(data, &calls, &ranges)), spec2); p.State != StateDone || atomic.LoadInt32(&calls) != 1 || ranges[0] != fmt.Sprintf("bytes=0-%d", len(data)-1) {
		t.Errorf("progress = %+v after %d requests with ranges %v, want done after one request from byte 0", p, calls, ranges)
	}
}

func TestAFileInPlaceOfTheWrongSizeIsNotEvenHashed(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	if err := os.MkdirAll(filepath.Dir(spec.Dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.Dest, data[:1000], 0o644); err != nil {
		t.Fatal(err)
	}
	var verifications int32
	var calls int32
	var ranges []string
	m := newManager(serve(data, &calls, &ranges), func(o *Options) { o.onVerify = func(string) { atomic.AddInt32(&verifications, 1) } })
	if p := run(t, m, spec); p.State != StateDone {
		t.Fatalf("progress = %+v", p)
	}
	if got := atomic.LoadInt32(&verifications); got != 1 {
		t.Errorf("%d verifications, want 1 (the new download only: a file of the wrong size is replaced without reading it)", got)
	}
}

func TestAStreamThatKeepsEndingCleanlyEarlyFailsAndKeepsThePart(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	// A whole-file answer (200) that ends cleanly at 700000 bytes, whatever is asked: the first request makes progress,
	// the retries get nothing new.
	s := streamFunc(func(ctx context.Context, _ string, opts fetch.StreamOptions) (*fetch.Stream, error) {
		first, last, _ := parseRange(opts.Header.Get("Range"), len(data))
		if first >= 700000 {
			return &fetch.Stream{Status: 503, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		}
		h := http.Header{}
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", first, last, len(data)))
		return &fetch.Stream{Status: 206, ContentLength: int64(last - first + 1), Header: h, Body: io.NopCloser(bytes.NewReader(data[first:700000]))}, nil
	})
	p := run(t, newManager(s), spec)
	if p.State != StateFailed || !strings.Contains(p.Error, "kept failing") || !strings.Contains(p.Error, "Start the download again") {
		t.Errorf("progress = %+v", p)
	}
	if PartialBytes(spec.Dest) != 700000 || fileExists(spec.Dest) {
		t.Errorf("the part file must keep what arrived (%d bytes) and nothing may be in place", PartialBytes(spec.Dest))
	}
}

func TestTheTimeLeftIsRoundedUpAndOnlyKnownWhileBytesAreMissing(t *testing.T) {
	now := at(1)
	m := New(Options{Clock: func() time.Time { return now }})
	r := newRate()
	r.add(at(0), 0)
	r.add(at(1), 1000)
	j := &job{id: "x", spec: Spec{Label: "l", Scope: "s"}, state: StateDownloading, done: 4500, total: 10000, lastProgress: at(1), rate: r}
	m.jobs["x"] = j
	v, _ := m.Get("x")
	if v.BytesPerSecond != 1000 || v.SecondsLeft == nil || *v.SecondsLeft != 6 {
		t.Errorf("speed %v left %v, want 1000 B/s and 6 seconds (5.5 rounded up)", v.BytesPerSecond, v.SecondsLeft)
	}
	j.done = 9999
	if v, _ := m.Get("x"); v.SecondsLeft == nil || *v.SecondsLeft != 1 {
		t.Errorf("left = %v, want 1", v.SecondsLeft)
	}
	j.done = 10000
	if v, _ := m.Get("x"); v.SecondsLeft != nil {
		t.Errorf("left = %v, want unknown once every byte is there", *v.SecondsLeft)
	}
	j.done, j.state = 4500, StateVerifying
	if v, _ := m.Get("x"); v.SecondsLeft != nil || v.BytesPerSecond != 0 || v.Stalled {
		t.Errorf("while verifying there is no speed, no time left and no stall: %+v", v.Progress)
	}
}

func TestTheLastBatchNeverAsksForBytesBeyondTheFile(t *testing.T) {
	data := content(1000000)
	var calls int32
	var ranges []string
	m := newManager(scripted(data, &calls, &ranges), func(o *Options) { o.BatchBytes = 250100 })
	if p := run(t, m, specFor(t, data)); p.State != StateDone {
		t.Fatalf("progress = %+v", p)
	}
	want := []string{"bytes=0-250099", "bytes=250100-500199", "bytes=500200-750299", "bytes=750300-999999"}
	if fmt.Sprint(ranges) != fmt.Sprint(want) {
		t.Errorf("ranges = %v, want %v: the last one stops at the last byte of the file", ranges, want)
	}
}

func TestAnAnswerThatIsEmptyAndEndsCleanlyKeepsFailingInsteadOfLooping(t *testing.T) {
	data := content(1 << 20)
	var calls int32
	s := streamFunc(func(ctx context.Context, _ string, opts fetch.StreamOptions) (*fetch.Stream, error) {
		atomic.AddInt32(&calls, 1)
		first, last, _ := parseRange(opts.Header.Get("Range"), len(data))
		h := http.Header{}
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", first, last, len(data)))
		return &fetch.Stream{Status: 206, ContentLength: int64(last - first + 1), Header: h, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})
	p := run(t, newManager(s), specFor(t, data))
	if p.State != StateFailed || !strings.Contains(p.Error, "kept failing") || !strings.Contains(p.Error, "ended early") {
		t.Errorf("progress = %+v", p)
	}
	if got := atomic.LoadInt32(&calls); got != 4 {
		t.Errorf("%d requests, want 4 (the first and three retries)", got)
	}
}

func TestTheDefaultsAreSensibleForAMultiGigabyteDownload(t *testing.T) {
	m := New(Options{})
	if m.opt.BatchBytes != 256<<20 || m.opt.MaxRetries != 6 || m.opt.MaxConcurrent != 2 || m.opt.Retention != 15*time.Minute ||
		m.opt.StallAfter != 15*time.Second || m.opt.StallFailAfter != time.Minute || m.opt.ReserveBytes != 64<<20 {
		t.Errorf("defaults = %+v", m.opt)
	}
	if m.opt.RetryBackoff(1) != time.Second || m.opt.RetryBackoff(3) != 4*time.Second {
		t.Error("the default backoff must start at a second and double")
	}
}

func TestActiveFindsTheRunningJobOfAKeyAndNothingElse(t *testing.T) {
	data := content(1 << 20)
	spec := specFor(t, data)
	release := make(chan struct{})
	s := streamFunc(func(ctx context.Context, _ string, _ fetch.StreamOptions) (*fetch.Stream, error) {
		<-release
		return &fetch.Stream{Status: 200, ContentLength: int64(len(data)), Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	m := newManager(s)
	if _, ok := m.Active(spec.Dest); ok {
		t.Error("nothing is running yet")
	}
	id, _ := m.Start(spec)
	if got, ok := m.Active(spec.Dest); !ok || got != id {
		t.Errorf("Active = %q %v, want the running job %s", got, ok, id)
	}
	if _, ok := m.Active(spec.Dest + "-other"); ok {
		t.Error("another key must not find it")
	}
	custom := specFor(t, data)
	custom.Key = "my-key"
	cid, _ := m.Start(custom)
	if got, ok := m.Active("my-key"); !ok || got != cid {
		t.Errorf("a spec with its own key must be found by it: %q %v", got, ok)
	}
	close(release)
	waitFor(t, "done", finished(m, id))
	waitFor(t, "done", finished(m, cid))
	if _, ok := m.Active(spec.Dest); ok {
		t.Error("a finished job is not active")
	}
}

func TestParseContentRangeEnd(t *testing.T) {
	good := map[string][2]int64{"bytes 5-9/10": {5, 9}, "bytes 0-0/1": {0, 0}, " bytes 7-9/*": {7, 9}}
	for in, want := range good {
		from, to, ok := parseContentRangeEnd(in)
		if !ok || from != want[0] || to != want[1] {
			t.Errorf("%q: %d %d %v, want %v", in, from, to, ok, want)
		}
	}
	for _, in := range []string{"", "5-9/10", "bytes 5-9", "bytes x-9/10", "bytes 5-x/10", "bytes 9-5/10", "bytes -5/10", "bytes 5-/10", "items 5-9/10"} {
		if _, _, ok := parseContentRangeEnd(in); ok {
			t.Errorf("%q must not parse", in)
		}
	}
}
