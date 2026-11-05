package match

import (
	"testing"

	"career-tool-backend/db"
)

// Small helpers the match tests share with the jobs tests (both packages used to be one); each package keeps its own copy.

func setupJobsDB(t *testing.T) {
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

func exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := db.JobsDB.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func exists(t *testing.T, id string) bool {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM jobs WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}
