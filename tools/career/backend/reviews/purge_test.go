package reviews

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type seenRequest struct {
	method, path, auth, contentType string
	body                            map[string]string
}

// fakeCore records what the tool sends to it.
func fakeCore(t *testing.T, status int) (*httptest.Server, func() []seenRequest) {
	t.Helper()
	var mu sync.Mutex
	var got []seenRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]string
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		got = append(got, seenRequest{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), body})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []seenRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]seenRequest(nil), got...)
	}
}

const company = "1fbd3006-0831-448c-8962-2851188e9de9"

func TestThePurgeRequestIsTheOneCoreExpects(t *testing.T) {
	srv, seen := fakeCore(t, http.StatusOK)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tool-secret")

	if err := PurgeEntityStrict(CompanyType, company); err != nil {
		t.Fatal(err)
	}
	got := seen()
	if len(got) != 1 {
		t.Fatalf("requests = %d, want 1", len(got))
	}
	r := got[0]
	if r.method != http.MethodPost || r.path != "/api/v1/reviews/purge" {
		t.Errorf("%s %s", r.method, r.path)
	}
	if r.auth != "ToolService tool-secret" {
		t.Errorf("Authorization = %q (must be the ToolService scheme, not Bearer)", r.auth)
	}
	if r.contentType != "application/json" || r.body["entityType"] != "company" || r.body["entityId"] != company || len(r.body) != 2 {
		t.Errorf("%q %v (no owner: core takes it from the token)", r.contentType, r.body)
	}
}

func TestTheCompanyTypeIsTheOneTheManifestDeclares(t *testing.T) {
	if CompanyType != "company" {
		t.Errorf("%q", CompanyType)
	}
}

func TestACoreThatIsNotConfiguredOrNotReachableIsAnError(t *testing.T) {
	t.Setenv("CORE_API_URL", "")
	t.Setenv("TOOL_SERVICE_TOKEN", "x")
	if err := PurgeEntityStrict(CompanyType, company); err == nil {
		t.Error("no core url")
	}
	t.Setenv("CORE_API_URL", "http://127.0.0.1:1")
	t.Setenv("TOOL_SERVICE_TOKEN", "")
	if err := PurgeEntityStrict(CompanyType, company); err == nil {
		t.Error("no token")
	}
	t.Setenv("TOOL_SERVICE_TOKEN", "x")
	err := PurgeEntityStrict(CompanyType, company)
	var refused *RefusedError
	if err == nil || errors.As(err, &refused) {
		t.Errorf("an unreachable core is a plain error, not a refusal: %v", err)
	}
	t.Setenv("CORE_API_URL", "http://bad host")
	if err := PurgeEntityStrict(CompanyType, company); err == nil {
		t.Error("an unusable url")
	}
}

func TestARefusalSaysWhetherAskingAgainCanHelp(t *testing.T) {
	for status, final := range map[int]bool{400: true, 401: true, 403: true, 404: true, 499: true, 500: false, 503: false, 301: false, 201: false} {
		srv, _ := fakeCore(t, status)
		t.Setenv("CORE_API_URL", srv.URL)
		t.Setenv("TOOL_SERVICE_TOKEN", "x")
		err := PurgeEntityStrict(CompanyType, company)
		var refused *RefusedError
		if !errors.As(err, &refused) || refused.Status != status || refused.Final() != final {
			t.Errorf("status %d: %v (final should be %v)", status, err, final)
			continue
		}
		if refused.EntityType != "company" || refused.EntityID != company || refused.Error() == "" {
			t.Errorf("%+v", refused)
		}
	}
}
