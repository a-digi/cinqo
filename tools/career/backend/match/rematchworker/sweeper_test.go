package rematchworker

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func sources(t *testing.T, seed func() (int, error), stale func() ([]string, error)) *int {
	t.Helper()
	oseed, ostale := seedState, staleProfiles
	seeds := 0
	seedState = func() (int, error) {
		seeds++
		if seed != nil {
			return seed()
		}
		return 0, nil
	}
	staleProfiles = stale
	t.Cleanup(func() { seedState, staleProfiles = oseed, ostale })
	return &seeds
}

func TestASweepNotesTheStaleProfiles(t *testing.T) {
	w := New(newClock().Now)
	sources(t, nil, func() ([]string, error) { return []string{"a", "b"}, nil })
	n, err := w.Sweep()
	if err != nil || n != 2 || w.Waiting() != 2 {
		t.Fatalf("noted %d, waiting %d, %v", n, w.Waiting(), err)
	}
	if n, _ := w.Sweep(); n != 0 || w.Waiting() != 2 {
		t.Errorf("a second sweep noted %d again", n)
	}
}

// A profile that is already waiting keeps its times: the sweeper never delays a re-match that is on its way.
func TestASweepNeverDelaysAWaitingProfile(t *testing.T) {
	clk := newClock()
	w := New(clk.Now)
	timing(t, 10*time.Second, 2*time.Minute)
	r := watch(t, nil)
	w.Note("p")
	clk.advance(8 * time.Second)
	sources(t, nil, func() ([]string, error) { return []string{"p"}, nil })
	if n, _ := w.Sweep(); n != 0 {
		t.Fatalf("noted %d", n)
	}
	clk.advance(2 * time.Second)
	w.processDue(context.Background())
	if !slices.Equal(r.list(), []string{"p"}) {
		t.Errorf("the sweep delayed it: %v", r.list())
	}
}

func TestTheSeedingRunsBeforeTheLookAndAFailureStopsTheSweep(t *testing.T) {
	w := New(newClock().Now)
	var order []string
	oseed, ostale := seedState, staleProfiles
	defer func() { seedState, staleProfiles = oseed, ostale }()
	seedState = func() (int, error) { order = append(order, "seed"); return 0, nil }
	staleProfiles = func() ([]string, error) { order = append(order, "stale"); return []string{"p"}, nil }
	w.Sweep()
	if !slices.Equal(order, []string{"seed", "stale"}) {
		t.Fatalf("order %v", order)
	}
	seedState = func() (int, error) { return 0, errors.New("db gone") }
	w2 := New(newClock().Now)
	if _, err := w2.Sweep(); err == nil || w2.Waiting() != 0 {
		t.Errorf("a failed seeding must stop the sweep: %v, waiting %d", err, w2.Waiting())
	}
	seedState = func() (int, error) { return 0, nil }
	staleProfiles = func() ([]string, error) { return nil, errors.New("db gone") }
	if _, err := w2.Sweep(); err == nil {
		t.Error("a failed look must be reported")
	}
}

func TestASweepThatFailsOrPanicsDoesNotStopTheSweeper(t *testing.T) {
	w := New(newClock().Now)
	calls := 0
	sources(t, nil, func() ([]string, error) {
		calls++
		switch calls {
		case 1:
			return nil, errors.New("boom")
		case 2:
			panic("boom")
		}
		return []string{"p"}, nil
	})
	w.sweepOnce()
	w.sweepOnce()
	w.sweepOnce()
	if w.Waiting() != 1 {
		t.Errorf("waiting %d", w.Waiting())
	}
}

func TestTheSweeperRunsOnATimerAndStops(t *testing.T) {
	w := New(time.Now)
	od, oi := sweepStartDelay, sweepInterval
	sweepStartDelay, sweepInterval = 5*time.Millisecond, 5*time.Millisecond
	defer func() { sweepStartDelay, sweepInterval = od, oi }()
	calls := make(chan struct{}, 100)
	sources(t, nil, func() ([]string, error) { calls <- struct{}{}; return nil, nil })
	ctx, cancel := context.WithCancel(context.Background())
	done := w.StartSweeper(ctx)
	for i := 0; i < 3; i++ { // it keeps going after the first sweep
		select {
		case <-calls:
		case <-time.After(5 * time.Second):
			t.Fatalf("sweep %d never ran", i+1)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the sweeper did not stop")
	}
}

func TestTheSweeperWaitsForTheStartDelay(t *testing.T) {
	w := New(time.Now)
	od := sweepStartDelay
	sweepStartDelay = time.Hour
	defer func() { sweepStartDelay = od }()
	sw := 0
	sources(t, func() (int, error) { sw++; return 0, nil }, func() ([]string, error) { return nil, nil })
	ctx, cancel := context.WithCancel(context.Background())
	done := w.StartSweeper(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	if sw != 0 {
		t.Error("swept before the start delay")
	}
}

func TestTheShippedSweepTiming(t *testing.T) {
	if sweepStartDelay != 30*time.Second || sweepInterval != 10*time.Minute {
		t.Errorf("%v %v", sweepStartDelay, sweepInterval)
	}
}

// A restart: a profile and its persona were created while the tool was down (no event, nothing waiting). The first sweep after the
// install only seeds what existed; one created afterwards is found by the next sweep and re-matched by the worker.
func TestARestartFindsAProfileWhoseEventWasLost(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('j1', 'https://x.example/1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`,
		"Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."); err != nil {
		t.Fatal(err)
	}
	build := func(name string) string {
		pid, err := profile.CreateProfile(name, "x")
		if err != nil {
			t.Fatal(err)
		}
		per, _ := persona.CreatePersona(pid, "p", "")
		h, d, l := "Backend Engineer", "Software Engineer, Backend Developer", "Hamburg"
		if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: per, Headline: &h, DesiredTitles: &d, Location: &l, DesiredLocations: &l}); err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{"Go", "Docker", "Kubernetes", "PostgreSQL"} {
			if err := persona.AddCareerSkill(per, s); err != nil {
				t.Fatal(err)
			}
		}
		return pid
	}
	existing := build("Existing") // there before the feature: seeded, not re-matched
	clk := newClock()
	w := New(clk.Now)
	timing(t, time.Second, time.Minute)
	if n, err := w.Sweep(); err != nil || n != 0 {
		t.Fatalf("first sweep noted %d, %v", n, err)
	}
	clk.advance(5 * time.Second)
	if w.processDue(context.Background()) != 0 {
		t.Fatal("the first sweep re-matched an existing profile")
	}
	var one int
	if err := db.JobsDB.QueryRow(`SELECT 1 FROM job_matches WHERE job_id = 'j1'`).Scan(&one); err == nil {
		t.Fatal("an existing profile's job was matched by the seeding")
	}
	lost := build("Lost") // created with no event
	if n, err := w.Sweep(); err != nil || n != 1 {
		t.Fatalf("second sweep noted %d, %v", n, err)
	}
	clk.advance(2 * time.Second)
	if w.processDue(context.Background()) != 1 {
		t.Fatal("the lost profile was not re-matched")
	}
	var got string
	if err := db.JobsDB.QueryRow(`SELECT profile_id FROM job_matches WHERE job_id = 'j1'`).Scan(&got); err != nil || got != lost {
		t.Fatalf("match %q, want the lost profile %q (the existing one is %q), %v", got, lost, existing, err)
	}
	// done: nothing is stale any more, a third sweep finds nothing
	if n, _ := w.Sweep(); n != 0 {
		t.Errorf("third sweep noted %d", n)
	}
}
