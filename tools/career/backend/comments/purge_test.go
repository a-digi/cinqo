package comments

import (
	"encoding/json"
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

func TestPurgeEntitySendsTheRightRequest(t *testing.T) {
	srv, seen := fakeCore(t, http.StatusOK)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tool-secret")

	PurgeEntity(JobType, "1fbd3006-0831-448c-8962-2851188e9de9")

	got := seen()
	if len(got) != 1 {
		t.Fatalf("requests = %d, want 1", len(got))
	}
	r := got[0]
	if r.method != http.MethodPost || r.path != "/api/v1/comments/purge" {
		t.Errorf("%s %s", r.method, r.path)
	}
	if r.auth != "ToolService tool-secret" {
		t.Errorf("Authorization = %q (must be the ToolService scheme, not Bearer)", r.auth)
	}
	if r.contentType != "application/json" || r.body["entityType"] != "job" || r.body["entityId"] != "1fbd3006-0831-448c-8962-2851188e9de9" {
		t.Errorf("content type %q, body %v", r.contentType, r.body)
	}
	if len(r.body) != 2 {
		t.Errorf("the request must carry only the entity, never an owner: %v", r.body)
	}
}

func TestPurgeEntityIsBestEffort(t *testing.T) {
	// Without the environment core gives a tool, nothing is sent and nothing breaks.
	srv, seen := fakeCore(t, http.StatusOK)
	t.Setenv("CORE_API_URL", "")
	t.Setenv("TOOL_SERVICE_TOKEN", "")
	PurgeEntity(JobType, "x")
	t.Setenv("CORE_API_URL", srv.URL)
	PurgeEntity(JobType, "x") // URL but no token
	t.Setenv("CORE_API_URL", "")
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	PurgeEntity(JobType, "x") // token but no URL
	if n := len(seen()); n != 0 {
		t.Errorf("%d requests went out without a complete environment", n)
	}

	// A refusal, a server error and an unreachable core are all survived (no panic, no return value to act on).
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError, http.StatusUnauthorized} {
		bad, _ := fakeCore(t, status)
		t.Setenv("CORE_API_URL", bad.URL)
		t.Setenv("TOOL_SERVICE_TOKEN", "tok")
		PurgeEntity(JobType, "1fbd3006-0831-448c-8962-2851188e9de9")
	}
	gone := httptest.NewServer(http.NotFoundHandler())
	url := gone.URL
	gone.Close()
	t.Setenv("CORE_API_URL", url)
	PurgeEntity(JobType, "1fbd3006-0831-448c-8962-2851188e9de9")
	t.Setenv("CORE_API_URL", "://not a url")
	PurgeEntity(JobType, "1fbd3006-0831-448c-8962-2851188e9de9")
}
