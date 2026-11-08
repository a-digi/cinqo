package portalhandlers

import (
	"net/http"
	"strings"
	"testing"
)

// plan 203, step 2: the job host of a link through the route, validated before anything is written.
func TestTheJobHostOfALinkIsSetThroughTheRouteAndValidatedBeforeAnythingIsWritten(t *testing.T) {
	pid, _ := loginSeed(t)
	link := loginLink(t, pid)
	put := func(body string) (int, string) {
		rec := linkCall(http.MethodPut, body)
		return rec.Code, rec.Body.String()
	}
	state := func() (jobHost, title string) {
		for _, l := range linksOf(t, pid) {
			if l.ID == link {
				return l.JobHost, l.Title
			}
		}
		return "?", "?"
	}
	if jh, _ := state(); jh != "" {
		t.Fatalf("a new link has no job host: %q", jh)
	}
	_, oldTitle := state()
	if code, body := put(`{"id":"` + link + `","jobHost":"Jobs.Other.Example"}`); code != 200 {
		t.Fatalf("set: %d %s", code, body)
	}
	if jh, _ := state(); jh != "jobs.other.example" {
		t.Errorf("set: %q", jh)
	}
	// a bad value changes nothing, not even the other fields of the same request
	for _, bad := range []string{"localhost", "10.0.0.1", "127.0.0.1", "[::1]", "x.example/path", "https://x.example", "jobs", "a b.example"} {
		code, body := put(`{"id":"` + link + `","title":"Changed","jobHost":"` + bad + `"}`)
		if code != http.StatusBadRequest || !strings.Contains(body, "job host") {
			t.Errorf("%q: %d %s", bad, code, body)
		}
	}
	if jh, title := state(); jh != "jobs.other.example" || title != oldTitle {
		t.Errorf("a refused request changed something: %q %q (was %q)", jh, title, oldTitle)
	}
	// the link's own host is refused
	if code, _ := put(`{"id":"` + link + `","jobHost":"` + strings.TrimPrefix(linkHostOf(t, pid, link), "") + `"}`); code != http.StatusBadRequest {
		t.Errorf("the own host: %d", code)
	}
	// an accepted value goes together with the other fields of the request
	if code, _ := put(`{"id":"` + link + `","title":"Renamed","jobHost":"other.example"}`); code != 200 {
		t.Fatalf("both: %d", code)
	}
	if jh, title := state(); jh != "other.example" || title != "Renamed" {
		t.Errorf("%q %q", jh, title)
	}
	// "" clears; nothing else asked is still "nothing asked"; an unknown link is an error
	if code, _ := put(`{"id":"` + link + `","jobHost":""}`); code != 200 {
		t.Errorf("clear: %d", code)
	}
	if jh, _ := state(); jh != "" {
		t.Errorf("cleared: %q", jh)
	}
	if code, body := put(`{"id":"` + link + `"}`); code != http.StatusBadRequest || !strings.Contains(body, "jobHost") {
		t.Errorf("an empty update names jobHost among the fields: %d %s", code, body)
	}
	if code, _ := put(`{"id":"nope","jobHost":"x.example.com"}`); code < 400 {
		t.Errorf("unknown link: %d", code)
	}
}

func linkHostOf(t *testing.T, pid, link string) string {
	t.Helper()
	for _, l := range linksOf(t, pid) {
		if l.ID == link {
			rest := strings.TrimPrefix(strings.TrimPrefix(l.URL, "https://"), "http://")
			return strings.ToLower(strings.SplitN(rest, "/", 2)[0])
		}
	}
	return ""
}
