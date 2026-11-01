package companydeletedconsumers

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
	CompanyCommentsDeleteHandler(rec, httptest.NewRequest(method, "/events/company-deleted-comments", strings.NewReader(body)))
	return rec
}

const goodEvent = `{"topic":"career.company.deleted","payload":{"company_id":"c1","name":"Acme"}}`

func TestTheCompanysCommentsArePurgedAndTheEventIsAnswered200(t *testing.T) {
	calls := fakeCore(t, http.StatusOK)
	if rec := deliver(http.MethodPost, goodEvent); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := calls()
	if len(got) != 1 || got[0].EntityType != "company" || got[0].EntityID != "c1" {
		t.Errorf("purges = %v, want one for company c1", got)
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
		"not json":         `nope`,
		"no payload":       `{"topic":"career.company.deleted"}`,
		"empty company id": `{"payload":{"company_id":""}}`,
		"only a name":      `{"payload":{"name":"Acme"}}`,
		"payload array":    `{"payload":[1]}`,
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

// The consumer is declared in the manifest, routed in main.go, and the company comment type is declared with the
// companies scopes (core refuses a purge for a type the tool did not declare).
func TestTheConsumerAndTheCompanyTypeAreDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		EventListeners []struct {
			Topic      string `json:"topic"`
			PathSuffix string `json:"path_suffix"`
		} `json:"event_listeners"`
		Comments []struct {
			EntityType string `json:"entity_type"`
			ReadScope  string `json:"read_scope"`
			WriteScope string `json:"write_scope"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	listener := false
	for _, l := range m.EventListeners {
		if l.Topic == "career.company.deleted" && l.PathSuffix == "events/company-deleted-comments" {
			listener = true
		}
	}
	if !listener {
		t.Error("manifest.json has no career.company.deleted -> events/company-deleted-comments listener")
	}
	typ := false
	for _, c := range m.Comments {
		if c.EntityType == "company" && c.ReadScope == "tool:career:companies" && c.WriteScope == "tool:career:companies" {
			typ = true
		}
	}
	if !typ {
		t.Error("manifest.json does not declare the company comment type with the companies scopes")
	}
	mainSrc, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), `"/events/company-deleted-comments", companydeletedconsumers.CompanyCommentsDeleteHandler`) {
		t.Error("main.go does not route /events/company-deleted-comments to CompanyCommentsDeleteHandler")
	}
}
