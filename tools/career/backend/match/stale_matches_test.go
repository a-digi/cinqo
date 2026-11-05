package match

import (
	"slices"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

// Step 164: the queries behind the sweepers' catch-up for lost description_changed events.

func addChangedJob(t *testing.T, id, changedAt string) {
	t.Helper()
	seedCrawled := `INSERT INTO jobs (id, source_url, title, description, source, description_changed_at) VALUES (?, ?, 'T', 'text', 'crawled', ?)`
	var stamp any
	if changedAt != "" {
		stamp = changedAt
	}
	exec(t, seedCrawled, id, "https://x/"+id, stamp)
}

func addMatchRow(t *testing.T, jobID, kind, status, updatedAt string) {
	t.Helper()
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status, kind, updated_at) VALUES (?, 'p', 50, ?, ?, ?)`, jobID, status, kind, updatedAt)
}

func TestStaleManualMatchesAreFoundByComparingWithTheTextChange(t *testing.T) {
	setupJobsDB(t)
	const changed = "2026-11-02 10:00:00"
	for id, c := range map[string]struct{ kind, status, updated string }{
		"older":   {"manual", "completed", "2026-11-02 09:59:59"}, // made before the change: stale
		"same":    {"manual", "completed", changed},               // the very second: cannot be told apart, handled once more
		"newer":   {"manual", "completed", "2026-11-02 10:00:01"},
		"ai":      {"ai", "completed", "2000-01-01 00:00:00"},
		"running": {"manual", "matching", "2000-01-01 00:00:00"},
		"failed":  {"manual", "failed", "2000-01-01 00:00:00"},
		"aimatch": {"ai", "matching", "2000-01-01 00:00:00"},
	} {
		addChangedJob(t, id, changed)
		addMatchRow(t, id, c.kind, c.status, c.updated)
	}
	addChangedJob(t, "norow", changed)
	addChangedJob(t, "never-changed", "")
	addMatchRow(t, "never-changed", "manual", "completed", "2000-01-01 00:00:00")

	got, err := StaleManualMatchJobIDs(100)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []string{"older", "same"}; !slices.Equal(got, want) {
		t.Errorf("stale = %v, want %v (an AI result, a run, a failed row, a fresh row, no row and a never-changed job are never offered)", got, want)
	}
}

func TestStaleMatchesComeMostRecentlyChangedFirstAndRespectTheLimit(t *testing.T) {
	setupJobsDB(t)
	for _, c := range [][2]string{{"a", "2026-11-02 10:00:00"}, {"b", "2026-11-02 12:00:00"}, {"c", "2026-11-02 11:00:00"}, {"d", "2026-11-02 12:00:00"}} {
		addChangedJob(t, c[0], c[1])
		addMatchRow(t, c[0], "manual", "completed", "2000-01-01 00:00:00")
	}
	got, _ := StaleManualMatchJobIDs(10)
	if want := []string{"b", "d", "c", "a"}; !slices.Equal(got, want) {
		t.Errorf("order %v, want %v", got, want)
	}
	if two, _ := StaleManualMatchJobIDs(2); !slices.Equal(two, []string{"b", "d"}) {
		t.Errorf("limit 2: %v", two)
	}
	for _, n := range []int{0, -1} {
		if none, err := StaleManualMatchJobIDs(n); err != nil || none == nil || len(none) != 0 {
			t.Errorf("limit %d: %v %v", n, none, err)
		}
	}
	if empty, err := func() ([]string, error) { exec(t, `DELETE FROM job_matches`); return StaleManualMatchJobIDs(5) }(); err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("nothing stale is an empty list: %v %v", empty, err)
	}
}

func TestJobsNeedingARulesCheck(t *testing.T) {
	setupJobsDB(t)
	const changed = "2026-11-02 10:00:00"
	checks := map[string]string{"never": "", "before": "2026-11-02 09:59:59", "same": changed, "after": "2026-11-02 10:00:01"}
	for id, checked := range checks {
		addChangedJob(t, id, changed)
		if checked != "" {
			exec(t, `UPDATE jobs SET rules_checked_at = ? WHERE id = ?`, checked, id)
		}
	}
	addChangedJob(t, "unchanged", "")                                                          // never changed after it was found: not offered
	exec(t, `UPDATE jobs SET rules_checked_at = '2000-01-01 00:00:00' WHERE id = 'unchanged'`) // whatever its check says
	addChangedJob(t, "found-only", "")                                                         // the common case: never changed and never checked
	got, err := JobsNeedingRulesCheck(100)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []string{"before", "never", "same"}; !slices.Equal(got, want) {
		t.Errorf("needing a check: %v, want %v", got, want)
	}
	if two, _ := JobsNeedingRulesCheck(2); len(two) != 2 {
		t.Errorf("limit 2: %v", two)
	}
	if none, err := JobsNeedingRulesCheck(0); err != nil || none == nil || len(none) != 0 {
		t.Errorf("limit 0: %v %v", none, err)
	}
}

func TestMarkingJobsCheckedEndsTheOffer(t *testing.T) {
	setupJobsDB(t)
	addChangedJob(t, "j1", "2000-01-01 00:00:00") // changed long ago, never checked
	addChangedJob(t, "j2", "2000-01-01 00:00:00")
	addChangedJob(t, "j3", "2000-01-01 00:00:00")
	if err := MarkRulesChecked([]string{"j1", "", "nope", "j2"}); err != nil {
		t.Fatal(err)
	}
	got, _ := JobsNeedingRulesCheck(10)
	if !slices.Equal(got, []string{"j3"}) {
		t.Errorf("only the unmarked job is still offered: %v", got)
	}
	var recent int
	_ = db.JobsDB.QueryRow(`SELECT rules_checked_at >= datetime('now', '-1 minute') AND rules_checked_at <= datetime('now', '+1 minute') FROM jobs WHERE id = 'j1'`).Scan(&recent)
	if recent != 1 {
		t.Error("the mark is the time of the check, now")
	}
	var unmarked *string
	_ = db.JobsDB.QueryRow(`SELECT rules_checked_at FROM jobs WHERE id = 'j3'`).Scan(&unmarked)
	if unmarked != nil {
		t.Errorf("a job that was not marked stays unmarked: %v", unmarked)
	}
	for _, none := range [][]string{nil, {}, {""}} {
		if err := MarkRulesChecked(none); err != nil {
			t.Errorf("%v: %v", none, err)
		}
	}
	if err := MarkRulesChecked([]string{"j1", "j1"}); err != nil { // idempotent
		t.Error(err)
	}
}

func TestAChangeAfterTheCheckBringsTheJobBack(t *testing.T) {
	setupJobsDB(t)
	addChangedJob(t, "j1", "2000-01-01 00:00:00")
	_ = MarkRulesChecked([]string{"j1"})
	if got, _ := JobsNeedingRulesCheck(10); len(got) != 0 {
		t.Fatalf("checked: %v", got)
	}
	exec(t, `UPDATE jobs SET description_changed_at = datetime('now', '+1 minute') WHERE id = 'j1'`) // the text changed again later
	if got, _ := JobsNeedingRulesCheck(10); !slices.Equal(got, []string{"j1"}) {
		t.Errorf("a newer change must bring it back: %v", got)
	}
}

// The real write paths set the columns these queries read: a detail crawl over an old match makes the job stale until the
// match is made again.
func TestADetailCrawlMakesTheMatchStaleUntilItIsMadeAgain(t *testing.T) {
	setupJobsDB(t)
	profile, _ := seedProfile(t, "Fit", fitSkills)
	addJobWithText(t, "j1", oldText)
	if _, err := RunManualMatch("j1", profile); err != nil {
		t.Fatal(err)
	}
	exec(t, `UPDATE job_matches SET updated_at = '2000-01-01 00:00:00' WHERE job_id = 'j1'`) // made long before the text changes
	if got, _ := StaleManualMatchJobIDs(10); len(got) != 0 {
		t.Fatalf("a job whose text never changed is never stale: %v", got)
	}
	if _, err := jobs.SaveJobDetailExtraction("j1", map[string]string{"description": matchJobDesc}); err != nil {
		t.Fatal(err)
	}
	if got, _ := StaleManualMatchJobIDs(10); !slices.Equal(got, []string{"j1"}) {
		t.Fatalf("after the change the old match is stale: %v", got)
	}
	if o := rematchOneJob(t, "j1"); o.Status != ChangedJobRescored {
		t.Fatalf("%+v", o)
	}
	exec(t, `UPDATE jobs SET description_changed_at = datetime('now', '-1 minute') WHERE id = 'j1'`) // the re-match ran after the change
	if got, _ := StaleManualMatchJobIDs(10); len(got) != 0 {
		t.Errorf("after the re-match the job is not stale: %v", got)
	}
}
