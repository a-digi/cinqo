package personarecomputeconsumer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func useStale(t *testing.T, fn func() ([]string, error)) {
	t.Helper()
	old := stalePersonas
	stalePersonas = fn
	t.Cleanup(func() { stalePersonas = old })
}

func TestASweepNotesTheStalePersonas(t *testing.T) {
	useStale(t, func() ([]string, error) { return []string{"a", "b", "c"}, nil })
	c := New(newClock().Now)
	n, err := c.Sweep()
	if err != nil || n != 3 || c.Waiting() != 3 {
		t.Fatalf("noted %d (%v), waiting %d", n, err, c.Waiting())
	}
	// a second sweep finds them waiting already and notes nothing new
	if n, _ := c.Sweep(); n != 0 || c.Waiting() != 3 {
		t.Errorf("second sweep noted %d, waiting %d", n, c.Waiting())
	}
}

// A persona that is already waiting keeps its times: the sweeper never postpones a recompute that is on its way.
func TestASweepDoesNotPostponeAPersonaThatIsAlreadyWaiting(t *testing.T) {
	timing(t, 10*time.Second, 2*time.Minute)
	clk := newClock()
	c := New(clk.Now)
	r := watch(t, nil)
	useStale(t, func() ([]string, error) { return []string{"p1"}, nil })
	c.Note("p1")
	clk.advance(9 * time.Second)
	if n, _ := c.Sweep(); n != 0 {
		t.Fatalf("noted %d", n)
	}
	clk.advance(1 * time.Second) // 10 s after the event: due, although the sweep ran 1 s ago
	if n := c.processDue(context.Background()); n != 1 || len(r.list()) != 1 {
		t.Errorf("handled %d, runs %v: the sweep must not have restarted the quiet period", n, r.list())
	}
}

func TestASweepOfNothingStaleNotesNothing(t *testing.T) {
	useStale(t, func() ([]string, error) { return nil, nil })
	c := New(newClock().Now)
	if n, err := c.Sweep(); err != nil || n != 0 || c.Waiting() != 0 {
		t.Errorf("%d %v", n, err)
	}
}

func TestASweepThatCannotLookReportsItAndNotesNothing(t *testing.T) {
	useStale(t, func() ([]string, error) { return []string{"a"}, errors.New("db gone") })
	c := New(newClock().Now)
	if n, err := c.Sweep(); err == nil || n != 0 || c.Waiting() != 0 {
		t.Errorf("%d %v waiting %d", n, err, c.Waiting())
	}
}

// What the sweeper notes goes through the same quiet period and the same worker as an event.
func TestWhatTheSweepNotesIsRecomputedByTheWorkerAfterTheQuietPeriod(t *testing.T) {
	timing(t, 10*time.Second, 2*time.Minute)
	clk := newClock()
	c := New(clk.Now)
	r := watch(t, nil)
	useStale(t, func() ([]string, error) { return []string{"a", "b"}, nil })
	_, _ = c.Sweep()
	if n := c.processDue(context.Background()); n != 0 {
		t.Fatal("recomputed before the quiet period")
	}
	clk.advance(10 * time.Second)
	if n := c.processDue(context.Background()); n != 2 || !reflect.DeepEqual(r.list(), []string{"a", "b"}) {
		t.Errorf("handled %d, runs %v", n, r.list())
	}
}

func TestTheSweeperRunsOnATimerAndStops(t *testing.T) {
	oldDelay, oldInterval := sweepStartDelay, sweepInterval
	sweepStartDelay, sweepInterval = 150*time.Millisecond, 30*time.Millisecond
	defer func() { sweepStartDelay, sweepInterval = oldDelay, oldInterval }()

	var calls int32
	useStale(t, func() ([]string, error) {
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			panic("boom")
		case 2:
			return nil, errors.New("temporary")
		}
		return []string{fmt.Sprintf("persona-%d", atomic.LoadInt32(&calls))}, nil
	})
	c := New(time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := c.StartSweeper(ctx)

	time.Sleep(60 * time.Millisecond)
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Errorf("a sweep ran %d time(s) before the start delay", n)
	}
	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt32(&calls) < 4 || c.Waiting() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %d sweeps, %d waiting (the sweeps after the panic and the error must note)", atomic.LoadInt32(&calls), c.Waiting())
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the sweeper did not stop")
	}
	after := atomic.LoadInt32(&calls)
	time.Sleep(120 * time.Millisecond)
	if atomic.LoadInt32(&calls) != after {
		t.Error("a stopped sweeper kept sweeping")
	}
}

func TestTheSweepTimingIsThirtySecondsAndTenMinutes(t *testing.T) {
	if sweepStartDelay != 30*time.Second || sweepInterval != 10*time.Minute {
		t.Errorf("delay %v, interval %v", sweepStartDelay, sweepInterval)
	}
}

// With the real lookup and the real recompute: a persona with no state is found, recomputed, and is not found again;
// a change made while no event was heard (here: no event at all) is found by the next sweep.
func TestEndToEndTheSweepFindsPersonasWhoseEventWasMissed(t *testing.T) {
	setup(t)
	timing(t, 10*time.Millisecond, time.Minute)
	clk := newClock()
	c := New(clk.Now)
	for _, id := range []string{"j1", "j2"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company, location, description) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, id, "https://x.example/"+id, jobDesc); err != nil {
			t.Fatal(err)
		}
	}
	profileID, _ := profile.CreateProfile("Ana", "Test")
	id, _ := persona.CreatePersona(profileID, "Backend", "")
	_ = persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: id, Headline: strp("Backend Engineer"), DesiredTitles: strp("Software Engineer, Backend Developer"), Location: strp("Hamburg")})
	_ = persona.AddCareerSkill(id, "Cobol")

	sweepAndRun := func() int {
		n, err := c.Sweep()
		if err != nil {
			t.Fatal(err)
		}
		clk.advance(time.Minute)
		c.processDue(context.Background())
		return n
	}
	if n := sweepAndRun(); n != 1 {
		t.Fatalf("the first sweep noted %d, want the persona that has no state", n)
	}
	first, ok := scoreOf(t, "j1", id)
	if !ok {
		t.Fatal("the persona has no row after the sweep")
	}
	if n := sweepAndRun(); n != 0 {
		t.Errorf("the second sweep noted %d, want none: nothing is stale", n)
	}
	// the persona changes and no event reaches the consumer
	for _, s := range []string{"Go", "Docker", "Kubernetes", "PostgreSQL"} {
		_ = persona.AddCareerSkill(id, s)
	}
	if n := sweepAndRun(); n != 1 {
		t.Fatalf("after a change the sweep noted %d, want 1", n)
	}
	if second, _ := scoreOf(t, "j1", id); second <= first || second < 60 {
		t.Errorf("score %d then %d: the sweep's recompute must follow the new skills", first, second)
	}
	if n := sweepAndRun(); n != 0 {
		t.Errorf("a sweep noted %d after the catch-up", n)
	}
}

func TestTheSweeperIsStartedInMainGo(t *testing.T) {
	mainSrc, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), `personarecomputeconsumer.StartSweeper(context.Background())`) {
		t.Error("main.go does not start the sweeper")
	}
}
