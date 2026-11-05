package match

import (
	"slices"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func stateOf(t *testing.T, profileID string) (string, bool) {
	t.Helper()
	var fp string
	if err := db.JobsDB.QueryRow(`SELECT fingerprint FROM profile_match_state WHERE profile_id = ?`, profileID).Scan(&fp); err != nil {
		return "", false
	}
	return fp, true
}

func stale(t *testing.T) []string {
	t.Helper()
	ids, err := StaleProfiles()
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func fingerprintOf(t *testing.T, id string) string {
	t.Helper()
	fp, ok, err := ProfileFingerprint(id)
	if err != nil || !ok || fp == "" {
		t.Fatalf("fingerprint %q %v %v", fp, ok, err)
	}
	return fp
}

// The fingerprint is the set of personas: a new persona changes it, an edit of one does not, the order of creation does not matter.
func TestTheProfileFingerprintIsTheSetOfPersonas(t *testing.T) {
	setupJobsDB(t)
	pid, per := seedProfile(t, "Fit", fitSkills)
	other, _ := profile.CreateProfile("Other", "x")
	first := fingerprintOf(t, pid)
	if fingerprintOf(t, other) == first {
		t.Error("two profiles with different personas share a fingerprint")
	}
	empty := fingerprintOf(t, other)

	// edits change nothing
	h := "Changed headline"
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: per, Headline: &h}); err != nil {
		t.Fatal(err)
	}
	if err := persona.AddCareerSkill(per, "Rust"); err != nil {
		t.Fatal(err)
	}
	if fingerprintOf(t, pid) != first {
		t.Error("an edit of a persona changed the fingerprint")
	}
	// a new persona does
	if _, err := persona.CreatePersona(pid, "second", ""); err != nil {
		t.Fatal(err)
	}
	if fingerprintOf(t, pid) == first {
		t.Error("a new persona did not change the fingerprint")
	}
	// deleting a persona does too, and an empty profile has a stable one
	if fingerprintOf(t, other) != empty {
		t.Error("an untouched empty profile changed")
	}
	if _, ok, err := ProfileFingerprint("nope"); err != nil || ok {
		t.Errorf("unknown profile: %v %v", ok, err)
	}
}

func TestTheFingerprintDoesNotDependOnTheOrder(t *testing.T) {
	if fingerprintOfIDs([]string{"a", "b", "c"}) != fingerprintOfIDs([]string{"c", "a", "b"}) {
		t.Error("order matters")
	}
	if fingerprintOfIDs([]string{"a", "b"}) == fingerprintOfIDs([]string{"ab"}) {
		t.Error("ids run together")
	}
	in := []string{"b", "a"}
	fingerprintOfIDs(in)
	if !slices.Equal(in, []string{"b", "a"}) {
		t.Error("the caller's slice was sorted")
	}
}

// A completed re-match stores the fingerprint the run started from; an unknown profile stores nothing; a run that fails to start does not.
func TestARematchStoresTheStateWhenItCompletes(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	pid, _ := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
	if _, ok := stateOf(t, pid); ok {
		t.Fatal("state before any run")
	}
	rematch(t, pid)
	fp, ok := stateOf(t, pid)
	if !ok || fp != fingerprintOf(t, pid) {
		t.Fatalf("state %q %v", fp, ok)
	}
	// with no job at all the run still completes and is remembered
	other, _ := profile.CreateProfile("Empty", "x")
	exec(t, `DELETE FROM jobs`)
	rematch(t, other)
	if fp, ok := stateOf(t, other); !ok || fp != fingerprintOf(t, other) {
		t.Errorf("no jobs: %q %v", fp, ok)
	}
	// an unknown profile has no state
	rematch(t, "nope")
	if _, ok := stateOf(t, "nope"); ok {
		t.Error("state for an unknown profile")
	}
}

func TestAFailedRunStoresNoState(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	pid, _ := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
	exec(t, `ALTER TABLE jobs RENAME TO jobs_gone`)
	if _, err := RematchProfile(pid); err == nil {
		t.Fatal("the run should fail without the job list")
	}
	if _, ok := stateOf(t, pid); ok {
		t.Error("a failed run stored its state")
	}
	if got := stale(t); !slices.Equal(got, []string{pid}) {
		t.Errorf("stale %v", got)
	}
}

// The fingerprint stored is the one read before scoring: a persona created while the run goes on leaves the profile stale.
func TestAPersonaCreatedDuringARunLeavesTheProfileStale(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	pid, _ := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
	old := afterRematchChunk
	afterRematchChunk = func() {
		if _, err := persona.CreatePersona(pid, "late", ""); err != nil {
			t.Error(err)
		}
	}
	defer func() { afterRematchChunk = old }()
	rematch(t, pid)
	afterRematchChunk = func() {}
	if got := stale(t); !slices.Equal(got, []string{pid}) {
		t.Errorf("stale %v, want the profile", got)
	}
}

// Stale means: no state, or a different fingerprint; a matched profile is not stale; oldest profile first.
func TestStaleProfiles(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	young, _ := profileAt(t, "Young", "2021-01-01 00:00:00", fitSkills)
	old, _ := profileAt(t, "Old", "2020-01-01 00:00:00", fitSkills)
	if got := stale(t); !slices.Equal(got, []string{old, young}) {
		t.Fatalf("no state: %v", got)
	}
	rematch(t, old)
	if got := stale(t); !slices.Equal(got, []string{young}) {
		t.Fatalf("one matched: %v", got)
	}
	rematch(t, young)
	if got := stale(t); len(got) != 0 {
		t.Fatalf("all matched: %v", got)
	}
	// an edit does not make it stale, a new persona does
	per, _ := persona.CreatePersona(old, "again", "")
	if got := stale(t); !slices.Equal(got, []string{old}) {
		t.Fatalf("new persona: %v", got)
	}
	rematch(t, old)
	if err := persona.AddCareerSkill(per, "Rust"); err != nil {
		t.Fatal(err)
	}
	if got := stale(t); len(got) != 0 {
		t.Errorf("an edit made it stale: %v", got)
	}
}

// The state of a profile that no longer exists is removed.
func TestStaleProfilesRemovesTheStateOfDeletedProfiles(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	pid, _ := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
	other, _ := profileAt(t, "Other", "2020-01-02 00:00:00", fitSkills)
	rematch(t, pid)
	rematch(t, other)
	if err := profile.DeleteProfile(pid); err != nil {
		t.Fatal(err)
	}
	if got := stale(t); len(got) != 0 {
		t.Fatalf("stale %v", got)
	}
	if _, ok := stateOf(t, pid); ok {
		t.Error("the deleted profile's state stayed")
	}
	if _, ok := stateOf(t, other); !ok {
		t.Error("the other profile's state was removed")
	}
}

// The first run gives the profiles that exist their fingerprint without re-matching; it happens once; what comes later is stale.
func TestSeedingGivesExistingProfilesTheirFingerprintOnce(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	a, _ := profileAt(t, "A", "2020-01-01 00:00:00", fitSkills)
	b, _ := profileAt(t, "B", "2020-01-02 00:00:00", fitSkills)
	n, err := SeedProfileMatchState()
	if err != nil || n != 2 {
		t.Fatalf("seeded %d, %v", n, err)
	}
	for _, id := range []string{a, b} {
		if fp, ok := stateOf(t, id); !ok || fp != fingerprintOf(t, id) {
			t.Errorf("state of %s: %q %v", id, fp, ok)
		}
	}
	if got := stale(t); len(got) != 0 {
		t.Errorf("seeded profiles are stale: %v", got)
	}
	if _, ok := readStoredMatch(t, "j-probe"); ok {
		t.Error("seeding re-matched a job")
	}
	// once: a profile created later is not seeded by a second call, so it is stale
	c, _ := profileAt(t, "C", "2020-01-03 00:00:00", fitSkills)
	if n, err := SeedProfileMatchState(); err != nil || n != 0 {
		t.Fatalf("second seeding %d, %v", n, err)
	}
	if got := stale(t); !slices.Equal(got, []string{c}) {
		t.Errorf("stale %v, want the new profile", got)
	}
}

// With no profile at the first run the seeding is still done, so the first profile after it is not mistaken for an existing one.
func TestSeedingWithNoProfilesIsStillDone(t *testing.T) {
	setupJobsDB(t)
	if n, err := SeedProfileMatchState(); err != nil || n != 0 {
		t.Fatalf("%d, %v", n, err)
	}
	pid, _ := profileAt(t, "Late", "2020-01-01 00:00:00", fitSkills)
	if n, _ := SeedProfileMatchState(); n != 0 {
		t.Error("seeded again")
	}
	if got := stale(t); !slices.Equal(got, []string{pid}) {
		t.Errorf("stale %v", got)
	}
}

// A profile that already has a state keeps it when the seeding runs.
func TestSeedingKeepsAnExistingState(t *testing.T) {
	setupJobsDB(t)
	pid, _ := profileAt(t, "A", "2020-01-01 00:00:00", fitSkills)
	if err := setProfileMatchState(pid, "old"); err != nil {
		t.Fatal(err)
	}
	if n, err := SeedProfileMatchState(); err != nil || n != 0 {
		t.Fatalf("%d, %v", n, err)
	}
	if fp, _ := stateOf(t, pid); fp != "old" {
		t.Errorf("state %q", fp)
	}
}
