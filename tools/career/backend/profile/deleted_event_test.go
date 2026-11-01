package profile

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"career-tool-backend/db"
)

// A fake core that records each event with what the jobs.db row of a probe persona looked like at the moment the event arrived, so a test
// can tell that the cleanup was already done when the event went out.
type deletedProbe struct {
	mu       sync.Mutex
	topics   []string
	payloads []map[string]string
	rowsSeen []int
}

func probeDeleted(t *testing.T) *deletedProbe {
	t.Helper()
	p := &deletedProbe{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/events/publish") { // the comments purge also goes to core: not an event
			w.WriteHeader(http.StatusOK)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var ev struct {
			Topic   string
			Payload map[string]string
		}
		_ = json.Unmarshal(raw, &ev)
		var n int
		_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM persona_score_state`).Scan(&n)
		p.mu.Lock()
		p.topics = append(p.topics, ev.Topic)
		p.payloads = append(p.payloads, ev.Payload)
		p.rowsSeen = append(p.rowsSeen, n)
		p.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return p
}

func (p *deletedProbe) of(topic string) []map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []map[string]string
	for i, tp := range p.topics {
		if tp == topic {
			out = append(out, p.payloads[i])
		}
	}
	return out
}

func (p *deletedProbe) total() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.topics)
}

// Deleting a profile announces it once, with its id and nothing else, and nothing else is sent.
func TestDeletingAProfileAnnouncesIt(t *testing.T) {
	setup(t)
	id, err := CreateProfile("Ada", "Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	ev := probeDeleted(t)
	if err := DeleteProfile(id); err != nil {
		t.Fatal(err)
	}
	got := ev.of("career.profile.deleted")
	if len(got) != 1 || !reflect.DeepEqual(got[0], map[string]string{"profile_id": id}) || ev.total() != 1 {
		t.Errorf("events %v, total %d", got, ev.total())
	}
}

// Each profile is announced with its own id; the other one is not.
func TestOnlyTheDeletedProfileIsAnnounced(t *testing.T) {
	setup(t)
	a, _ := CreateProfile("A", "a")
	b, _ := CreateProfile("B", "b")
	ev := probeDeleted(t)
	if err := DeleteProfile(b); err != nil {
		t.Fatal(err)
	}
	if got := ev.of("career.profile.deleted"); len(got) != 1 || got[0]["profile_id"] != b {
		t.Errorf("events %v (a is %s, b is %s)", got, a, b)
	}
}

// A profile that does not exist, or was already deleted, announces nothing.
func TestDeletingAProfileThatIsNotThereAnnouncesNothing(t *testing.T) {
	setup(t)
	id, _ := CreateProfile("Ada", "L")
	ev := probeDeleted(t)
	if err := DeleteProfile("nope"); err != nil || ev.total() != 0 {
		t.Fatalf("unknown id: %v, %d events", err, ev.total())
	}
	if err := DeleteProfile(id); err != nil {
		t.Fatal(err)
	}
	if err := DeleteProfile(id); err != nil {
		t.Fatal(err)
	}
	if got := ev.of("career.profile.deleted"); len(got) != 1 {
		t.Errorf("a second delete announced again: %v", got)
	}
}

// A failed delete announces nothing.
func TestAFailedProfileDeleteAnnouncesNothing(t *testing.T) {
	setup(t)
	id, _ := CreateProfile("Ada", "L")
	ev := probeDeleted(t)
	db.CareerDB.Close()
	if err := DeleteProfile(id); err == nil {
		t.Fatal("expected an error")
	}
	if ev.total() != 0 {
		t.Errorf("%d events after a failed delete", ev.total())
	}
}

// The event goes out after the existing cleanups, not before: when it arrives the persona score state of the profile is already gone.
func TestTheEventComesAfterTheCleanup(t *testing.T) {
	setup(t)
	id, _ := CreateProfile("Ada", "L")
	if _, err := db.JobsDB.Exec(`INSERT INTO persona_score_state (persona_id, profile_id, fingerprint) VALUES ('per', ?, 'x')`, id); err != nil {
		t.Fatal(err)
	}
	ev := probeDeleted(t)
	if err := DeleteProfile(id); err != nil {
		t.Fatal(err)
	}
	if len(ev.rowsSeen) != 1 || ev.rowsSeen[0] != 0 {
		t.Errorf("rows left when the event arrived: %v", ev.rowsSeen)
	}
}

// Creating a profile still announces only its creation.
func TestCreatingAProfileDoesNotAnnounceADelete(t *testing.T) {
	setup(t)
	ev := probeDeleted(t)
	if _, err := CreateProfile("Ada", "L"); err != nil {
		t.Fatal(err)
	}
	if len(ev.of("career.profile.deleted")) != 0 || ev.total() != 1 {
		t.Errorf("events %v", ev.topics)
	}
}
