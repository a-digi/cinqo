package jobs

import (
	"errors"
	"strings"
	"testing"

	"career-tool-backend/db"
)

// sortFixture saves six jobs, oldest A to newest F, with matches that cover every case: a high score (A 90), a tie
// between an older and a newer job (B and E, both 50), an AI match still running (C), a failed one that still carries a score (D, which must not count) and none (F).
func sortFixture(t *testing.T) {
	t.Helper()
	setupJobsDB(t)
	titles := []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot"}
	for i, title := range titles {
		exec(t, `INSERT INTO jobs (id, source_url, title, crawled_at) VALUES (?, ?, ?, ?)`,
			string(rune('A'+i)), "https://x.example/"+title, title, "2026-01-0"+string(rune('1'+i))+" 10:00:00")
	}
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status) VALUES ('A', 'p', 90, 'completed')`)
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status) VALUES ('B', 'p', 50, 'completed')`)
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status) VALUES ('C', 'p', NULL, 'matching')`)
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status, error) VALUES ('D', 'p', 70, 'failed', 'boom')`)
	exec(t, `INSERT INTO job_matches (job_id, profile_id, score, status) VALUES ('E', 'p', 50, 'completed')`)
}

func idsOfPage(t *testing.T, sort string, limit, offset int) string {
	t.Helper()
	res, err := SearchJobsSorted("", "", nil, "", "", limit, offset, sort)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, j := range res.Jobs {
		ids = append(ids, j.ID)
	}
	return strings.Join(ids, "")
}

func TestBestMatchFirstPutsScoresHighToLowThenTheUnscoredNewestFirst(t *testing.T) {
	sortFixture(t)
	// A 90, then the 50s newest first (E before B), then no score at all: F (none), D (failed), C (running), newest first
	if got := idsOfPage(t, SortMatch, 25, 0); got != "AEBFDC" {
		t.Errorf("best match first = %s, want AEBFDC", got)
	}
	if got := idsOfPage(t, SortNewest, 25, 0); got != "FEDCBA" {
		t.Errorf("newest first = %s, want FEDCBA", got)
	}
}

func TestNewestIsTheDefaultAndSearchJobsKeepsItsOrder(t *testing.T) {
	sortFixture(t)
	if got := idsOfPage(t, "", 25, 0); got != "FEDCBA" {
		t.Errorf("an empty sort = %s", got)
	}
	res, err := SearchJobs("", "", nil, "", "", 25, 0)
	if err != nil || len(res.Jobs) != 6 || res.Jobs[0].ID != "F" || res.Jobs[5].ID != "A" {
		t.Errorf("SearchJobs (the AI's list) must stay newest first: %+v %v", res.Jobs, err)
	}
}

func TestPagingThroughBestMatchRepeatsAndSkipsNothing(t *testing.T) {
	sortFixture(t)
	all := idsOfPage(t, SortMatch, 25, 0)
	paged := idsOfPage(t, SortMatch, 2, 0) + idsOfPage(t, SortMatch, 2, 2) + idsOfPage(t, SortMatch, 2, 4)
	if paged != all || len(paged) != 6 {
		t.Errorf("pages = %s, all = %s", paged, all)
	}
	if res, _ := SearchJobsSorted("", "", nil, "", "", 2, 0, SortMatch); res.Total != 6 {
		t.Errorf("total = %d", res.Total)
	}
}

func TestSortCombinesWithTheFilters(t *testing.T) {
	sortFixture(t)
	// the query "e" keeps Charlie, Delta and Echo; by match Echo (50) leads, then the unscored, newest first
	res, err := SearchJobsSorted("e", "", nil, "", "", 25, 0, SortMatch)
	if err != nil {
		t.Fatal(err)
	}
	var ids string
	for _, j := range res.Jobs {
		ids += j.ID
	}
	if ids != "EDC" || res.Total != 3 {
		t.Errorf("filtered and sorted = %s (total %d), want EDC (3)", ids, res.Total)
	}
}

func TestAnUnknownSortIsRefused(t *testing.T) {
	sortFixture(t)
	for _, bad := range []string{"oldest", "MATCH", "match;DROP TABLE jobs", "score"} {
		if _, err := SearchJobsSorted("", "", nil, "", "", 25, 0, bad); !errors.Is(err, ErrInvalidSort) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&n); err != nil || n != 6 {
		t.Errorf("jobs table damaged: %d %v", n, err)
	}
}
