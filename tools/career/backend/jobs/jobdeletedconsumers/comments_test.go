package jobdeletedconsumers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

type purgeCall struct{ EntityType, EntityID string }

// fakeCore answers every purge with status and records what it was asked to purge.
func fakeCore(t *testing.T, status int) func() []purgeCall {
	t.Helper()
	var mu sync.Mutex
	var calls []purgeCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var c purgeCall
		_ = json.Unmarshal(raw, &c)
		mu.Lock()
		calls = append(calls, c)
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return func() []purgeCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]purgeCall(nil), calls...)
	}
}

func deliver(method, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	JobCommentsDeleteHandler(rec, httptest.NewRequest(method, "/events/job-deleted-comments", strings.NewReader(body)))
	return rec
}

const goodEvent = `{"topic":"career.job.deleted","payload":{"job_id":"j1","title":"T","company":"Acme","company_id":"c1","source":"crawled"}}`

func TestTheJobsCommentsArePurgedAndTheEventIsAnswered200(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	if rec := deliver(http.MethodPost, goodEvent); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := calls()
	if len(got) != 1 || got[0].EntityType != "job" || got[0].EntityID != "j1" {
		t.Errorf("purges = %v, want one for job j1", got)
	}
}

func TestOnlyTheJobIdIsUsedNotTheCompany(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	deliver(http.MethodPost, goodEvent)
	for _, c := range calls() {
		if c.EntityID == "c1" || c.EntityType == "company" {
			t.Errorf("the company's comments must not be touched here: %v", c)
		}
	}
}

func TestADeliveryTwiceIsHarmless(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	for i := 0; i < 2; i++ {
		if rec := deliver(http.MethodPost, goodEvent); rec.Code != http.StatusOK {
			t.Fatalf("delivery %d: %d", i, rec.Code)
		}
	}
	if n := len(calls()); n != 2 {
		t.Errorf("purges = %d, want 2 (a purge is repeatable)", n)
	}
}

func TestABadEventIsA400AndPurgesNothing(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	for name, body := range map[string]string{
		"not json":      `nope`,
		"no payload":    `{"topic":"career.job.deleted"}`,
		"empty job id":  `{"payload":{"job_id":""}}`,
		"missing id":    `{"payload":{"company_id":"c1"}}`,
		"payload array": `{"payload":[1]}`,
	} {
		if rec := deliver(http.MethodPost, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	if n := len(calls()); n != 0 {
		t.Errorf("%d purges for bad events", n)
	}
}

func TestOnlyPostIsAccepted(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		if rec := deliver(m, goodEvent); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status %d, want 405", m, rec.Code)
		}
	}
	if n := len(calls()); n != 0 {
		t.Errorf("%d purges for wrong methods", n)
	}
}

// A failure that may pass is a 5xx so core's own retries deliver the event again.
func TestACoreFailureThatMayPassIsA500SoCoreRetries(t *testing.T) {
	fakeCore(t, http.StatusServiceUnavailable)
	if rec := deliver(http.MethodPost, goodEvent); rec.Code != http.StatusInternalServerError {
		t.Errorf("5xx core: status %d, want 500", rec.Code)
	}
	t.Setenv("CORE_API_URL", "http://127.0.0.1:1")
	if rec := deliver(http.MethodPost, goodEvent); rec.Code != http.StatusInternalServerError {
		t.Errorf("unreachable core: status %d, want 500", rec.Code)
	}
	t.Setenv("CORE_API_URL", "")
	if rec := deliver(http.MethodPost, goodEvent); rec.Code != http.StatusInternalServerError {
		t.Errorf("unconfigured: status %d, want 500", rec.Code)
	}
}

// A refusal with a 4xx is final: core would say the same again, so the event is let go (200) instead of retried.
func TestACoreRefusalWithA4xxIsLetGo(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden} {
		calls := fakeCore(t, status)
		if rec := deliver(http.MethodPost, goodEvent); rec.Code != http.StatusOK {
			t.Errorf("core %d: status %d, want 200", status, rec.Code)
		}
		if n := len(calls()); n != 1 {
			t.Errorf("core %d: %d purge calls, want 1", status, n)
		}
	}
}

// The consumer is declared in the manifest and routed in main.go under the same path.
func TestTheConsumerIsDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		EventListeners []struct {
			Topic      string `json:"topic"`
			PathSuffix string `json:"path_suffix"`
		} `json:"event_listeners"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"events/job-deleted-comments", "events/job-deleted-company", "events/job-deleted-persona-matches"} {
		found := false
		for _, l := range m.EventListeners {
			if l.Topic == "career.job.deleted" && l.PathSuffix == want {
				found = true
			}
		}
		if !found {
			t.Errorf("manifest.json has no career.job.deleted -> %s listener", want)
		}
	}
	mainSrc, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), `"/events/job-deleted-comments", jobdeletedconsumers.JobCommentsDeleteHandler`) {
		t.Error("main.go does not route /events/job-deleted-comments to JobCommentsDeleteHandler")
	}
	if !strings.Contains(string(mainSrc), `"/events/job-deleted-company", jobdeletedconsumers.CompanyDeleteHandler`) {
		t.Error("main.go does not route /events/job-deleted-company to CompanyDeleteHandler")
	}
	if !strings.Contains(string(mainSrc), `"/events/job-deleted-persona-matches", jobdeletedconsumers.PersonaMatchJobDeletedHandler`) {
		t.Error("main.go does not route /events/job-deleted-persona-matches to PersonaMatchJobDeletedHandler")
	}
}
