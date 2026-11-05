package match

import (
	"slices"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

// Step 164: a job whose text changed gets its match made again, for free; an AI result and a run in progress are never touched.

const oldText = "Backend Engineer in Hamburg. Wir arbeiten mit Cobol und Fortran auf dem Mainframe. Mindestens 3 Jahre Berufserfahrung."

func addJobWithText(t *testing.T, id, text string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`,
		id, "https://x.example/"+id, text)
}

func rematchOneJob(t *testing.T, id string) ChangedJobOutcome {
	t.Helper()
	out, err := RematchChangedJobs([]string{id})
	if err != nil || len(out) != 1 {
		t.Fatalf("RematchChangedJobs: %v %v", out, err)
	}
	return out[0]
}

func setUpdatedAt(t *testing.T, jobID, stamp string) {
	t.Helper()
	exec(t, `UPDATE job_matches SET updated_at = ? WHERE job_id = ?`, stamp, jobID)
}

func updatedAt(t *testing.T, jobID string) string {
	t.Helper()
	var s string
	if err := db.JobsDB.QueryRow(`SELECT updated_at FROM job_matches WHERE job_id = ?`, jobID).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAManualMatchIsMadeAgainFromTheNewText(t *testing.T) {
	setupJobsDB(t)
	profile, persona := seedProfile(t, "Fit", fitSkills)
	addJobWithText(t, "j1", oldText)
	if _, err := RunManualMatch("j1", profile); err != nil {
		t.Fatal(err)
	}
	before, _ := readStoredMatch(t, "j1")
	setUpdatedAt(t, "j1", "2000-01-01 00:00:00")

	exec(t, `UPDATE jobs SET description = ? WHERE id = 'j1'`, matchJobDesc) // the detail crawl brought the full text
	o := rematchOneJob(t, "j1")
	after, ok := readStoredMatch(t, "j1")
	if !ok || o.Status != ChangedJobRescored || o.ProfileID != profile || o.Score != after.score {
		t.Fatalf("outcome %+v, stored %+v", o, after)
	}
	if after.score <= before.score {
		t.Errorf("the full text fits better: %d -> %d", before.score, after.score)
	}
	if after.kind != "manual" || after.status != "completed" || after.profileID != profile || after.personaID != persona {
		t.Errorf("the row stays a finished manual result of the same profile and persona: %+v", after)
	}
	if !slices.Equal(after.skills, after.breakdown.MatchedSkills) || len(after.skills) <= len(before.skills) {
		t.Errorf("the skills follow the new result: before %v, after %v", before.skills, after.skills)
	}
	if updatedAt(t, "j1") == "2000-01-01 00:00:00" {
		t.Error("updated_at must move")
	}
	// the same text again gives the same row, and nothing is added
	again := rematchOneJob(t, "j1")
	second, _ := readStoredMatch(t, "j1")
	if again.Status != ChangedJobRescored || second.score != after.score || !slices.Equal(second.skills, after.skills) {
		t.Errorf("idempotent: %+v %+v", again, second)
	}
}

func TestAnAIResultIsNeverReplaced(t *testing.T) {
	setupJobsDB(t)
	profile, _ := seedProfile(t, "Fit", fitSkills)
	addMatchJob(t, "j1")
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status, kind, conversation_id, updated_at) VALUES ('j1', ?, 7, 'completed', 'ai', 'conv-1', '2000-01-01 00:00:00')`, profile)
	exec(t, `INSERT INTO job_match_skills (job_id, skill) VALUES ('j1', 'by the AI')`)
	if o := rematchOneJob(t, "j1"); o.Status != ChangedJobSkippedAI {
		t.Fatalf("%+v", o)
	}
	m, _ := readStoredMatch(t, "j1")
	if m.kind != "ai" || m.score != 7 || !slices.Equal(m.skills, []string{"by the AI"}) || updatedAt(t, "j1") != "2000-01-01 00:00:00" {
		t.Errorf("the AI result must be untouched: %+v", m)
	}
}

func TestARunInProgressOrAnUnfinishedRowIsLeftAlone(t *testing.T) {
	for name, c := range map[string]struct{ kind, status, want string }{
		"an AI run in progress":    {"ai", "matching", ChangedJobSkippedAI},
		"a manual run in progress": {"manual", "matching", ChangedJobSkippedRunning},
		"a failed manual row":      {"manual", "failed", ChangedJobSkippedExisting},
		"a failed AI row":          {"ai", "failed", ChangedJobSkippedAI},
	} {
		t.Run(name, func(t *testing.T) {
			setupJobsDB(t)
			profile, _ := seedProfile(t, "Fit", fitSkills)
			addMatchJob(t, "j1")
			exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status, kind, updated_at) VALUES ('j1', ?, 33, ?, ?, '2000-01-01 00:00:00')`, profile, c.status, c.kind)
			if o := rematchOneJob(t, "j1"); o.Status != c.want {
				t.Fatalf("%+v, want %s", o, c.want)
			}
			m, _ := readStoredMatch(t, "j1")
			if m.score != 33 || m.status != c.status || m.kind != c.kind || updatedAt(t, "j1") != "2000-01-01 00:00:00" {
				t.Errorf("the row must be untouched: %+v", m)
			}
		})
	}
}

func TestAJobWithoutAMatchGetsTheBestOne(t *testing.T) {
	setupJobsDB(t)
	fit, _ := profileAt(t, "Fit", "2024-01-01 00:00:00", fitSkills)
	profileAt(t, "Other", "2024-01-02 00:00:00", otherSkills)
	addMatchJob(t, "j1")
	o := rematchOneJob(t, "j1")
	m, ok := readStoredMatch(t, "j1")
	if !ok || o.Status != ChangedJobCreated || o.ProfileID != fit || m.profileID != fit || m.kind != "manual" || o.Score != m.score {
		t.Fatalf("%+v %+v", o, m)
	}
}

func TestWithNoProfileNothingIsCreated(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	if o := rematchOneJob(t, "j1"); o.Status != ChangedJobUnjudged {
		t.Fatalf("%+v", o)
	}
	if _, ok := readStoredMatch(t, "j1"); ok {
		t.Error("nothing may be stored")
	}
}

// A person who chose a profile (or the best one at the time) keeps that profile: the re-match judges the same profile again and
// never switches the row to another profile.
func TestTheRowKeepsItsProfile(t *testing.T) {
	setupJobsDB(t)
	profileAt(t, "Fit", "2024-01-01 00:00:00", fitSkills)
	chosen, _ := profileAt(t, "Chosen", "2024-01-02 00:00:00", otherSkills) // scores lower, but it is the one the row was made for
	addJobWithText(t, "j1", oldText)
	if _, err := RunManualMatch("j1", chosen); err != nil {
		t.Fatal(err)
	}
	exec(t, `UPDATE jobs SET description = ? WHERE id = 'j1'`, matchJobDesc)
	o := rematchOneJob(t, "j1")
	m, _ := readStoredMatch(t, "j1")
	if o.Status != ChangedJobRescored || m.profileID != chosen {
		t.Errorf("the row must stay with the chosen profile: %+v %+v", o, m)
	}
}

func TestAProfileThatIsGoneLeavesTheRowAsItIs(t *testing.T) {
	setupJobsDB(t)
	profile, _ := seedProfile(t, "Fit", fitSkills)
	addMatchJob(t, "j1")
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status, kind, updated_at) VALUES ('j1', 'a-deleted-profile', 40, 'completed', 'manual', '2000-01-01 00:00:00')`)
	_ = profile
	if o := rematchOneJob(t, "j1"); o.Status != ChangedJobUnjudged {
		t.Fatalf("%+v", o)
	}
	if m, _ := readStoredMatch(t, "j1"); m.score != 40 || updatedAt(t, "j1") != "2000-01-01 00:00:00" {
		t.Errorf("%+v", m)
	}
}

func TestAnUnknownJobIsReportedAndStoresNothing(t *testing.T) {
	setupJobsDB(t)
	seedProfile(t, "Fit", fitSkills)
	if o := rematchOneJob(t, "nope"); o.Status != ChangedJobUnknownJob {
		t.Fatalf("%+v", o)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_matches`).Scan(&n)
	if n != 0 {
		t.Errorf("%d rows", n)
	}
}

// The write itself refuses to overwrite what a person's run took over between the check and the write, and one for a profile
// that no longer exists writes nothing.
func TestTheWritesRefuseWhatChangedMeanwhile(t *testing.T) {
	setupJobsDB(t)
	profile, _ := seedProfile(t, "Fit", fitSkills)
	addMatchJob(t, "j1")
	sc, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	result, ok, err := sc.ScoreProfile("j1", profile)
	if err != nil || !ok {
		t.Fatalf("setup: %v %v", ok, err)
	}

	for name, row := range map[string]string{
		"an AI run took the row":          `INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('j1', '` + profile + `', 5, 'matching', 'ai')`,
		"a manual run of another profile": `INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('j1', 'other', 5, 'completed', 'manual')`,
	} {
		exec(t, `DELETE FROM job_matches`)
		exec(t, row)
		wrote, err := replaceManualMatchIfStillManual("j1", profile, result)
		if err != nil || wrote {
			t.Errorf("%s: wrote=%v err=%v", name, wrote, err)
		}
		if m, _ := readStoredMatch(t, "j1"); m.score != 5 {
			t.Errorf("%s: the row changed: %+v", name, m)
		}
	}
	exec(t, `DELETE FROM job_matches`)
	if wrote, err := replaceManualMatchIfStillManual("j1", profile, result); err != nil || wrote {
		t.Errorf("no row: wrote=%v err=%v", wrote, err)
	}
	if wrote, err := writeUnderProfileLock("a-deleted-profile", func() (bool, error) { t.Error("the write must not run"); return true, nil }); err != nil || wrote {
		t.Errorf("a profile that is gone: wrote=%v err=%v", wrote, err)
	}
}

func TestABatchHandlesEachJobOnceAndInOrder(t *testing.T) {
	setupJobsDB(t)
	profile, _ := seedProfile(t, "Fit", fitSkills)
	addJobWithText(t, "manual", oldText)
	if _, err := RunManualMatch("manual", profile); err != nil {
		t.Fatal(err)
	}
	addMatchJob(t, "fresh")
	addMatchJob(t, "ai")
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('ai', ?, 9, 'completed', 'ai')`, profile)
	out, err := RematchChangedJobs([]string{"ai", "manual", "", "fresh", "manual", "gone"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, o := range out {
		got = append(got, o.JobID+":"+o.Status)
	}
	want := []string{"ai:" + ChangedJobSkippedAI, "manual:" + ChangedJobRescored, "fresh:" + ChangedJobCreated, "gone:" + ChangedJobUnknownJob}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if out, err := RematchChangedJobs(nil); err != nil || out != nil {
		t.Errorf("an empty batch: %v %v", out, err)
	}
}

// The write is guarded in the SQL too: any row that is not a finished manual result of the same profile is refused, whatever the
// caller checked before.
func TestTheWriteRefusesEveryRowThatIsNotAFinishedManualResultOfTheProfile(t *testing.T) {
	setupJobsDB(t)
	profile, _ := seedProfile(t, "Fit", fitSkills)
	addMatchJob(t, "j1")
	sc, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	result, ok, err := sc.ScoreProfile("j1", profile)
	if err != nil || !ok {
		t.Fatalf("setup: %v %v", ok, err)
	}
	for name, kindStatus := range map[string][2]string{
		"a finished AI result": {"ai", "completed"},
		"a failed manual row":  {"manual", "failed"},
		"a manual run":         {"manual", "matching"},
	} {
		exec(t, `DELETE FROM job_matches`)
		exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('j1', ?, 5, ?, ?)`, profile, kindStatus[1], kindStatus[0])
		if wrote, err := replaceManualMatchIfStillManual("j1", profile, result); err != nil || wrote {
			t.Errorf("%s: wrote=%v err=%v", name, wrote, err)
		}
		if m, _ := readStoredMatch(t, "j1"); m.score != 5 {
			t.Errorf("%s: the row changed: %+v", name, m)
		}
	}
}

// With no profile that has a persona, the job is not even read: a catch-up that cannot match anything costs almost nothing.
func TestWithNoProfileTheJobIsNotEvenRead(t *testing.T) {
	setupJobsDB(t)
	if o := rematchOneJob(t, "does-not-exist"); o.Status != ChangedJobUnjudged {
		t.Errorf("%+v", o)
	}
}

// A write for a profile waits while another holder has the profile's lock (a re-match of the profile, or the purge of a deleted
// profile's matches), and only then goes on.
func TestAWriteWaitsForTheProfilesLock(t *testing.T) {
	setupJobsDB(t)
	profile, _ := seedProfile(t, "Fit", fitSkills)
	unlock := jobs.LockProfile(profile)
	finished := make(chan bool, 1)
	go func() {
		wrote, _ := writeUnderProfileLock(profile, func() (bool, error) { return true, nil })
		finished <- wrote
	}()
	select {
	case <-finished:
		t.Fatal("the write must wait while the profile's lock is held")
	case <-time.After(150 * time.Millisecond):
	}
	unlock()
	select {
	case wrote := <-finished:
		if !wrote {
			t.Error("the write must go through once the lock is free")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the write never went on")
	}
}
