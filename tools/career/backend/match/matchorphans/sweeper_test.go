package matchorphans

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func watch(t *testing.T, freed []string, report jobs.OrphanReport, err error) (calls *int, handed *[][]string) {
	t.Helper()
	n := 0
	var h [][]string
	op, oh := purgeOrphans, handOver
	purgeOrphans = func() (jobs.OrphanReport, []string, error) { n++; return report, freed, err }
	handOver = func(ids []string) int { h = append(h, ids); return len(ids) }
	t.Cleanup(func() { purgeOrphans, handOver = op, oh })
	return &n, &h
}

func TestASweepRemovesTheOrphansAndHandsTheFreedJobsOver(t *testing.T) {
	calls, handed := watch(t, []string{"j1", "j2"}, jobs.OrphanReport{Matches: 2}, nil)
	rep, err := Sweep()
	if err != nil || rep.Matches != 2 || *calls != 1 {
		t.Fatalf("%+v %v calls %d", rep, err, *calls)
	}
	if len(*handed) != 1 || !slices.Equal((*handed)[0], []string{"j1", "j2"}) {
		t.Errorf("handed over %v", *handed)
	}
}

func TestNothingIsHandedOverWhenNothingWasFreed(t *testing.T) {
	_, handed := watch(t, nil, jobs.OrphanReport{}, nil)
	if _, err := Sweep(); err != nil || len(*handed) != 0 {
		t.Errorf("%v handed %v", err, *handed)
	}
}

func TestAFailedSweepReportsTheErrorAndHandsNothingOver(t *testing.T) {
	_, handed := watch(t, []string{"j1"}, jobs.OrphanReport{}, errors.New("db gone"))
	if _, err := Sweep(); err == nil || len(*handed) != 0 {
		t.Errorf("%v handed %v", err, *handed)
	}
}

func TestASweepThatFailsOrPanicsDoesNotStopTheSweeper(t *testing.T) {
	n := 0
	op, oh := purgeOrphans, handOver
	defer func() { purgeOrphans, handOver = op, oh }()
	handOver = func(ids []string) int { return len(ids) }
	purgeOrphans = func() (jobs.OrphanReport, []string, error) {
		n++
		switch n {
		case 1:
			return jobs.OrphanReport{}, nil, errors.New("boom")
		case 2:
			panic("boom")
		}
		return jobs.OrphanReport{Matches: 1}, []string{"j"}, nil
	}
	sweepOnce()
	sweepOnce()
	sweepOnce()
	if n != 3 {
		t.Errorf("%d sweeps ran", n)
	}
}

func TestTheSweeperRunsOnATimerAndStops(t *testing.T) {
	od, oi := sweepStartDelay, sweepInterval
	sweepStartDelay, sweepInterval = 5*time.Millisecond, 5*time.Millisecond
	defer func() { sweepStartDelay, sweepInterval = od, oi }()
	calls := make(chan struct{}, 100)
	op, oh := purgeOrphans, handOver
	defer func() { purgeOrphans, handOver = op, oh }()
	handOver = func(ids []string) int { return 0 }
	purgeOrphans = func() (jobs.OrphanReport, []string, error) { calls <- struct{}{}; return jobs.OrphanReport{}, nil, nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := StartSweeper(ctx)
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
	od := sweepStartDelay
	sweepStartDelay = time.Hour
	defer func() { sweepStartDelay = od }()
	calls, _ := watch(t, nil, jobs.OrphanReport{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := StartSweeper(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	if *calls != 0 {
		t.Error("swept before the start delay")
	}
}

func TestTheShippedTiming(t *testing.T) {
	if sweepStartDelay != 30*time.Second || sweepInterval != 10*time.Minute {
		t.Errorf("%v %v", sweepStartDelay, sweepInterval)
	}
}

// With real tables: the matches of a profile whose delete event was lost are removed by the sweep and its jobs are handed over; what
// belongs to a profile that exists stays.
func TestARestartRemovesTheMatchesOfAProfileWhoseEventWasLost(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	pid, _ := profile.CreateProfile("Live", "x")
	per, _ := persona.CreatePersona(pid, "p", "")
	for _, id := range []string{"j-live", "j-zombie"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title) VALUES (?, ?, 'T')`, id, "https://x.example/"+id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO job_matches (job_id, profile_id, persona_id, score, status, kind) VALUES ('j-live', ?, ?, 60, 'completed', 'manual'), ('j-zombie', 'deleted-long-ago', 'its-persona', 85, 'completed', 'ai')`, pid, per); err != nil {
		t.Fatal(err)
	}
	var handed []string
	oh := handOver
	handOver = func(ids []string) int { handed = append(handed, ids...); return len(ids) }
	defer func() { handOver = oh }()
	rep, err := Sweep()
	if err != nil || rep.Matches != 1 || rep.Profiles != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_matches`).Scan(&n)
	if n != 1 || !slices.Equal(handed, []string{"j-zombie"}) {
		t.Errorf("%d matches left, handed %v", n, handed)
	}
	// the next sweep has nothing to do
	handed = nil
	if rep, _ := Sweep(); rep.Matches != 0 || len(handed) != 0 {
		t.Errorf("second sweep: %+v %v", rep, handed)
	}
}
