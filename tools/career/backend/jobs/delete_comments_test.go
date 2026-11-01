package jobs

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// Removing a job's comments is the JobCommentsDelete consumer's work (step 139, jobdeletedconsumers): DeleteJob only
// announces the delete, so it must not also call core's purge (one path, no double call).
func TestDeletingAJobDoesNotPurgeCommentsItself(t *testing.T) {
	setupJobsDB(t)
	var purges atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/comments/purge" {
			purges.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	id := seedJob(t, "Goner", "Acme")
	if err := DeleteJob(id); err != nil {
		t.Fatal(err)
	}
	if n := purges.Load(); n != 0 {
		t.Errorf("DeleteJob called the purge %d time(s); the consumer does that now", n)
	}
}

func TestDeletingAJobStillWorksWhenCoreIsUnreachable(t *testing.T) {
	setupJobsDB(t)
	t.Setenv("CORE_API_URL", "http://127.0.0.1:1") // nothing listens here
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	id := seedJob(t, "Goner", "Acme")
	if err := DeleteJob(id); err != nil {
		t.Fatalf("a failed announcement must not fail the delete: %v", err)
	}
	if _, err := GetJobByID(id); err == nil {
		t.Error("the job was not deleted")
	}
}
