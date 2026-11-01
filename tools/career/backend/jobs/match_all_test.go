package jobs

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobmatcher"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

// profileAt creates a profile (via seedProfile) and pins its created_at, so the profile order is known.
func profileAt(t *testing.T, name, createdAt string, skills []string) (profileID, personaID string) {
	t.Helper()
	profileID, personaID = seedProfile(t, name, skills)
	if _, err := db.CareerDB.Exec(`UPDATE profiles SET created_at = ? WHERE id = ?`, createdAt, profileID); err != nil {
		t.Fatal(err)
	}
	return profileID, personaID
}

type storedMatch struct {
	profileID, personaID, status, kind string
	score                              int
	breakdown                          jobmatcher.Result
	skills                             []string
}

func readStoredMatch(t *testing.T, jobID string) (storedMatch, bool) {
	t.Helper()
	var m storedMatch
	var raw, personaID *string
	var score *int
	err := db.JobsDB.QueryRow(`SELECT profile_id, persona_id, status, kind, score, breakdown_json FROM job_matches WHERE job_id = ?`, jobID).
		Scan(&m.profileID, &personaID, &m.status, &m.kind, &score, &raw)
	if err != nil {
		return storedMatch{}, false
	}
	if personaID != nil {
		m.personaID = *personaID
	}
	if score != nil {
		m.score = *score
	}
	if raw != nil {
		_ = json.Unmarshal([]byte(*raw), &m.breakdown)
	}
	rows, _ := db.JobsDB.Query(`SELECT skill FROM job_match_skills WHERE job_id = ? ORDER BY skill`, jobID)
	defer rows.Close()
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		m.skills = append(m.skills, s)
	}
	return m, true
}

func autoOne(t *testing.T, id string) AutoMatchOutcome {
	t.Helper()
	out, err := AutoMatchJobs([]string{id})
	if err != nil || len(out) != 1 {
		t.Fatalf("AutoMatchJobs: %v, %v", out, err)
	}
	return out[0]
}

var fitSkills = []string{"Go", "Docker", "Kubernetes", "PostgreSQL"}
var otherSkills = []string{"Cobol", "Fortran"}

// The best profile is stored, whatever order the profiles were created in; the row is a normal manual match with its
// persona, breakdown and skills.
func TestTheBestProfileIsStoredWhateverTheOrder(t *testing.T) {
	for name, fitFirst := range map[string]bool{"fit first": true, "fit last": false} {
		t.Run(name, func(t *testing.T) {
			setupJobsDB(t)
			addMatchJob(t, "j1")
			var fit, fitPersona, other string
			if fitFirst {
				fit, fitPersona = profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
				other, _ = profileAt(t, "Other", "2020-01-02 00:00:00", otherSkills)
			} else {
				other, _ = profileAt(t, "Other", "2020-01-01 00:00:00", otherSkills)
				fit, fitPersona = profileAt(t, "Fit", "2020-01-02 00:00:00", fitSkills)
			}
			o := autoOne(t, "j1")
			if o.Status != AutoMatched || o.ProfileID != fit {
				t.Fatalf("outcome %+v, want matched by %s", o, fit)
			}
			m, ok := readStoredMatch(t, "j1")
			if !ok || m.profileID != fit || m.personaID != fitPersona || m.kind != "manual" || m.status != "completed" {
				t.Fatalf("stored %+v", m)
			}
			// the stored number is what the manual run gives that profile; the other profile scores lower
			want, err := RunManualMatch("j1", fit)
			if err != nil {
				t.Fatal(err)
			}
			otherRes, err := RunManualMatch("j1", other)
			if err != nil {
				t.Fatal(err)
			}
			if m.score != want.Score || o.Score != want.Score || otherRes.Score >= want.Score {
				t.Errorf("stored %d, outcome %d, fit run %d, other run %d", m.score, o.Score, want.Score, otherRes.Score)
			}
			if m.breakdown.PersonaID != fitPersona || m.breakdown.EngineVersion == "" || len(m.breakdown.Components) == 0 {
				t.Errorf("breakdown %+v", m.breakdown)
			}
			if !slices.Equal(m.skills, slices.Sorted(slices.Values(want.MatchedSkills))) || len(m.skills) == 0 {
				t.Errorf("skills %v, want %v", m.skills, want.MatchedSkills)
			}
		})
	}
}

// Equal scores go to the profile listed first (the oldest), not to chance.
func TestATieGoesToTheOlderProfile(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	younger, _ := profileAt(t, "Younger", "2021-01-01 00:00:00", fitSkills)
	older, _ := profileAt(t, "Older", "2020-01-01 00:00:00", fitSkills)
	a, _ := RunManualMatch("j1", younger)
	b, _ := RunManualMatch("j1", older)
	if a.Score != b.Score {
		t.Fatalf("the two profiles should tie: %d vs %d", a.Score, b.Score)
	}
	exec(t, `DELETE FROM job_matches`)
	exec(t, `DELETE FROM job_match_skills`)
	if o := autoOne(t, "j1"); o.Status != AutoMatched || o.ProfileID != older {
		t.Errorf("%+v, want the older profile %s", o, older)
	}
}

func TestNoProfilesMeansNothingIsStored(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	if o := autoOne(t, "j1"); o.Status != AutoUnjudged {
		t.Errorf("%+v", o)
	}
	if _, ok := readStoredMatch(t, "j1"); ok {
		t.Error("a match was stored without profiles")
	}
}

// A profile without personas and a persona with nothing to judge cannot match: nothing stored. A profile that can
// judge still wins when the others cannot.
func TestProfilesThatCannotJudgeAreSkipped(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	if _, err := profile.CreateProfile("No", "Personas"); err != nil {
		t.Fatal(err)
	}
	bare, _ := profile.CreateProfile("Bare", "Persona")
	if _, err := persona.CreatePersona(bare, "bare", ""); err != nil {
		t.Fatal(err)
	}
	if o := autoOne(t, "j1"); o.Status != AutoUnjudged {
		t.Fatalf("nobody can judge: %+v", o)
	}
	if _, ok := readStoredMatch(t, "j1"); ok {
		t.Fatal("stored")
	}
	fit, _ := seedProfile(t, "Fit", fitSkills)
	if o := autoOne(t, "j1"); o.Status != AutoMatched || o.ProfileID != fit {
		t.Errorf("one profile can judge: %+v", o)
	}
}

// An existing match is never replaced, whatever it is: an AI result, a manual one, a failed one, a running one.
func TestAnExistingMatchIsNeverReplaced(t *testing.T) {
	setupJobsDB(t)
	seedProfile(t, "Fit", fitSkills)
	rows := map[string]string{
		"ai":       `INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('ai', 'p', 5, 'completed', 'ai')`,
		"manual":   `INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('manual', 'p', 6, 'completed', 'manual')`,
		"failed":   `INSERT INTO job_matches (job_id, profile_id, status, kind, error) VALUES ('failed', 'p', 'failed', 'ai', 'boom')`,
		"matching": `INSERT INTO job_matches (job_id, profile_id, status, kind) VALUES ('matching', 'p', 'matching', 'ai')`,
	}
	for id, insert := range rows {
		addMatchJob(t, id)
		exec(t, insert)
		exec(t, `INSERT INTO job_match_skills (job_id, skill) VALUES (?, 'Keep')`, id)
		before, _ := readStoredMatch(t, id)
		if o := autoOne(t, id); o.Status != AutoSkippedExisting {
			t.Errorf("%s: %+v", id, o)
		}
		after, _ := readStoredMatch(t, id)
		if after.profileID != before.profileID || after.score != before.score || after.status != before.status || after.kind != before.kind ||
			!slices.Equal(after.skills, []string{"Keep"}) {
			t.Errorf("%s changed: %+v -> %+v", id, before, after)
		}
	}
}

// The insert-only store itself: it writes once, then reports false and leaves the first row and its skills alone.
func TestTheInsertOnlyStore(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	first := jobmatcher.Result{Score: 70, PersonaID: "p1", MatchedSkills: []string{"Go", "Docker"}}
	second := jobmatcher.Result{Score: 20, PersonaID: "p2", MatchedSkills: []string{"Cobol"}}
	if ok, err := storeManualMatchIfAbsent("j1", "prof1", first); err != nil || !ok {
		t.Fatalf("first: %v, %v", ok, err)
	}
	if ok, err := storeManualMatchIfAbsent("j1", "prof2", second); err != nil || ok {
		t.Fatalf("second wrote: %v, %v", ok, err)
	}
	m, _ := readStoredMatch(t, "j1")
	if m.profileID != "prof1" || m.score != 70 || !slices.Equal(m.skills, []string{"Docker", "Go"}) {
		t.Errorf("%+v", m)
	}
}

func TestAnUnknownJobStoresNothing(t *testing.T) {
	setupJobsDB(t)
	seedProfile(t, "Fit", fitSkills)
	if o := autoOne(t, "nope"); o.Status != AutoUnknownJob {
		t.Errorf("%+v", o)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_matches`).Scan(&n)
	if n != 0 {
		t.Errorf("%d rows", n)
	}
}

// A batch: each job gets its own best; a repeated id counts once; a second run changes nothing; empty ids are ignored;
// an empty batch is no work.
func TestABatch(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "a")
	addMatchJob(t, "b")
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('legacy', 'https://x.example/l', 'COBOL Entwickler (m/w/d)', 'Bank', 'Hamburg',
		'Wir pflegen unsere Cobol und Fortran Systeme im Rechenzentrum. Mehrjährige Erfahrung mit Mainframes erforderlich.')`)
	fit, _ := profileAt(t, "Fit", "2020-01-01 00:00:00", fitSkills)
	other, _ := profileAt(t, "Other", "2020-01-02 00:00:00", otherSkills)

	out, err := AutoMatchJobs([]string{"a", "", "legacy", "a", "b"})
	if err != nil || len(out) != 3 {
		t.Fatalf("%v, %v", out, err)
	}
	want := map[string]string{"a": fit, "legacy": other, "b": fit}
	for i, id := range []string{"a", "legacy", "b"} {
		if out[i].JobID != id || out[i].Status != AutoMatched || out[i].ProfileID != want[id] {
			t.Errorf("outcome %d: %+v, want %s by %s", i, out[i], id, want[id])
		}
		if m, _ := readStoredMatch(t, id); m.profileID != want[id] {
			t.Errorf("%s stored for %s", id, m.profileID)
		}
	}
	again, _ := AutoMatchJobs([]string{"a", "legacy", "b"})
	for _, o := range again {
		if o.Status != AutoSkippedExisting {
			t.Errorf("second run: %+v", o)
		}
	}
	if none, err := AutoMatchJobs(nil); err != nil || none != nil {
		t.Errorf("empty batch: %v, %v", none, err)
	}
	if none, _ := AutoMatchJobs([]string{"", ""}); none != nil {
		t.Errorf("blank ids: %v", none)
	}
}

// One job's failure does not stop the others: the match of a job whose profile row is unreadable is its own
// AutoFailed, the batch goes on. (Provoked by a job whose requirements table read fails is not possible here, so the
// storing step is made to fail with a foreign-key violation: a profile-less match cannot, but a job deleted between
// scoring and storing can.)
func TestAJobDeletedBetweenScoringAndStoringIsAFailureNotACrash(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	seedProfile(t, "Fit", fitSkills)
	scorer, err := NewProfileScorer()
	if err != nil {
		t.Fatal(err)
	}
	profileID, result, ok, err := scorer.Best("j1")
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	exec(t, `DELETE FROM jobs WHERE id = 'j1'`)
	if _, err := storeManualMatchIfAbsent("j1", profileID, result); err == nil {
		t.Error("storing for a deleted job should fail (foreign key)")
	}
	if _, ok := readStoredMatch(t, "j1"); ok {
		t.Error("a row was left behind")
	}
}

func TestBestReportsNoJudgeAndUnknownJob(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	s, _ := NewProfileScorer()
	if _, _, ok, err := s.Best("j1"); ok || err != nil {
		t.Errorf("no profiles: %v, %v", ok, err)
	}
	if _, _, _, err := s.Best("nope"); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("err = %v", err)
	}
}

// A job that already has a match is reported as such even when no profile could judge it, because the check comes
// before any scoring (so an existing match costs no scoring at all).
func TestAnExistingMatchIsFoundBeforeAnyScoring(t *testing.T) {
	setupJobsDB(t) // no profiles at all
	addMatchJob(t, "j1")
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status, kind) VALUES ('j1', 'p', 5, 'completed', 'ai')`)
	if o := autoOne(t, "j1"); o.Status != AutoSkippedExisting {
		t.Errorf("%+v", o)
	}
}
