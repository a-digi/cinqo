package profile

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"career-tool-backend/db"
)

// sentEvents is a fake core that records every event it receives as topic and payload.
type sentEvents struct {
	mu     sync.Mutex
	events []struct {
		topic   string
		payload map[string]string
	}
}

func captureEvents(t *testing.T) *sentEvents {
	t.Helper()
	s := &sentEvents{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var ev struct {
			Topic   string
			Payload map[string]string
		}
		_ = json.Unmarshal(raw, &ev)
		s.mu.Lock()
		s.events = append(s.events, struct {
			topic   string
			payload map[string]string
		}{ev.Topic, ev.Payload})
		s.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return s
}

func (s *sentEvents) of(topic string) []map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]string
	for _, e := range s.events {
		if e.topic == topic {
			out = append(out, e.payload)
		}
	}
	return out
}

func (s *sentEvents) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
}

// Creating a profile announces it once, with its id and nothing else.
func TestCreatingAProfileAnnouncesIt(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	id, err := CreateProfile("Ada", "Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	got := ev.of("career.profile.created")
	if len(got) != 1 || !reflect.DeepEqual(got[0], map[string]string{"profile_id": id}) {
		t.Errorf("events %v, want one with only the profile id %q", got, id)
	}
	if ev.total() != 1 {
		t.Errorf("%d events in all: nothing else may be announced", ev.total())
	}
}

func TestEveryNewProfileIsAnnouncedWithItsOwnId(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	a, _ := CreateProfile("A", "One")
	b, _ := CreateProfile("B", "Two")
	got := ev.of("career.profile.created")
	if len(got) != 2 || got[0]["profile_id"] != a || got[1]["profile_id"] != b || a == b {
		t.Errorf("%v (a=%s b=%s)", got, a, b)
	}
}

// A create that fails announces nothing.
func TestAProfileThatCouldNotBeCreatedIsNotAnnounced(t *testing.T) {
	setup(t)
	ev := captureEvents(t)
	db.CareerDB.Close() // the insert now fails
	if _, err := CreateProfile("X", "Y"); err == nil {
		t.Fatal("expected an error")
	}
	if ev.total() != 0 {
		t.Errorf("%d events for a failed create", ev.total())
	}
}
