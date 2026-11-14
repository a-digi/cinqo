package recruiterdeletedconsumers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func deliverReviews(method, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	RecruiterReviewsDeleteHandler(rec, httptest.NewRequest(method, "/events/recruiter-deleted-reviews", strings.NewReader(body)))
	return rec
}

// fakeCore and goodEvent are in comments_test.go.
func TestTheRecruitersReviewsArePurgedAndTheEventIsAnswered200(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	if rec := deliverReviews(http.MethodPost, goodEvent); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := calls()
	if len(got) != 1 || got[0].EntityType != "recruiter" || got[0].EntityID != "c1" {
		t.Errorf("purges = %v, want one for recruiter c1", got)
	}
}

func TestTheRecruiterPurgeGoesToTheReviewRouteNotTheCommentsRoute(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	deliverReviews(http.MethodPost, goodEvent)
	if len(paths) != 1 || paths[0] != "/api/v1/reviews/purge" {
		t.Errorf("%v", paths)
	}
}

func TestARecruiterReviewsDeliveryTwiceIsHarmless(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	for i := 0; i < 2; i++ {
		if rec := deliverReviews(http.MethodPost, goodEvent); rec.Code != http.StatusOK {
			t.Fatalf("delivery %d: %d", i, rec.Code)
		}
	}
	if n := len(calls()); n != 2 {
		t.Errorf("purges = %d, want 2", n)
	}
}

func TestABadEventPurgesNoRecruiterReviews(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	for name, body := range map[string]string{
		"not json": `nope`, "no payload": `{"topic":"career.recruiter.deleted"}`, "empty recruiter id": `{"payload":{"recruiter_id":""}}`,
		"only a name": `{"payload":{"name":"Ann"}}`, "a company id": `{"payload":{"company_id":"c1"}}`, "payload array": `{"payload":[1]}`,
	} {
		if rec := deliverReviews(http.MethodPost, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	big := `{"payload":{"recruiter_id":"c1","name":"` + strings.Repeat("x", 70*1024) + `"}}`
	if rec := deliverReviews(http.MethodPost, big); rec.Code != http.StatusBadRequest {
		t.Errorf("a huge event: %d", rec.Code)
	}
	if n := len(calls()); n != 0 {
		t.Errorf("%d purges for bad events", n)
	}
}

func TestOnlyPostIsAcceptedForRecruiterReviews(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		if rec := deliverReviews(m, goodEvent); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status %d, want 405", m, rec.Code)
		}
	}
	if n := len(calls()); n != 0 {
		t.Errorf("%d purges for wrong methods", n)
	}
}

func TestARecruiterReviewsFailureThatMayPassIsA500SoCoreRetries(t *testing.T) {
	fakeCore(t, http.StatusServiceUnavailable)
	if rec := deliverReviews(http.MethodPost, goodEvent); rec.Code != http.StatusInternalServerError {
		t.Errorf("5xx core: status %d, want 500", rec.Code)
	}
	t.Setenv("CORE_API_URL", "http://127.0.0.1:1")
	if rec := deliverReviews(http.MethodPost, goodEvent); rec.Code != http.StatusInternalServerError {
		t.Errorf("unreachable core: status %d, want 500", rec.Code)
	}
	t.Setenv("CORE_API_URL", "")
	if rec := deliverReviews(http.MethodPost, goodEvent); rec.Code != http.StatusInternalServerError {
		t.Errorf("unconfigured: status %d, want 500", rec.Code)
	}
}

func TestARecruiterReviewsRefusalWithA4xxIsLetGo(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden} {
		calls := fakeCore(t, status)
		if rec := deliverReviews(http.MethodPost, goodEvent); rec.Code != http.StatusOK {
			t.Errorf("core %d: status %d, want 200", status, rec.Code)
		}
		if n := len(calls()); n != 1 {
			t.Errorf("core %d: %d purge calls, want 1", status, n)
		}
	}
}
