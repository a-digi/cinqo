package reviews

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestTheRecruiterTypeIsTheOneTheManifestDeclares(t *testing.T) {
	if RecruiterType != "recruiter" {
		t.Errorf("%q", RecruiterType)
	}
}

// Every review type the manifest declares is wired end to end (plan 271): a constant here, the scopes the tool declares, a
// "career.<type>.deleted" listener to events/<type>-deleted-reviews, and that route in main.go. A new type that misses one of them leaves
// reviews behind when its entity is deleted, or core refuses the purge.
func TestEveryDeclaredReviewTypeIsWiredEndToEnd(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
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
	if len(m.Reviews) != 2 {
		t.Fatalf("%d review types declared, want 2 (company, recruiter)", len(m.Reviews))
	}
	purgeSrc, err := os.ReadFile("purge.go")
	if err != nil {
		t.Fatal(err)
	}
	mainSrc, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	scopes := map[string]bool{}
	for _, s := range m.Scopes {
		scopes[s.Scope] = true
	}
	for _, r := range m.Reviews {
		name := r.EntityType
		if !strings.Contains(string(purgeSrc), `= "`+name+`"`) {
			t.Errorf("%s: no constant in purge.go", name)
		}
		if !scopes[r.ReadScope] || !scopes[r.WriteScope] {
			t.Errorf("%s: scopes %q / %q are not declared by the tool", name, r.ReadScope, r.WriteScope)
		}
		listeners := 0
		for _, l := range m.EventListeners {
			if l.Topic == "career."+name+".deleted" && l.PathSuffix == "events/"+name+"-deleted-reviews" {
				listeners++
			}
		}
		if listeners != 1 {
			t.Errorf("%s: %d event listeners for events/%s-deleted-reviews, want 1", name, listeners, name)
		}
		if !strings.Contains(string(mainSrc), `"/events/`+name+`-deleted-reviews", `) {
			t.Errorf("%s: main.go does not route /events/%s-deleted-reviews", name, name)
		}
		if r.Scale != 5 {
			t.Errorf("%s: scale %d, want 5", name, r.Scale)
		}
	}
}
