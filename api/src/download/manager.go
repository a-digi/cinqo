package download

import (
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
	"time"

	"github.com/google/uuid"

	"github.com/a-digi/cinqo/src/toolrepository/fetch"
)

// Streamer opens a guarded streaming download; the app's fetch.Fetcher is the real one.
type Streamer interface {
	Stream(ctx context.Context, rawURL string, opts fetch.StreamOptions) (*fetch.Stream, error)
}

// Options configure a Manager. Zero values take the defaults; the function fields exist so tests can replace them.
type Options struct {
	Fetcher        Streamer
	Clock          func() time.Time
	DiskFree       func(dir string) (uint64, error)
	NewID          func() string
	MaxConcurrent  int                             // jobs transferring at the same time (default 2); the others wait as "queued"
	Retention      time.Duration                   // how long a finished job stays listed (default 15 minutes)
	StallAfter     time.Duration                   // no byte for this long shows as "stalled" (default 15 seconds)
	StallFailAfter time.Duration                   // no byte for this long ends the transfer (default 60 seconds)
	ReserveBytes   uint64                          // free space that must remain after the download (default 64 MiB)
	BatchBytes     int64                           // the file is fetched in ranged requests of at most this size (default 256 MiB)
	MaxRetries     int                             // reconnects in a row without a single new byte before the job fails (default 6)
	RetryBackoff   func(attempt int) time.Duration // the wait before reconnect number attempt (default 1 s doubling up to 30 s)
	// onVerify is a seam for tests: called when a job has entered the verifying phase, before the file is read.
	onVerify func(id string)
}

// Manager runs download jobs. It is safe for concurrent use.
type Manager struct {
	opt   Options
	slots chan struct{}

	mu    sync.Mutex
	jobs  map[string]*job
	byKey map[string]string
}

// New returns a Manager with its defaults filled in.
func New(opt Options) *Manager {
	if opt.Fetcher == nil {
		opt.Fetcher = fetch.New()
	}
	if opt.Clock == nil {
		opt.Clock = time.Now
	}
	if opt.DiskFree == nil {
		opt.DiskFree = diskFree
	}
	if opt.NewID == nil {
		opt.NewID = uuid.NewString
	}
	if opt.MaxConcurrent <= 0 {
		opt.MaxConcurrent = 2
	}
	if opt.Retention <= 0 {
		opt.Retention = 15 * time.Minute
	}
	if opt.StallAfter <= 0 {
		opt.StallAfter = 15 * time.Second
	}
	if opt.StallFailAfter <= 0 {
		opt.StallFailAfter = 60 * time.Second
	}
	if opt.ReserveBytes == 0 {
		opt.ReserveBytes = 64 << 20
	}
	if opt.BatchBytes <= 0 {
		opt.BatchBytes = 256 << 20
	}
	if opt.MaxRetries <= 0 {
		opt.MaxRetries = 6
	}
	if opt.RetryBackoff == nil {
		opt.RetryBackoff = defaultBackoff
	}
	return &Manager{opt: opt, slots: make(chan struct{}, opt.MaxConcurrent), jobs: map[string]*job{}, byKey: map[string]string{}}
}

type job struct {
	id   string
	spec Spec

	mu           sync.Mutex
	state        State
	done, total  int64
	errMsg       string
	started      time.Time
	finished     time.Time
	lastProgress time.Time
	rate         *rate
	received     int64  // bytes received in this run, for the speed
	notice       string // what is going on besides downloading ("The connection was lost. Trying again in 4 s…")
	cancel       context.CancelFunc
	userCancel   bool
}

// Start begins downloading spec and returns the job id at once. If a job for the same key is still active, its id is
// returned and nothing new starts.
func (m *Manager) Start(spec Spec) (string, error) {
	if err := spec.validate(); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked()
	if id, ok := m.byKey[spec.key()]; ok {
		if j := m.jobs[id]; j != nil {
			j.mu.Lock()
			active := j.state.Active()
			j.mu.Unlock()
			if active {
				return id, nil
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := m.opt.Clock()
	j := &job{id: m.opt.NewID(), spec: spec, state: StateQueued, total: spec.SizeBytes, started: now, lastProgress: now, rate: newRate(), cancel: cancel}
	m.jobs[j.id] = j
	m.byKey[spec.key()] = j.id
	go m.run(ctx, j)
	return j.id, nil
}

// Active returns the id of the running job for a key (the destination path unless the spec named another), so a page that
// is opened while a download is under way can find it again.
func (m *Manager) Active(key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byKey[key]
	if !ok {
		return "", false
	}
	j := m.jobs[id]
	if j == nil {
		return "", false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return id, j.state.Active()
}

// Latest returns the most recent job started for a key, finished or not (a finished job stays listed for the retention time):
// how a caller finds out why the last attempt ended, which Active cannot say.
func (m *Manager) Latest(key string) (View, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked()
	id, ok := m.byKey[key]
	if !ok {
		return View{}, false
	}
	j := m.jobs[id]
	if j == nil {
		return View{}, false
	}
	return m.view(j), true
}

// View is a job's progress together with the scope needed to see it.
type View struct {
	Progress
	Scope string `json:"-"`
}

// Get returns one job.
func (m *Manager) Get(id string) (View, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked()
	j := m.jobs[id]
	if j == nil {
		return View{}, false
	}
	return m.view(j), true
}

// List returns every job still listed, newest first.
func (m *Manager) List() []View {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked()
	out := make([]View, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, m.view(j))
	}
	for i := 1; i < len(out); i++ {
		for k := i; k > 0 && out[k].StartedAt > out[k-1].StartedAt; k-- {
			out[k], out[k-1] = out[k-1], out[k]
		}
	}
	return out
}

// Cancel stops an active job and deletes its partial file. It reports whether there was an active job to cancel.
func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	j := m.jobs[id]
	m.mu.Unlock()
	if j == nil {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.state.Active() {
		return false
	}
	j.userCancel = true
	j.cancel()
	return true
}

func (m *Manager) view(j *job) View {
	j.mu.Lock()
	defer j.mu.Unlock()
	now := m.opt.Clock()
	p := Progress{
		ID: j.id, Label: j.spec.Label, State: j.state,
		BytesDone: j.done, BytesTotal: j.total, Error: j.errMsg, Notice: j.notice,
		StartedAt: formatRFC3339(j.started), FinishedAt: formatRFC3339(j.finished),
	}
	if j.state == StateDownloading {
		p.BytesPerSecond = j.rate.perSecond(now)
		if p.BytesPerSecond > 0 && j.total > j.done {
			left := int64((float64(j.total-j.done) / p.BytesPerSecond) + 0.999)
			p.SecondsLeft = &left
		}
		p.Stalled = j.notice == "" && now.Sub(j.lastProgress) >= m.opt.StallAfter
	}
	return View{Progress: p, Scope: j.spec.Scope}
}

// purgeLocked forgets finished jobs older than the retention time. m.mu must be held.
func (m *Manager) purgeLocked() {
	now := m.opt.Clock()
	for id, j := range m.jobs {
		j.mu.Lock()
		old := !j.state.Active() && !j.finished.IsZero() && now.Sub(j.finished) > m.opt.Retention
		key := j.spec.key()
		j.mu.Unlock()
		if old {
			delete(m.jobs, id)
			if m.byKey[key] == id {
				delete(m.byKey, key)
			}
		}
	}
}

func (j *job) set(f func()) {
	j.mu.Lock()
	f()
	j.mu.Unlock()
}

func (j *job) setState(s State) { j.set(func() { j.state = s }) }

func (m *Manager) run(ctx context.Context, j *job) {
	defer j.cancel()
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		m.finish(j, ctx.Err())
		return
	}
	m.finish(j, m.execute(ctx, j))
}

// failure is an error whose message is safe to show to a person.
type failure struct{ msg string }

func (f failure) Error() string { return f.msg }

func fail(format string, args ...any) error { return failure{msg: fmt.Sprintf(format, args...)} }

func (m *Manager) finish(j *job, err error) {
	j.mu.Lock()
	cancelled := err != nil && (j.userCancel || errors.Is(err, context.Canceled))
	j.mu.Unlock()
	// The partial file of a cancelled job goes first, and the state is published after: whoever sees "cancelled" may start
	// the same download again at once, and the old job must not delete the new one's file.
	if cancelled {
		_ = os.Remove(j.spec.Dest + ".part")
	}
	now := m.opt.Clock()
	j.mu.Lock()
	defer j.mu.Unlock()
	j.finished = now
	switch {
	case err == nil:
		j.state = StateDone
		j.done = j.total
	case cancelled:
		j.state = StateCancelled
	default:
		j.state = StateFailed
		var f failure
		if errors.As(err, &f) {
			j.errMsg = f.msg
		} else {
			j.errMsg = "the download failed"
		}
	}
}

// PartialBytes is the size of the unfinished file of a download to dest, or 0: how much a new start would resume from.
func PartialBytes(dest string) int64 {
	st, err := os.Stat(dest + ".part")
	if err != nil || !st.Mode().IsRegular() {
		return 0
	}
	return st.Size()
}

func (m *Manager) execute(ctx context.Context, j *job) error {
	spec := j.spec
	dir := filepath.Dir(spec.Dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail("the folder for the file could not be created")
	}
	part := spec.Dest + ".part"

	// A finished file of the right size is checked, and kept if it is the right one.
	if st, err := os.Stat(spec.Dest); err == nil {
		if st.Mode().IsRegular() && st.Size() == spec.SizeBytes {
			if ok, verr := m.verify(ctx, j, spec.Dest); verr != nil {
				return verr
			} else if ok {
				return m.install(ctx, j)
			}
		}
		_ = os.Remove(spec.Dest)
	}

	for attempt := 0; ; attempt++ {
		offset := PartialBytes(spec.Dest)
		if offset > spec.SizeBytes {
			_ = os.Remove(part)
			offset = 0
		}
		if offset < spec.SizeBytes {
			if err := m.checkSpace(dir, spec.SizeBytes-offset); err != nil {
				return err
			}
			restart, err := m.transfer(ctx, j, part, offset)
			if err != nil {
				return err
			}
			if restart && attempt == 0 {
				_ = os.Remove(part)
				continue
			}
			if restart {
				return fail("the server cannot resume this download")
			}
		}
		break
	}

	ok, err := m.verify(ctx, j, part)
	if err != nil {
		return err
	}
	if !ok {
		_ = os.Remove(part)
		return fail("the downloaded file does not match its checksum, so it was deleted")
	}
	if err := os.Rename(part, spec.Dest); err != nil {
		return fail("the finished file could not be put in place")
	}
	return m.install(ctx, j)
}

func (m *Manager) install(ctx context.Context, j *job) error {
	if j.spec.Install == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	j.setState(StateInstalling)
	if err := j.spec.Install(j.spec.Dest); err != nil {
		return fail("installing the file failed: %s", err.Error())
	}
	return nil
}

func (m *Manager) checkSpace(dir string, need int64) error {
	free, err := m.opt.DiskFree(dir)
	if err != nil {
		return nil // unknown: the check is skipped, the transfer itself reports a full disk
	}
	if free < uint64(need)+m.opt.ReserveBytes {
		return fail("there is not enough free disk space: %s more are needed and %s are free", humanBytes(uint64(need)), humanBytes(free))
	}
	return nil
}

func humanBytes(n uint64) string {
	const gib, mib = 1 << 30, 1 << 20
	if n >= gib {
		return fmt.Sprintf("%.1f GiB", float64(n)/gib)
	}
	return fmt.Sprintf("%d MiB", (n+mib/2)/mib)
}

func defaultBackoff(attempt int) time.Duration {
	d := time.Second << uint(attempt-1)
	if attempt > 5 || d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}

// transientError marks a failure that is worth trying again: a dropped or stalled connection, a server that could not be
// reached for the moment, a 5xx, 429 or 408 answer.
type transientError struct{ reason string }

func (e transientError) Error() string { return e.reason }

// transfer downloads into part, starting at offset, in ranged batches. A batch that fails for a transient reason is
// retried from the byte where it stopped (the bytes already received are kept), after a short wait that grows with every
// failure in a row; every request starts from the pinned address, so a signed redirect link that expired is replaced by a
// fresh one. The job fails only after MaxRetries failures in a row without a single new byte, or at once for a failure that
// trying again cannot cure (a forbidden host, a 4xx answer, a different file, a full disk). It reports restart when the
// server cannot serve the range, so the partial file should be thrown away once and tried again from the start.
func (m *Manager) transfer(ctx context.Context, j *job, part string, offset int64) (restart bool, err error) {
	spec := j.spec
	j.set(func() {
		j.state = StateDownloading
		j.done = offset
		j.total = spec.SizeBytes
		j.received = 0
		j.lastProgress = m.opt.Clock()
		j.rate = newRate()
		j.notice = ""
	})

	f, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return false, fail("the file could not be written")
	}
	closed := false
	defer func() {
		if !closed {
			f.Close()
		}
	}()

	written := offset
	failures := 0
	for written < spec.SizeBytes {
		got, restartNow, berr := m.fetchBatch(ctx, j, f, written)
		written += got
		if restartNow {
			return true, nil
		}
		if berr == nil {
			failures = 0
			continue
		}
		var tr transientError
		if !errors.As(berr, &tr) {
			return false, berr
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		// Only failures in a row without a single new byte count: a request that brought bytes before it broke starts over.
		if got > 0 {
			failures = 0
		} else {
			failures++
			if failures > m.opt.MaxRetries {
				return false, fail("the connection kept failing (%s) after %s of %s. Start the download again to continue", tr.reason, humanBytes(uint64(written)), humanBytes(uint64(spec.SizeBytes)))
			}
		}
		attempt := failures
		if attempt == 0 {
			attempt = 1
		}
		wait := m.opt.RetryBackoff(attempt)
		j.set(func() {
			j.notice = fmt.Sprintf("The connection was lost. Trying again in %d s (attempt %d of %d)…", int((wait+time.Second-1)/time.Second), attempt, m.opt.MaxRetries)
		})
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	if err := f.Sync(); err != nil {
		return false, fail("the file could not be written (is the disk full?)")
	}
	closed = true
	if err := f.Close(); err != nil {
		return false, fail("the file could not be written")
	}
	return false, nil
}

// fetchBatch asks for the bytes from start (up to one batch) and writes what arrives at that position of f. It returns the
// number of bytes written, restart when the server cannot serve the range, and a transientError or a plain failure.
func (m *Manager) fetchBatch(ctx context.Context, j *job, f *os.File, start int64) (got int64, restart bool, err error) {
	spec := j.spec
	end := start + m.opt.BatchBytes
	if end > spec.SizeBytes {
		end = spec.SizeBytes
	}
	last := end - 1 // the last byte asked for

	streamCtx, stopStream := context.WithCancel(ctx)
	defer stopStream()
	st, serr := m.opt.Fetcher.Stream(streamCtx, spec.URL, fetch.StreamOptions{
		Policy: fetch.LocalForbidden,
		Header: http.Header{"Range": {"bytes=" + strconv.FormatInt(start, 10) + "-" + strconv.FormatInt(last, 10)}},
		AllowURL: func(u *url.URL) error {
			if u.Scheme != "https" {
				return &fetch.RefusedError{Reason: "downloads are only made over https"}
			}
			if !spec.AllowHost(u.Hostname()) {
				return &fetch.RefusedError{Reason: fmt.Sprintf("the host %s is not on the list of allowed download hosts", u.Hostname())}
			}
			return nil
		},
	})
	if serr != nil {
		if ctx.Err() != nil {
			return 0, false, ctx.Err()
		}
		if retryable(serr) {
			return 0, false, transientError{reason: serr.Error()}
		}
		return 0, false, fail("the download could not start: %s", serr.Error())
	}
	defer st.Close()

	// How many bytes this answer is to deliver, and from where it starts.
	var expect int64
	switch st.Status {
	case http.StatusPartialContent:
		from, total, ok := parseContentRange(st.Header.Get("Content-Range"))
		_, to, hasEnd := parseContentRangeEnd(st.Header.Get("Content-Range"))
		if !ok || !hasEnd || from != start || to > last || (total >= 0 && total != spec.SizeBytes) {
			return 0, false, fail("the server answered the resume request with a different file")
		}
		expect = to - from + 1
	case http.StatusOK:
		if start > 0 { // the server ignores ranges: start again from the beginning
			return 0, true, nil
		}
		if st.ContentLength >= 0 && st.ContentLength != spec.SizeBytes {
			return 0, false, fail("the server announces %d bytes but %d were expected", st.ContentLength, spec.SizeBytes)
		}
		expect = spec.SizeBytes
	case http.StatusRequestedRangeNotSatisfiable:
		return 0, true, nil
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return 0, false, transientError{reason: "the server answered with status " + strconv.Itoa(st.Status)}
	default:
		if st.Status >= 500 {
			return 0, false, transientError{reason: "the server answered with status " + strconv.Itoa(st.Status)}
		}
		return 0, false, fail("the server answered with status %d", st.Status)
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return 0, false, fail("the file could not be written")
	}

	// No byte for StallFailAfter ends this request; the bytes already written stay.
	stalled := false
	watchdog := time.AfterFunc(m.opt.StallFailAfter, func() {
		j.set(func() { stalled = true })
		stopStream()
	})
	defer watchdog.Stop()

	buf := make([]byte, 256<<10)
	for got < expect {
		n, rerr := st.Body.Read(buf)
		if n > 0 {
			watchdog.Reset(m.opt.StallFailAfter)
			if int64(n) > expect-got {
				n = int(expect - got)
				if n > 0 {
					_, _ = f.Write(buf[:n])
					got += int64(n)
				}
				return got, false, fail("the server sent more data than the announced size")
			}
			if _, werr := f.Write(buf[:n]); werr != nil {
				return got, false, fail("the file could not be written (is the disk full?)")
			}
			got += int64(n)
			now := m.opt.Clock()
			position := start + got
			j.set(func() {
				j.done = position
				j.lastProgress = now
				j.received += int64(n)
				j.rate.add(now, j.received)
				j.notice = ""
			})
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			if ctx.Err() != nil {
				return got, false, ctx.Err()
			}
			j.mu.Lock()
			wasStalled := stalled
			j.mu.Unlock()
			if wasStalled {
				return got, false, transientError{reason: fmt.Sprintf("no data arrived for %s", m.opt.StallFailAfter.Round(time.Second))}
			}
			return got, false, transientError{reason: "the connection was interrupted"}
		}
	}
	if got < expect {
		return got, false, transientError{reason: "the connection ended early"}
	}
	// A whole-file answer (a server that ignores ranges) is not bounded by what was asked for, so a stream that goes on past
	// the announced size is caught here even when the size is an exact multiple of the read buffer. An error or a stall in
	// this last look changes nothing: every announced byte is there.
	if st.Status == http.StatusOK {
		if n, _ := st.Body.Read(buf[:1]); n > 0 {
			return got, false, fail("the server sent more data than the announced size")
		}
	}
	return got, false, nil
}

// retryable says whether a failure to open a request is worth trying again: the server could not be reached for the moment,
// as opposed to an address that is refused by the rules.
func retryable(err error) bool {
	var refused *fetch.RefusedError
	if errors.As(err, &refused) {
		return false
	}
	for _, permanent := range []string{
		"this address is not allowed",
		"a public address must not lead to a local address",
		"plain http is only allowed for local addresses",
		"too many redirects",
		"redirect to a link with credentials",
		"not a valid http(s) address",
		"must not contain credentials",
	} {
		if strings.Contains(err.Error(), permanent) {
			return false
		}
	}
	return true
}

// parseContentRange reads "bytes 5-9/10" (total -1 for "*").
func parseContentRange(h string) (start, total int64, ok bool) {
	h = strings.TrimSpace(h)
	rest, found := strings.CutPrefix(h, "bytes ")
	if !found {
		return 0, 0, false
	}
	span, totalText, found := strings.Cut(rest, "/")
	if !found {
		return 0, 0, false
	}
	startText, _, found := strings.Cut(span, "-")
	if !found {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(startText, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}
	if totalText == "*" {
		return start, -1, true
	}
	total, err = strconv.ParseInt(totalText, 10, 64)
	if err != nil || total <= 0 {
		return 0, 0, false
	}
	return start, total, true
}

// parseContentRangeEnd reads the last byte of "bytes 5-9/10".
func parseContentRangeEnd(h string) (start, end int64, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(h), "bytes ")
	if !found {
		return 0, 0, false
	}
	span, _, found := strings.Cut(rest, "/")
	if !found {
		return 0, 0, false
	}
	a, b, found := strings.Cut(span, "-")
	if !found {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(a, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}
	end, err = strconv.ParseInt(b, 10, 64)
	if err != nil || end < start {
		return 0, 0, false
	}
	return start, end, true
}

// verify reads the whole file and compares its SHA-256, showing the progress of the check.
func (m *Manager) verify(ctx context.Context, j *job, path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, fail("the file could not be read to check it")
	}
	defer f.Close()
	j.set(func() {
		j.state = StateVerifying
		j.done = 0
		j.total = j.spec.SizeBytes
	})
	if m.opt.onVerify != nil {
		m.opt.onVerify(j.id)
	}
	h := sha256.New()
	buf := make([]byte, 1<<20)
	var read int64
	for {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		n, rerr := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			read += int64(n)
			j.set(func() { j.done = read })
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return false, fail("the file could not be read to check it")
		}
	}
	return hex.EncodeToString(h.Sum(nil)) == j.spec.SHA256, nil
}
