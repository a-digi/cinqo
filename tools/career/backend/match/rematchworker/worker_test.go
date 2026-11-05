package rematchworker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/match"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

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

func timing(t *testing.T, quiet, max time.Duration) {
	t.Helper()
	oq, om := quietPeriod, maxWait
	quietPeriod, maxWait = quiet, max
	t.Cleanup(func() { quietPeriod, maxWait = oq, om })
}

// runs replaces the re-match with one that records the profiles it is asked for.
type runs struct {
	mu   sync.Mutex
	ids  []string
	then func(id string) (match.RematchReport, error)
}

func (r *runs) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ids...)
}

func watch(t *testing.T, then func(id string) (match.RematchReport, error)) *runs {
	t.Helper()
	r := &runs{then: then}
	old := rematch
	rematch = func(id string) (match.RematchReport, error) {
		r.mu.Lock()
		r.ids = append(r.ids, id)
		r.mu.Unlock()
		if r.then != nil {
			return r.then(id)
		}
		return match.RematchReport{Jobs: 1, Stored: 1}, nil
	}
	t.Cleanup(func() { rematch = old })
	return r
}

func send(h http.HandlerFunc, method, body string) int {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, "/events/x", strings.NewReader(body)))
	return rec.Code
}

func event(topic, profileID string) string {
	return fmt.Sprintf(`{"id":"e1","topic":%q,"payload":{"persona_id":"per","profile_id":%q}}`, topic, profileID)
}

// ---- the routes ----

func TestTheRouteNotesTheProfileAndAnswersAtOnce(t *testing.T) {
	w := New(newClock().Now)
	r := watch(t, nil)
	if code := send(w.NoteHandler("profile_id"), http.MethodPost, event("career.persona.created", "prof")); code != http.StatusAccepted {
		t.Fatalf("status %d", code)
	}
	if w.Waiting() != 1 {
		t.Error("the profile was not noted")
	}
	if len(r.list()) != 0 {
		t.Error("work ran inside the request")
	}
}

func TestTheRouteRefusesWhatItCannotUse(t *testing.T) {
	w := New(newClock().Now)
	h := w.NoteHandler("profile_id")
	for name, tc := range map[string]struct {
		method, body string
		want         int
	}{
		"GET":               {http.MethodGet, "", http.StatusMethodNotAllowed},
		"PUT":               {http.MethodPut, event("t", "a"), http.StatusMethodNotAllowed},
		"not json":          {http.MethodPost, "nope", http.StatusBadRequest},
		"empty body":        {http.MethodPost, "", http.StatusBadRequest},
		"no payload":        {http.MethodPost, `{"topic":"t"}`, http.StatusBadRequest},
		"payload not json":  {http.MethodPost, `{"payload":"x"}`, http.StatusBadRequest},
		"empty profile id":  {http.MethodPost, `{"payload":{"profile_id":""}}`, http.StatusBadRequest},
		"only persona id":   {http.MethodPost, `{"payload":{"persona_id":"p"}}`, http.StatusBadRequest},
		"profile id number": {http.MethodPost, `{"payload":{"profile_id":5}}`, http.StatusBadRequest},
		"overlong id":       {http.MethodPost, fmt.Sprintf(`{"payload":{"profile_id":%q}}`, strings.Repeat("x", 201)), http.StatusBadRequest},
	} {
		if code := send(h, tc.method, tc.body); code != tc.want {
			t.Errorf("%s: status %d, want %d", name, code, tc.want)
		}
	}
	if w.Waiting() != 0 {
		t.Errorf("%d profiles were noted by refused events", w.Waiting())
	}
	if code := send(h, http.MethodPost, fmt.Sprintf(`{"payload":{"profile_id":%q}}`, strings.Repeat("x", 200))); code != http.StatusAccepted {
		t.Errorf("an id of exactly 200 characters: %d", code)
	}
}

// A persona event names its profile: the persona's own id is never noted.
func TestAPersonaEventNotesItsProfileNotThePersona(t *testing.T) {
	w := New(newClock().Now)
	clk := newClock()
	w.now = clk.Now
	timing(t, time.Second, time.Minute)
	r := watch(t, nil)
	send(w.NoteHandler("profile_id"), http.MethodPost, event("career.persona.created", "the-profile"))
	clk.advance(2 * time.Second)
	w.processDue(context.Background())
	if got := r.list(); !slices.Equal(got, []string{"the-profile"}) {
		t.Errorf("re-matched %v", got)
	}
}

// ---- the waiting ----

func TestAProfileWaitsForTheQuietPeriod(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, 10*time.Second, 2*time.Minute)
	r := watch(t, nil)
	w.Note("p")
	clk.advance(9 * time.Second)
	if w.processDue(context.Background()) != 0 || len(r.list()) != 0 || w.Waiting() != 1 {
		t.Fatal("ran before the quiet period")
	}
	clk.advance(time.Second)
	if w.processDue(context.Background()) != 1 || !slices.Equal(r.list(), []string{"p"}) || w.Waiting() != 0 {
		t.Fatalf("did not run after it: %v", r.list())
	}
}

func TestEventsKeepAProfileWaitingUntilTheMaximum(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, 10*time.Second, 2*time.Minute)
	r := watch(t, nil)
	w.Note("p")
	for i := 0; i < 11; i++ { // an event every 9 seconds keeps the quiet period from ever ending
		clk.advance(9 * time.Second)
		w.Note("p")
		w.processDue(context.Background())
	}
	if len(r.list()) != 0 {
		t.Fatalf("ran before the maximum: %v", r.list())
	}
	clk.advance(30 * time.Second)
	w.Note("p")
	w.processDue(context.Background())
	if !slices.Equal(r.list(), []string{"p"}) {
		t.Errorf("not run at the maximum: %v", r.list())
	}
}

func TestAProfileNamedManyTimesRunsOnce(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, time.Second, time.Minute)
	r := watch(t, nil)
	w.Note("p") // the profile event
	w.Note("p") // the persona event
	w.Note("p")
	clk.advance(2 * time.Second)
	w.processDue(context.Background())
	w.processDue(context.Background())
	if !slices.Equal(r.list(), []string{"p"}) {
		t.Errorf("runs %v", r.list())
	}
}

func TestRearmMovesTheQuietTimerOfAPendingProfileOnly(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, 10*time.Second, 2*time.Minute)
	r := watch(t, nil)
	if w.Rearm("nobody") || w.Waiting() != 0 {
		t.Fatal("a re-arm started a wait")
	}
	w.Note("p")
	clk.advance(8 * time.Second)
	if !w.Rearm("p") {
		t.Fatal("a pending profile was not re-armed")
	}
	clk.advance(8 * time.Second) // 16 s after the note, 8 s after the re-arm
	w.processDue(context.Background())
	if len(r.list()) != 0 {
		t.Fatal("ran although a change just re-armed it")
	}
	clk.advance(2 * time.Second)
	w.processDue(context.Background())
	if !slices.Equal(r.list(), []string{"p"}) {
		t.Errorf("runs %v", r.list())
	}
}

func TestARearmNeverPushesPastTheMaximum(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, 10*time.Second, time.Minute)
	r := watch(t, nil)
	w.Note("p")
	for i := 0; i < 8; i++ {
		clk.advance(9 * time.Second)
		w.Rearm("p")
	}
	w.processDue(context.Background()) // 72 s after the first note
	if !slices.Equal(r.list(), []string{"p"}) {
		t.Errorf("runs %v", r.list())
	}
}

func TestTheChangedRouteOnlyRearms(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	h := w.RearmHandler("profile_id")
	if code := send(h, http.MethodPost, event("career.persona.changed", "p")); code != http.StatusAccepted || w.Waiting() != 0 {
		t.Fatalf("status %d, waiting %d: it started a wait", code, w.Waiting())
	}
	timing(t, 10*time.Second, 2*time.Minute)
	watch(t, nil)
	w.Note("p")
	clk.advance(8 * time.Second)
	if code := send(h, http.MethodPost, event("career.persona.changed", "p")); code != http.StatusAccepted {
		t.Fatal(code)
	}
	clk.advance(8 * time.Second)
	if w.processDue(context.Background()) != 0 {
		t.Error("the change did not re-arm the pending profile")
	}
	if code := send(h, http.MethodGet, ""); code != http.StatusMethodNotAllowed {
		t.Errorf("GET %d", code)
	}
	if code := send(h, http.MethodPost, `{"payload":{"persona_id":"x"}}`); code != http.StatusBadRequest {
		t.Errorf("no profile id %d", code)
	}
}

func TestProfilesAreHandledOldestFirstAndNotReadyOnesStay(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, 10*time.Second, 2*time.Minute)
	r := watch(t, nil)
	w.Note("b")
	clk.advance(time.Second)
	w.Note("a")
	clk.advance(time.Second)
	w.Note("c")
	clk.advance(9 * time.Second) // b and a are ready (10 s, 9... a: 10 s since note), c is not (9 s)
	w.processDue(context.Background())
	if !slices.Equal(r.list(), []string{"b", "a"}) || w.Waiting() != 1 {
		t.Errorf("runs %v, waiting %d", r.list(), w.Waiting())
	}
}

// ---- failures ----

func TestAFailureOrPanicDoesNotStopTheWorker(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, time.Second, time.Minute)
	r := watch(t, func(id string) (match.RematchReport, error) {
		switch id {
		case "boom":
			panic("boom")
		case "err":
			return match.RematchReport{}, errors.New("db gone")
		}
		return match.RematchReport{Jobs: 3, Stored: 1}, nil
	})
	for _, id := range []string{"boom", "err", "fine"} {
		w.Note(id)
		clk.advance(time.Millisecond)
	}
	clk.advance(2 * time.Second)
	if n := w.processDue(context.Background()); n != 3 {
		t.Fatalf("handled %d", n)
	}
	if !slices.Equal(r.list(), []string{"boom", "err", "fine"}) {
		t.Errorf("runs %v", r.list())
	}
}

func TestAProfileNotedWhileItRunsIsANewEntry(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, time.Second, time.Minute)
	r := watch(t, func(id string) (match.RematchReport, error) {
		w.Note(id)
		return match.RematchReport{}, nil
	})
	w.Note("p")
	clk.advance(2 * time.Second)
	w.processDue(context.Background())
	if w.Waiting() != 1 || len(r.list()) != 1 {
		t.Fatalf("waiting %d, runs %v", w.Waiting(), r.list())
	}
	clk.advance(2 * time.Second)
	w.processDue(context.Background())
	if len(r.list()) != 2 {
		t.Errorf("runs %v", r.list())
	}
}

func TestACancelledWorkerStopsBetweenProfiles(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, time.Second, time.Minute)
	r := watch(t, nil)
	w.Note("a")
	w.Note("b")
	clk.advance(2 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.processDue(ctx)
	if len(r.list()) != 0 {
		t.Errorf("ran %v after cancel", r.list())
	}
}

func TestTheWorkerRunsOnATimerAndStops(t *testing.T) {
	w := New(time.Now)
	oq, ot := quietPeriod, tick
	quietPeriod, tick = 0, 5*time.Millisecond
	defer func() { quietPeriod, tick = oq, ot }()
	r := watch(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := w.Start(ctx)
	w.Note("p")
	deadline := time.Now().Add(5 * time.Second)
	for len(r.list()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !slices.Equal(r.list(), []string{"p"}) {
		t.Fatalf("runs %v", r.list())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not stop")
	}
}

// ---- the real re-match ----

// With the real tables and engine: a profile that gets its persona after the event is matched with it, because the worker waits.
func TestTheWorkerRematchesWithTheRealEngine(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('j1', 'https://x.example/1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`,
		"Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."); err != nil {
		t.Fatal(err)
	}
	clk := newClock()
	w := New(clk.Now)
	timing(t, 10*time.Second, 2*time.Minute)

	pid, err := profile.CreateProfile("New", "Test")
	if err != nil {
		t.Fatal(err)
	}
	w.Note(pid) // the profile event: the profile has no persona yet
	clk.advance(5 * time.Second)
	per, err := persona.CreatePersona(pid, "p", "")
	if err != nil {
		t.Fatal(err)
	}
	h, d, l := "Backend Engineer", "Software Engineer, Backend Developer", "Hamburg"
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: per, Headline: &h, DesiredTitles: &d, Location: &l, DesiredLocations: &l}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Go", "Docker", "Kubernetes", "PostgreSQL"} {
		if err := persona.AddCareerSkill(per, s); err != nil {
			t.Fatal(err)
		}
	}
	w.Note(pid) // the persona event
	clk.advance(9 * time.Second)
	if w.processDue(context.Background()) != 0 {
		t.Fatal("ran too early")
	}
	clk.advance(2 * time.Second)
	if w.processDue(context.Background()) != 1 {
		t.Fatal("did not run")
	}
	var got string
	var score int
	if err := db.JobsDB.QueryRow(`SELECT profile_id, score FROM job_matches WHERE job_id = 'j1'`).Scan(&got, &score); err != nil || got != pid || score == 0 {
		t.Fatalf("match %q %d, %v", got, score, err)
	}
}

// The maximum wait is reached exactly: with events still arriving, the profile runs at maxWait and not a moment later.
func TestTheMaximumWaitEndsExactlyAtTheLimit(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, 10*time.Second, time.Minute)
	r := watch(t, nil)
	w.Note("p")
	clk.advance(59 * time.Second)
	w.Note("p")
	if w.processDue(context.Background()) != 0 {
		t.Fatal("ran before the maximum")
	}
	clk.advance(time.Second) // exactly 60 s after the first note, 1 s after the last
	w.Note("p")
	if w.processDue(context.Background()) != 1 || len(r.list()) != 1 {
		t.Errorf("did not run at the maximum: %v", r.list())
	}
}

// What ships: quiet for 10 seconds, at most 2 minutes, a look every second.
func TestTheShippedTiming(t *testing.T) {
	if quietPeriod != 10*time.Second || maxWait != 2*time.Minute || tick != time.Second || maxIDLength != 200 {
		t.Errorf("quiet %v, max %v, tick %v, id length %d", quietPeriod, maxWait, tick, maxIDLength)
	}
}

// Forget (step 152) takes a deleted profile out of the waiting set and nothing else; a forgotten profile is never re-matched.
func TestForgetTakesAProfileOutOfTheWaitingSet(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, time.Second, time.Minute)
	r := watch(t, nil)
	w.Note("gone")
	w.Note("kept")
	if !w.Forget("gone") {
		t.Fatal("a waiting profile was not reported as forgotten")
	}
	if w.Forget("gone") || w.Forget("never-there") {
		t.Error("a profile that was not waiting was reported as forgotten")
	}
	if w.Waiting() != 1 {
		t.Fatalf("waiting %d", w.Waiting())
	}
	clk.advance(2 * time.Second)
	w.processDue(context.Background())
	if !slices.Equal(r.list(), []string{"kept"}) {
		t.Errorf("re-matched %v", r.list())
	}
	// a profile can be noted again after it was forgotten (a new profile with the same id is not a thing, but the set must not remember)
	w.Note("gone")
	if w.Waiting() != 1 {
		t.Error("noting after forgetting did not work")
	}
}

func TestThePackageForgetUsesTheDefaultWorker(t *testing.T) {
	Default.Note("forget-default-test")
	if !Forget("forget-default-test") {
		t.Error("the default worker did not forget it")
	}
}
