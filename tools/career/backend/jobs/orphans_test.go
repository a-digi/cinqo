package jobs

import (
	"slices"
	"testing"
	"time"

	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

// An orphan world: "live" is a real profile with a real persona; "gone" and "gone-persona" name things that do not exist.
type world struct {
	liveProfile, livePersona string
	siblingPersona           string // a second real persona of the live profile
}

func newWorld(t *testing.T) world {
	t.Helper()
	setupJobsDB(t)
	pid, err := profile.CreateProfile("Live", "x")
	if err != nil {
		t.Fatal(err)
	}
	per, err := persona.CreatePersona(pid, "live persona", "")
	if err != nil {
		t.Fatal(err)
	}
	sib, err := persona.CreatePersona(pid, "sibling", "")
	if err != nil {
		t.Fatal(err)
	}
	return world{pid, per, sib}
}

func purgeOrphans(t *testing.T) (OrphanReport, []string) {
	t.Helper()
	r, freed, err := PurgeOrphans()
	if err != nil {
		t.Fatalf("PurgeOrphans: %v", err)
	}
	return r, freed
}

func seedOrphans(t *testing.T, w world) {
	t.Helper()
	zombieJob(t, "j-live", w.liveProfile, w.livePersona, "manual", "completed")             // fine
	zombieJob(t, "j-sibling", w.liveProfile, w.siblingPersona, "ai", "completed")           // fine
	zombieJob(t, "j-run", w.liveProfile, "", "ai", "matching")                              // a running AI match has no persona: fine
	zombieJob(t, "j-gone-manual", "gone", "gone-persona", "manual", "completed")            // profile deleted
	zombieJob(t, "j-gone-ai", "gone", "gone-persona", "ai", "completed")                    // profile deleted, AI
	zombieJob(t, "j-gone-run", "gone", "", "ai", "matching")                                // profile deleted, running
	zombieJob(t, "j-gone-persona", w.liveProfile, "deleted-persona", "manual", "completed") // persona deleted, profile fine
	zombieJob(t, "j-moved", w.liveProfile, "other-profiles-persona", "manual", "completed")
	personaRows(t, "j-live", w.livePersona, w.liveProfile)
	personaRows(t, "j-sibling", w.siblingPersona, w.liveProfile)
	personaRows(t, "j-gone-manual", "gone-persona", "gone")
	personaRows(t, "j-gone-persona", "deleted-persona", w.liveProfile)
	exec(t, `INSERT INTO persona_score_state (persona_id, profile_id, fingerprint) VALUES (?, ?, 'x'), (?, ?, 'x'), ('gone-persona', 'gone', 'x'), ('deleted-persona', ?, 'x')`,
		w.livePersona, w.liveProfile, w.siblingPersona, w.liveProfile, w.liveProfile)
	exec(t, `INSERT INTO profile_match_state (profile_id, fingerprint) VALUES (?, 'x'), ('gone', 'x')`, w.liveProfile)
}

// The preview lists exactly the matches whose profile or persona is gone, with the reason, and removes nothing.
func TestThePreviewListsTheOrphansAndRemovesNothing(t *testing.T) {
	w := newWorld(t)
	seedOrphans(t, w)
	got, err := OrphanMatches()
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]string{}
	for _, m := range got {
		reasons[m.JobID] = m.Reason
	}
	want := map[string]string{
		"j-gone-manual": "profile deleted", "j-gone-ai": "profile deleted", "j-gone-run": "profile deleted",
		"j-gone-persona": "persona deleted", "j-moved": "persona deleted",
	}
	if len(reasons) != len(want) {
		t.Fatalf("listed %v, want %v", reasons, want)
	}
	for id, r := range want {
		if reasons[id] != r {
			t.Errorf("%s: %q, want %q", id, reasons[id], r)
		}
	}
	for _, m := range got {
		if m.JobID == "j-gone-ai" && (m.Kind != "ai" || !m.HasScore || m.Score != 50 || m.Title == "" || m.ProfileID != "gone" || m.PersonaID != "gone-persona") {
			t.Errorf("the row does not describe the match: %+v", m)
		}
		if m.JobID == "j-gone-run" && (m.PersonaID != "" || m.Status != "matching") {
			t.Errorf("%+v", m)
		}
	}
	if count(t, `SELECT COUNT(*) FROM job_matches`) != 8 || count(t, `SELECT COUNT(*) FROM job_persona_matches`) != 4 {
		t.Error("the preview removed something")
	}
}

// PurgeOrphans removes the matches, skills, scores and states of what is gone and nothing that belongs to what exists.
func TestPurgeOrphansRemovesOnlyWhatIsGone(t *testing.T) {
	w := newWorld(t)
	seedOrphans(t, w)
	r, freed := purgeOrphans(t)
	if want := []string{"j-gone-ai", "j-gone-manual", "j-gone-persona", "j-gone-run", "j-moved"}; !slices.Equal(freed, want) {
		t.Fatalf("freed %v, want %v", freed, want)
	}
	if r.Profiles != 1 || r.Personas != 2 || r.Matches != 5 || r.PersonaScores != 2 || r.States != 3 {
		t.Errorf("report %+v", r)
	}
	for q, want := range map[string]int{
		`SELECT COUNT(*) FROM job_matches`:                                                3,
		`SELECT COUNT(*) FROM job_matches WHERE job_id IN ('j-live','j-sibling','j-run')`: 3,
		`SELECT COUNT(*) FROM job_match_skills`:                                           3,
		`SELECT COUNT(*) FROM job_persona_matches`:                                        2,
		`SELECT COUNT(*) FROM persona_score_state`:                                        2,
		`SELECT COUNT(*) FROM profile_match_state`:                                        1,
		`SELECT COUNT(*) FROM jobs`:                                                       8,
	} {
		if n := count(t, q); n != want {
			t.Errorf("%s: %d, want %d", q, n, want)
		}
	}
	// the preview is empty now, a second purge removes nothing
	if got, _ := OrphanMatches(); len(got) != 0 {
		t.Errorf("preview after the purge: %v", got)
	}
	if r2, freed2 := purgeOrphans(t); r2 != (OrphanReport{}) || len(freed2) != 0 {
		t.Errorf("second purge: %+v %v", r2, freed2)
	}
}

// With nothing orphaned nothing is touched, and an empty database is fine.
func TestNoOrphansMeansNothingHappens(t *testing.T) {
	w := newWorld(t)
	if r, freed := purgeOrphans(t); r != (OrphanReport{}) || freed != nil {
		t.Errorf("empty: %+v %v", r, freed)
	}
	zombieJob(t, "j1", w.liveProfile, w.livePersona, "manual", "completed")
	personaRows(t, "j1", w.livePersona, w.liveProfile)
	if r, freed := purgeOrphans(t); r != (OrphanReport{}) || len(freed) != 0 {
		t.Errorf("clean: %+v %v", r, freed)
	}
	if count(t, `SELECT COUNT(*) FROM job_matches`) != 1 || count(t, `SELECT COUNT(*) FROM job_persona_matches`) != 1 {
		t.Error("something was removed")
	}
	if got, err := OrphanMatches(); err != nil || got != nil {
		t.Errorf("preview %v %v", got, err)
	}
}

// A profile that is created, and gets matches, while the sweep is between listing and checking is never mistaken for an orphan: the ids
// are listed first, so a profile that appears afterwards is not in the list at all.
func TestAProfileCreatedDuringTheSweepIsNotAnOrphan(t *testing.T) {
	w := newWorld(t)
	zombieJob(t, "j-gone", "gone", "", "ai", "completed")
	var newcomer string
	old := afterOrphanListing
	afterOrphanListing = func() {
		afterOrphanListing = func() {}
		pid, err := profile.CreateProfile("Newcomer", "x")
		if err != nil {
			t.Error(err)
			return
		}
		newcomer = pid
		zombieJob(t, "j-new", pid, "", "ai", "completed")
	}
	defer func() { afterOrphanListing = old }()
	_, freed := purgeOrphans(t)
	if !slices.Equal(freed, []string{"j-gone"}) {
		t.Fatalf("freed %v", freed)
	}
	if count(t, `SELECT COUNT(*) FROM job_matches WHERE profile_id = ?`, newcomer) != 1 {
		t.Error("the newcomer's match was removed")
	}
	_ = w
}

// A persona that moved to another profile (the pair no longer exists) is an orphan pair; a persona under its own profile is not.
func TestAPairIsJudgedByItsOwnProfile(t *testing.T) {
	w := newWorld(t)
	other, _ := profile.CreateProfile("Other", "x")
	otherPersona, _ := persona.CreatePersona(other, "o", "")
	zombieJob(t, "j-ok", other, otherPersona, "manual", "completed")
	zombieJob(t, "j-wrong", w.liveProfile, otherPersona, "manual", "completed") // the persona exists, but under another profile
	_, freed := purgeOrphans(t)
	if !slices.Equal(freed, []string{"j-wrong"}) {
		t.Errorf("freed %v", freed)
	}
	// the match of the same persona under its own profile is still there: the removal is scoped to the pair
	if count(t, `SELECT COUNT(*) FROM job_matches WHERE job_id = 'j-ok'`) != 1 || count(t, `SELECT COUNT(*) FROM job_match_skills WHERE job_id = 'j-ok'`) != 1 {
		t.Error("the valid match of the same persona was removed with the orphan pair")
	}
	_ = w
}

// A failed purge changes nothing (one transaction).
func TestAFailedOrphanPurgeChangesNothing(t *testing.T) {
	w := newWorld(t)
	seedOrphans(t, w)
	// The failure has to come late, after the matches and scores were already deleted inside the transaction (a dropped table would fail the
	// detection before anything is deleted): a trigger refuses the delete of the re-match state, which is the last step for a profile.
	exec(t, `CREATE TRIGGER refuse_state_delete BEFORE DELETE ON profile_match_state BEGIN SELECT RAISE(ABORT, 'refused'); END`)
	if _, _, err := PurgeOrphans(); err == nil {
		t.Fatal("expected an error")
	}
	if count(t, `SELECT COUNT(*) FROM job_matches`) != 8 || count(t, `SELECT COUNT(*) FROM job_match_skills`) != 8 || count(t, `SELECT COUNT(*) FROM job_persona_matches`) != 4 {
		t.Error("a failed purge removed part of the rows")
	}
	exec(t, `INSERT INTO jobs (id, source_url, title) VALUES ('after', 'https://x.example/after', 't')`) // the connection is usable
}

// The sweep takes the lock of the orphan profile: it waits for a re-match that is running for it, and not for any other profile.
func TestTheOrphanPurgeWaitsForTheLockOfAnOrphanProfile(t *testing.T) {
	w := newWorld(t)
	zombieJob(t, "j-gone", "gone", "", "ai", "completed")
	zombieJob(t, "j-live", w.liveProfile, w.livePersona, "manual", "completed")
	unlock := lockProfile("gone")
	done := make(chan struct{})
	go func() { _, _, _ = PurgeOrphans(); close(done) }()
	select {
	case <-done:
		t.Fatal("the purge ran while the orphan profile was locked")
	case <-time.After(200 * time.Millisecond):
	}
	unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the purge did not run after the unlock")
	}
	if count(t, `SELECT COUNT(*) FROM job_matches WHERE profile_id = 'gone'`) != 0 {
		t.Error("the orphan was not removed")
	}
	// the lock of a profile that exists does not hold a purge that has nothing to do with it
	zombieJob(t, "j-gone2", "gone2", "", "ai", "completed")
	hold := lockProfile(w.liveProfile)
	defer hold()
	ok := make(chan struct{})
	go func() { _, _, _ = PurgeOrphans(); close(ok) }()
	select {
	case <-ok:
	case <-time.After(5 * time.Second):
		t.Fatal("a live profile's lock held up the purge of another profile's orphans")
	}
}

// More ids than one lookup carries: the detection chunks its questions to career.db.
func TestManyIdsAreCheckedInChunks(t *testing.T) {
	w := newWorld(t)
	for i := 0; i < 950; i++ {
		id := "p" + time.Duration(i).String()
		exec(t, `INSERT INTO profile_match_state (profile_id, fingerprint) VALUES (?, 'x')`, id)
	}
	exec(t, `INSERT INTO profile_match_state (profile_id, fingerprint) VALUES (?, 'x')`, w.liveProfile)
	r, _ := purgeOrphans(t)
	if r.Profiles != 950 || r.States != 950 {
		t.Errorf("%+v", r)
	}
	if count(t, `SELECT COUNT(*) FROM profile_match_state WHERE profile_id = ?`, w.liveProfile) != 1 {
		t.Error("the live profile's state was removed")
	}
}

func TestTheHelpers(t *testing.T) {
	if got := chunks([]string{"a", "b", "c", "d", "e"}, 2); len(got) != 3 || len(got[0]) != 2 || len(got[2]) != 1 {
		t.Errorf("%v", got)
	}
	if got := chunks(nil, 2); got != nil {
		t.Errorf("%v", got)
	}
	if got := chunks([]string{"a", "b"}, 2); len(got) != 1 {
		t.Errorf("%v", got)
	}
	if placeholders(3) != "?,?,?" || placeholders(1) != "?" {
		t.Error("placeholders")
	}
}

// A profile that is gone is found whichever table still mentions it: scores only, score state only, re-match state only.
func TestADeletedProfileIsFoundInAnyOfTheTables(t *testing.T) {
	for name, insert := range map[string]string{
		"only persona scores": `INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES ('j1', 'p', 'gone', 40)`,
		"only score state":    `INSERT INTO persona_score_state (persona_id, profile_id, fingerprint) VALUES ('p', 'gone', 'x')`,
		"only re-match state": `INSERT INTO profile_match_state (profile_id, fingerprint) VALUES ('gone', 'x')`,
		"only a job match":    `INSERT INTO job_matches (job_id, profile_id, status, kind) VALUES ('j1', 'gone', 'matching', 'ai')`,
	} {
		t.Run(name, func(t *testing.T) {
			newWorld(t)
			addMatchJob(t, "j1")
			exec(t, insert)
			r, _ := purgeOrphans(t)
			if r.Profiles != 1 {
				t.Fatalf("%+v", r)
			}
			for _, q := range []string{`SELECT COUNT(*) FROM job_persona_matches`, `SELECT COUNT(*) FROM persona_score_state`, `SELECT COUNT(*) FROM profile_match_state`, `SELECT COUNT(*) FROM job_matches`} {
				if n := count(t, q); n != 0 {
					t.Errorf("%s: %d left", q, n)
				}
			}
		})
	}
}

// A match whose persona id is an empty string (not NULL) has no persona to be gone: it is not an orphan pair.
func TestAnEmptyPersonaIDIsNotAnOrphanPair(t *testing.T) {
	w := newWorld(t)
	addMatchJob(t, "j1")
	exec(t, `INSERT INTO job_matches (job_id, profile_id, persona_id, status, kind) VALUES ('j1', ?, '', 'matching', 'ai')`, w.liveProfile)
	r, freed := purgeOrphans(t)
	if r != (OrphanReport{}) || len(freed) != 0 {
		t.Errorf("%+v %v", r, freed)
	}
	if got, _ := OrphanMatches(); len(got) != 0 {
		t.Errorf("preview %v", got)
	}
}
