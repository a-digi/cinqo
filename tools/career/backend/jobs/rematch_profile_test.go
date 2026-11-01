package jobs

import (
	"slices"
	"sync"
	"testing"
	"time"

	"career-tool-backend/jobmatcher"
	"career-tool-backend/profile"
)

var weakSkills = []string{"Go"}

func rematch(t *testing.T, profileID string) RematchReport {
	t.Helper()
	r, err := RematchProfile(profileID)
	if err != nil {
		t.Fatalf("RematchProfile: %v", err)
	}
	return r
}

// weakAndFit sets up a weak and a fit profile where the fit one scores strictly higher on the test job.
func weakAndFit(t *testing.T) (weak, fit string) {
	t.Helper()
	probeJob(t)
	weak, _ = profileAt(t, "Weak", "2020-01-01 00:00:00", weakSkills)
	fit, _ = profileAt(t, "Fit", "2020-01-02 00:00:00", fitSkills)
	a, err := RunManualMatch("j-probe", weak)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RunManualMatch("j-probe", fit)
	if err != nil {
		t.Fatal(err)
	}
	if a.Score >= b.Score {
		t.Fatalf("setup: the weak profile must score lower: %d vs %d", a.Score, b.Score)
	}
	exec(t, `DELETE FROM job_matches`)
	exec(t, `DELETE FROM job_match_skills`)
	exec(t, `DELETE FROM jobs WHERE id = 'j-probe'`)
	return weak, fit
}

func probeJob(t *testing.T) { addMatchJob(t, "j-probe") }

// A job without a match gets the profile's result, a normal manual match with persona, breakdown and skills.
func TestARematchFillsAJobWithoutAMatch(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	fit, fitPersona := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
	want, err := RunManualMatch("j-probe", fit)
	if err != nil {
		t.Fatal(err)
	}
	exec(t, `DELETE FROM job_matches`)
	exec(t, `DELETE FROM job_match_skills`)
	r := rematch(t, fit)
	if r != (RematchReport{Jobs: 1, Stored: 1}) {
		t.Fatalf("report %+v", r)
	}
	m, ok := readStoredMatch(t, "j-probe")
	if !ok || m.profileID != fit || m.personaID != fitPersona || m.kind != "manual" || m.status != "completed" || m.score != want.Score {
		t.Fatalf("stored %+v, want score %d", m, want.Score)
	}
	if m.breakdown.EngineVersion == "" || len(m.skills) == 0 || !slices.Equal(m.skills, slices.Sorted(slices.Values(want.MatchedSkills))) {
		t.Errorf("breakdown/skills %+v", m)
	}
}

// A manual match with a lower score is replaced, skills included; an equal or higher one is left alone; a second run changes nothing.
func TestARematchReplacesOnlyAStrictlyLowerManualMatch(t *testing.T) {
	setupJobsDB(t)
	weak, fit := weakAndFit(t)
	probeJob(t)
	if _, err := RunManualMatch("j-probe", weak); err != nil {
		t.Fatal(err)
	}
	weakRow, _ := readStoredMatch(t, "j-probe")

	if r := rematch(t, fit); r != (RematchReport{Jobs: 1, Improved: 1}) {
		t.Fatalf("report %+v", r)
	}
	fitRow, _ := readStoredMatch(t, "j-probe")
	if fitRow.profileID != fit || fitRow.score <= weakRow.score || fitRow.kind != "manual" || fitRow.status != "completed" {
		t.Fatalf("replaced %+v -> %+v", weakRow, fitRow)
	}
	if !slices.Equal(fitRow.skills, slices.Sorted(slices.Values(fitRow.breakdown.MatchedSkills))) || len(fitRow.skills) <= len(weakRow.skills) {
		t.Errorf("skills %v -> %v", weakRow.skills, fitRow.skills)
	}

	// idempotent, and a lower profile does not take it back
	if r := rematch(t, fit); r != (RematchReport{Jobs: 1, Unchanged: 1}) {
		t.Errorf("second run %+v", r)
	}
	if r := rematch(t, weak); r != (RematchReport{Jobs: 1, Unchanged: 1}) {
		t.Errorf("lower profile %+v", r)
	}
	if again, _ := readStoredMatch(t, "j-probe"); again.profileID != fit || again.score != fitRow.score || !slices.Equal(again.skills, fitRow.skills) {
		t.Errorf("changed: %+v -> %+v", fitRow, again)
	}
}

// An equal score is not "strictly higher": a manual match of another profile with the same number stays.
func TestAnEqualScoreDoesNotReplace(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	a, _ := profileAt(t, "A", "2020-01-01 00:00:00", fitSkills)
	b, _ := profileAt(t, "B", "2020-01-02 00:00:00", fitSkills)
	if o := autoOne(t, "j-probe"); o.ProfileID != a {
		t.Fatalf("%+v", o)
	}
	if r := rematch(t, b); r != (RematchReport{Jobs: 1, Unchanged: 1}) {
		t.Fatalf("%+v", r)
	}
	if m, _ := readStoredMatch(t, "j-probe"); m.profileID != a {
		t.Errorf("replaced by %s", m.profileID)
	}
}

// AI, running and failed matches are never touched, and a manual match that is not completed is not a manual result either.
func TestARematchNeverTouchesAiRunningOrFailedMatches(t *testing.T) {
	setupJobsDB(t)
	_, fit := weakAndFit(t)
	rows := map[string]string{
		"ai":            `INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('ai', 'p', 1, 'completed', 'ai')`,
		"matching":      `INSERT INTO job_matches (job_id, profile_id, status, kind) VALUES ('matching', 'p', 'matching', 'ai')`,
		"failed":        `INSERT INTO job_matches (job_id, profile_id, status, kind, error) VALUES ('failed', 'p', 'failed', 'ai', 'boom')`,
		"manualfailed":  `INSERT INTO job_matches (job_id, profile_id, status, kind, error) VALUES ('manualfailed', 'p', 'failed', 'manual', 'boom')`,
		"manualrunning": `INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('manualrunning', 'p', 1, 'matching', 'manual')`,
	}
	for id, insert := range rows {
		addMatchJob(t, id)
		exec(t, insert)
		exec(t, `INSERT INTO job_match_skills (job_id, skill) VALUES (?, 'Keep')`, id)
	}
	// None of them is scored: a table the scoring needs is gone, so scoring one would be a failure, not a skip.
	exec(t, `DROP TABLE job_requirements`)
	r := rematch(t, fit)
	if r != (RematchReport{Jobs: 5, SkippedProtected: 5}) {
		t.Fatalf("report %+v", r)
	}
	for id := range rows {
		m, _ := readStoredMatch(t, id)
		if m.profileID != "p" || !slices.Equal(m.skills, []string{"Keep"}) || (id == "ai" && (m.score != 1 || m.kind != "ai")) {
			t.Errorf("%s changed: %+v", id, m)
		}
	}
}

// The decision is made on the row inside the write's own transaction: a protected row is refused there too.
func TestTheStoreRefusesAProtectedRowItself(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	exec(t, `INSERT INTO job_matches (job_id, profile_id, status, kind) VALUES ('j1', 'p', 'matching', 'ai')`)
	got, err := storeIfBetter("j1", "q", jobmatcher.Result{Score: 99, MatchedSkills: []string{"X"}})
	if err != nil || got != rematchProtected {
		t.Fatalf("%v, %v", got, err)
	}
	m, _ := readStoredMatch(t, "j1")
	if m.profileID != "p" || m.status != "matching" || len(m.skills) != 0 {
		t.Errorf("%+v", m)
	}
	// a manual completed row without a score is replaced
	exec(t, `UPDATE job_matches SET kind='manual', status='completed', score=NULL`)
	if got, err := storeIfBetter("j1", "q", jobmatcher.Result{Score: 3}); err != nil || got != rematchImproved {
		t.Errorf("null score: %v, %v", got, err)
	}
}

// A profile that cannot judge, or does not exist, changes nothing.
func TestAProfileThatCannotJudgeChangesNothing(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('j-probe', 'p', 5, 'completed', 'manual')`)
	empty, _ := profile.CreateProfile("Empty", "")
	for name, id := range map[string]string{"no persona": empty, "unknown": "nope", "blank": ""} {
		r := rematch(t, id)
		if r.Stored+r.Improved+r.Unchanged+r.SkippedProtected+r.Failed != 0 {
			t.Errorf("%s: %+v", name, r)
		}
		if m, _ := readStoredMatch(t, "j-probe"); m.profileID != "p" || m.score != 5 {
			t.Errorf("%s changed the match: %+v", name, m)
		}
	}
	if r := rematch(t, empty); r.Jobs != 1 || r.Unjudged != 1 {
		t.Errorf("no persona: %+v", r)
	}
	if r := rematch(t, ""); r != (RematchReport{}) {
		t.Errorf("blank: %+v", r)
	}
}

// Jobs the profile cannot judge (too little text) are counted, and the others still go on.
func TestAJobTheProfileCannotJudgeIsCountedAndSkipped(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	fit, _ := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('bad', 'https://x.example/bad', '', '', '', '')`)
	r := rematch(t, fit)
	if r.Jobs != 2 || r.Stored+r.Unjudged+r.Failed != 2 || r.Stored < 1 {
		t.Fatalf("%+v", r)
	}
	if _, ok := readStoredMatch(t, "j-probe"); !ok {
		t.Error("the readable job was not matched")
	}
}

// Every job is seen exactly once whatever the chunk size, and the counts add up.
func TestARematchWalksEveryChunk(t *testing.T) {
	for _, chunk := range []int{1, 2, 3, 5, 6, 200} {
		old := rematchChunk
		rematchChunk = chunk
		t.Run("chunk", func(t *testing.T) {
			defer func() { rematchChunk = old }()
			setupJobsDB(t)
			fit, _ := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
			for _, id := range []string{"a", "b", "c", "d", "e"} {
				addMatchJob(t, id)
			}
			r := rematch(t, fit)
			if r != (RematchReport{Jobs: 5, Stored: 5}) {
				t.Fatalf("chunk %d: %+v", chunk, r)
			}
			for _, id := range []string{"a", "b", "c", "d", "e"} {
				if _, ok := readStoredMatch(t, id); !ok {
					t.Errorf("chunk %d: %s not matched", chunk, id)
				}
			}
		})
	}
}

// Two profiles one after the other leave the best result, whatever the order.
func TestTwoProfilesLeaveTheBestResult(t *testing.T) {
	for name, order := range map[string]bool{"weak first": true, "fit first": false} {
		t.Run(name, func(t *testing.T) {
			setupJobsDB(t)
			weak, fit := weakAndFit(t)
			probeJob(t)
			if order {
				rematch(t, weak)
				rematch(t, fit)
			} else {
				rematch(t, fit)
				rematch(t, weak)
			}
			if m, _ := readStoredMatch(t, "j-probe"); m.profileID != fit {
				t.Errorf("stored profile %s, want %s", m.profileID, fit)
			}
		})
	}
}

// Two runs for the same profile do not overlap: the second waits for the first.
func TestARunWaitsForAnotherRunOfTheSameProfile(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	fit, _ := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
	lock, _ := rematchLocks.LoadOrStore(fit, newMutex())
	mu := lock.(interface {
		Lock()
		Unlock()
	})
	mu.Lock()
	done := make(chan RematchReport, 1)
	go func() { r, _ := RematchProfile(fit); done <- r }()
	select {
	case <-done:
		t.Fatal("ran while the profile was locked")
	case <-time.After(200 * time.Millisecond):
	}
	mu.Unlock()
	select {
	case r := <-done:
		if r.Stored != 1 {
			t.Errorf("%+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not run after the unlock")
	}
	// another profile is not held up by it
	other, _ := profileAt(t, "Other", "2020-01-02 00:00:00", fitSkills)
	mu.Lock()
	defer mu.Unlock()
	if r := rematch(t, other); r.Jobs != 1 {
		t.Errorf("%+v", r)
	}
}

func newMutex() *sync.Mutex { return &sync.Mutex{} }

// The write's own check refuses each kind of row that is not a completed manual result, not only the one shape tried first.
func TestTheStoreRefusesEveryNonManualCompletedRow(t *testing.T) {
	setupJobsDB(t)
	for name, row := range map[string]string{
		"ai completed":    `kind='ai', status='completed', score=1`,
		"manual failed":   `kind='manual', status='failed', score=1`,
		"manual matching": `kind='manual', status='matching', score=1`,
	} {
		exec(t, `DELETE FROM jobs`)
		addMatchJob(t, "j1")
		exec(t, `INSERT INTO job_matches (job_id, profile_id, status, kind) VALUES ('j1', 'p', 'matching', 'ai')`)
		exec(t, `UPDATE job_matches SET `+row)
		if got, err := storeIfBetter("j1", "q", jobmatcher.Result{Score: 99}); err != nil || got != rematchProtected {
			t.Errorf("%s: %v, %v", name, got, err)
		}
		if m, _ := readStoredMatch(t, "j1"); m.profileID != "p" || m.score != 1 {
			t.Errorf("%s changed: %+v", name, m)
		}
	}
}

// A profile that cannot judge does not read the jobs at all (a table scoring needs is gone, and it is still just "unjudged").
func TestAProfileThatCannotJudgeReadsNoJob(t *testing.T) {
	setupJobsDB(t)
	probeJob(t)
	empty, _ := profile.CreateProfile("Empty", "")
	exec(t, `DROP TABLE job_requirements`)
	if r := rematch(t, empty); r != (RematchReport{Jobs: 1, Unjudged: 1}) {
		t.Errorf("%+v", r)
	}
}

// A job deleted between listing and scoring is "unjudged", not a failure.
func TestAJobDeletedMeanwhileIsNotAFailure(t *testing.T) {
	setupJobsDB(t)
	fit, _ := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
	scorer, err := NewProfileScorerFor(map[string]bool{fit: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := rematchOne(scorer, fit, "gone"); got != rematchUnjudged {
		t.Errorf("got %v", got)
	}
}

// Only the asked profile is used: a rematch for the weaker profile stores the weaker profile's result, not the best one's.
func TestARematchUsesOnlyTheAskedProfile(t *testing.T) {
	setupJobsDB(t)
	weak, _ := weakAndFit(t)
	probeJob(t)
	if r := rematch(t, weak); r.Stored != 1 {
		t.Fatalf("%+v", r)
	}
	if m, _ := readStoredMatch(t, "j-probe"); m.profileID != weak {
		t.Errorf("stored profile %s, want the asked %s", m.profileID, weak)
	}
}
