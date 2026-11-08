package personamatching

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/match/jobmatcher"
	"career-tool-backend/match/personamatch"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func base() jobmatcher.Persona {
	return jobmatcher.Persona{
		ID: "p1", Name: "Backend", Headline: "Backend Engineer", Summary: "Builds services", Location: "Hamburg",
		DesiredTitles: "Software Engineer", DesiredLocations: "Remote", RemotePreference: "open",
		Skills: []string{"Go", "Docker", "SQL"},
		Experience: []jobmatcher.Experience{
			{Title: "Developer", Description: "Go services", StartDate: "2019-01", EndDate: "2021-01"},
			{Title: "Senior Developer", Description: "Platform", StartDate: "2021-02", EndDate: ""},
		},
	}
}

func TestTheFingerprintIsStableAndSeesEverythingTheMatcherReads(t *testing.T) {
	p := base()
	fp := PersonaFingerprint(p)
	if len(fp) != 64 || fp != PersonaFingerprint(base()) {
		t.Fatalf("not stable or not a sha256: %q", fp)
	}
	changes := map[string]func(*jobmatcher.Persona){
		"headline":          func(p *jobmatcher.Persona) { p.Headline += "!" },
		"summary":           func(p *jobmatcher.Persona) { p.Summary += "!" },
		"location":          func(p *jobmatcher.Persona) { p.Location = "Berlin" },
		"desired titles":    func(p *jobmatcher.Persona) { p.DesiredTitles += ", Lead" },
		"desired locations": func(p *jobmatcher.Persona) { p.DesiredLocations = "Berlin" },
		"remote preference": func(p *jobmatcher.Persona) { p.RemotePreference = "remote_only" },
		"a skill added":     func(p *jobmatcher.Persona) { p.Skills = append(p.Skills, "Kubernetes") },
		"a skill removed":   func(p *jobmatcher.Persona) { p.Skills = p.Skills[:2] },
		"a skill renamed":   func(p *jobmatcher.Persona) { p.Skills = []string{"Go", "Docker", "PostgreSQL"} },
		"an entry added": func(p *jobmatcher.Persona) {
			p.Experience = append(p.Experience, jobmatcher.Experience{Title: "Intern", StartDate: "2017-01", EndDate: "2018-01"})
		},
		"an entry removed":       func(p *jobmatcher.Persona) { p.Experience = p.Experience[:1] },
		"an entry's title":       func(p *jobmatcher.Persona) { p.Experience[0].Title = "Lead Developer" },
		"an entry's description": func(p *jobmatcher.Persona) { p.Experience[0].Description = "Kubernetes" },
		"an entry's start":       func(p *jobmatcher.Persona) { p.Experience[0].StartDate = "2018-06" },
		"an entry's end":         func(p *jobmatcher.Persona) { p.Experience[0].EndDate = "2021-06" },
	}
	for name, change := range changes {
		q := base()
		change(&q)
		if PersonaFingerprint(q) == fp {
			t.Errorf("%s: the fingerprint did not change", name)
		}
	}
}

func TestTheFingerprintIgnoresWhatTheMatcherDoesNotRead(t *testing.T) {
	fp := PersonaFingerprint(base())
	same := map[string]func(*jobmatcher.Persona){
		"the id":                  func(p *jobmatcher.Persona) { p.ID = "another" },
		"the name":                func(p *jobmatcher.Persona) { p.Name = "Renamed" },
		"the order of the skills": func(p *jobmatcher.Persona) { p.Skills = []string{"SQL", "Go", "Docker"} },
		"the order of the entries": func(p *jobmatcher.Persona) {
			p.Experience[0], p.Experience[1] = p.Experience[1], p.Experience[0]
		},
	}
	for name, change := range same {
		q := base()
		change(&q)
		if PersonaFingerprint(q) != fp {
			t.Errorf("%s changed the fingerprint", name)
		}
	}
}

func TestTheFingerprintDependsOnTheEngineVersion(t *testing.T) {
	p := base()
	if fingerprintWith("manual-1", p) == fingerprintWith("manual-2", p) {
		t.Error("another engine version must give another fingerprint")
	}
	if fingerprintWith(jobmatcher.EngineVersion, p) != PersonaFingerprint(p) {
		t.Error("PersonaFingerprint must use the current engine version")
	}
}

// Fields that look alike must not run together: "ab"+"c" is not "a"+"bc".
func TestTheFingerprintKeepsFieldsApart(t *testing.T) {
	a := jobmatcher.Persona{Headline: "ab", Summary: "c"}
	b := jobmatcher.Persona{Headline: "a", Summary: "bc"}
	c := jobmatcher.Persona{Skills: []string{"a", "b"}}
	d := jobmatcher.Persona{Skills: []string{"ab"}}
	if PersonaFingerprint(a) == PersonaFingerprint(b) || PersonaFingerprint(c) == PersonaFingerprint(d) {
		t.Error("two different personas share a fingerprint")
	}
}

func rowsOfJob(t *testing.T, jobID string) []string {
	t.Helper()
	rows, err := personamatch.ForJob(jobID)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, r := range rows {
		out = append(out, r.PersonaID+":"+itoa(r.Score))
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func scoreOf(t *testing.T, jobID, personaID string) (int, bool) {
	t.Helper()
	rows, _ := personamatch.ForJob(jobID)
	for _, r := range rows {
		if r.PersonaID == personaID {
			return r.Score, true
		}
	}
	return 0, false
}

// The recompute gives exactly the numbers the PersonaJobMatchConsumer's scorer gives for that persona.
func TestRecomputeGivesTheSameScoresAsThePersonaScorer(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	addMatchJob(t, "j2")
	_, fit := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	seedProfile(t, "Other", []string{"Cobol", "Fortran"})

	out, err := RecomputePersona(context.Background(), fit)
	if err != nil || out.Skipped || out.Jobs != 2 || out.Scored != 2 {
		t.Fatalf("outcome %+v, err %v", out, err)
	}
	s, _ := NewPersonaScorer()
	for _, job := range []string{"j1", "j2"} {
		want, _ := s.Rows(job)
		var wantScore int
		for _, r := range want {
			if r.PersonaID == fit {
				wantScore = r.Score
			}
		}
		if got, ok := scoreOf(t, job, fit); !ok || got != wantScore {
			t.Errorf("%s: recompute %d (row %v), scorer %d", job, got, ok, wantScore)
		}
	}
}

// Only the persona's own rows change: another persona's rows (here with a recognisable value) stay as they are.
func TestRecomputeLeavesOtherPersonasRowsAlone(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	_, fit := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	if err := personamatch.Replace("j1", []personamatch.Row{{PersonaID: "someone-else", ProfileID: "X", Score: 99, EngineVersion: "sentinel"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := RecomputePersona(context.Background(), fit); err != nil {
		t.Fatal(err)
	}
	if got, ok := scoreOf(t, "j1", "someone-else"); !ok || got != 99 {
		t.Errorf("another persona's row = %d (%v), want 99", got, ok)
	}
	if _, ok := scoreOf(t, "j1", fit); !ok {
		t.Error("the persona must have its row")
	}
}

// A change to the persona changes its scores; an unchanged persona is skipped without touching anything.
func TestRecomputeFollowsAChangeAndSkipsWhenNothingChanged(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	_, id := seedProfile(t, "Starter", []string{"Cobol"})
	if _, err := RecomputePersona(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	before, _ := scoreOf(t, "j1", id)

	// untouched: skipped, and a row we change by hand is not rewritten
	if _, err := db.JobsDB.Exec(`UPDATE job_persona_matches SET score = 99 WHERE job_id = 'j1' AND persona_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	out, err := RecomputePersona(context.Background(), id)
	if err != nil || !out.Skipped || out.Jobs != 0 {
		t.Fatalf("an unchanged persona: %+v %v", out, err)
	}
	if got, _ := scoreOf(t, "j1", id); got != 99 {
		t.Errorf("a skipped run rewrote the row: %d", got)
	}

	// changed: the persona learns the skills the job asks for, so the run is not skipped and the score follows
	for _, s := range []string{"Go", "Docker", "Kubernetes", "PostgreSQL"} {
		if err := persona.AddCareerSkill(id, s); err != nil {
			t.Fatal(err)
		}
	}
	out, err = RecomputePersona(context.Background(), id)
	if err != nil || out.Skipped || out.Jobs != 1 {
		t.Fatalf("after a change: %+v %v", out, err)
	}
	after, _ := scoreOf(t, "j1", id)
	if after == 99 || after <= before || after < 60 {
		t.Errorf("score before %d, after %d: it must follow the persona's new skills", before, after)
	}
}

// A persona that could not judge a job (nothing to compare) has no row; once it can, the recompute gives it one.
func TestRecomputeGivesARowToAPersonaThatCanNowJudgeAndRemovesItWhenItCannot(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	_, id := seedProfile(t, "Fit", []string{"Go", "Docker"})
	// a stale row from when it could judge
	if err := personamatch.Replace("j1", []personamatch.Row{{PersonaID: id, ProfileID: "p", Score: 88}}); err != nil {
		t.Fatal(err)
	}
	// now it has nothing to compare: no skills and no desired titles / headline
	for _, s := range []string{"Go", "Docker"} {
		_ = persona.RemoveCareerSkill(id, s)
	}
	details, err := persona.FetchPersonaDetails(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range details.Experience { // an entry's title is something to compare, too
		if err := persona.RemoveCareerExperience(id, e.ID); err != nil {
			t.Fatal(err)
		}
	}
	empty := ""
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: id, Headline: &empty, DesiredTitles: &empty, Summary: &empty}); err != nil {
		t.Fatal(err)
	}
	out, err := RecomputePersona(context.Background(), id)
	if err != nil || out.Jobs != 1 || out.Scored != 0 {
		t.Fatalf("outcome %+v err %v", out, err)
	}
	if _, ok := scoreOf(t, "j1", id); ok {
		t.Error("a persona that cannot judge must lose its row")
	}
	// and when it can again, the row comes back
	if err := persona.AddCareerSkill(id, "Go"); err != nil {
		t.Fatal(err)
	}
	if _, err := RecomputePersona(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, ok := scoreOf(t, "j1", id); !ok {
		t.Error("the row must come back once the persona can judge the job")
	}
}

func TestRecomputeStoresTheStateOnlyAfterTheWholeRun(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	_, id := seedProfile(t, "Fit", []string{"Go", "Docker"})
	p, profileID, _ := LoadPersonaForMatch(id)
	if _, err := RecomputePersona(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	st, ok, err := personamatch.GetState(id)
	if err != nil || !ok || st.Fingerprint != PersonaFingerprint(p) {
		t.Fatalf("state %+v ok=%v err=%v", st, ok, err)
	}
	var stored string
	if err := db.JobsDB.QueryRow(`SELECT profile_id FROM persona_score_state WHERE persona_id = ?`, id).Scan(&stored); err != nil || stored != profileID {
		t.Errorf("stored profile %q, want %q (%v)", stored, profileID, err)
	}
}

// Chunking visits every job once, in id order, whatever the chunk size.
func TestRecomputeCoversEveryJobInChunks(t *testing.T) {
	setupJobsDB(t)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		addMatchJob(t, id)
	}
	_, pid := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes"})
	old := recomputeChunk
	recomputeChunk = 2
	chunks := 0
	oldHook := afterRecomputeChunk
	afterRecomputeChunk = func() { chunks++ }
	t.Cleanup(func() { recomputeChunk = old; afterRecomputeChunk = oldHook })

	out, err := RecomputePersona(context.Background(), pid)
	if err != nil || out.Jobs != 5 || out.Scored != 5 || chunks != 3 {
		t.Fatalf("outcome %+v chunks %d err %v", out, chunks, err)
	}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if _, ok := scoreOf(t, id, pid); !ok {
			t.Errorf("job %s has no row", id)
		}
	}
}

// A cancelled run stops between chunks, keeps what it stored, and does NOT store the state, so it is run again later.
func TestACancelledRunStoresNoStateAndIsRunAgain(t *testing.T) {
	setupJobsDB(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		addMatchJob(t, id)
	}
	_, pid := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes"})
	old := recomputeChunk
	recomputeChunk = 2
	ctx, cancel := context.WithCancel(context.Background())
	oldHook := afterRecomputeChunk
	afterRecomputeChunk = func() { cancel() } // after the first chunk
	t.Cleanup(func() { recomputeChunk = old; afterRecomputeChunk = oldHook })

	out, err := RecomputePersona(ctx, pid)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if out.Jobs != 2 {
		t.Errorf("jobs done = %d, want the first chunk only", out.Jobs)
	}
	if _, ok, _ := personamatch.GetState(pid); ok {
		t.Error("a cancelled run must not store the state")
	}
	afterRecomputeChunk = func() {}
	out, err = RecomputePersona(context.Background(), pid)
	if err != nil || out.Skipped || out.Jobs != 4 {
		t.Fatalf("the next run: %+v %v", out, err)
	}
	if _, ok, _ := personamatch.GetState(pid); !ok {
		t.Error("the finished run must store the state")
	}
}

// A job deleted while the run is on is skipped without an error, and the run still finishes.
func TestAJobDeletedDuringTheRunIsSkipped(t *testing.T) {
	setupJobsDB(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		addMatchJob(t, id)
	}
	_, pid := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes"})
	old := recomputeChunk
	recomputeChunk = 2
	oldHook := afterRecomputeChunk
	afterRecomputeChunk = func() { exec(t, `DELETE FROM jobs WHERE id = 'd'`) } // before the second chunk is read
	t.Cleanup(func() { recomputeChunk = old; afterRecomputeChunk = oldHook })

	out, err := RecomputePersona(context.Background(), pid)
	if err != nil {
		t.Fatalf("a deleted job must not fail the run: %v", err)
	}
	if out.Jobs != 3 {
		t.Errorf("jobs = %d, want 3 (d is gone)", out.Jobs)
	}
	if _, ok := scoreOf(t, "d", pid); ok {
		t.Error("a row for a deleted job")
	}
}

func TestRecomputeOfAnUnknownPersonaWritesNothing(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	if _, err := RecomputePersona(context.Background(), "nope"); !errors.Is(err, persona.ErrUnknownPersona) {
		t.Errorf("err = %v", err)
	}
	if countRows(t, "job_persona_matches") != 0 || countRows(t, "persona_score_state") != 0 {
		t.Error("something was written")
	}
}

func TestRecomputeWithNoJobsStoresTheStateAndNoRows(t *testing.T) {
	setupJobsDB(t)
	_, pid := seedProfile(t, "Fit", []string{"Go"})
	out, err := RecomputePersona(context.Background(), pid)
	if err != nil || out.Jobs != 0 || out.Skipped {
		t.Fatalf("%+v %v", out, err)
	}
	if _, ok, _ := personamatch.GetState(pid); !ok || countRows(t, "job_persona_matches") != 0 {
		t.Error("the state must be stored even with no job, and no row written")
	}
}

func TestLoadPersonaForMatchFindsThePersonaWithItsProfile(t *testing.T) {
	setupJobsDB(t)
	profileID, id := seedProfile(t, "Fit", []string{"Go", "Docker"})
	seedPersonaIn(t, profileID, "Second", []string{"Cobol"})
	p, prof, err := LoadPersonaForMatch(id)
	if err != nil || p.ID != id || prof != profileID || !reflect.DeepEqual(p.Skills, []string{"Docker", "Go"}) {
		t.Errorf("%+v %q %v", p, prof, err)
	}
	if _, _, err := LoadPersonaForMatch("nope"); !errors.Is(err, persona.ErrUnknownPersona) {
		t.Errorf("unknown persona: %v", err)
	}
}

// A job deleted after its id was listed but before it is loaded (the small window inside a chunk) is skipped, not an error.
func TestAJobDeletedBetweenListingAndLoadingIsSkipped(t *testing.T) {
	setupJobsDB(t)
	for _, id := range []string{"a", "b", "c"} {
		addMatchJob(t, id)
	}
	_, pid := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes"})
	deleted := false
	old := afterJobIDsListed
	afterJobIDsListed = func() {
		if !deleted {
			deleted = true
			exec(t, `DELETE FROM jobs WHERE id = 'b'`)
		}
	}
	t.Cleanup(func() { afterJobIDsListed = old })

	out, err := RecomputePersona(context.Background(), pid)
	if err != nil {
		t.Fatalf("a job gone before it was loaded must not fail the run: %v", err)
	}
	if out.Jobs != 2 {
		t.Errorf("jobs = %d, want 2 (b is gone)", out.Jobs)
	}
	if _, ok := scoreOf(t, "b", pid); ok {
		t.Error("a row for the deleted job")
	}
	if _, ok := scoreOf(t, "a", pid); !ok {
		t.Error("the other jobs must have their rows")
	}
}

// A cancellation that arrives while the LAST chunk is being stored still counts: no state is stored for that run.
func TestACancellationAfterTheLastChunkStoresNoState(t *testing.T) {
	setupJobsDB(t)
	for _, id := range []string{"a", "b"} {
		addMatchJob(t, id)
	}
	_, pid := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes"})
	ctx, cancel := context.WithCancel(context.Background())
	old := afterRecomputeChunk
	afterRecomputeChunk = func() { cancel() } // the one and only chunk
	t.Cleanup(func() { afterRecomputeChunk = old })

	if _, err := RecomputePersona(ctx, pid); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, ok, _ := personamatch.GetState(pid); ok {
		t.Error("the state must not be stored for a cancelled run")
	}
}

// A store that fails mid-run fails the run, and no state is stored (the sweeper will try again).
func TestAStoreFailureFailsTheRunAndStoresNoState(t *testing.T) {
	setupJobsDB(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		addMatchJob(t, id)
	}
	_, pid := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes"})
	old := recomputeChunk
	recomputeChunk = 2
	oldHook := afterRecomputeChunk
	afterRecomputeChunk = func() { exec(t, `DROP TABLE job_persona_matches`) } // the second chunk cannot be stored
	t.Cleanup(func() { recomputeChunk = old; afterRecomputeChunk = oldHook })

	if _, err := RecomputePersona(context.Background(), pid); err == nil {
		t.Fatal("a failing store must fail the run")
	}
	if _, ok, _ := personamatch.GetState(pid); ok {
		t.Error("a failed run must not store the state")
	}
}

// With several personas in a profile the right one is found, with its own data.
func TestLoadPersonaForMatchPicksTheRequestedPersonaOfSeveral(t *testing.T) {
	setupJobsDB(t)
	profileID, first := seedProfile(t, "Fit", []string{"Go", "Docker"})
	second := seedPersonaIn(t, profileID, "Second", []string{"Cobol", "Fortran"})
	p, prof, err := LoadPersonaForMatch(second)
	if err != nil || p.ID != second || prof != profileID || !reflect.DeepEqual(p.Skills, []string{"Cobol", "Fortran"}) {
		t.Errorf("second persona: %+v %q %v", p, prof, err)
	}
	if q, _, _ := LoadPersonaForMatch(first); q.ID != first || reflect.DeepEqual(q.Skills, p.Skills) {
		t.Errorf("first persona: %+v", q)
	}
}

func TestStalePersonasAreThoseWithoutStateOrWithAnotherFingerprint(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	profileID, a := seedProfile(t, "Ana", []string{"Go", "Docker"})
	b := seedPersonaIn(t, profileID, "Second", []string{"Cobol"})
	_, c := seedProfile(t, "Cy", []string{"Rust"})

	stale, err := StalePersonas()
	if err != nil || len(stale) != 3 {
		t.Fatalf("with no state at all every persona is stale: %v %v", stale, err)
	}
	for _, id := range []string{a, b, c} {
		if _, err := RecomputePersona(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if stale, _ := StalePersonas(); len(stale) != 0 {
		t.Fatalf("after a recompute of each, nothing is stale: %v", stale)
	}

	// a change makes exactly that persona stale
	if err := persona.AddCareerSkill(b, "Fortran"); err != nil {
		t.Fatal(err)
	}
	if stale, _ := StalePersonas(); !reflect.DeepEqual(stale, []string{b}) {
		t.Errorf("after changing b: %v, want only b", stale)
	}
	if _, err := RecomputePersona(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if stale, _ := StalePersonas(); len(stale) != 0 {
		t.Errorf("after the recompute: %v", stale)
	}

	// a stored fingerprint of another engine version (or any other data) is stale
	if err := personamatch.SetState(a, profileID, "made-by-an-older-engine"); err != nil {
		t.Fatal(err)
	}
	if stale, _ := StalePersonas(); !reflect.DeepEqual(stale, []string{a}) {
		t.Errorf("a different stored fingerprint: %v, want only a", stale)
	}

	// a change that cannot move a score does not make a persona stale (the name, a salary)
	if _, err := RecomputePersona(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := persona.UpdatePersona(a, strPtr("Renamed"), nil); err != nil {
		t.Fatal(err)
	}
	if stale, _ := StalePersonas(); len(stale) != 0 {
		t.Errorf("a rename made %v stale", stale)
	}
}

func strPtr(s string) *string { return &s }

// The order is the profiles' and then the personas' creation order, whatever the ids: checked with the older profile
// holding the smaller id and with it holding the larger one, so neither id order can give the right answer by chance.
func TestStalePersonasComeInCreationOrder(t *testing.T) {
	for _, olderHasLargerID := range []bool{true, false} {
		t.Run(fmt.Sprintf("older profile has the larger id=%v", olderHasLargerID), func(t *testing.T) {
			setupJobsDB(t)
			p1, _ := profile.CreateProfile("First", "Profile")
			p2, _ := profile.CreateProfile("Second", "Profile")
			a, _ := persona.CreatePersona(p1, "A", "")
			b, _ := persona.CreatePersona(p1, "B", "")
			x, _ := persona.CreatePersona(p2, "X", "")
			set := func(q string, args ...any) {
				if _, err := db.CareerDB.Exec(q, args...); err != nil {
					t.Fatal(err)
				}
			}
			older, newer := p1, p2
			if (older > newer) != olderHasLargerID {
				older, newer = newer, older
			}
			set(`UPDATE profiles SET created_at = '2026-01-02 10:00:00' WHERE id = ?`, newer)
			set(`UPDATE profiles SET created_at = '2026-01-01 10:00:00' WHERE id = ?`, older)
			// within the first profile, a is created after b, whatever their ids
			small, large := a, b
			if large < small {
				small, large = large, small
			}
			late, early := small, large
			if !olderHasLargerID {
				late, early = large, small
			}
			set(`UPDATE personas SET created_at = '2026-02-02 10:00:00' WHERE id = ?`, late)
			set(`UPDATE personas SET created_at = '2026-02-01 10:00:00' WHERE id = ?`, early)
			var want []string
			if older == p1 {
				want = []string{early, late, x}
			} else {
				want = []string{x, early, late}
			}
			stale, err := StalePersonas()
			if err != nil || !reflect.DeepEqual(stale, want) {
				t.Errorf("order = %v (%v), want %v", stale, err, want)
			}
		})
	}
}

func TestStalePersonasOfAnEmptyDatabaseIsAnEmptyList(t *testing.T) {
	setupJobsDB(t)
	stale, err := StalePersonas()
	if err != nil || stale == nil || len(stale) != 0 {
		t.Errorf("%v %v", stale, err)
	}
}

// A state left behind by a persona that no longer exists is not a stale persona.
func TestAStateOfADeletedPersonaIsIgnored(t *testing.T) {
	setupJobsDB(t)
	if err := personamatch.SetState("ghost", "nobody", "fp"); err != nil {
		t.Fatal(err)
	}
	if stale, _ := StalePersonas(); len(stale) != 0 {
		t.Errorf("stale = %v", stale)
	}
}

// A state table that cannot be read is an error, not "everything is fine".
func TestStalePersonasReportsAStateThatCannotBeRead(t *testing.T) {
	setupJobsDB(t)
	seedProfile(t, "Ana", []string{"Go"})
	exec(t, `DROP TABLE persona_score_state`)
	if stale, err := StalePersonas(); err == nil {
		t.Errorf("stale = %v, want an error", stale)
	}
}
