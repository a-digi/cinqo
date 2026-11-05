package match

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/profile"
)

func count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// zombieJob adds a job with a job_matches row (and one skill) for the profile and persona.
func zombieJob(t *testing.T, id, profileID, personaID, kind, status string) {
	t.Helper()
	addMatchJob(t, id)
	var persona any
	if personaID != "" {
		persona = personaID
	}
	exec(t, `INSERT INTO job_matches (job_id, profile_id, persona_id, score, status, kind) VALUES (?, ?, ?, 50, ?, ?)`, id, profileID, persona, status, kind)
	exec(t, `INSERT INTO job_match_skills (job_id, skill) VALUES (?, 'Go')`, id)
}

func personaRows(t *testing.T, jobID, personaID, profileID string) {
	t.Helper()
	exec(t, `INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES (?, ?, ?, 40)`, jobID, personaID, profileID)
}

func purgeProfile(t *testing.T, id string) []string {
	t.Helper()
	got, err := PurgeProfileMatches(id)
	if err != nil {
		t.Fatalf("PurgeProfileMatches: %v", err)
	}
	return got
}

func purgePersona(t *testing.T, personaID, profileID string) []string {
	t.Helper()
	got, err := PurgePersonaMatches(personaID, profileID)
	if err != nil {
		t.Fatalf("PurgePersonaMatches: %v", err)
	}
	return got
}

// Everything of the profile goes, whatever the kind or status of the match, and only that: the other profile's rows stay.
func TestPurgingAProfileRemovesAllItsMatchesAndNoOthers(t *testing.T) {
	setupJobsDB(t)
	for _, c := range []struct{ id, kind, status string }{
		{"j-manual", "manual", "completed"}, {"j-ai", "ai", "completed"}, {"j-run", "ai", "matching"}, {"j-fail", "ai", "failed"}, {"j-mfail", "manual", "failed"},
	} {
		zombieJob(t, c.id, "gone", "p-gone", c.kind, c.status)
	}
	zombieJob(t, "j-other", "other", "p-other", "manual", "completed")
	personaRows(t, "j-manual", "p-gone", "gone")
	personaRows(t, "j-ai", "p-gone2", "gone")
	personaRows(t, "j-other", "p-other", "other")
	exec(t, `INSERT INTO persona_score_state (persona_id, profile_id, fingerprint) VALUES ('p-gone', 'gone', 'x'), ('p-gone2', 'gone', 'x'), ('p-other', 'other', 'x')`)
	exec(t, `INSERT INTO profile_match_state (profile_id, fingerprint) VALUES ('gone', 'x'), ('other', 'x')`)

	got := purgeProfile(t, "gone")
	if want := []string{"j-ai", "j-fail", "j-manual", "j-mfail", "j-run"}; !slices.Equal(got, want) {
		t.Fatalf("freed %v, want %v", got, want)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM job_matches WHERE profile_id = 'gone'`,
		`SELECT COUNT(*) FROM job_match_skills WHERE job_id IN ('j-manual','j-ai','j-run','j-fail','j-mfail')`,
		`SELECT COUNT(*) FROM job_persona_matches WHERE profile_id = 'gone'`,
		`SELECT COUNT(*) FROM persona_score_state WHERE profile_id = 'gone'`,
		`SELECT COUNT(*) FROM profile_match_state WHERE profile_id = 'gone'`,
	} {
		if n := count(t, q); n != 0 {
			t.Errorf("%s: %d left", q, n)
		}
	}
	// the other profile and the jobs themselves are untouched
	for q, want := range map[string]int{
		`SELECT COUNT(*) FROM job_matches WHERE profile_id = 'other'`:         1,
		`SELECT COUNT(*) FROM job_match_skills WHERE job_id = 'j-other'`:      1,
		`SELECT COUNT(*) FROM job_persona_matches WHERE profile_id = 'other'`: 1,
		`SELECT COUNT(*) FROM persona_score_state WHERE profile_id = 'other'`: 1,
		`SELECT COUNT(*) FROM profile_match_state WHERE profile_id = 'other'`: 1,
		`SELECT COUNT(*) FROM jobs`:                                           6,
	} {
		if n := count(t, q); n != want {
			t.Errorf("%s: %d, want %d", q, n, want)
		}
	}
}

// A second purge removes nothing and says so.
func TestPurgesAreIdempotent(t *testing.T) {
	setupJobsDB(t)
	zombieJob(t, "j1", "gone", "p-gone", "manual", "completed")
	personaRows(t, "j1", "p-gone", "gone")
	if got := purgeProfile(t, "gone"); !slices.Equal(got, []string{"j1"}) {
		t.Fatal(got)
	}
	if got := purgeProfile(t, "gone"); len(got) != 0 {
		t.Errorf("second profile purge freed %v", got)
	}
	zombieJob(t, "j2", "keep", "p1", "manual", "completed")
	if got := purgePersona(t, "p1", "keep"); !slices.Equal(got, []string{"j2"}) {
		t.Fatal(got)
	}
	if got := purgePersona(t, "p1", "keep"); len(got) != 0 {
		t.Errorf("second persona purge freed %v", got)
	}
	if got := purgeProfile(t, "never-existed"); len(got) != 0 {
		t.Errorf("unknown profile freed %v", got)
	}
}

// A persona purge removes the matches that persona produced for that profile, its scores and its state; nothing else.
func TestPurgingAPersonaRemovesOnlyItsOwnMatches(t *testing.T) {
	setupJobsDB(t)
	zombieJob(t, "j-mine", "prof", "p1", "manual", "completed")
	zombieJob(t, "j-mine-ai", "prof", "p1", "ai", "completed")
	zombieJob(t, "j-sibling", "prof", "p2", "manual", "completed")                   // another persona of the same profile
	zombieJob(t, "j-samepersona-otherprofile", "other", "p1", "manual", "completed") // same persona id under another profile
	zombieJob(t, "j-running", "prof", "", "ai", "matching")                          // a running AI match has no persona yet
	personaRows(t, "j-mine", "p1", "prof")
	personaRows(t, "j-sibling", "p2", "prof")
	exec(t, `INSERT INTO persona_score_state (persona_id, profile_id, fingerprint) VALUES ('p1', 'prof', 'x'), ('p2', 'prof', 'x')`)
	exec(t, `INSERT INTO profile_match_state (profile_id, fingerprint) VALUES ('prof', 'x')`)

	got := purgePersona(t, "p1", "prof")
	if want := []string{"j-mine", "j-mine-ai"}; !slices.Equal(got, want) {
		t.Fatalf("freed %v, want %v", got, want)
	}
	for q, want := range map[string]int{
		`SELECT COUNT(*) FROM job_matches WHERE persona_id = 'p1' AND profile_id = 'prof'`:                             0,
		`SELECT COUNT(*) FROM job_match_skills WHERE job_id IN ('j-mine','j-mine-ai')`:                                 0,
		`SELECT COUNT(*) FROM job_persona_matches WHERE persona_id = 'p1'`:                                             0,
		`SELECT COUNT(*) FROM persona_score_state WHERE persona_id = 'p1'`:                                             0,
		`SELECT COUNT(*) FROM job_matches`:                                                                             3,
		`SELECT COUNT(*) FROM job_match_skills WHERE job_id IN ('j-sibling','j-samepersona-otherprofile','j-running')`: 3,
		`SELECT COUNT(*) FROM job_persona_matches WHERE persona_id = 'p2'`:                                             1,
		`SELECT COUNT(*) FROM persona_score_state WHERE persona_id = 'p2'`:                                             1,
		`SELECT COUNT(*) FROM profile_match_state WHERE profile_id = 'prof'`:                                           1,
		`SELECT COUNT(*) FROM job_matches WHERE job_id = 'j-samepersona-otherprofile'`:                                 1,
		`SELECT COUNT(*) FROM job_matches WHERE job_id = 'j-running'`:                                                  1,
	} {
		if n := count(t, q); n != want {
			t.Errorf("%s: %d, want %d", q, n, want)
		}
	}
}

// An empty id is refused and nothing is removed.
func TestAPurgeWithoutAnIDRemovesNothing(t *testing.T) {
	setupJobsDB(t)
	zombieJob(t, "j1", "prof", "p1", "manual", "completed")
	personaRows(t, "j1", "p1", "prof")
	for name, call := range map[string]func() ([]string, error){
		"profile":           func() ([]string, error) { return PurgeProfileMatches("") },
		"persona, no id":    func() ([]string, error) { return PurgePersonaMatches("", "prof") },
		"persona, no owner": func() ([]string, error) { return PurgePersonaMatches("p1", "") },
		"persona, neither":  func() ([]string, error) { return PurgePersonaMatches("", "") },
	} {
		if got, err := call(); !errors.Is(err, ErrNoID) || got != nil {
			t.Errorf("%s: %v, %v", name, got, err)
		}
	}
	if count(t, `SELECT COUNT(*) FROM job_matches`) != 1 || count(t, `SELECT COUNT(*) FROM job_persona_matches`) != 1 {
		t.Error("something was removed")
	}
}

// If any part fails, nothing is removed (one transaction): the later tables are missing here, the job_matches rows must stay.
func TestAFailedPurgeChangesNothing(t *testing.T) {
	for name, tc := range map[string]struct {
		drop string
		run  func() error
	}{
		"profile": {"profile_match_state", func() error { _, err := PurgeProfileMatches("prof"); return err }},
		"persona": {"persona_score_state", func() error { _, err := PurgePersonaMatches("p1", "prof"); return err }},
	} {
		t.Run(name, func(t *testing.T) {
			setupJobsDB(t)
			zombieJob(t, "j1", "prof", "p1", "manual", "completed")
			personaRows(t, "j1", "p1", "prof")
			exec(t, `DROP TABLE `+tc.drop)
			if err := tc.run(); err == nil {
				t.Fatal("expected an error")
			}
			if count(t, `SELECT COUNT(*) FROM job_matches`) != 1 || count(t, `SELECT COUNT(*) FROM job_match_skills`) != 1 || count(t, `SELECT COUNT(*) FROM job_persona_matches`) != 1 {
				t.Error("a failed purge removed part of the rows")
			}
			// and the connection is usable again (the transaction was rolled back)
			exec(t, `INSERT INTO jobs (id, source_url, title) VALUES ('after', 'https://x.example/after', 't')`)
		})
	}
}

// The purges take the profile's lock: they wait for a running re-match of that profile and not for another profile's.
func TestAPurgeWaitsForTheProfilesLock(t *testing.T) {
	setupJobsDB(t)
	zombieJob(t, "j1", "prof", "p1", "manual", "completed")
	unlock := jobs.LockProfile("prof")
	done := make(chan struct{}, 2)
	go func() { _, _ = PurgeProfileMatches("prof"); done <- struct{}{} }()
	go func() { _, _ = PurgePersonaMatches("p1", "prof"); done <- struct{}{} }()
	select {
	case <-done:
		t.Fatal("a purge ran while the profile was locked")
	case <-time.After(200 * time.Millisecond):
	}
	// another profile's purge is not held up
	zombieJob(t, "j2", "other", "p9", "manual", "completed")
	otherDone := make(chan struct{})
	go func() { _, _ = PurgeProfileMatches("other"); close(otherDone) }()
	select {
	case <-otherDone:
	case <-time.After(5 * time.Second):
		t.Fatal("another profile's purge was blocked")
	}
	unlock()
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("a purge did not run after the unlock")
		}
	}
}

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
			_, _ = PurgeProfileMatches(pid)
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
