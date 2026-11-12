// Package runs is the flight recorder of the app (plan 223, step 3). A program that is killed cannot write down that it was killed, so while it lives
// it writes how it is doing every few seconds, and says so when it stops properly. At the next start, a run that never said it stopped is an
// unexpected stop: an event is written with the last readings and a guess at why.
package runs

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/a-digi/cinqo/src/system/runs/entity"
	"github.com/a-digi/cinqo/src/system/runs/evidence"
	"github.com/a-digi/cinqo/src/system/runs/repository/persistent"
	"github.com/a-digi/cinqo/src/system/runs/repository/query"
)

// HeartbeatEvery is how often a living app writes down how it is doing (plan 223, Q4).
const HeartbeatEvery = 10 * time.Second

// KeepRuns and KeepEvents bound what is kept. The unexpected stops (plan 259) are shown in a table with pagination, so more of them are kept than the dropdown ever showed.
const (
	KeepRuns   = 20
	KeepEvents = 500
)

// Options are the sources of a Recorder; every one but DB has a real default.
type Options struct {
	DB        *sql.DB
	Now       func() time.Time
	PID       int
	ProcStart func(pid int) string
	Alive     func(pid int) bool
	Sample    func() entity.Heartbeat // what the app knows about itself now
	// Evidence returns what the system wrote down about a stop that happened between from and to (macOS reports); nil reads nothing.
	Evidence func(from, to time.Time) []evidence.Finding
	Every    time.Duration
	NewID    func() string
	Log      func(format string, args ...any) // failures are reported here and never stop the app
}

// Recorder writes the heartbeat of the current run and closes it.
type Recorder struct {
	o     Options
	runID string
	pers  *persistent.RunsPersistentRepo

	stopOnce sync.Once
	done     chan struct{}
	wg       sync.WaitGroup
}

func (o *Options) defaults() {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.ProcStart == nil {
		o.ProcStart = StartTime
	}
	if o.Alive == nil {
		o.Alive = Alive
	}
	if o.Every <= 0 {
		o.Every = HeartbeatEvery
	}
	if o.NewID == nil {
		o.NewID = uuid.NewString
	}
	if o.Log == nil {
		o.Log = func(string, ...any) {}
	}
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// Start finds what an earlier run left (an event for each run that stopped without a trace), trims the old records, writes the current run and starts
// its heartbeat. A failure to read or write is returned only when the current run could not be recorded at all; the app starts either way.
func Start(o Options) (*Recorder, error) {
	if o.DB == nil {
		return nil, errors.New("run recorder: no database")
	}
	o.defaults()
	pers := persistent.NewRunsPersistentRepo(o.DB)
	q := query.NewRunsQueryRepo(o.DB)

	open, err := q.OpenRuns()
	if err != nil {
		o.Log("run recorder: could not read the earlier runs: %v", err)
	}
	for _, run := range open {
		// another instance that is alive right now (same id AND same start time) is not an unexpected stop
		if run.PID > 0 && o.Alive(run.PID) && run.ProcessStart != "" && o.ProcStart(run.PID) == run.ProcessStart {
			continue
		}
		var hb *entity.Heartbeat
		if run.LastHeartbeat != "" {
			var parsed entity.Heartbeat
			if json.Unmarshal([]byte(run.LastHeartbeat), &parsed) == nil {
				hb = &parsed
			}
		}
		var findings []evidence.Finding
		if o.Evidence != nil {
			if seen, err := time.Parse(time.RFC3339, run.LastSeenAt); err == nil {
				findings = o.Evidence(seen, o.Now())
			}
		}
		verdict, facts := VerdictWith(hb, run.StartedAt, run.LastSeenAt, findings)
		if err := pers.InsertEvent(entity.Event{ID: o.NewID(), At: run.LastSeenAt, Kind: entity.KindAppUncleanExit, Verdict: verdict, Evidence: facts}); err != nil {
			o.Log("run recorder: could not write the event of an unexpected stop: %v", err)
			continue // leave the run open: the next start tries again
		}
		if err := pers.CloseRun(run.ID, run.LastSeenAt, false); err != nil {
			o.Log("run recorder: could not close an earlier run: %v", err)
		}
	}

	now := o.Now()
	hb := o.sample()
	run := entity.Run{
		ID: o.NewID(), StartedAt: stamp(now), LastSeenAt: stamp(now), PID: o.PID, ProcessStart: o.ProcStart(o.PID), LastHeartbeat: hb,
	}
	if err := pers.InsertRun(run); err != nil {
		return nil, err
	}
	if err := pers.Prune(KeepRuns, KeepEvents); err != nil {
		o.Log("run recorder: could not trim the old records: %v", err)
	}

	r := &Recorder{o: o, runID: run.ID, pers: pers, done: make(chan struct{})}
	r.wg.Add(1)
	go r.loop()
	return r, nil
}

func (o *Options) sample() string {
	if o.Sample == nil {
		return ""
	}
	h := o.Sample()
	h.At = stamp(o.Now())
	raw, err := json.Marshal(h)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (r *Recorder) loop() {
	defer r.wg.Done()
	t := time.NewTicker(r.o.Every)
	defer t.Stop()
	for {
		select {
		case <-r.done:
			return
		case <-t.C:
			r.beat()
		}
	}
}

func (r *Recorder) beat() {
	if err := r.pers.Heartbeat(r.runID, stamp(r.o.Now()), r.o.sample()); err != nil {
		r.o.Log("run recorder: heartbeat not written: %v", err)
	}
}

// AddEvent writes an event now (the local model program stopped on its own, for one). The time, the id and the evidence are the caller's; a failure
// is reported and never stops the app.
func (r *Recorder) AddEvent(kind, verdict string, facts []entity.EvidenceItem) {
	e := entity.Event{ID: r.o.NewID(), At: stamp(r.o.Now()), Kind: kind, Verdict: verdict, Evidence: facts}
	if err := r.pers.InsertEvent(e); err != nil {
		r.o.Log("run recorder: could not write an event: %v", err)
	}
}

// Close says the app is stopping properly: the heartbeat ends and the run is closed as clean. Safe to call more than once.
func (r *Recorder) Close() {
	r.stopOnce.Do(func() {
		close(r.done)
		r.wg.Wait()
		if err := r.pers.CloseRun(r.runID, stamp(r.o.Now()), true); err != nil {
			r.o.Log("run recorder: could not close this run: %v", err)
		}
	})
}

// RunID is the id of the current run.
func (r *Recorder) RunID() string { return r.runID }
