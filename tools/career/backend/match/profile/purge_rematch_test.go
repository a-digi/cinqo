package profilematch

import (
	"career-tool-backend/match"
	"slices"
	"sync"
	"testing"
	"time"

	"career-tool-backend/profile"
)

// A purge that arrives while a re-match of the profile runs waits for it, so the re-match cannot write rows after the purge.
func TestAPurgeWaitsForARunningRematch(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	pid, _ := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
	var purged sync.WaitGroup
	finished := make(chan bool, 1)
	old := afterRematchChunk
	afterRematchChunk = func() {
		afterRematchChunk = func() {}
		purged.Add(1)
		go func() {
			defer purged.Done()
			_, _ = match.PurgeProfileMatches(pid)
		}()
		time.Sleep(200 * time.Millisecond)
		// the run is still going (it holds the lock): the purge cannot have removed what the run wrote
		finished <- count(t, `SELECT COUNT(*) FROM job_matches WHERE profile_id = ?`, pid) == 1
	}
	defer func() { afterRematchChunk = old }()
	rematch(t, pid)
	purged.Wait()
	if !<-finished {
		t.Error("the purge ran in the middle of the re-match")
	}
	if n := count(t, `SELECT COUNT(*) FROM job_matches WHERE profile_id = ?`, pid); n != 0 {
		t.Errorf("%d rows left after the purge", n)
	}
}

// After the profile is gone and its matches purged, a re-match for it writes nothing and stores no state.
func TestARematchAfterThePurgeWritesNothing(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	pid, _ := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
	rematch(t, pid)
	if count(t, `SELECT COUNT(*) FROM job_matches`) != 1 {
		t.Fatal("setup")
	}
	if err := profile.DeleteProfile(pid); err != nil {
		t.Fatal(err)
	}
	purgeProfile(t, pid)
	rematch(t, pid)
	if count(t, `SELECT COUNT(*) FROM job_matches`) != 0 || count(t, `SELECT COUNT(*) FROM profile_match_state`) != 0 {
		t.Error("a re-match after the purge wrote rows")
	}
}

// With the real engine: the jobs a deleted profile had matched lose the match and are returned, and matching them again gives the
// profile that remains.
func TestTheFreedJobsAreMatchedAgainByTheRemainingProfile(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	addMatchJob(t, "j2")
	gone, _ := profileAt(t, "Gone", "2020-01-01 00:00:00", fitSkills)
	kept, _ := profileAt(t, "Kept", "2020-01-02 00:00:00", weakSkills)
	rematch(t, gone)
	if m, _ := readStoredMatch(t, "j-probe"); m.profileID != gone {
		t.Fatalf("setup: %+v", m)
	}
	if err := profile.DeleteProfile(gone); err != nil {
		t.Fatal(err)
	}
	freed := purgeProfile(t, gone)
	if !slices.Equal(freed, []string{"j-probe", "j2"}) {
		t.Fatalf("freed %v", freed)
	}
	if _, ok := readStoredMatch(t, "j-probe"); ok {
		t.Fatal("the zombie match is still there")
	}
	out, err := AutoMatchJobs(freed)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range out {
		if o.Status != AutoMatched || o.ProfileID != kept {
			t.Errorf("%+v, want matched by the remaining profile %s", o, kept)
		}
	}
}
