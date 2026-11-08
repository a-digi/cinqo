package profilematch

import (
	"career-tool-backend/match"
	"slices"
	"testing"

	"career-tool-backend/jobs"
)

// The real write paths set the columns these queries read: a detail crawl over an old match makes the job stale until the
// match is made again.
func TestADetailCrawlMakesTheMatchStaleUntilItIsMadeAgain(t *testing.T) {
	setupJobsDB(t)
	profile, _ := seedProfile(t, "Fit", fitSkills)
	addJobWithText(t, "j1", oldText)
	if _, err := match.RunManualMatch("j1", profile); err != nil {
		t.Fatal(err)
	}
	exec(t, `UPDATE job_matches SET updated_at = '2000-01-01 00:00:00' WHERE job_id = 'j1'`) // made long before the text changes
	if got, _ := match.StaleManualMatchJobIDs(10); len(got) != 0 {
		t.Fatalf("a job whose text never changed is never stale: %v", got)
	}
	if _, err := jobs.SaveJobDetailExtraction("j1", map[string]string{"description": matchJobDesc}); err != nil {
		t.Fatal(err)
	}
	if got, _ := match.StaleManualMatchJobIDs(10); !slices.Equal(got, []string{"j1"}) {
		t.Fatalf("after the change the old match is stale: %v", got)
	}
	if o := rematchOneJob(t, "j1"); o.Status != ChangedJobRescored {
		t.Fatalf("%+v", o)
	}
	exec(t, `UPDATE jobs SET description_changed_at = datetime('now', '-1 minute') WHERE id = 'j1'`) // the re-match ran after the change
	if got, _ := match.StaleManualMatchJobIDs(10); len(got) != 0 {
		t.Errorf("after the re-match the job is not stale: %v", got)
	}
}
