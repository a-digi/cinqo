// Package commentstest is a stand-in for core's comments purge route, for the tests of the packages
// whose deletes purge comments. It is test support only: nothing but _test files import it.
package commentstest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
)

// Start runs a fake core, points CORE_API_URL and TOOL_SERVICE_TOKEN at it for the test, and returns a
// function that lists what was purged as "type:id", sorted.
func Start(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]string
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		got = append(got, body["entityType"]+":"+body["entityId"])
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "test-token")
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := append([]string(nil), got...)
		sort.Strings(out)
		return out
	}
}
