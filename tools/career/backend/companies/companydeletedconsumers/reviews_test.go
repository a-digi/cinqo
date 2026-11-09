package companydeletedconsumers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func deliverReviews(method, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	CompanyReviewsDeleteHandler(rec, httptest.NewRequest(method, "/events/company-deleted-reviews", strings.NewReader(body)))
	return rec
}

// fakeCore (comments_test.go) answers every purge with a status and records the calls; here the path matters too.
func TestTheCompanysReviewsArePurgedAndTheEventIsAnswered200(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	if rec := deliverReviews(http.MethodPost, goodEvent); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := calls()
	if len(got) != 1 || got[0].EntityType != "company" || got[0].EntityID != "c1" {
		t.Errorf("purges = %v, want one for company c1", got)
	}
}

func TestThePurgeGoesToTheReviewRouteNotTheCommentsRoute(t *testing.T) {
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

func TestAReviewsDeliveryTwiceIsHarmless(t *testing.T) {
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

func TestABadEventPurgesNoReviews(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	for name, body := range map[string]string{
		"not json": `nope`, "no payload": `{"topic":"career.company.deleted"}`, "empty company id": `{"payload":{"company_id":""}}`,
		"only a name": `{"payload":{"name":"Acme"}}`, "payload array": `{"payload":[1]}`,
	} {
		if rec := deliverReviews(http.MethodPost, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	big := `{"payload":{"company_id":"c1","name":"` + strings.Repeat("x", 70*1024) + `"}}`
	if rec := deliverReviews(http.MethodPost, big); rec.Code != http.StatusBadRequest {
		t.Errorf("a huge event: %d", rec.Code)
	}
	if n := len(calls()); n != 0 {
		t.Errorf("%d purges for bad events", n)
	}
}

func TestOnlyPostIsAcceptedForReviews(t *testing.T) {
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

func TestAReviewsFailureThatMayPassIsA500SoCoreRetries(t *testing.T) {
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

func TestAReviewsRefusalWithA4xxIsLetGo(t *testing.T) {
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

// The consumer is declared in the manifest, routed in main.go, and the company review type is declared with the companies
// scopes, 5 stars and halves (core refuses a purge for a type the tool did not declare).
func TestTheReviewConsumerAndTheCompanyReviewTypeAreDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		EventListeners []struct {
			Topic      string `json:"topic"`
			PathSuffix string `json:"path_suffix"`
		} `json:"event_listeners"`
		Reviews []struct {
			EntityType string `json:"entity_type"`
			Scale      int    `json:"scale"`
			HalfStars  bool   `json:"half_stars"`
			ReadScope  string `json:"read_scope"`
			WriteScope string `json:"write_scope"`
		} `json:"reviews"`
		Scopes []struct {
			Scope string `json:"scope"`
		} `json:"scopes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, l := range m.EventListeners {
		if l.Topic == "career.company.deleted" && l.PathSuffix == "events/company-deleted-reviews" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("manifest.json has %d career.company.deleted -> events/company-deleted-reviews listeners, want 1", n)
	}
	if len(m.Reviews) != 1 {
		t.Fatalf("%d review types declared, want 1 (company)", len(m.Reviews))
	}
	r := m.Reviews[0]
	if r.EntityType != "company" || r.Scale != 5 || !r.HalfStars || r.ReadScope != "tool:career:companies" || r.WriteScope != "tool:career:companies" {
		t.Errorf("the company review type: %+v", r)
	}
	declared := false
	for _, s := range m.Scopes {
		if s.Scope == r.ReadScope {
			declared = true
		}
	}
	if !declared {
		t.Error("the review scope is not one the tool declares")
	}
	mainSrc, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), `"/events/company-deleted-reviews", companydeletedconsumers.CompanyReviewsDeleteHandler`) {
		t.Error("main.go does not route /events/company-deleted-reviews to CompanyReviewsDeleteHandler")
	}
}
