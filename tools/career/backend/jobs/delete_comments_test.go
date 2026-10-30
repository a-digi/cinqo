package jobs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// corePurges runs a fake core and records the entity ids it is asked to purge.
func corePurges(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var ids []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/comments/purge" {
			buf := make([]byte, 512)
			n, _ := r.Body.Read(buf)
			mu.Lock()
			ids = append(ids, string(buf[:n]))
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), ids...)
	}
}

func TestDeletingAJobPurgesItsComments(t *testing.T) {
	setupJobsDB(t)
	got := corePurges(t)
	keep := seedJob(t, "Keeper", "Acme")
	gone := seedJob(t, "Goner", "Acme")

	if err := DeleteJob(gone); err != nil {
		t.Fatal(err)
	}
	purges := got()
	if len(purges) != 1 || !strings.Contains(purges[0], gone) || !strings.Contains(purges[0], `"entityType":"job"`) {
		t.Fatalf("purges = %v", purges)
	}
	if strings.Contains(purges[0], keep) {
		t.Error("the wrong job's comments were purged")
	}
	if _, err := GetJobByID(keep); err != nil {
		t.Errorf("the other job must still exist: %v", err)
	}
}

func TestDeletingAJobThatDoesNotExistPurgesNothing(t *testing.T) {
	setupJobsDB(t)
	got := corePurges(t)
	if err := DeleteJob("00000000-0000-4000-8000-000000000000"); err != nil {
		t.Fatal(err)
	}
	if n := len(got()); n != 0 {
		t.Errorf("%d purge calls for a job that was never there", n)
	}
}

func TestDeletingAJobStillWorksWhenCoreIsUnreachable(t *testing.T) {
	setupJobsDB(t)
	t.Setenv("CORE_API_URL", "http://127.0.0.1:1") // nothing listens here
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	id := seedJob(t, "Goner", "Acme")
	if err := DeleteJob(id); err != nil {
		t.Fatalf("a failed comment cleanup must not fail the delete: %v", err)
	}
	if _, err := GetJobByID(id); err == nil {
		t.Error("the job was not deleted")
	}
}
