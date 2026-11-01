package personamatch

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"career-tool-backend/db"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

func addJob(t *testing.T, id, crawledAt string) {
	t.Helper()
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, crawled_at) VALUES (?, ?, ?, ?)`, id, "https://x.example/"+id, "Job "+id, crawledAt); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func scores(rows []Row) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s:%d", r.PersonaID, r.Score))
	}
	return out
}

func TestReplaceWritesTheRowsAndForJobReadsThemHighestFirst(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	rows := []Row{{"pa", "A", 40, "v1"}, {"pb", "A", 90, "v1"}, {"pc", "B", 65, "v1"}}
	if err := Replace("j1", rows); err != nil {
		t.Fatal(err)
	}
	got, err := ForJob("j1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(scores(got), []string{"pb:90", "pc:65", "pa:40"}) {
		t.Errorf("order = %v", scores(got))
	}
	if got[0].ProfileID != "A" || got[0].EngineVersion != "v1" || got[1].ProfileID != "B" {
		t.Errorf("rows = %+v", got)
	}
}

func TestEqualScoresComeInPersonaIdOrder(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	if err := Replace("j1", []Row{{"pc", "A", 70, ""}, {"pa", "A", 70, ""}, {"pb", "A", 70, ""}}); err != nil {
		t.Fatal(err)
	}
	got, _ := ForJob("j1")
	if !reflect.DeepEqual(scores(got), []string{"pa:70", "pb:70", "pc:70"}) {
		t.Errorf("order = %v", scores(got))
	}
}

func TestForJobOfAJobWithoutRowsIsAnEmptyListNotNil(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	got, err := ForJob("j1")
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("%v %v", got, err)
	}
	if got, err := ForJob("nope"); err != nil || len(got) != 0 {
		t.Errorf("unknown job: %v %v", got, err)
	}
}

// Replace is a replacement: the old rows go, also those of personas that are not in the new set.
func TestReplaceReplacesTheWholeSet(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	addJob(t, "j2", "2026-06-01 10:00:00")
	_ = Replace("j1", []Row{{"pa", "A", 40, ""}, {"pb", "A", 90, ""}})
	_ = Replace("j2", []Row{{"pa", "A", 11, ""}})
	if err := Replace("j1", []Row{{"pb", "A", 55, ""}, {"pc", "A", 66, ""}}); err != nil {
		t.Fatal(err)
	}
	got, _ := ForJob("j1")
	if !reflect.DeepEqual(scores(got), []string{"pc:66", "pb:55"}) {
		t.Errorf("j1 = %v", scores(got))
	}
	other, _ := ForJob("j2")
	if !reflect.DeepEqual(scores(other), []string{"pa:11"}) {
		t.Errorf("another job's rows changed: %v", scores(other))
	}
}

func TestReplaceWithAnEmptySetRemovesTheRows(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	_ = Replace("j1", []Row{{"pa", "A", 40, ""}})
	if err := Replace("j1", nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := ForJob("j1"); len(got) != 0 {
		t.Errorf("rows left: %v", scores(got))
	}
}

func TestReplaceOfAnUnknownJobWritesNothing(t *testing.T) {
	setup(t)
	if err := Replace("nope", []Row{{"pa", "A", 40, ""}}); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("err = %v", err)
	}
	if n := count(t, `SELECT COUNT(*) FROM job_persona_matches`); n != 0 {
		t.Errorf("rows = %d", n)
	}
}

func TestAPersonaListedTwiceKeepsItsFirstEntry(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	if err := Replace("j1", []Row{{"pa", "A", 40, ""}, {"pa", "A", 99, ""}, {"pb", "A", 10, ""}}); err != nil {
		t.Fatal(err)
	}
	got, _ := ForJob("j1")
	if !reflect.DeepEqual(scores(got), []string{"pa:40", "pb:10"}) {
		t.Errorf("rows = %v", scores(got))
	}
}

// A bad score rolls the whole replacement back: the earlier rows are still there.
func TestAScoreOutOfRangeRollsEverythingBack(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	_ = Replace("j1", []Row{{"pa", "A", 40, ""}})
	for _, bad := range []int{-1, 101} {
		if err := Replace("j1", []Row{{"pb", "A", 70, ""}, {"pc", "A", bad, ""}}); err == nil {
			t.Errorf("score %d must be refused", bad)
		}
	}
	got, _ := ForJob("j1")
	if !reflect.DeepEqual(scores(got), []string{"pa:40"}) {
		t.Errorf("rows after the refused replacements = %v", scores(got))
	}
	for _, ok := range []int{0, 100} {
		if err := Replace("j1", []Row{{"pa", "A", ok, ""}}); err != nil {
			t.Errorf("score %d must be accepted: %v", ok, err)
		}
	}
}

func TestForJobsReturnsOnlyThatProfilesPersonasByJob(t *testing.T) {
	setup(t)
	for _, id := range []string{"j1", "j2", "j3"} {
		addJob(t, id, "2026-06-01 10:00:00")
	}
	_ = Replace("j1", []Row{{"pa", "A", 40, ""}, {"pb", "A", 90, ""}, {"px", "B", 99, ""}})
	_ = Replace("j2", []Row{{"pa", "A", 10, ""}})
	_ = Replace("j3", []Row{{"px", "B", 50, ""}}) // only another profile's persona

	got, err := ForJobs("A", []string{"j1", "j2", "j3", "nope"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(scores(got["j1"]), []string{"pb:90", "pa:40"}) || !reflect.DeepEqual(scores(got["j2"]), []string{"pa:10"}) {
		t.Errorf("A: %v", got)
	}
	if _, ok := got["j3"]; ok {
		t.Error("a job with only another profile's rows must be absent")
	}
	if _, ok := got["nope"]; ok {
		t.Error("an unknown job must be absent")
	}
	if none, _ := ForJobs("Z", []string{"j1"}); len(none) != 0 {
		t.Errorf("a profile without rows: %v", none)
	}
	if empty, err := ForJobs("A", nil); err != nil || len(empty) != 0 {
		t.Errorf("no jobs: %v %v", empty, err)
	}
}

// More jobs than one query takes are all read (the id list is cut into chunks).
func TestForJobsReadsEveryJobOfALongList(t *testing.T) {
	setup(t)
	const n = 1000
	ids := make([]string, 0, n)
	tx, _ := db.JobsDB.Begin()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("job-%04d", i)
		ids = append(ids, id)
		_, _ = tx.Exec(`INSERT INTO jobs (id, source_url, title) VALUES (?, ?, 'T')`, id, "https://x.example/"+id)
		_, _ = tx.Exec(`INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES (?, 'pa', 'A', ?)`, id, i%101)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := ForJobs("A", ids)
	if err != nil || len(got) != n {
		t.Fatalf("jobs read = %d (%v), want %d", len(got), err, n)
	}
	if got["job-0999"][0].Score != 999%101 || got["job-0000"][0].Score != 0 || got["job-0400"][0].Score != 400%101 {
		t.Error("a score of a job past a chunk border is wrong")
	}
}

func TestPurgePersonaRemovesOnlyThatPersonasRows(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	addJob(t, "j2", "2026-06-01 10:00:00")
	_ = Replace("j1", []Row{{"pa", "A", 40, ""}, {"pb", "A", 90, ""}})
	_ = Replace("j2", []Row{{"pa", "A", 10, ""}, {"pb", "A", 20, ""}})
	if err := PurgePersona("pa"); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT COUNT(*) FROM job_persona_matches WHERE persona_id = 'pa'`); n != 0 {
		t.Errorf("pa rows left: %d", n)
	}
	if n := count(t, `SELECT COUNT(*) FROM job_persona_matches WHERE persona_id = 'pb'`); n != 2 {
		t.Errorf("pb rows = %d, want 2", n)
	}
	if err := PurgePersona("nobody"); err != nil {
		t.Errorf("an unknown persona: %v", err)
	}
}

func TestPurgeProfileRemovesOnlyThatProfilesRows(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	_ = Replace("j1", []Row{{"pa", "A", 40, ""}, {"pa2", "A", 50, ""}, {"px", "B", 60, ""}})
	if err := PurgeProfile("A"); err != nil {
		t.Fatal(err)
	}
	got, _ := ForJob("j1")
	if !reflect.DeepEqual(scores(got), []string{"px:60"}) {
		t.Errorf("rows = %v", scores(got))
	}
}

func TestDeleteForJobRemovesOnlyThatJobsRows(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	addJob(t, "j2", "2026-06-01 10:00:00")
	_ = Replace("j1", []Row{{"pa", "A", 40, ""}})
	_ = Replace("j2", []Row{{"pa", "A", 50, ""}})
	if err := DeleteForJob("j1"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteForJob("nope"); err != nil {
		t.Errorf("unknown job: %v", err)
	}
	if a, _ := ForJob("j1"); len(a) != 0 {
		t.Error("j1 rows left")
	}
	if b, _ := ForJob("j2"); len(b) != 1 {
		t.Error("j2 rows must stay")
	}
}

// The job's own delete takes its rows with it (the cascade), with nothing else involved.
func TestDeletingAJobRemovesItsRowsByTheCascade(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	addJob(t, "j2", "2026-06-01 10:00:00")
	_ = Replace("j1", []Row{{"pa", "A", 40, ""}, {"pb", "A", 90, ""}})
	_ = Replace("j2", []Row{{"pa", "A", 10, ""}})
	if _, err := db.JobsDB.Exec(`DELETE FROM jobs WHERE id = 'j1'`); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT COUNT(*) FROM job_persona_matches WHERE job_id = 'j1'`); n != 0 {
		t.Errorf("j1 rows left after the job was deleted: %d", n)
	}
	if n := count(t, `SELECT COUNT(*) FROM job_persona_matches WHERE job_id = 'j2'`); n != 1 {
		t.Errorf("j2 rows = %d", n)
	}
}

func TestJobsWithoutRowsListsNewestFirstAndRespectsTheLimit(t *testing.T) {
	setup(t)
	addJob(t, "old", "2026-01-01 10:00:00")
	addJob(t, "mid", "2026-03-01 10:00:00")
	addJob(t, "new", "2026-06-01 10:00:00")
	addJob(t, "done", "2026-07-01 10:00:00")
	_ = Replace("done", []Row{{"pa", "A", 40, ""}})

	got, err := JobsWithoutRows(10)
	if err != nil || !reflect.DeepEqual(got, []string{"new", "mid", "old"}) {
		t.Errorf("%v %v", got, err)
	}
	if two, _ := JobsWithoutRows(2); !reflect.DeepEqual(two, []string{"new", "mid"}) {
		t.Errorf("limit 2: %v", two)
	}
	_ = Replace("new", []Row{{"pa", "A", 1, ""}})
	if after, _ := JobsWithoutRows(10); !reflect.DeepEqual(after, []string{"mid", "old"}) {
		t.Errorf("after new got rows: %v", after)
	}
}

func TestJobsWithoutRowsOfAnEmptyDatabaseIsAnEmptyList(t *testing.T) {
	setup(t)
	got, err := JobsWithoutRows(5)
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("%v %v", got, err)
	}
}

func TestTheTableRefusesBadValuesByItself(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	if _, err := db.JobsDB.Exec(`INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES ('nope','p','A',50)`); err == nil {
		t.Error("the foreign key must refuse an unknown job")
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES ('j1','p','A',101)`); err == nil {
		t.Error("the CHECK must refuse 101")
	}
	for i := 0; i < 2; i++ {
		_, err := db.JobsDB.Exec(`INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES ('j1','p','A',50)`)
		if (i == 0) != (err == nil) {
			t.Errorf("insert %d: err=%v (the primary key must refuse the second)", i, err)
		}
	}
}

// A board can name far more jobs than SQLite takes parameters in one query (32766 at most): the list is cut into chunks.
func TestForJobsTakesMoreIdsThanOneQueryCan(t *testing.T) {
	setup(t)
	addJob(t, "real-first", "2026-06-01 10:00:00")
	addJob(t, "real-last", "2026-06-01 10:00:00")
	_ = Replace("real-first", []Row{{"pa", "A", 11, ""}})
	_ = Replace("real-last", []Row{{"pa", "A", 22, ""}})
	ids := []string{"real-first"}
	for i := 0; i < 40000; i++ {
		ids = append(ids, fmt.Sprintf("missing-%d", i))
	}
	ids = append(ids, "real-last")
	got, err := ForJobs("A", ids)
	if err != nil {
		t.Fatalf("a long id list failed: %v", err)
	}
	if len(got) != 2 || got["real-first"][0].Score != 11 || got["real-last"][0].Score != 22 {
		t.Errorf("rows = %v", got)
	}
}

func TestReplaceForPersonaWritesUpdatesAndRemovesOnlyThatPersonasRows(t *testing.T) {
	setup(t)
	for _, id := range []string{"j1", "j2", "j3", "j4"} {
		addJob(t, id, "2026-06-01 10:00:00")
	}
	_ = Replace("j1", []Row{{"pa", "A", 10, "v1"}, {"px", "B", 77, "v1"}})
	_ = Replace("j2", []Row{{"pa", "A", 20, "v1"}, {"px", "B", 88, "v1"}})
	_ = Replace("j3", []Row{{"px", "B", 66, "v1"}})
	// pa: j1 updated, j2 can no longer be judged (row removed), j3 newly judged (row added), j4 never judged (nothing)
	err := ReplaceForPersona("pa", "A", []PersonaJobScore{
		{JobID: "j1", Judged: true, Score: 55, EngineVersion: "v2"},
		{JobID: "j2", Judged: false},
		{JobID: "j3", Judged: true, Score: 33, EngineVersion: "v2"},
		{JobID: "j4", Judged: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	for job, want := range map[string][]string{
		"j1": {"px:77", "pa:55"},
		"j2": {"px:88"},
		"j3": {"px:66", "pa:33"},
		"j4": {},
	} {
		got, _ := ForJob(job)
		if !reflect.DeepEqual(scores(got), want) {
			t.Errorf("%s = %v, want %v", job, scores(got), want)
		}
	}
	got, _ := ForJob("j1")
	if got[1].EngineVersion != "v2" || got[1].ProfileID != "A" {
		t.Errorf("the updated row = %+v", got[1])
	}
}

func TestReplaceForPersonaSkipsAJobThatIsGone(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	err := ReplaceForPersona("pa", "A", []PersonaJobScore{{JobID: "gone", Judged: true, Score: 50}, {JobID: "j1", Judged: true, Score: 60}})
	if err != nil {
		t.Fatalf("a deleted job must not be an error: %v", err)
	}
	if n := count(t, `SELECT COUNT(*) FROM job_persona_matches`); n != 1 {
		t.Errorf("rows = %d, want only the existing job's", n)
	}
	if err := ReplaceForPersona("pa", "A", nil); err != nil {
		t.Errorf("an empty call: %v", err)
	}
}

// A refused score rolls the whole chunk back: what was there before is still there.
func TestReplaceForPersonaRollsBackAChunkWithABadScore(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	addJob(t, "j2", "2026-06-01 10:00:00")
	_ = Replace("j1", []Row{{"pa", "A", 40, ""}})
	err := ReplaceForPersona("pa", "A", []PersonaJobScore{
		{JobID: "j1", Judged: true, Score: 90}, // would update j1 ...
		{JobID: "j2", Judged: true, Score: 101},
	})
	if err == nil {
		t.Fatal("a score of 101 must be refused")
	}
	got, _ := ForJob("j1")
	if !reflect.DeepEqual(scores(got), []string{"pa:40"}) {
		t.Errorf("j1 after the rolled back call = %v, want the old pa:40", scores(got))
	}
	if n := count(t, `SELECT COUNT(*) FROM job_persona_matches WHERE job_id = 'j2'`); n != 0 {
		t.Error("j2 must have no row")
	}
}

func TestTheScoreStateIsReadWrittenAndUpdated(t *testing.T) {
	setup(t)
	if _, ok, err := GetState("pa"); err != nil || ok {
		t.Fatalf("no state yet: ok=%v err=%v", ok, err)
	}
	if err := SetState("pa", "A", "fp-1"); err != nil {
		t.Fatal(err)
	}
	s, ok, err := GetState("pa")
	if err != nil || !ok || s.Fingerprint != "fp-1" || s.ScoredAt == "" {
		t.Fatalf("state = %+v ok=%v err=%v", s, ok, err)
	}
	if err := SetState("pa", "A", "fp-2"); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := GetState("pa"); s.Fingerprint != "fp-2" {
		t.Errorf("after an update: %+v", s)
	}
	if n := count(t, `SELECT COUNT(*) FROM persona_score_state`); n != 1 {
		t.Errorf("states = %d, want one row per persona", n)
	}
	if _, ok, _ := GetState("pb"); ok {
		t.Error("another persona must not have a state")
	}
}

func TestThePurgesAlsoRemoveTheScoreState(t *testing.T) {
	setup(t)
	addJob(t, "j1", "2026-06-01 10:00:00")
	_ = Replace("j1", []Row{{"pa", "A", 40, ""}, {"pa2", "A", 41, ""}, {"px", "B", 60, ""}})
	for _, st := range [][2]string{{"pa", "A"}, {"pa2", "A"}, {"px", "B"}} {
		_ = SetState(st[0], st[1], "fp")
	}
	if err := PurgePersona("pa"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := GetState("pa"); ok {
		t.Error("pa's state must be gone")
	}
	if _, ok, _ := GetState("pa2"); !ok {
		t.Error("pa2's state must stay")
	}
	if err := PurgeProfile("A"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := GetState("pa2"); ok {
		t.Error("the profile's states must be gone")
	}
	if _, ok, _ := GetState("px"); !ok {
		t.Error("another profile's state must stay")
	}
	if got, _ := ForJob("j1"); !reflect.DeepEqual(scores(got), []string{"px:60"}) {
		t.Errorf("rows = %v", scores(got))
	}
}

func TestAllStatesReadsEveryPersonasState(t *testing.T) {
	setup(t)
	got, err := AllStates()
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	_ = SetState("pa", "A", "fp-a")
	_ = SetState("pb", "B", "fp-b")
	_ = SetState("pa", "A", "fp-a2")
	got, err = AllStates()
	if err != nil || len(got) != 2 || got["pa"].Fingerprint != "fp-a2" || got["pb"].Fingerprint != "fp-b" || got["pa"].ScoredAt == "" {
		t.Errorf("states = %+v (%v)", got, err)
	}
}
