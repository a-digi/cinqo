package crawl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

var gotEngine atomic.Value

// scriptedEngine replaces the read of the Browser tool's engine setting.
func scriptedEngine(t *testing.T, engine string, err error) *int {
	t.Helper()
	calls := 0
	var mu sync.Mutex
	orig := readBrowserEngine
	readBrowserEngine = func(context.Context, string, string) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return engine, err
	}
	t.Cleanup(func() { readBrowserEngine = orig })
	return &calls
}

func TestRequireKatanaSelected(t *testing.T) {
	scriptedEngine(t, "katana", nil)
	if err := requireKatanaSelected(context.Background(), "http://core.invalid", "t"); err != nil {
		t.Errorf("katana: %v", err)
	}
	scriptedEngine(t, "rod", nil)
	if err := requireKatanaSelected(context.Background(), "http://core.invalid", "t"); !errors.Is(err, errKatanaNotSelected) {
		t.Errorf("rod: %v", err)
	}
	scriptedEngine(t, "", errors.New("boom"))
	if err := requireKatanaSelected(context.Background(), "http://core.invalid", "t"); !errors.Is(err, errBrowserEngineUnknown) {
		t.Errorf("unreadable: %v", err)
	}
}

// The real reader against a stand-in for core: it asks the Browser's settings
// route with the person's own token and understands its answer.
func TestReadBrowserEngineViaCore(t *testing.T) {
	var gotAuth, gotPath, gotMethod string
	status := http.StatusOK
	body := `{"debugEnabled":false,"fetchEngine":"katana","humanAssist":true}`
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotMethod = r.Header.Get("Authorization"), r.URL.Path, r.Method
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer core.Close()

	got, err := readBrowserEngineViaCore(context.Background(), core.URL, "tok-42")
	if err != nil || got != "katana" {
		t.Fatalf("got %q %v", got, err)
	}
	if gotAuth != "Bearer tok-42" || gotPath != "/api/v1/tools/browser/proxy/browser-settings" || gotMethod != http.MethodGet {
		t.Errorf("request: %s %s %s", gotMethod, gotPath, gotAuth)
	}

	for name, c := range map[string]struct {
		status int
		body   string
	}{
		// a failing status must be an error even when its body happens to look like a good answer
		"tool not installed": {http.StatusNotFound, `{"fetchEngine":"katana"}`},
		"session expired":    {http.StatusUnauthorized, `{"fetchEngine":"katana"}`},
		"tool crashed":       {http.StatusBadGateway, `{"fetchEngine":"katana"}`},
		"not json":           {http.StatusOK, `<html>`},
	} {
		status, body = c.status, c.body
		if got, err := readBrowserEngineViaCore(context.Background(), core.URL, "t"); err == nil {
			t.Errorf("%s: got %q without an error", name, got)
		}
	}
	core.Close()
	if _, err := readBrowserEngineViaCore(context.Background(), core.URL, "t"); err == nil {
		t.Error("a core that is down must be an error")
	}

	// the whole chain end to end against the stand-in core: the gate follows what the Browser says
	core2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"fetchEngine":"` + strings.TrimSpace(gotEngine.Load().(string)) + `"}`))
	}))
	defer core2.Close()
	for engine, want := range map[string]bool{"katana": true, "rod": false, "selenium": false} {
		gotEngine.Store(engine)
		err := requireKatanaSelected(context.Background(), core2.URL, "t")
		if (err == nil) != want {
			t.Errorf("%s: %v", engine, err)
		}
	}
}
