package runs

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/a-digi/cinqo/src/system/runs/entity"
	"github.com/a-digi/cinqo/src/system/runs/repository/persistent"
	"github.com/a-digi/cinqo/src/system/runs/repository/query"
)

const migrationsDir = "../../../config/db/migrations"

// openMainDB applies every real main-database migration, in file-name order, to a fresh in-memory database (the app's runner cuts a file at every
// semicolon, comments included, so the helper does exactly that).
func openMainDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations (%v)", err)
	}
	sort.Strings(files)
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		for _, stmt := range strings.Split(string(raw), ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", filepath.Base(f), err)
			}
		}
	}
	return db
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type world struct {
	db    *sql.DB
	clk   *clock
	alive map[int]bool
	start map[int]string
	ids   int
	logs  []string
}

func newWorld(t *testing.T) *world {
	return &world{db: openMainDB(t), clk: &clock{t: time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)}, alive: map[int]bool{}, start: map[int]string{}}
}

func (w *world) opts() Options {
	return Options{
		DB: w.db, Now: w.clk.now, PID: 5000,
		ProcStart: func(pid int) string { return w.start[pid] },
		Alive:     func(pid int) bool { return w.alive[pid] },
		Sample: func() entity.Heartbeat {
			return entity.Heartbeat{Memory: entity.HeartbeatMemory{TotalBytes: 8 << 30, AvailableBytes: 4 << 30, Pressure: "normal"}, AppByte: 70 << 20}
		},
		Every: 20 * time.Millisecond,
		NewID: func() string { w.ids++; return fmt.Sprintf("id-%03d", w.ids) },
		Log:   func(f string, a ...any) { w.logs = append(w.logs, fmt.Sprintf(f, a...)) },
	}
}

func events(t *testing.T, db *sql.DB) []entity.Event {
	t.Helper()
	ev, err := query.NewRunsQueryRepo(db).Events()
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func run(t *testing.T, db *sql.DB, id string) entity.Run {
	t.Helper()
	r, err := query.NewRunsQueryRepo(db).FindRun(id)
	if err != nil {
		t.Fatalf("run %s: %v", id, err)
	}
	return r
}

// leftBehind writes a run that never got an end, as a crashed app leaves it.
func leftBehind(t *testing.T, w *world, id string, pid int, procStart, startedAt, lastSeen, heartbeat string) {
	t.Helper()
	if err := persistent.NewRunsPersistentRepo(w.db).InsertRun(entity.Run{ID: id, StartedAt: startedAt, LastSeenAt: lastSeen, PID: pid, ProcessStart: procStart, LastHeartbeat: heartbeat}); err != nil {
		t.Fatal(err)
	}
}

func hbJSON(h entity.Heartbeat) string { raw, _ := json.Marshal(h); return string(raw) }

func TestAFirstStartRecordsTheRunAndMakesNoEvent(t *testing.T) {
	w := newWorld(t)
	w.start[5000] = "Mon Nov 9 14:00:00 2026"
	rec, err := Start(w.opts())
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	r := run(t, w.db, rec.RunID())
	if r.PID != 5000 || r.ProcessStart != "Mon Nov 9 14:00:00 2026" || r.StartedAt != "2026-11-09T14:00:00Z" || r.LastSeenAt != r.StartedAt || r.EndedAt != nil || r.Clean {
		t.Errorf("%+v", r)
	}
	var hb entity.Heartbeat
	if err := json.Unmarshal([]byte(r.LastHeartbeat), &hb); err != nil || hb.At != "2026-11-09T14:00:00Z" || hb.Memory.AvailableBytes != 4<<30 || hb.AppByte != 70<<20 {
		t.Errorf("the run starts with a first heartbeat: %q %v", r.LastHeartbeat, err)
	}
	if n := len(events(t, w.db)); n != 0 {
		t.Errorf("%d events on a first start", n)
	}
}

func TestTheHeartbeatKeepsWritingWhileTheAppLivesAndStopsAtClose(t *testing.T) {
	w := newWorld(t)
	rec, _ := Start(w.opts())
	w.clk.add(10 * time.Second)
	deadline := time.Now().Add(3 * time.Second)
	for run(t, w.db, rec.RunID()).LastSeenAt == "2026-11-09T14:00:00Z" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	r := run(t, w.db, rec.RunID())
	if r.LastSeenAt != "2026-11-09T14:00:10Z" || r.EndedAt != nil {
		t.Fatalf("the heartbeat must move last_seen_at: %+v", r)
	}
	if !strings.Contains(r.LastHeartbeat, `"at":"2026-11-09T14:00:10Z"`) {
		t.Errorf("and the heartbeat body: %s", r.LastHeartbeat)
	}
	w.clk.add(time.Minute)
	rec.Close()
	rec.Close() // twice is fine
	closed := run(t, w.db, rec.RunID())
	if closed.EndedAt == nil || *closed.EndedAt != "2026-11-09T14:01:10Z" || !closed.Clean {
		t.Errorf("a proper stop closes the run as clean: %+v", closed)
	}
	seen := closed.LastSeenAt
	w.clk.add(time.Hour)
	time.Sleep(120 * time.Millisecond)
	if again := run(t, w.db, rec.RunID()); again.LastSeenAt != seen {
		t.Error("no heartbeat may be written after Close")
	}
}

func TestACleanStopLeavesNoEventAtTheNextStart(t *testing.T) {
	w := newWorld(t)
	rec, _ := Start(w.opts())
	rec.Close()
	rec2, err := Start(w.opts())
	if err != nil {
		t.Fatal(err)
	}
	defer rec2.Close()
	if n := len(events(t, w.db)); n != 0 {
		t.Errorf("%d events after a clean stop", n)
	}
}

func TestARunThatNeverSaidItStoppedBecomesAnEventWithItsLastReadings(t *testing.T) {
	w := newWorld(t)
	hb := entity.Heartbeat{At: "2026-11-09T13:59:50Z", Memory: entity.HeartbeatMemory{TotalBytes: 8 << 30, AvailableBytes: 300 << 20, SwapUsedBytes: 2 << 30, Pressure: "critical"}, AppByte: 90 << 20,
		LocalAi: entity.HeartbeatAgent{State: "working", Name: "phi-4-mini", Bytes: 2900 << 20}}
	leftBehind(t, w, "old-1", 4242, "Mon Nov 9 13:00:00 2026", "2026-11-09T13:00:00Z", "2026-11-09T13:59:50Z", hbJSON(hb))
	// the old process is gone (the world's tables say nothing is alive)
	rec, err := Start(w.opts())
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()

	ev := events(t, w.db)
	if len(ev) != 1 {
		t.Fatalf("%d events, want 1", len(ev))
	}
	e := ev[0]
	if e.Kind != entity.KindAppUncleanExit || e.At != "2026-11-09T13:59:50Z" || e.AcknowledgedAt != nil {
		t.Errorf("%+v", e)
	}
	for _, want := range []string{"very tight", "300 MB of 8 GB free", "may have stopped it to free memory", "Phi-4-mini was loaded (working)"} {
		if !strings.Contains(e.Verdict, want) {
			t.Errorf("%q missing in %q", want, e.Verdict)
		}
	}
	facts := map[string]string{}
	for _, it := range e.Evidence {
		facts[it.Label] = it.Value
	}
	if facts["Last seen"] != "2026-11-09T13:59:50Z" || facts["Running for"] != "59 min" || facts["Memory pressure"] != "critical" || facts["Swap in use"] != "2 GB" || facts["Cinqo's own memory"] != "90 MB" || facts["Local model"] != "phi-4-mini, working (2.8 GB)" || facts["Memory free"] != "300 MB of 8 GB" {
		t.Errorf("%v", facts)
	}
	old := run(t, w.db, "old-1")
	if old.EndedAt == nil || *old.EndedAt != "2026-11-09T13:59:50Z" || old.Clean {
		t.Errorf("the dead run is closed when it was last seen, never as clean: %+v", old)
	}
	// starting again does not repeat the event
	rec.Close()
	rec2, _ := Start(w.opts())
	defer rec2.Close()
	if n := len(events(t, w.db)); n != 1 {
		t.Errorf("%d events after another start, want still 1", n)
	}
}

func TestARunWhoseProcessIsStillAliveIsAnotherInstanceNotAnUnexpectedStop(t *testing.T) {
	w := newWorld(t)
	leftBehind(t, w, "live-1", 4242, "Mon Nov 9 13:00:00 2026", "2026-11-09T13:00:00Z", "2026-11-09T13:59:50Z", "")
	w.alive[4242], w.start[4242] = true, "Mon Nov 9 13:00:00 2026"
	rec, _ := Start(w.opts())
	defer rec.Close()
	if n := len(events(t, w.db)); n != 0 {
		t.Errorf("%d events for a run that is alive", n)
	}
	if r := run(t, w.db, "live-1"); r.EndedAt != nil {
		t.Error("a live run must stay open")
	}
}

func TestAReusedProcessIdIsNotTheSameRun(t *testing.T) {
	w := newWorld(t)
	leftBehind(t, w, "old-2", 4242, "Mon Nov 9 13:00:00 2026", "2026-11-09T13:00:00Z", "2026-11-09T13:59:50Z", "")
	w.alive[4242], w.start[4242] = true, "Mon Nov 9 13:30:00 2026" // another program now has that id
	rec, _ := Start(w.opts())
	defer rec.Close()
	if n := len(events(t, w.db)); n != 1 {
		t.Errorf("%d events, want 1", n)
	}
	// and a run with no recorded start time cannot be told from a reused id: it counts as dead
	leftBehind(t, w, "old-3", 4343, "", "2026-11-09T12:00:00Z", "2026-11-09T12:30:00Z", "")
	w.alive[4343] = true
	rec.Close()
	rec2, _ := Start(w.opts())
	defer rec2.Close()
	if n := len(events(t, w.db)); n != 2 {
		t.Errorf("%d events, want 2", n)
	}
}

func TestARunThatStoppedBeforeItsFirstHeartbeatSaysSo(t *testing.T) {
	w := newWorld(t)
	leftBehind(t, w, "old-4", 1, "", "2026-11-09T13:00:00Z", "2026-11-09T13:00:00Z", "")
	rec, _ := Start(w.opts())
	defer rec.Close()
	e := events(t, w.db)[0]
	if !strings.Contains(e.Verdict, "within its first seconds") {
		t.Errorf("%s", e.Verdict)
	}
	if len(e.Evidence) != 2 || e.Evidence[0].Label != "Last seen" || e.Evidence[1].Value != "0 s" {
		t.Errorf("%+v", e.Evidence)
	}
}

func TestAHeartbeatThatCannotBeReadIsTreatedAsNoHeartbeat(t *testing.T) {
	w := newWorld(t)
	leftBehind(t, w, "old-5", 1, "", "2026-11-09T13:00:00Z", "2026-11-09T13:00:30Z", "{not json")
	rec, _ := Start(w.opts())
	defer rec.Close()
	if e := events(t, w.db)[0]; !strings.Contains(e.Verdict, "within its first seconds") {
		t.Errorf("%s", e.Verdict)
	}
}

func TestSeveralDeadRunsEachGetTheirOwnEventNewestFirst(t *testing.T) {
	w := newWorld(t)
	leftBehind(t, w, "a", 1, "", "2026-11-09T10:00:00Z", "2026-11-09T10:10:00Z", "")
	leftBehind(t, w, "b", 2, "", "2026-11-09T11:00:00Z", "2026-11-09T11:10:00Z", "")
	rec, _ := Start(w.opts())
	defer rec.Close()
	ev := events(t, w.db)
	if len(ev) != 2 || ev[0].At != "2026-11-09T11:10:00Z" || ev[1].At != "2026-11-09T10:10:00Z" {
		t.Errorf("%+v", ev)
	}
}

func TestTheOldRecordsAreTrimmedButAnOpenRunNeverIs(t *testing.T) {
	w := newWorld(t)
	pers := persistent.NewRunsPersistentRepo(w.db)
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("r%02d", i)
		at := fmt.Sprintf("2026-10-%02dT10:00:00Z", i+1)
		if err := pers.InsertRun(entity.Run{ID: id, StartedAt: at, LastSeenAt: at}); err != nil {
			t.Fatal(err)
		}
		if err := pers.CloseRun(id, at, true); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 70; i++ {
		at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		if err := pers.InsertEvent(entity.Event{ID: fmt.Sprintf("e%02d", i), At: at, Kind: "x", Verdict: "v"}); err != nil {
			t.Fatal(err)
		}
	}
	rec, err := Start(w.opts())
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	q := query.NewRunsQueryRepo(w.db)
	if n, _ := q.CountRuns(); n != KeepRuns+1 {
		t.Errorf("%d runs kept, want %d finished plus the open one", n, KeepRuns)
	}
	if _, err := q.FindRun("r00"); err == nil {
		t.Error("the oldest finished run must be gone")
	}
	if _, err := q.FindRun("r29"); err != nil {
		t.Error("the newest finished run must stay")
	}
	if _, err := q.FindRun(rec.RunID()); err != nil {
		t.Error("the open run must stay")
	}
	ev := events(t, w.db)
	if len(ev) != KeepEvents || ev[0].ID != "e69" || ev[len(ev)-1].ID != "e20" {
		t.Errorf("%d events, first %s last %s", len(ev), ev[0].ID, ev[len(ev)-1].ID)
	}
	if KeepRuns != 20 || KeepEvents != 50 || HeartbeatEvery != 10*time.Second {
		t.Error("the plan's numbers: 20 runs, 50 events, a heartbeat every 10 s")
	}
}

func TestAnUnwritableDatabaseIsAnErrorForTheCallerNotAPanic(t *testing.T) {
	w := newWorld(t)
	w.db.Close()
	if _, err := Start(w.opts()); err == nil {
		t.Error("the run cannot be recorded: Start must say so")
	}
	if len(w.logs) == 0 {
		t.Error("the unreadable earlier runs are reported")
	}
}

func TestAHeartbeatThatFailsIsReportedAndTheAppGoesOn(t *testing.T) {
	w := newWorld(t)
	rec, _ := Start(w.opts())
	w.db.Exec(`DROP TABLE system_runs`)
	time.Sleep(100 * time.Millisecond)
	rec.Close() // closing with a broken table is reported, never a crash
	if len(w.logs) == 0 {
		t.Error("failures must be reported")
	}
}

func TestAnEventWhoseWriteFailsLeavesTheRunOpenForTheNextStart(t *testing.T) {
	w := newWorld(t)
	leftBehind(t, w, "old-6", 1, "", "2026-11-09T13:00:00Z", "2026-11-09T13:30:00Z", "")
	w.db.Exec(`DROP TABLE system_events`)
	rec, err := Start(w.opts())
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	if r := run(t, w.db, "old-6"); r.EndedAt != nil {
		t.Error("without its event the run must stay open so the next start can try again")
	}
}

func TestTheRealProcessReadersKnowThisProcess(t *testing.T) {
	if !Alive(os.Getpid()) || Alive(0) || Alive(-3) {
		t.Error("Alive")
	}
	start := StartTime(os.Getpid())
	if start == "" {
		t.Skip("ps cannot say here")
	}
	if StartTime(os.Getpid()) != start || StartTime(0) != "" || StartTime(-1) != "" {
		t.Error("StartTime")
	}
}

func TestTheRealDefaultsWorkEndToEnd(t *testing.T) {
	db := openMainDB(t)
	rec, err := Start(Options{DB: db, PID: os.Getpid(), Every: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	rec.Close()
	r := run(t, db, rec.RunID())
	if r.EndedAt == nil || !r.Clean || r.PID != os.Getpid() {
		t.Errorf("%+v", r)
	}
}

func TestNoDatabaseIsAnErrorNotAPanic(t *testing.T) {
	if _, err := Start(Options{}); err == nil {
		t.Error("Start without a database must say so")
	}
}

func TestAClosedRunIsNeverChangedAgain(t *testing.T) {
	w := newWorld(t)
	pers := persistent.NewRunsPersistentRepo(w.db)
	if err := pers.InsertRun(entity.Run{ID: "x", StartedAt: "2026-11-09T10:00:00Z", LastSeenAt: "2026-11-09T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := pers.CloseRun("x", "2026-11-09T10:30:00Z", true); err != nil {
		t.Fatal(err)
	}
	// a late heartbeat and a second close must change nothing
	if err := pers.Heartbeat("x", "2026-11-09T11:00:00Z", `{"late":true}`); err != nil {
		t.Fatal(err)
	}
	if err := pers.CloseRun("x", "2026-11-09T12:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	r := run(t, w.db, "x")
	if r.LastSeenAt != "2026-11-09T10:00:00Z" || r.LastHeartbeat != "" || r.EndedAt == nil || *r.EndedAt != "2026-11-09T10:30:00Z" || !r.Clean {
		t.Errorf("%+v", r)
	}
}

func TestOpenRunsAreListedOldestFirst(t *testing.T) {
	w := newWorld(t)
	leftBehind(t, w, "late", 1, "", "2026-11-09T12:00:00Z", "2026-11-09T12:00:00Z", "")
	leftBehind(t, w, "early", 2, "", "2026-11-09T09:00:00Z", "2026-11-09T09:00:00Z", "")
	got, err := query.NewRunsQueryRepo(w.db).OpenRuns()
	if err != nil || len(got) != 2 || got[0].ID != "early" || got[1].ID != "late" {
		t.Errorf("%+v %v", got, err)
	}
}
