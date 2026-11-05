package personarecomputeconsumer

import (
	personamatching "career-tool-backend/match/persona"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/match/personamatch"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

// clock is a clock the test moves by hand.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// timing sets the quiet period and the maximum wait for one test.
func timing(t *testing.T, quiet, max time.Duration) {
	t.Helper()
	oq, om := quietPeriod, maxWait
	quietPeriod, maxWait = quiet, max
	t.Cleanup(func() { quietPeriod, maxWait = oq, om })
}

// runs replaces the recompute with one that records the personas it is asked for.
type runs struct {
	mu   sync.Mutex
	ids  []string
	then func(id string) (personamatching.RecomputeOutcome, error)
}

func (r *runs) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ids...)
}

func watch(t *testing.T, then func(id string) (personamatching.RecomputeOutcome, error)) *runs {
	t.Helper()
	r := &runs{then: then}
	old := recompute
	recompute = func(ctx context.Context, id string) (personamatching.RecomputeOutcome, error) {
		r.mu.Lock()
		r.ids = append(r.ids, id)
		r.mu.Unlock()
		if r.then != nil {
			return r.then(id)
		}
		return personamatching.RecomputeOutcome{Jobs: 1, Scored: 1}, nil
	}
	t.Cleanup(func() { recompute = old })
	return r
}

func send(h func(http.ResponseWriter, *http.Request), method, body string) int {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, "/events/persona-changed-recompute", strings.NewReader(body)))
	return rec.Code
}

func event(personaID string) string {
	return fmt.Sprintf(`{"id":"e1","topic":"career.persona.changed","payload":{"persona_id":%q,"profile_id":"prof","change":"skill_added"}}`, personaID)
}

// ---- the route ----

func TestTheRouteNotesTheChangedPersonaAndAnswersAtOnce(t *testing.T) {
	c := New(newClock().Now)
	if code := send(c.Handler, http.MethodPost, event("p1")); code != http.StatusAccepted {
		t.Fatalf("status %d", code)
	}
	if c.Waiting() != 1 {
		t.Error("the persona was not noted")
	}
}

func TestTheRouteRefusesWhatItCannotUse(t *testing.T) {
	c := New(newClock().Now)
	for name, tc := range map[string]struct {
		method, body string
		want         int
	}{
		"GET":                {http.MethodGet, "", http.StatusMethodNotAllowed},
		"PUT":                {http.MethodPut, event("a"), http.StatusMethodNotAllowed},
		"DELETE":             {http.MethodDelete, event("a"), http.StatusMethodNotAllowed},
		"not json":           {http.MethodPost, "nope", http.StatusBadRequest},
		"empty body":         {http.MethodPost, "", http.StatusBadRequest},
		"no payload":         {http.MethodPost, `{"topic":"career.persona.changed"}`, http.StatusBadRequest},
		"empty persona id":   {http.MethodPost, `{"payload":{"persona_id":""}}`, http.StatusBadRequest},
		"missing persona id": {http.MethodPost, `{"payload":{"profile_id":"x"}}`, http.StatusBadRequest},
		"persona id number":  {http.MethodPost, `{"payload":{"persona_id":5}}`, http.StatusBadRequest},
		"overlong id":        {http.MethodPost, fmt.Sprintf(`{"payload":{"persona_id":%q}}`, strings.Repeat("x", 201)), http.StatusBadRequest},
	} {
		if code := send(c.Handler, tc.method, tc.body); code != tc.want {
			t.Errorf("%s: status %d, want %d", name, code, tc.want)
		}
	}
	if c.Waiting() != 0 {
		t.Errorf("%d personas were noted by refused events", c.Waiting())
	}
	if code := send(c.Handler, http.MethodPost, fmt.Sprintf(`{"payload":{"persona_id":%q}}`, strings.Repeat("x", 200))); code != http.StatusAccepted {
		t.Errorf("an id of exactly 200 characters: %d", code)
	}
}

// Only persona_id of the payload is used: the profile and the kind of change do not matter.
func TestOnlyThePersonaIdIsUsed(t *testing.T) {
	c := New(newClock().Now)
	for _, body := range []string{
		`{"payload":{"persona_id":"a"}}`,
		`{"payload":{"persona_id":"b","profile_id":"x","change":"details","extra":[1,2]}}`,
	} {
		if code := send(c.Handler, http.MethodPost, body); code != http.StatusAccepted {
			t.Errorf("%s: %d", body, code)
		}
	}
	if c.Waiting() != 2 {
		t.Errorf("waiting %d", c.Waiting())
	}
}

// ---- the quiet period and the maximum wait ----

func TestAPersonaIsRecomputedOnlyAfterItHasBeenQuiet(t *testing.T) {
	timing(t, 10*time.Second, 2*time.Minute)
	clk := newClock()
	c := New(clk.Now)
	r := watch(t, nil)
	c.Note("p1")

	clk.advance(9 * time.Second)
	if n := c.processDue(context.Background()); n != 0 || len(r.list()) != 0 {
		t.Fatalf("after 9 s: handled %d, runs %v", n, r.list())
	}
	clk.advance(1 * time.Second) // exactly the quiet period
	if n := c.processDue(context.Background()); n != 1 || !reflect.DeepEqual(r.list(), []string{"p1"}) {
		t.Fatalf("after 10 s: handled %d, runs %v", n, r.list())
	}
	if c.Waiting() != 0 {
		t.Error("a recomputed persona must not wait any more")
	}
	if n := c.processDue(context.Background()); n != 0 || len(r.list()) != 1 {
		t.Error("a persona is recomputed once, not at every tick")
	}
}

// A burst of events becomes one recompute, after the burst has ended.
func TestABurstOfEventsIsOneRecompute(t *testing.T) {
	timing(t, 10*time.Second, 2*time.Minute)
	clk := newClock()
	c := New(clk.Now)
	r := watch(t, nil)
	for i := 0; i < 50; i++ {
		c.Note("p1")
		clk.advance(2 * time.Second) // events keep arriving every 2 s, for 100 s
		if n := c.processDue(context.Background()); n != 0 {
			t.Fatalf("recomputed during the burst at event %d", i)
		}
	}
	clk.advance(10 * time.Second)
	if n := c.processDue(context.Background()); n != 1 || !reflect.DeepEqual(r.list(), []string{"p1"}) {
		t.Fatalf("after the burst: handled %d, runs %v", n, r.list())
	}
}

// Events that never stop do not postpone the work for ever: the maximum wait forces it.
func TestTheMaximumWaitForcesARecompute(t *testing.T) {
	timing(t, 10*time.Second, 2*time.Minute)
	clk := newClock()
	c := New(clk.Now)
	r := watch(t, nil)
	c.Note("p1")
	for i := 0; i < 59; i++ { // 118 s, an event every 2 s
		clk.advance(2 * time.Second)
		c.Note("p1")
		if n := c.processDue(context.Background()); n != 0 {
			t.Fatalf("recomputed too early at %d s", (i+1)*2)
		}
	}
	clk.advance(2 * time.Second) // 120 s after the first event, with an event 2 s ago
	c.Note("p1")
	if n := c.processDue(context.Background()); n != 1 || len(r.list()) != 1 {
		t.Fatalf("at the maximum wait: handled %d, runs %v", n, r.list())
	}
}

// Each persona has its own clock: one that is ready is recomputed, one that is not stays; oldest first.
func TestPersonasAreHandledOnTheirOwnTimeOldestFirst(t *testing.T) {
	timing(t, 10*time.Second, 2*time.Minute)
	clk := newClock()
	c := New(clk.Now)
	r := watch(t, nil)
	c.Note("old-b")
	clk.advance(time.Second)
	c.Note("old-a")
	clk.advance(5 * time.Second)
	c.Note("young")
	clk.advance(6 * time.Second) // old-b 12 s, old-a 11 s, young 6 s
	if n := c.processDue(context.Background()); n != 2 || !reflect.DeepEqual(r.list(), []string{"old-b", "old-a"}) {
		t.Fatalf("handled %d, runs %v, want the two old ones, oldest first", n, r.list())
	}
	if c.Waiting() != 1 {
		t.Errorf("waiting %d, want the young one", c.Waiting())
	}
	clk.advance(5 * time.Second)
	if n := c.processDue(context.Background()); n != 1 || r.list()[2] != "young" {
		t.Errorf("handled %d, runs %v", n, r.list())
	}
}

// A persona changed again while it is being recomputed is not lost: it waits for a run of its own.
func TestAChangeDuringTheRunIsNotLost(t *testing.T) {
	timing(t, 10*time.Second, 2*time.Minute)
	clk := newClock()
	c := New(clk.Now)
	calls := 0
	r := watch(t, func(id string) (personamatching.RecomputeOutcome, error) {
		calls++
		if calls == 1 {
			c.Note(id) // an event arrives in the middle of the first run
		}
		return personamatching.RecomputeOutcome{}, nil
	})
	c.Note("p1")
	clk.advance(10 * time.Second)
	c.processDue(context.Background())
	if c.Waiting() != 1 {
		t.Fatalf("the change during the run must be waiting, waiting = %d", c.Waiting())
	}
	clk.advance(10 * time.Second)
	c.processDue(context.Background())
	if !reflect.DeepEqual(r.list(), []string{"p1", "p1"}) || c.Waiting() != 0 {
		t.Errorf("runs %v, waiting %d", r.list(), c.Waiting())
	}
}

// ---- what a run does and what goes wrong ----

func TestAPersonaThatIsGoneIsDroppedWithoutAFuss(t *testing.T) {
	timing(t, time.Second, time.Minute)
	clk := newClock()
	c := New(clk.Now)
	r := watch(t, func(id string) (personamatching.RecomputeOutcome, error) {
		if id == "gone" {
			return personamatching.RecomputeOutcome{}, persona.ErrUnknownPersona
		}
		return personamatching.RecomputeOutcome{Jobs: 1}, nil
	})
	c.Note("gone")
	c.Note("here")
	clk.advance(time.Second)
	if n := c.processDue(context.Background()); n != 2 || len(r.list()) != 2 || c.Waiting() != 0 {
		t.Errorf("handled %d, runs %v, waiting %d", n, r.list(), c.Waiting())
	}
}

func TestAFailureOrAPanicForOnePersonaDoesNotStopTheOthers(t *testing.T) {
	timing(t, time.Second, time.Minute)
	clk := newClock()
	c := New(clk.Now)
	r := watch(t, func(id string) (personamatching.RecomputeOutcome, error) {
		switch id {
		case "a":
			return personamatching.RecomputeOutcome{}, errors.New("boom")
		case "b":
			panic("kaboom")
		}
		return personamatching.RecomputeOutcome{Jobs: 1, Scored: 1}, nil
	})
	c.Note("a")
	clk.advance(time.Millisecond)
	c.Note("b")
	clk.advance(time.Millisecond)
	c.Note("c")
	clk.advance(time.Second)
	if n := c.processDue(context.Background()); n != 3 || !reflect.DeepEqual(r.list(), []string{"a", "b", "c"}) {
		t.Errorf("handled %d, runs %v", n, r.list())
	}
	if c.Waiting() != 0 {
		t.Error("a failed persona is not retried by the consumer itself (the sweeper does that)")
	}
}

// A cancelled context stops the personas still to do; they are not recomputed.
func TestACancelledContextStopsTheRemainingPersonas(t *testing.T) {
	timing(t, time.Second, time.Minute)
	clk := newClock()
	c := New(clk.Now)
	ctx, cancel := context.WithCancel(context.Background())
	r := watch(t, func(id string) (personamatching.RecomputeOutcome, error) {
		cancel() // the shutdown arrives during the first run
		return personamatching.RecomputeOutcome{}, context.Canceled
	})
	c.Note("a")
	clk.advance(time.Millisecond)
	c.Note("b")
	clk.advance(time.Second)
	c.processDue(ctx)
	if !reflect.DeepEqual(r.list(), []string{"a"}) {
		t.Errorf("runs %v, want only the first", r.list())
	}
}

// ---- the worker ----

func TestTheWorkerRecomputesAPersonaAfterTheQuietPeriodAndStops(t *testing.T) {
	timing(t, 120*time.Millisecond, time.Minute)
	oldTick := tick
	tick = 10 * time.Millisecond
	t.Cleanup(func() { tick = oldTick })
	r := watch(t, nil)
	c := New(time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	done := c.Start(ctx)

	c.Note("p1")
	time.Sleep(60 * time.Millisecond)
	if len(r.list()) != 0 {
		t.Fatal("recomputed before the quiet period")
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(r.list()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the recompute")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the worker did not stop")
	}
	c.Note("p2")
	time.Sleep(200 * time.Millisecond)
	if !reflect.DeepEqual(r.list(), []string{"p1"}) {
		t.Errorf("a stopped worker kept working: %v", r.list())
	}
}

func TestTheDefaultsAreTenSecondsTwoMinutesAndOneSecond(t *testing.T) {
	if quietPeriod != 10*time.Second || maxWait != 2*time.Minute || tick != time.Second {
		t.Errorf("quiet %v, max wait %v, tick %v", quietPeriod, maxWait, tick)
	}
}

// ---- with the real recompute and real tables ----

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

func strp(s string) *string { return &s }

const jobDesc = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
	"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."

func scoreOf(t *testing.T, jobID, personaID string) (int, bool) {
	t.Helper()
	rows, err := personamatch.ForJob(jobID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.PersonaID == personaID {
			return r.Score, true
		}
	}
	return 0, false
}

// Through the route and the worker, with the real recompute: a new persona gets its rows for the existing jobs, and after
// its skills change a second event brings them up to date; an event that changes nothing is skipped.
func TestEndToEndAPersonaChangeUpdatesItsScores(t *testing.T) {
	setup(t)
	timing(t, 30*time.Millisecond, time.Minute)
	clk := newClock()
	c := New(clk.Now)
	for _, id := range []string{"j1", "j2"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company, location, description) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, id, "https://x.example/"+id, jobDesc); err != nil {
			t.Fatal(err)
		}
	}
	profileID, _ := profile.CreateProfile("Ana", "Test")
	id, _ := persona.CreatePersona(profileID, "Backend", "")
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: id, Headline: strp("Backend Engineer"), DesiredTitles: strp("Software Engineer, Backend Developer"), Location: strp("Hamburg")}); err != nil {
		t.Fatal(err)
	}
	if err := persona.AddCareerSkill(id, "Cobol"); err != nil {
		t.Fatal(err)
	}

	handle := func() {
		clk.advance(time.Minute)
		c.processDue(context.Background())
	}
	if code := send(c.Handler, http.MethodPost, event(id)); code != http.StatusAccepted {
		t.Fatalf("status %d", code)
	}
	handle()
	first, ok := scoreOf(t, "j1", id)
	if !ok {
		t.Fatal("the persona has no row for an existing job after its event")
	}
	if _, ok := scoreOf(t, "j2", id); !ok {
		t.Fatal("the persona has no row for the second job")
	}

	// the persona learns what the jobs ask for; its next event brings the rows up to date
	for _, s := range []string{"Go", "Docker", "Kubernetes", "PostgreSQL"} {
		if err := persona.AddCareerSkill(id, s); err != nil {
			t.Fatal(err)
		}
	}
	send(c.Handler, http.MethodPost, event(id))
	handle()
	second, _ := scoreOf(t, "j1", id)
	if second <= first || second < 60 {
		t.Errorf("score %d then %d: the new skills must raise it", first, second)
	}

	// an event for a persona whose data did not change is skipped: a row changed by hand stays
	if _, err := db.JobsDB.Exec(`UPDATE job_persona_matches SET score = 99 WHERE job_id = 'j1' AND persona_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	send(c.Handler, http.MethodPost, event(id))
	handle()
	if got, _ := scoreOf(t, "j1", id); got != 99 {
		t.Errorf("an event for an unchanged persona rewrote the row: %d", got)
	}
}

// The consumer is declared next to the other listeners and routed in main.go; the worker starts.
func TestTheConsumerIsDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../../../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"topic": "career.persona.changed",
      "path_suffix": "events/persona-changed-recompute"`) {
		t.Error("manifest.json has no career.persona.changed -> events/persona-changed-recompute listener")
	}
	mainSrc, err := os.ReadFile("../../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"/events/persona-changed-recompute", personarecomputeconsumer.EventHandler`,
		`personarecomputeconsumer.Start(context.Background())`,
	} {
		if !strings.Contains(string(mainSrc), want) {
			t.Errorf("main.go lacks %s", want)
		}
	}
}

// Many personas that are ready at once come out in the order their first events arrived (a map would give a random order).
func TestManyReadyPersonasAreHandledInTheOrderTheyWereNoted(t *testing.T) {
	timing(t, 10*time.Second, 2*time.Minute)
	clk := newClock()
	c := New(clk.Now)
	r := watch(t, nil)
	var want []string
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("persona-%02d", 29-i) // ids in the opposite order of the arrival
		want = append(want, id)
		c.Note(id)
		clk.advance(10 * time.Millisecond)
	}
	clk.advance(time.Minute)
	if n := c.processDue(context.Background()); n != 30 || !reflect.DeepEqual(r.list(), want) {
		t.Errorf("handled %d, order %v", n, r.list())
	}
}

// Personas noted in the same instant are ordered by id, so the order never depends on chance.
func TestPersonasNotedInTheSameInstantAreOrderedById(t *testing.T) {
	timing(t, time.Second, time.Minute)
	clk := newClock()
	c := New(clk.Now)
	r := watch(t, nil)
	for _, id := range []string{"d", "b", "e", "a", "c", "h", "g", "f"} {
		c.Note(id)
	}
	clk.advance(time.Second)
	c.processDue(context.Background())
	if want := []string{"a", "b", "c", "d", "e", "f", "g", "h"}; !reflect.DeepEqual(r.list(), want) {
		t.Errorf("order %v, want %v", r.list(), want)
	}
}
