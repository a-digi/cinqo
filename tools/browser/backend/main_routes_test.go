package main

import (
	"os"
	"strings"
	"testing"
)

// Every route that drives Chrome must take a crawl-session slot, and the cheap routes must not (plan 224, step 5).
func TestOnlyTheChromeDrivingRoutesTakeASessionSlot(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	wrapped := map[string]bool{
		"/crawl": true, "/find-login-elements": true, "/extract": true, "/crawl-paginated": true, "/find-repeating-items": true,
		"/fetch-links": true, "/fetch-detail-candidates": true, "/diagnose-url": true, "/engine-test": true,
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "http.HandleFunc(\"") {
			continue
		}
		path := line[len("http.HandleFunc(\""):]
		path = path[:strings.Index(path, "\"")]
		seen[path] = true
		has := strings.Contains(line, "chromeproc.LimitSessions(")
		if wrapped[path] && !has {
			t.Errorf("%s drives Chrome and must be wrapped in LimitSessions", path)
		}
		if !wrapped[path] && has {
			t.Errorf("%s must not take a slot (it is cheap or person-driven)", path)
		}
	}
	for p := range wrapped {
		if !seen[p] {
			t.Errorf("route %s is not registered in main.go any more", p)
		}
	}
	// the login windows and the visible window are driven by a person and never take a slot
	for _, p := range []string{"/login-window/open", "/katana-window-open", "/chrome-processes", "/engine-status", "/healthz"} {
		if wrapped[p] {
			t.Errorf("%s must not be wrapped", p)
		}
	}
}
